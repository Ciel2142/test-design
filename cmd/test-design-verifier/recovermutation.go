package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"test-design/internal/evidence"
	"test-design/internal/mutation"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

type recoverMutationRequest struct {
	verifyRequest
	RunDir string `json:"runDir"`
}

type recoverMutationOutput struct {
	evidence.Envelope
	RunID           string   `json:"runId"`
	RowID           string   `json:"rowId"`
	WorktreePath    string   `json:"worktreePath"`
	Recreated       bool     `json:"recreated"`
	CompletedSites  []string `json:"completedSites"`
	InterruptedSite string   `json:"interruptedSite"`
}

func newRecoverMutationFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("recover-mutation", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

// validateRunDir checks the runDir request-shape rule every mutation-row op
// that takes one shares: present, absolute, and naming a directory that
// exists.
func validateRunDir(op, runDir string) *cliError {
	if runDir == "" {
		return &cliError{Code: "missing-run-dir", Message: fmt.Sprintf("%s: runDir is required", op)}
	}
	if !filepath.IsAbs(runDir) {
		return &cliError{Code: "run-dir-not-absolute", Message: fmt.Sprintf("%s: runDir %q must be an absolute path", op, runDir)}
	}
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		return &cliError{Code: "run-dir-not-found", Message: fmt.Sprintf("%s: runDir %q does not exist", op, runDir)}
	}
	return nil
}

func decodeRecoverMutationRequest(op string, requestBody []byte) (*recoverMutationRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	var extra struct {
		RunDir string `json:"runDir"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if cliErr := validateRunDir(op, extra.RunDir); cliErr != nil {
		return nil, cliErr
	}
	return &recoverMutationRequest{verifyRequest: *base, RunDir: extra.RunDir}, nil
}

// mutationRowPrelude loads runDir's prepared state, requires it to match
// req's run/source/HEAD identity, and requires the row it names to be
// started and not yet completed in this run's ledger. It is the entry
// sequence recover-mutation and discard-mutation-edit both run before doing
// anything specific to either op.
func mutationRowPrelude(op string, req *verifyRequest, runDir string) (*mutation.State, *cliError) {
	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return nil, cliErr
	}
	st, err := mutation.LoadState(runDir)
	if err != nil {
		return nil, &cliError{Code: "state-load-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	head, cliErr := headCommit(op, req.RepoRoot)
	if cliErr != nil {
		return nil, cliErr
	}
	if st.RunID != req.RunID || st.RepoCommit != head || st.Source != src.Path || st.SourceSHA256 != src.SHA256 {
		return nil, &cliError{
			Code:    "stale-evidence",
			Message: fmt.Sprintf("%s: prepared state does not match request run/source/HEAD; prepare again", op),
		}
	}

	store, cliErr := openLedger(op, req)
	if cliErr != nil {
		return nil, cliErr
	}
	ledger, cliErr := loadLedgerFor(op, runstate.KindMutationRow, store)
	if cliErr != nil {
		return nil, cliErr
	}
	switch current := runstate.Current(ledger, runstate.KindMutationRow, st.RowID); {
	case current == nil:
		return nil, &cliError{
			Code:    "mutation-row-not-started",
			Message: fmt.Sprintf("%s: %s was never prepared for this run", op, st.RowID),
		}
	case current.Status == runstate.StatusCompleted:
		return nil, completedRecordRefusal(op, runstate.KindMutationRow, st.RowID)
	}
	return st, nil
}

// recoverMutationHandler puts a mutation row's workspace back into the state
// prepare-mutation left it in, after an evaluation was interrupted inside a
// site. It is the only op that clears the site marker without measuring
// anything: the marker says the workspace is of unknown cleanliness, and this
// op answers that question by restoring the workspace and proving the result,
// or by refusing and leaving the marker exactly where it found it.
func recoverMutationHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	const op = "recover-mutation"

	req, cliErr := decodeRecoverMutationRequest(op, requestBody)
	if cliErr != nil {
		return cliErr
	}

	st, cliErr := mutationRowPrelude(op, &req.verifyRequest, req.RunDir)
	if cliErr != nil {
		return cliErr
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	worktreePath, recreated, cliErr := restoreMutationWorkspace(op, ctrl, st)
	if cliErr != nil {
		return cliErr
	}

	// Only a proved-clean workspace rewrites the state, and it rewrites it once:
	// the marker goes out and the surviving worktree is named in the same write,
	// so a reader never sees a cleared marker beside a path that no longer exists.
	interrupted := ""
	if st.Started != nil {
		interrupted = st.Started.Site
	}
	completed := make([]string, 0, len(st.Sites))
	for _, site := range st.Sites {
		completed = append(completed, site.Site)
	}
	st.Started = nil
	st.WorktreePath = worktreePath
	if err := mutation.WriteState(req.RunDir, st); err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	return writeStdout(stdout, recoverMutationOutput{
		Envelope:        evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:           st.RunID,
		RowID:           st.RowID,
		WorktreePath:    worktreePath,
		Recreated:       recreated,
		CompletedSites:  completed,
		InterruptedSite: interrupted,
	})
}

// restoreMutationWorkspace returns the workspace named by st to the pristine
// checkout of st.RepoCommit and reports the path that survived, and whether it
// had to be built anew. A directory that is still there is restored in place;
// one that no longer exists is replaced, its own registry entry forgotten
// first — never the registry pruned, which would drop every worktree of the
// repository whose directory is absent at that moment. Only a path that does
// not exist counts as vanished: one the verifier cannot stat is refused, since
// nothing proved the workspace gone. Every failure is a refusal: the caller
// must not clear the marker over a workspace nothing proved clean.
func restoreMutationWorkspace(op string, ctrl *worktree.Controller, st *mutation.State) (string, bool, *cliError) {
	if _, err := os.Stat(st.WorktreePath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", false, &cliError{Code: "restore-failed", Message: fmt.Sprintf("%s: stat worktree %s: %v", op, st.WorktreePath, err)}
		}
		// A registry that already forgot the path has nothing left to forget;
		// any other failure to clear the entry leaves nothing to create over.
		if err := ctrl.Forget(st.WorktreePath); err != nil && !errors.Is(err, worktree.ErrWorktreeNotRegistered) {
			return "", false, &cliError{Code: "recreate-failed", Message: fmt.Sprintf("%s: %v", op, err)}
		}
		wt, err := ctrl.Create(st.RepoCommit)
		if err != nil {
			return "", false, &cliError{Code: "recreate-failed", Message: fmt.Sprintf("%s: %v", op, err)}
		}
		return wt.Path, true, nil
	}

	// An attach failure here is a workspace that cannot be restored, not a
	// missing one: the directory exists, so replacing it would abandon whatever
	// the interrupted run left in it without ever having looked.
	wt, err := ctrl.Attach(st.WorktreePath)
	if err != nil {
		return "", false, &cliError{Code: "restore-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	switch err := ctrl.RestoreAll(wt, st.RepoCommit); {
	case err == nil:
		return wt.Path, false, nil
	case errors.Is(err, worktree.ErrResidue):
		return "", false, &cliError{Code: "residue-remains", Message: fmt.Sprintf("%s: %v", op, err)}
	case errors.Is(err, worktree.ErrWrongRevision):
		return "", false, &cliError{Code: "wrong-revision", Message: fmt.Sprintf("%s: %v", op, err)}
	default:
		return "", false, &cliError{Code: "restore-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
}
