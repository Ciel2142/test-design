package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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
	// The row is the ledger's seq-1 record; the file carries that seq so a
	// repeated row never writes over the evidence its superseded record digested.
	if filepath.Base(out.EvidencePath) != "mutation-unappliable-SC-10-1.json" {
		t.Errorf("evidencePath = %q, want it to name mutation-unappliable-SC-10-1.json", out.EvidencePath)
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

// TestRecordMutation_ARepeatedRowKeepsTheSupersededEvidence: the row is
// reopened under a repeat reason and closed again with a different account.
// Each completion lands in a file named by its own record, so the superseded
// record still verifies against the reason it recorded.
func TestRecordMutation_ARepeatedRowKeepsTheSupersededEvidence(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "mutation-repeat-evidence"
	prepared := mutationRowInputs{
		Identity:     mutationRowIdentity{Scope: "fixturemod/orders", Name: "TestReserveRejectsNonPositive"},
		RunnerKind:   "go",
		RunDir:       filepath.Join(t.TempDir(), "run"),
		WorktreePath: filepath.Join(t.TempDir(), "worktree"),
	}
	record := func(reason string) recordMutationOutput {
		t.Helper()
		code, stdout, stderr := runVerifyOp(t, "record-mutation", requestWith(t, repo, runID, map[string]any{
			"rowId": "SC-10", "outcome": "unappliable", "reason": reason,
		}))
		if code != exitOK {
			t.Fatalf("record-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		var out recordMutationOutput
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("record-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
		}
		return out
	}

	startLedgerRecord(t, repo, runID, runstate.KindMutationRow, "SC-10", prepared)
	first := record("the guard is generated; no source site applies")
	startLedgerRecordWith(t, repo, runID, runstate.KindMutationRow, "SC-10", prepared, "mutator re-run against a rebuilt workspace")
	second := record("the guard is generated and the generator is vendored; no source site applies")
	if second.EvidencePath == first.EvidencePath {
		t.Fatalf("repeated completion wrote %q, want a file of its own beside the superseded %q", second.EvidencePath, first.EvidencePath)
	}

	out := runResume(t, repo, runID)
	replaced := supersededRecord(t, out, runstate.KindMutationRow, "SC-10")
	if replaced.Artifacts[artifactEvidence].Path != first.EvidencePath {
		t.Errorf("superseded SC-10 evidence path = %q, want %q", replaced.Artifacts[artifactEvidence].Path, first.EvidencePath)
	}
	if reasons := runstate.Verify(&replaced); len(reasons) != 0 {
		t.Errorf("Verify(superseded SC-10) = %v, want the retained evidence to still digest to what the record carries", reasons)
	}
	current := ledgerRecord(t, out, runstate.KindMutationRow, "SC-10")
	if want := filepath.Join(filepath.Dir(second.EvidencePath), "mutation-unappliable-SC-10-"+strconv.Itoa(current.Seq)+".json"); second.EvidencePath != want {
		t.Errorf("current evidence = %q, want %q, named after the record that completed it", second.EvidencePath, want)
	}
	if reasons := runstate.Verify(&current); len(reasons) != 0 {
		t.Errorf("Verify(current SC-10) = %v, want none", reasons)
	}

	data, err := os.ReadFile(first.EvidencePath)
	if err != nil {
		t.Fatalf("read the superseded evidence: %v", err)
	}
	var doc mutationUnappliableEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("superseded evidence did not decode: %v\ndata=%s", err, data)
	}
	if doc.Reason != "the guard is generated; no source site applies" {
		t.Errorf("superseded evidence reason = %q, want the first account retained", doc.Reason)
	}
}
