package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// TestRecordMutation_RefusesAMalformedRequest walks the checks an unappliable
// verdict must clear. The last of them is the ledger's: a row nothing prepared
// has no obligation for this op to discharge.
func TestRecordMutation_RefusesAMalformedRequest(t *testing.T) {
	repo := fixtureRepo(t)
	cases := []struct {
		code  string
		extra map[string]any
	}{
		{"missing-row-id", map[string]any{"outcome": "unappliable", "reason": "no site applies"}},
		{"unknown-row", map[string]any{"rowId": "SC-77", "outcome": "unappliable", "reason": "no site applies"}},
		{"invalid-outcome", map[string]any{"rowId": "SC-10", "outcome": "survived", "reason": "no site applies"}},
		{"missing-reason", map[string]any{"rowId": "SC-10", "outcome": "unappliable"}},
		{"mutation-row-not-started", map[string]any{"rowId": "SC-10", "outcome": "unappliable", "reason": "no site applies"}},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			code, stdout, stderr := runVerifyOp(t, "record-mutation", requestWith(t, repo, "mutation-refusals", tc.extra))
			if code != exitError || stdout != "" {
				t.Fatalf("record-mutation was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if got := decodeCLIError(t, stderr); got.Code != tc.code {
				t.Errorf("error code = %q, want %q (message %q)", got.Code, tc.code, got.Message)
			}
		})
	}
}

// TestRecordMutation_UnappliableCompletesTheStartedRow proves the one outcome
// this op may record: the row a mutator found no site for is completed against
// evidence naming the prepared workspace, and the row is closed to a second
// verdict.
func TestRecordMutation_UnappliableCompletesTheStartedRow(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "mutation-unappliable"
	prepared := mutationRowInputs{
		Identity:     mutationRowIdentity{Scope: "fixturemod/orders", Name: "TestReserveRejectsNonPositive"},
		RunnerKind:   "go",
		RunDir:       filepath.Join(t.TempDir(), "run"),
		WorktreePath: filepath.Join(t.TempDir(), "worktree"),
	}
	startLedgerRecord(t, repo, runID, runstate.KindMutationRow, "SC-10", prepared)

	const reason = "the guard the row names is generated; no source site applies"
	request := requestWith(t, repo, runID, map[string]any{
		"rowId": "SC-10", "outcome": "unappliable", "reason": reason,
	})
	code, stdout, stderr := runVerifyOp(t, "record-mutation", request)
	if code != exitOK {
		t.Fatalf("record-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out recordMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("record-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.RunID != runID || out.RowID != "SC-10" || out.Outcome != "unappliable" {
		t.Fatalf("output = %+v, want SC-10 unappliable for %s", out, runID)
	}
	if filepath.Base(out.EvidencePath) != "mutation-unappliable-SC-10.json" {
		t.Errorf("evidencePath = %q, want it to name mutation-unappliable-SC-10.json", out.EvidencePath)
	}

	data, err := os.ReadFile(out.EvidencePath)
	if err != nil {
		t.Fatalf("read unappliable evidence: %v", err)
	}
	var doc mutationUnappliableEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unappliable evidence did not decode: %v\ndata=%s", err, data)
	}
	if doc.SchemaVersion != evidence.SchemaVersion || doc.VerifierVersion != evidence.VerifierVersion {
		t.Errorf("envelope = %+v, want the verifier's own", doc.Envelope)
	}
	if doc.RunID != runID || doc.RowID != "SC-10" || doc.Reason != reason {
		t.Errorf("evidence = %+v, want the row, run and reason it was recorded for", doc)
	}
	if doc.Source != fixtureCoverageSource || doc.SourceSHA256 == "" || doc.RepoCommit == "" {
		t.Errorf("evidence = %+v, want the coverage source and commit identity", doc)
	}
	if doc.RunDir != prepared.RunDir || doc.WorktreePath != prepared.WorktreePath {
		t.Errorf("evidence workspace = %s / %s, want the prepared %s / %s",
			doc.RunDir, doc.WorktreePath, prepared.RunDir, prepared.WorktreePath)
	}

	code, stdout, stderr = runVerifyOp(t, "record-mutation", request)
	if code != exitError || stdout != "" {
		t.Fatalf("a second verdict overwrote the row: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "mutation-row-completed" {
		t.Errorf("error code = %q, want mutation-row-completed", got.Code)
	}
}
