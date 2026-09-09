package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/runstate"
)

// The record keys of the run's two singleton obligations, and the roles a
// record's artifacts are keyed by.
const (
	tracePhaseID  = "trace"
	liveAttemptID = "live"

	artifactEvidence       = "evidence"
	artifactDispatchInputs = "dispatchInputs"
	artifactAttempt        = "attempt"
)

// The kind-specific input identity each record captures at Start. These shapes
// are the cross-wave contract: an op that resumes a run reads them back to
// reach the workspace the obligation was opened against, so a producer and a
// reader of one kind must agree on the field names, not merely on the meaning.

// phaseInputs is the trace phase's input identity: the run directory its
// evidence lands in.
type phaseInputs struct {
	RunDir string `json:"runDir"`
}

// mutationRowIdentity is the bound test a row's mutation was applied to.
type mutationRowIdentity struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

// mutationRowInputs is a mutation row's input identity: the bound test, the
// runner that executes it, and the workspace prepare-mutation left behind.
type mutationRowInputs struct {
	Identity     mutationRowIdentity `json:"identity"`
	RunnerKind   string              `json:"runnerKind"`
	RunDir       string              `json:"runDir"`
	WorktreePath string              `json:"worktreePath"`
}

// liveAttemptInputs is the live attempt's input identity: the workspace and
// the profile and plan the attempt was made against.
type liveAttemptInputs struct {
	RunDir       string `json:"runDir"`
	WorktreePath string `json:"worktreePath"`
	ProfilePath  string `json:"profilePath"`
	PlanSHA256   string `json:"planSha256"`
}

// ledgerIdentity is the run identity every transition of this run's ledger
// presents. The repository root is resolved through its symlinks, so two ops
// handed /tmp/x and /private/tmp/x address one run rather than two.
func ledgerIdentity(req *verifyRequest, repoRoot, repoCommit string, src *coverage.Source) runstate.Identity {
	root, err := resolveThroughSymlinks(repoRoot)
	if err != nil {
		root = repoRoot
	}
	return runstate.Identity{
		RunID:        req.RunID,
		RepoRoot:     root,
		RepoCommit:   repoCommit,
		Source:       src.Path,
		SourceSHA256: src.SHA256,
	}
}

// repeatReason is the caller's reason for repeating an obligation this run
// already completed, empty when the caller asked for no repetition.
func repeatReason(req *verifyRequest) string {
	if req == nil || req.Repeat == nil {
		return ""
	}
	return req.Repeat.Reason
}

// openLedger resolves the verifier-owned state root, refuses one that resolves
// inside the repository under verification, and opens the run's ledger
// directory. The confinement check is this op layer's, not the store's: the
// ledger must never be able to dirty the working tree a run is about to
// snapshot, so it is answered before anything is created.
func openLedger(op string, req *verifyRequest) (*runstate.Store, *cliError) {
	fail := func(err error) *cliError {
		return &cliError{Code: "ledger-open-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	stateDir, err := runstate.DefaultStateDir()
	if err != nil {
		return nil, fail(err)
	}
	resolvedState, err := resolveExistingPrefix(stateDir)
	if err != nil {
		return nil, fail(fmt.Errorf("resolve state dir %s: %w", stateDir, err))
	}
	repoRoot, err := resolveThroughSymlinks(req.RepoRoot)
	if err != nil {
		return nil, fail(fmt.Errorf("resolve repo root %s: %w", req.RepoRoot, err))
	}
	if isWithinPath(repoRoot, resolvedState) {
		return nil, fail(fmt.Errorf("state dir %s resolves inside repository %s", resolvedState, repoRoot))
	}

	dir, err := runstate.Locate(resolvedState, repoRoot, req.RunID)
	if err != nil {
		return nil, fail(err)
	}
	store, err := runstate.Open(dir)
	if err != nil {
		return nil, fail(err)
	}
	return store, nil
}

// loadLedgerFor reads the run's checkpoint. A run with no ledger yet is not a
// failure: it reports a nil ledger, which every caller reads as "no record for
// any obligation".
func loadLedgerFor(op string, kind runstate.Kind, store *runstate.Store) (*runstate.Ledger, *cliError) {
	l, err := store.Load()
	if errors.Is(err, runstate.ErrNoLedger) {
		return nil, nil
	}
	if err != nil {
		return nil, ledgerError(op, kind, err)
	}
	return l, nil
}

// ledgerError names a ledger transition's refusal in the op's own vocabulary.
// The four sentinels carry the obligation's kind into the code, so a parent
// reads which obligation refused it, not merely that some record did. Anything
// else is a checkpoint that could not be read or could not be replaced; the two
// are told apart by the layer that failed, which only the store's message
// records.
func ledgerError(op string, kind runstate.Kind, err error) *cliError {
	message := fmt.Sprintf("%s: %v", op, err)
	switch {
	case errors.Is(err, runstate.ErrIdentityMismatch):
		return &cliError{Code: "ledger-identity-mismatch", Message: message}
	case errors.Is(err, runstate.ErrCompleted):
		return &cliError{Code: string(kind) + "-completed", Message: message}
	case errors.Is(err, runstate.ErrNotStarted):
		return &cliError{Code: string(kind) + "-not-started", Message: message}
	case errors.Is(err, runstate.ErrDuplicateCompletion):
		return &cliError{Code: "duplicate-completion", Message: message}
	case strings.Contains(err.Error(), "load ledger "):
		return &cliError{Code: "ledger-invalid", Message: message}
	default:
		return &cliError{Code: "ledger-write-failed", Message: message}
	}
}

// updateError names what an Update refused in the op's vocabulary. A refusal
// the transaction's own checks decided comes back as it was; anything else is
// the ledger's, named the way ledgerError names it.
func updateError(op string, kind runstate.Kind, err error) *cliError {
	if err == nil {
		return nil
	}
	var refusal *cliError
	if errors.As(err, &refusal) {
		return refusal
	}
	return ledgerError(op, kind, err)
}

// completedRecordRefusal names the obligation a caller may not record twice.
// Start reports it for itself; the ops that only finish an obligation check it
// before they write an artifact nothing will point at.
func completedRecordRefusal(op string, kind runstate.Kind, key string) *cliError {
	return &cliError{
		Code: string(kind) + "-completed",
		Message: fmt.Sprintf("%s: %s %s is already completed for this run; repeat with a reason or use a new run ID",
			op, kind, key),
	}
}

// loadDispatchInputs reads back the dispatch inputs a trace phase produced —
// the executable-owned record of what each critic was handed.
func loadDispatchInputs(path string) (*dispatchInputs, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dispatch inputs %s: %w", path, err)
	}
	var payload dispatchInputs
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("read dispatch inputs %s: malformed JSON: %w", path, err)
	}
	return &payload, nil
}

// recordInputs decodes a record's captured input identity into target.
func recordInputs(rec *runstate.Record, target any) error {
	if rec == nil || len(rec.Inputs) == 0 {
		return nil
	}
	if err := json.Unmarshal(rec.Inputs, target); err != nil {
		return fmt.Errorf("decode %s/%s inputs: %w", rec.Kind, rec.ID, err)
	}
	return nil
}

func resolveThroughSymlinks(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// resolveExistingPrefix resolves path's symlinks as far as the filesystem
// allows: the longest existing ancestor is resolved and the missing tail
// rejoined. A state dir that does not exist yet still has to be placed
// correctly, and placing it is what the confinement check answers.
func resolveExistingPrefix(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current, tail := abs, ""
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, tail), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		tail = filepath.Join(filepath.Base(current), tail)
		current = parent
	}
}

func isWithinPath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
