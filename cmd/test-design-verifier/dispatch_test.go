package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTraceDispatchUsesBoundSourceAndNeverInventsGapSource(t *testing.T) {
	repo := fixtureRepo(t)
	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "dispatch-source"))
	if code != exitOK {
		t.Fatalf("trace: exit=%d stderr=%s", code, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	data, err := os.ReadFile(out.DispatchPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload dispatchInputs
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]dispatchRow)
	for _, row := range payload.Rows {
		byID[row.RowID] = row
	}
	// Both packages declare TestSameName. Only the orders assertion is this row's evidence.
	bound := byID["SC-09"]
	if !strings.Contains(bound.TestSource, "ValidateQuantity(1)") || strings.Contains(bound.TestSource, "ValidateAmount") {
		t.Fatalf("critic received a different test's assertion: %q", bound.TestSource)
	}
	if !strings.HasSuffix(bound.TestSource, "\n}\n") {
		t.Fatalf("critic source was truncated: %q", bound.TestSource)
	}
	for _, id := range []string{"SC-04", "SC-08"} {
		if row := byID[id]; row.TestSource != "" || row.TestSourceSHA256 != "" {
			t.Fatalf("unbound %s received source evidence: %+v", id, row)
		}
	}
}
