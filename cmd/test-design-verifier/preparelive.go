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
	"test-design/internal/worktree"
)

const liveAttemptEvidenceFile = "live-attempt.json"

type prepareLiveOutput struct {
	evidence.Envelope
	RunID        string          `json:"runID"`
	WorktreePath string          `json:"worktreePath"`
	RunDir       string          `json:"runDir"`
	ProfilePath  string          `json:"profilePath"`
	Descriptor   json.RawMessage `json:"descriptor"`
}

type liveAttemptOutput struct {
	evidence.Envelope
	RunID   string `json:"runID"`
	Profile string `json:"profile"`
	Refusal string `json:"refusal"`
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

	if !hasApplicableLiveRows(src) {
		return &cliError{
			Code:    "no-applicable-rows",
			Message: fmt.Sprintf("prepare-live: no active row at level smoke or e2e in %s", src.Path),
		}
	}

	repoCommit, cliErr := headCommit("prepare-live", req.RepoRoot)
	if cliErr != nil {
		return cliErr
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

	if src.LiveProfile == "" {
		return failProfileUnavailable(runDir, req.RunID, "", errors.New("no live-profile configured for coverage source "+src.Path))
	}
	profileRelPath, confineErr := confinedRelPath("prepare-live", "live-profile", "live-profile-outside-repo", src.LiveProfile)
	if confineErr != nil {
		return failProfileUnavailable(runDir, req.RunID, src.LiveProfile, fmt.Errorf("%s: %s", confineErr.Code, confineErr.Message))
	}
	profileAbsPath := filepath.Join(req.RepoRoot, filepath.FromSlash(profileRelPath))

	profile, err := live.LoadProfile(profileAbsPath)
	if err != nil {
		return failProfileUnavailable(runDir, req.RunID, src.LiveProfile, err)
	}
	if err := profile.Validate(req.RunID); err != nil {
		return failProfileUnavailable(runDir, req.RunID, src.LiveProfile, err)
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
		WorktreePath: wt.Path,
		RunDir:       runDir,
		ProfilePath:  profileRelPath,
		Descriptor:   descriptorJSON,
	}
	return writeStdout(stdout, out)
}

func hasApplicableLiveRows(src *coverage.Source) bool {
	return len(applicableLiveRows(src)) > 0
}

// failProfileUnavailable writes attempt evidence first: a run that never
// starts still leaves finalize an auditable record to yield PARTIAL against.
func failProfileUnavailable(runDir, runID, profileName string, cause error) *cliError {
	attemptPath := filepath.Join(runDir, liveAttemptEvidenceFile)
	doc := liveAttemptOutput{
		Envelope: evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:    runID,
		Profile:  profileName,
		Refusal:  cause.Error(),
	}
	if err := evidence.WriteJSON(attemptPath, &doc); err != nil {
		return &cliError{
			Code:    "attempt-evidence-write-failed",
			Message: fmt.Sprintf("prepare-live: %v (after profile refusal: %v)", err, cause),
		}
	}
	return &cliError{
		Code:    "profile-unavailable",
		Message: fmt.Sprintf("prepare-live: %v (attempt evidence: %s)", cause, attemptPath),
	}
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
