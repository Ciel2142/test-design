package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/live"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

const liveAttemptEvidenceFile = "live-attempt.json"

type prepareLiveOutput struct {
	evidence.Envelope
	RunID        string          `json:"runID"`
	RepoCommit   string          `json:"repoCommit"`
	Source       string          `json:"source"`
	SourceSHA256 string          `json:"sourceSha256"`
	WorktreePath string          `json:"worktreePath"`
	RunDir       string          `json:"runDir"`
	ProfilePath  string          `json:"profilePath"`
	Descriptor   json.RawMessage `json:"descriptor"`
}

type liveAttemptOutput struct {
	evidence.Envelope
	RunID        string `json:"runID"`
	RepoCommit   string `json:"repoCommit"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
	Profile      string `json:"profile"`
	Refusal      string `json:"refusal"`
}

func prepareLiveHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeVerifyRequest("prepare-live", requestBody)
	if cliErr != nil {
		return cliErr
	}
	src, cliErr := loadVerifiedSource("prepare-live", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("prepare-live: %v", err)}
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("prepare-live: %v", err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "source-not-reproducible",
			Message: fmt.Sprintf("prepare-live: source not reproducible: %s", describeDirty(req.RepoRoot)),
		}
	}

	if cliErr := requireLiveDecision("prepare-live", src); cliErr != nil {
		return cliErr
	}
	if len(live.ApplicableRows(src)) == 0 {
		return &cliError{
			Code:    "no-applicable-rows",
			Message: fmt.Sprintf("prepare-live: the live decision in %s is not-applicable: no row to verify live", src.Path),
		}
	}

	repoCommit, cliErr := headCommit("prepare-live", req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}

	// The live layer is one obligation per run, and this run may already have
	// answered it — with a journey or with a refusal. The ledger is re-read
	// under the lock when the record is written; asking here spares the caller
	// a run directory and a worktree it would not keep.
	store, cliErr := openLedger("prepare-live", req)
	if cliErr != nil {
		return cliErr
	}
	ledger, cliErr := loadLedgerFor("prepare-live", runstate.KindLiveAttempt, store)
	if cliErr != nil {
		return cliErr
	}
	current := runstate.Current(ledger, runstate.KindLiveAttempt, liveAttemptID)
	if current != nil && current.Status == runstate.StatusCompleted && repeatReason(req) == "" {
		return completedRecordRefusal("prepare-live", runstate.KindLiveAttempt, liveAttemptID)
	}

	runDir, err := ctrl.NewRunDir(req.RunID)
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("prepare-live: %v", err)}
	}

	wt, err := ctrl.Create(repoCommit)
	if err != nil {
		return &cliError{Code: "worktree-create-failed", Message: fmt.Sprintf("prepare-live: %v", err)}
	}
	disposed := false
	dispose := func() *cliError {
		if disposed {
			return nil
		}
		disposed = true
		if err := ctrl.Dispose(wt); err != nil {
			return &cliError{Code: "worktree-dispose-failed", Message: fmt.Sprintf("prepare-live: %v", err)}
		}
		return nil
	}
	defer func() { _ = dispose() }()

	// Unreachable behind the decision gate, which refuses a required decision
	// without a live-profile as target-missing; kept as this op's own guard.
	if src.LiveProfile == "" {
		return failProfileUnavailable(req, runDir, repoCommit, src, "", errors.New("no live-profile configured for coverage source "+src.Path))
	}
	profileRelPath, confineErr := confinedRelPath("prepare-live", "live-profile", "live-profile-outside-repo", src.LiveProfile)
	if confineErr != nil {
		return failProfileUnavailable(req, runDir, repoCommit, src, src.LiveProfile, fmt.Errorf("%s: %s", confineErr.Code, confineErr.Message))
	}
	profileAbsPath := filepath.Join(req.RepoRoot, filepath.FromSlash(profileRelPath))

	profile, err := live.LoadProfile(profileAbsPath)
	if err != nil {
		return failProfileUnavailable(req, runDir, repoCommit, src, src.LiveProfile, err)
	}
	if err := profile.ValidateTemplate(); err != nil {
		return failProfileUnavailable(req, runDir, repoCommit, src, src.LiveProfile, err)
	}

	descriptor := substituteRunID(profile, req.RunID)
	descriptorJSON, err := json.Marshal(descriptor)
	if err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("prepare-live: marshal descriptor: %v", err)}
	}
	snapshotJSON, err := json.Marshal(before)
	if err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("prepare-live: marshal snapshot: %v", err)}
	}

	st := &live.State{
		RunID:        req.RunID,
		RepoCommit:   repoCommit,
		Source:       src.Path,
		SourceSHA256: src.SHA256,
		WorktreePath: wt.Path,
		Snapshot:     snapshotJSON,
		ProfilePath:  profileRelPath,
		Descriptor:   descriptorJSON,
	}
	if err := live.WriteState(runDir, st); err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("prepare-live: %v", err)}
	}

	// Success hands the worktree off to execute-live, which owns disposing
	// it; disarm the deferred cleanup rather than call dispose().
	disposed = true

	out := prepareLiveOutput{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:        req.RunID,
		RepoCommit:   st.RepoCommit,
		Source:       st.Source,
		SourceSHA256: st.SourceSHA256,
		WorktreePath: wt.Path,
		RunDir:       runDir,
		ProfilePath:  profileRelPath,
		Descriptor:   descriptorJSON,
	}
	return writeStdout(stdout, out)
}

// requireLiveDecision is the layer-3 gate prepare-live, execute-live and a
// layer-3 record-attempt each pass before deciding what is applicable: an
// absent or incomplete ## Live decision is incomplete design, never an implicit
// "nothing to verify live".
func requireLiveDecision(op string, src *coverage.Source) *cliError {
	if err := coverage.ValidateLiveDecision(src); err != nil {
		return &cliError{Code: "live-decision-incomplete", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	return nil
}

// failProfileUnavailable writes attempt evidence first and records it second:
// a run that never starts still leaves finalize an auditable record to yield
// PARTIAL against, and the run's own ledger says the live layer was answered
// rather than never attempted.
func failProfileUnavailable(
	req *verifyRequest, runDir, repoCommit string, src *coverage.Source, profileName string, cause error,
) *cliError {
	const op = "prepare-live"
	attemptPath := filepath.Join(runDir, liveAttemptEvidenceFile)
	doc := liveAttemptOutput{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:        req.RunID,
		RepoCommit:   repoCommit,
		Source:       src.Path,
		SourceSHA256: src.SHA256,
		Profile:      profileName,
		Refusal:      cause.Error(),
	}
	if err := evidence.WriteJSON(attemptPath, &doc); err != nil {
		return &cliError{
			Code:    "attempt-evidence-write-failed",
			Message: fmt.Sprintf("%s: %v (after profile refusal: %v)", op, err, cause),
		}
	}
	if cliErr := recordUnavailableAttempt(op, req, src, repoCommit, runDir, attemptPath); cliErr != nil {
		cliErr.Message = fmt.Sprintf("%s (after profile refusal: %v; attempt evidence: %s)", cliErr.Message, cause, attemptPath)
		return cliErr
	}
	return &cliError{
		Code:    "profile-unavailable",
		Message: fmt.Sprintf("%s: %v (attempt evidence: %s)", op, cause, attemptPath),
	}
}

// recordUnavailableAttempt records the refused attempt exactly as a layer-3
// record-attempt does: the obligation is opened and closed in one transaction,
// because a refusal is the whole of that obligation's life. A record this run
// already started is finished where it stands rather than replaced, so an
// interrupted attempt keeps the workspace it was opened against.
func recordUnavailableAttempt(
	op string, req *verifyRequest, src *coverage.Source, repoCommit, runDir, attemptPath string,
) *cliError {
	const kind = runstate.KindLiveAttempt
	store, cliErr := openLedger(op, req)
	if cliErr != nil {
		return cliErr
	}
	identity := ledgerIdentity(req, req.RepoRoot, repoCommit, src)
	err := store.Update(identity, func(l *runstate.Ledger) error {
		current := runstate.Current(l, kind, liveAttemptID)
		if current != nil && current.Status == runstate.StatusCompleted && repeatReason(req) == "" {
			return completedRecordRefusal(op, kind, liveAttemptID)
		}
		if current == nil || current.Status == runstate.StatusCompleted {
			inputs := liveAttemptInputs{RunDir: runDir, ProfilePath: src.LiveProfile}
			if _, err := l.Start(kind, liveAttemptID, inputs, "", repeatReason(req)); err != nil {
				return err
			}
		}
		_, err := l.Complete(kind, liveAttemptID, outcomeUnavailable, map[string]string{artifactAttempt: attemptPath}, "")
		return err
	})
	return updateError(op, kind, err)
}

// substituteRunID copies p rather than mutating the loaded profile.
func substituteRunID(p *live.Profile, runID string) *live.Profile {
	out := *p
	out.Start = substituteArgvGroups(p.Start, runID)
	out.Health = substituteArgvGroups(p.Health, runID)
	out.Walkthrough = substituteArgvGroups(p.Walkthrough, runID)
	out.Cleanup = substituteArgvGroups(p.Cleanup, runID)
	out.ResourcePrefixes = substituteStrings(p.ResourcePrefixes, runID)
	out.Endpoints = substituteStrings(p.Endpoints, runID)
	return &out
}

func substituteArgvGroups(groups [][]string, runID string) [][]string {
	out := make([][]string, len(groups))
	for i, argv := range groups {
		out[i] = substituteStrings(argv, runID)
	}
	return out
}

func substituteStrings(in []string, runID string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ReplaceAll(s, "{run-id}", runID)
	}
	return out
}
