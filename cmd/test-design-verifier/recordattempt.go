package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/live"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

type attemptRequest struct {
	Layer   int    `json:"layer"`
	RowID   string `json:"rowID"`
	Refusal string `json:"refusal"`
}

type attemptEvidence struct {
	evidence.Envelope
	RunID        string `json:"runID"`
	RowID        string `json:"rowID,omitempty"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
	RepoCommit   string `json:"repoCommit"`
	Profile      string `json:"profile,omitempty"`
	Refusal      string `json:"refusal"`
}

type attemptOutput struct {
	evidence.Envelope
	EvidencePath string `json:"evidencePath"`
}

func newRecordAttemptFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("record-attempt", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func recordAttemptHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	const op = "record-attempt"
	req, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return cliErr
	}
	var attempt attemptRequest
	if err := json.Unmarshal(requestBody, &attempt); err != nil {
		return &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if req.RunID == "" {
		return &cliError{Code: "missing-run-id", Message: op + ": runID is required"}
	}
	if strings.TrimSpace(attempt.Refusal) == "" {
		return &cliError{Code: "missing-refusal", Message: op + ": name the observed refusal; an empty attempt is not evidence"}
	}
	if attempt.Layer != 3 && attempt.Layer != 4 {
		return &cliError{Code: "invalid-layer", Message: op + ": layer must be 3 or 4"}
	}
	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	name := liveAttemptEvidenceFile
	kind, key, outcome := runstate.KindLiveAttempt, liveAttemptID, "unavailable"
	if attempt.Layer == 3 {
		if cliErr := requireLiveDecision(op, src); cliErr != nil {
			return cliErr
		}
		// Two different faults, two different repairs: a row-scoped attempt is
		// the caller scoping evidence too narrowly, an empty applicable set is
		// the document saying there is no live layer to attest to at all.
		if attempt.RowID != "" {
			return &cliError{Code: "no-applicable-rows", Message: op + ": live attempts apply to the whole applicable live layer, not an individual row"}
		}
		if len(live.ApplicableRows(src)) == 0 {
			return &cliError{
				Code:    "no-applicable-rows",
				Message: fmt.Sprintf("%s: the live decision in %s is not-applicable: no row to verify live", op, src.Path),
			}
		}
	} else {
		index := slices.IndexFunc(src.Rows, func(row coverage.Row) bool { return row.ID == attempt.RowID })
		if index < 0 || slices.Contains(src.Retired, attempt.RowID) {
			return &cliError{Code: "unknown-row", Message: fmt.Sprintf("%s: %s is not an active row", op, attempt.RowID)}
		}
		if src.Rows[index].Risk != "high" {
			return &cliError{Code: "not-high-risk", Message: fmt.Sprintf("%s: %s does not require mutation evidence", op, attempt.RowID)}
		}
		name = "mutation-attempt.json"
		kind, key, outcome = runstate.KindMutationRow, attempt.RowID, "environmental"
	}

	// An obligation this run already answered is refused before anything is
	// created for it: the ledger is re-read under the lock at Start, so the
	// check here only spares the caller a run directory it would not keep.
	store, cliErr := openLedger(op, req)
	if cliErr != nil {
		return cliErr
	}
	ledger, cliErr := loadLedgerFor(op, kind, store)
	if cliErr != nil {
		return cliErr
	}
	current := runstate.Current(ledger, kind, key)
	if current != nil && current.Status == runstate.StatusCompleted && repeatReason(req) == "" {
		return completedRecordRefusal(op, kind, key)
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if !before.Clean() {
		return &cliError{Code: "source-not-reproducible", Message: fmt.Sprintf("%s: %s", op, describeDirty(req.RepoRoot))}
	}
	commit, cliErr := headCommit(op, req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	runDir, err := ctrl.NewRunDir(req.RunID)
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	identity := ledgerIdentity(req, req.RepoRoot, commit, src)
	// A row or a layer nothing opened is opened here: a refused attempt is the
	// whole of the obligation's life, start to finish, in one call. One already
	// started is finished where it stands, so an interrupted attempt is not
	// silently replaced.
	if current == nil || current.Status == runstate.StatusCompleted {
		var inputs any = liveAttemptInputs{RunDir: runDir, ProfilePath: src.LiveProfile}
		if kind == runstate.KindMutationRow {
			inputs = mutationRowInputs{RunnerKind: src.Runner.Kind, RunDir: runDir}
		}
		if _, err := store.Start(identity, kind, key, inputs, "", repeatReason(req)); err != nil {
			return ledgerError(op, kind, err)
		}
	}

	doc := attemptEvidence{
		Envelope: evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:    req.RunID, RowID: attempt.RowID, Source: src.Path, SourceSHA256: src.SHA256,
		RepoCommit: commit, Refusal: attempt.Refusal,
	}
	if attempt.Layer == 3 {
		doc.Profile = src.LiveProfile
	}
	path := filepath.Join(runDir, name)
	if err := evidence.WriteJSON(path, &doc); err != nil {
		return &cliError{Code: "attempt-evidence-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	after, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if equal, paths := worktree.Compare(before, after); !equal {
		return &cliError{Code: "source-changed-during-verification", Message: fmt.Sprintf("%s: repository changed: %s", op, strings.Join(paths, ", "))}
	}
	if _, err := store.Complete(identity, kind, key, outcome, map[string]string{artifactAttempt: path}, ""); err != nil {
		return ledgerError(op, kind, err)
	}
	return writeStdout(stdout, attemptOutput{Envelope: doc.Envelope, EvidencePath: path})
}
