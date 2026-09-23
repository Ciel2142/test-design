package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"test-design/internal/evidence"
	"test-design/internal/mutation"
	"test-design/internal/worktree"
)

// discardLimitPerSite: a third discard of one site cannot be applied.
const discardLimitPerSite = 2

type discardMutationEditRequest struct {
	verifyRequest
	RunDir string `json:"runDir"`
	Site   string `json:"site"`
	Reason string `json:"reason"`
}

type discardMutationEditOutput struct {
	evidence.Envelope
	RunID        string `json:"runId"`
	RowID        string `json:"rowId"`
	Site         string `json:"site"`
	Reason       string `json:"reason"`
	DiffPath     string `json:"diffPath"`
	WorktreePath string `json:"worktreePath"`
}

func newDiscardMutationEditFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("discard-mutation-edit", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func decodeDiscardMutationEditRequest(op string, requestBody []byte) (*discardMutationEditRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	var extra struct {
		RunDir string `json:"runDir"`
		Site   string `json:"site"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if cliErr := validateRunDir(op, extra.RunDir); cliErr != nil {
		return nil, cliErr
	}
	if strings.TrimSpace(extra.Reason) == "" {
		return nil, &cliError{Code: "missing-reason", Message: fmt.Sprintf("%s: reason is required", op)}
	}
	// Shape is checked here regardless of registration; discardMutationEditOp
	// narrows it further to list membership once a registered list exists.
	if err := mutation.ValidateSiteLabel(extra.Site); err != nil {
		return nil, siteRegistryRefusal(op, err)
	}
	return &discardMutationEditRequest{verifyRequest: *base, RunDir: extra.RunDir, Site: extra.Site, Reason: extra.Reason}, nil
}

func discardMutationEditHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeDiscardMutationEditRequest("discard-mutation-edit", requestBody)
	if cliErr != nil {
		return cliErr
	}
	return discardMutationEditOp(req, stdout)
}

// discardMutationEditOp is the discard-mutation-edit op body: a confirmation
// failure discards the mutator's rejected edit instead of abandoning the row,
// so the same site can be re-dispatched. Every check below runs before the
// worktree is touched; only a write of discard-<n>.diff, RestoreAll and the
// state write that follows it change anything on disk or in mutation-state.json.
func discardMutationEditOp(req *discardMutationEditRequest, stdout io.Writer) *cliError {
	const op = "discard-mutation-edit"

	st, cliErr := mutationRowPrelude(op, &req.verifyRequest, req.RunDir)
	if cliErr != nil {
		return cliErr
	}

	if st.Started != nil {
		return &cliError{
			Code: "site-interrupted",
			Message: fmt.Sprintf("%s: site %q was interrupted during its %s run; recover-mutation must restore %s before another site is touched",
				op, st.Started.Site, st.Started.Phase, st.WorktreePath),
		}
	}

	if len(st.Enumerated) > 0 {
		if err := mutation.CheckSiteKnown(st.Enumerated, req.Site); err != nil {
			return siteRegistryRefusal(op, err)
		}
	}

	for _, so := range st.Sites {
		if so.Site == req.Site {
			return &cliError{
				Code:    "duplicate-site",
				Message: fmt.Sprintf("%s: site %q is already evaluated and recorded in %s", op, req.Site, req.RunDir),
			}
		}
	}

	siteDiscards := 0
	for _, d := range st.Discarded {
		if d.Site == req.Site {
			siteDiscards++
		}
	}
	if siteDiscards >= discardLimitPerSite {
		return &cliError{
			Code: "discard-limit",
			Message: fmt.Sprintf("%s: site %q has already been discarded %d times for this row; it cannot be applied, record the row unappliable through record-mutation",
				op, req.Site, siteDiscards),
		}
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	wt, err := ctrl.Attach(st.WorktreePath)
	if err != nil {
		return &cliError{Code: "worktree-attach-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	diff, err := ctrl.Diff(wt)
	if err != nil {
		return &cliError{Code: "diff-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if strings.TrimSpace(diff) == "" {
		return &cliError{Code: "empty-diff", Message: fmt.Sprintf("%s: %v", op, mutation.ErrEmptyDiff)}
	}

	diffPath := filepath.Join(req.RunDir, fmt.Sprintf("discard-%d.diff", len(st.Discarded)+1))
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		return &cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("%s: write diff: %v", op, err)}
	}
	digest, err := evidence.SHA256File(diffPath)
	if err != nil {
		return &cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("%s: digest diff: %v", op, err)}
	}

	switch err := ctrl.RestoreAll(wt, st.RepoCommit); {
	case err == nil:
	case errors.Is(err, worktree.ErrResidue):
		return &cliError{Code: "residue-remains", Message: fmt.Sprintf("%s: %v", op, err)}
	default:
		return &cliError{Code: "restore-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	st.Discarded = append(st.Discarded, mutation.DiscardRecord{
		Site:       req.Site,
		Reason:     req.Reason,
		DiffPath:   diffPath,
		DiffSHA256: digest,
	})
	if err := mutation.WriteState(req.RunDir, st); err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	return writeStdout(stdout, discardMutationEditOutput{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:        st.RunID,
		RowID:        st.RowID,
		Site:         req.Site,
		Reason:       req.Reason,
		DiffPath:     diffPath,
		WorktreePath: wt.Path,
	})
}
