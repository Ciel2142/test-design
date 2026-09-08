package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRecordAttemptLayer3RefusesASourceWithoutALiveDecision locks the layer-3
// gate: an attempt cannot stand in for a live layer the document never decided
// it has.
func TestRecordAttemptLayer3RefusesASourceWithoutALiveDecision(t *testing.T) {
	repo := fixtureRepo(t)
	body, err := json.Marshal(map[string]any{
		"schemaVersion": "1", "repoRoot": repo, "coverageSource": fixtureCoverageSource,
		"runID": "undecided-live-attempt", "layer": 3, "refusal": "live profile unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(body))
	if code != exitError || stdout != "" {
		t.Fatalf("undecided live layer acquired evidence: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want live-decision-incomplete", got.Code)
	}
	if !strings.Contains(got.Message, "missing") {
		t.Errorf("message = %q, want it to carry the live decision's own refusal code", got.Message)
	}
}

func TestRecordAttemptCannotAttestToAnUnrelatedRow(t *testing.T) {
	repo := fixtureRepo(t)
	body, err := json.Marshal(map[string]any{
		"schemaVersion": "1", "repoRoot": repo, "coverageSource": fixtureCoverageSource,
		"runID": "foreign-attempt", "layer": 4, "rowID": "SC-999", "refusal": "runner unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(body))
	if code != exitError || stdout != "" || !strings.Contains(stderr, "unknown-row") {
		t.Fatalf("unrelated row acquired evidence: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}
