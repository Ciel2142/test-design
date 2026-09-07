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
	if len(bound.Contributors) != 1 {
		t.Fatalf("SC-09 contributors = %+v, want the one tagged site", bound.Contributors)
	}
	// SC-04 is cited by no test at all. Only a row with no bound site is
	// sourceless: a bound row whose status is a finding kind keeps its
	// primary contributor's source.
	if row := byID["SC-04"]; row.TestSource != "" || row.TestSourceSHA256 != "" || len(row.Contributors) != 0 {
		t.Fatalf("unbound SC-04 received source evidence: %+v", row)
	}
	if row := byID["SC-03"]; row.Status != "gap" || row.TestSource == "" || len(row.Contributors) != 1 {
		t.Fatalf("gap row SC-03 lost its bound site's source: %+v", row)
	}

	// SC-08 is cited by one test in each package: the critic receives both
	// declarations, each with its own source and hash, and the row's own
	// scalar fields come from the primary contributor.
	multi := byID["SC-08"]
	if len(multi.Contributors) != 2 {
		t.Fatalf("SC-08 contributors = %+v, want one per citing test", multi.Contributors)
	}
	first, second := multi.Contributors[0], multi.Contributors[1]
	if first.TestName != "TestAmbiguousBindingOne" || first.File != "orders/orders_test.go" ||
		second.TestName != "TestAmbiguousBindingTwo" || second.File != "payments/payments_test.go" {
		t.Fatalf("SC-08 contributors = %+v, want both citing tests in file order", multi.Contributors)
	}
	if !strings.Contains(first.TestSource, "ValidateQuantity(3)") || !strings.Contains(second.TestSource, "ValidateAmount(300)") {
		t.Errorf("SC-08 contributors carry the wrong assertions: %q / %q", first.TestSource, second.TestSource)
	}
	if first.TestSourceSHA256 == "" || first.TestSourceSHA256 == second.TestSourceSHA256 {
		t.Errorf("SC-08 contributor hashes = %q / %q, want one per declaration", first.TestSourceSHA256, second.TestSourceSHA256)
	}
	if multi.TestName != first.TestName || multi.TestSource != first.TestSource || multi.TestSourceSHA256 != first.TestSourceSHA256 {
		t.Errorf("SC-08 row identity and source do not come from the primary contributor: %+v", multi)
	}
}
