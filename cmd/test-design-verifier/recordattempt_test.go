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
	if !strings.Contains(got.Message, "no ## Live decision section") {
		t.Errorf("message = %q, want the missing code's own detail, not any refusal carrying \"missing\"", got.Message)
	}
}

// TestRecordAttemptLayer3NotApplicableSaysSo names the fault the operator has
// to repair: under a not-applicable decision there is no live layer to attest
// to, which is a different repair from scoping an attempt to one row.
func TestRecordAttemptLayer3NotApplicableSaysSo(t *testing.T) {
	repo := fixtureRepo(t)
	addNotApplicableLiveDecision(t, repo)
	body, err := json.Marshal(map[string]any{
		"schemaVersion": "1", "repoRoot": repo, "coverageSource": fixtureCoverageSource,
		"runID": "not-applicable-live-attempt", "layer": 3, "refusal": "live profile unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(body))
	if code != exitError || stdout != "" {
		t.Fatalf("a document with no live layer acquired evidence: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "no-applicable-rows" {
		t.Errorf("error code = %q, want no-applicable-rows", got.Code)
	}
	if !strings.Contains(got.Message, "not-applicable") {
		t.Errorf("message = %q, want it to name the not-applicable live decision", got.Message)
	}
	if strings.Contains(got.Message, "individual row") {
		t.Errorf("message = %q, want it not to blame a row-scoped attempt that was never made", got.Message)
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
