package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/jvmsource"
	"test-design/internal/mutation"
	"test-design/internal/trace"
	"test-design/internal/worktree"
)

type prepareMutationRequest struct {
	verifyRequest
	RowID string `json:"rowID"`
}

type identityOutput struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

type prepareMutationOutput struct {
	evidence.Envelope
	RunID string `json:"runID"`
	RepoCommit string `json:"repoCommit"`
	Source string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
	RowID        string         `json:"rowID"`
	Identity     identityOutput `json:"identity"`
	RunnerKind   string         `json:"runnerKind"`
	WorktreePath string         `json:"worktreePath"`
	RunDir       string         `json:"runDir"`
}

func decodePrepareMutationRequest(op string, requestBody []byte) (*prepareMutationRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	var extra struct {
		RowID string `json:"rowID"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if extra.RowID == "" {
		return nil, &cliError{Code: "missing-row-id", Message: fmt.Sprintf("%s: rowID is required", op)}
	}
	return &prepareMutationRequest{verifyRequest: *base, RowID: extra.RowID}, nil
}

func prepareMutationHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodePrepareMutationRequest("prepare-mutation", requestBody)
	if cliErr != nil {
		return cliErr
	}
	src, cliErr := loadVerifiedSource("prepare-mutation", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "dirty-repository",
			Message: fmt.Sprintf("prepare-mutation: %v: %s", worktree.ErrDirtyBeforeSnapshot, describeDirty(req.RepoRoot)),
		}
	}

	repoCommit, cliErr := headCommit("prepare-mutation", req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	runDir, err := ctrl.NewRunDir(req.RunID)
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}

	wt, err := ctrl.Create(repoCommit)
	if err != nil {
		return &cliError{Code: "worktree-create-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}
	disposed := false
	dispose := func() *cliError {
		if disposed {
			return nil
		}
		disposed = true
		if err := ctrl.Dispose(wt); err != nil {
			return &cliError{Code: "worktree-dispose-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
		}
		return nil
	}
	defer func() { _ = dispose() }()

	row, cliErr := findActiveRow(src, req.RowID)
	if cliErr != nil {
		return cliErr
	}

	suiteDir := filepath.Join(wt.Path, filepath.FromSlash(src.Runner.Root))
	identity, cliErr := resolveMutationIdentity(src, suiteDir, row.ID)
	if cliErr != nil {
		return cliErr
	}

	snapshotJSON, err := json.Marshal(before)
	if err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("prepare-mutation: marshal snapshot: %v", err)}
	}
	st := &mutation.State{
		RunID: req.RunID,
		RepoCommit: repoCommit,
		Source: src.Path,
		SourceSHA256: src.SHA256,
		RowID:        row.ID,
		Identity:     identity,
		WorktreePath: wt.Path,
		Snapshot:     snapshotJSON,
		Sites:        []mutation.SiteOutcome{},
	}
	if err := mutation.WriteState(runDir, st); err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}

	// Success hands the worktree off to evaluate-mutation, which owns
	// disposing it; disarm the deferred cleanup rather than call dispose().
	disposed = true

	out := prepareMutationOutput{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID: st.RunID,
		RepoCommit: st.RepoCommit,
		Source: st.Source,
		SourceSHA256: st.SourceSHA256,
		RowID:        row.ID,
		Identity:     identityOutput{Scope: identity.Scope, Name: identity.Name},
		RunnerKind:   src.Runner.Kind,
		WorktreePath: wt.Path,
		RunDir:       runDir,
	}
	return writeStdout(stdout, out)
}

// findActiveRow checks Retired before Rows so a row ID cited in both
// answers with the more specific retired refusal.
func findActiveRow(src *coverage.Source, rowID string) (*coverage.Row, *cliError) {
	for _, id := range src.Retired {
		if id == rowID {
			return nil, &cliError{
				Code:    "row-not-found",
				Message: fmt.Sprintf("prepare-mutation: row %s is retired in %s", rowID, src.Path),
			}
		}
	}
	for i := range src.Rows {
		if src.Rows[i].ID == rowID {
			return &src.Rows[i], nil
		}
	}
	return nil, &cliError{
		Code:    "row-not-found",
		Message: fmt.Sprintf("prepare-mutation: row %s not found in %s", rowID, src.Path),
	}
}

func resolveMutationIdentity(src *coverage.Source, suiteDir, rowID string) (mutation.Identity, *cliError) {
	if src.Runner.Kind == "go" {
		modulePath, cliErr := readModulePath(suiteDir)
		if cliErr != nil {
			return mutation.Identity{}, cliErr
		}
		sites, err := trace.ScanTags(suiteDir, modulePath)
		if err != nil {
			return mutation.Identity{}, &cliError{Code: "scan-tags-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
		}
		identity, err := mutation.ResolveGoIdentity(sites, rowID)
		if err != nil {
			return mutation.Identity{}, &cliError{Code: "row-not-bound", Message: fmt.Sprintf("prepare-mutation: %v", err)}
		}
		return identity, nil
	}

	sites, err := jvmsource.ScanTags(suiteDir)
	if err != nil {
		return mutation.Identity{}, &cliError{Code: "scan-tags-failed", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}
	identity, err := mutation.ResolveJVMIdentity(sites, rowID)
	if err != nil {
		return mutation.Identity{}, &cliError{Code: "row-not-bound", Message: fmt.Sprintf("prepare-mutation: %v", err)}
	}
	return identity, nil
}
