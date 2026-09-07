package main

import (
	"encoding/json"
	"strings"
	"testing"
)

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
