package main

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"test-design/internal/evidence"
)

func decodeTraceOutput(t *testing.T, stdout string) traceOutput {
	t.Helper()
	var out traceOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("trace stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

func readTraceEvidence(t *testing.T, out traceOutput) traceEvidence {
	t.Helper()
	data, err := os.ReadFile(out.EvidencePath)
	if err != nil {
		t.Fatalf("read trace evidence: %v", err)
	}
	if _, err := evidence.DecodeEnvelope(data); err != nil {
		t.Fatalf("trace evidence envelope: %v", err)
	}
	var ev traceEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("decode trace evidence: %v", err)
	}
	return ev
}

func rowResultFor(ev traceEvidence, rowID string) (traceRowResult, bool) {
	for _, r := range ev.RowResults {
		if r.RowID == rowID {
			return r, true
		}
	}
	return traceRowResult{}, false
}

func findingsOf(ev traceEvidence, kind, rowID string) []traceFinding {
	var out []traceFinding
	for _, f := range ev.Findings {
		if f.Kind == kind && f.RowID == rowID {
			out = append(out, f)
		}
	}
	return out
}

func wantRowResult(t *testing.T, ev traceEvidence, rowID, status string) {
	t.Helper()
	got, ok := rowResultFor(ev, rowID)
	if !ok {
		t.Errorf("%s: no RowResult, want status %s", rowID, status)
		return
	}
	if got.Status != status {
		t.Errorf("%s: RowResult status = %q, want %q", rowID, got.Status, status)
	}
}

func wantFinding(t *testing.T, ev traceEvidence, kind, rowID, detailSubstring string) {
	t.Helper()
	for _, f := range findingsOf(ev, kind, rowID) {
		if strings.Contains(f.Detail, detailSubstring) {
			return
		}
	}
	t.Errorf("no %s finding for row %q containing %q; findings=%v", kind, rowID, detailSubstring, ev.Findings)
}

var maskedEvidenceFields = regexp.MustCompile(`(?m)^(\s*"(runId|startedAt|finishedAt)": ).*$`)

func maskVolatile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return maskedEvidenceFields.ReplaceAllString(string(data), `${1}"MASKED",`)
}

// TestE2E_FixturePipeline runs baseline then trace over fixtures/go-project and
// checks every seeded scenario reaches evidence as the scenario map describes.
func TestE2E_FixturePipeline(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "e2e-baseline"))
	if code != exitOK {
		t.Fatalf("baseline exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var baseOut baselineOutput
	if err := json.Unmarshal([]byte(stdout), &baseOut); err != nil {
		t.Fatalf("baseline stdout did not decode: %v", err)
	}

	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "BASELINE_SHA256", baseOut.EvidenceSHA256)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline sha256")

	code, stdout, stderr = runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "e2e-trace-1"))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	ev := readTraceEvidence(t, out)

	if ev.BaselineStatus != "ok" {
		t.Fatalf("baselineStatus = %q, want %q", ev.BaselineStatus, "ok")
	}
	if ev.ModulePath != "fixturemod" {
		t.Errorf("modulePath = %q, want %q", ev.ModulePath, "fixturemod")
	}

	wantRowResult(t, ev, "SC-01", "passed")
	if got, ok := rowResultFor(ev, "SC-01"); !ok || got.File != "orders/orders_test.go" || got.Line != 5 || got.EndLine != 10 {
		t.Errorf("SC-01 site = %+v, want orders/orders_test.go:5-10", got)
	}
	wantRowResult(t, ev, "SC-02", "failed")
	wantFinding(t, ev, "red", "SC-02", "TestValidateQuantityRejectsZero")
	wantRowResult(t, ev, "SC-03", "skipped")
	wantFinding(t, ev, "gap", "SC-03", "skipped")
	wantFinding(t, ev, "gap", "SC-04", "no test cites SC-04")
	wantRowResult(t, ev, "SC-05", "failed")
	wantFinding(t, ev, "red", "SC-05", "TestAmountValidation")
	wantRowResult(t, ev, "SC-06", "skipped")
	wantFinding(t, ev, "gap", "SC-06", "TestCurrencyValidation")
	wantFinding(t, ev, "overclaim", "", "TestOverclaimedCoverage")
	wantFinding(t, ev, "gap", "SC-07", "no test cites SC-07")
	wantFinding(t, ev, "ambiguity", "SC-08", "orders/orders_test.go:31")
	wantFinding(t, ev, "ambiguity", "SC-08", "payments/payments_test.go:38")
	wantFinding(t, ev, "drift", "", "retired ID SC-90")
	wantFinding(t, ev, "drift", "", "unknown ID SC-77")
	wantFinding(t, ev, "ambiguity", "", "orders/orders_test.go:45")
	wantRowResult(t, ev, "SC-09", "passed")
	wantRowResult(t, ev, "SC-10", "passed")

	if _, ok := rowResultFor(ev, "SC-90"); ok {
		t.Error("retired SC-90 produced a RowResult, want none")
	}
	if !slices.ContainsFunc(ev.TagClasses, func(c traceTagClass) bool { return c.ID == "SC-77" && c.Class == "unknown" }) {
		t.Errorf("tagClasses = %v, want SC-77 classified unknown", ev.TagClasses)
	}

	activeRows := []string{"SC-01", "SC-02", "SC-03", "SC-04", "SC-05", "SC-06", "SC-07", "SC-08", "SC-09", "SC-10"}
	accounted := map[string]bool{}
	for _, r := range ev.RowResults {
		if accounted[r.RowID] {
			t.Errorf("%s: appears twice in RowResults", r.RowID)
		}
		accounted[r.RowID] = true
	}
	for _, f := range ev.Findings {
		if f.RowID != "" {
			accounted[f.RowID] = true
		}
	}
	var got []string
	for id := range accounted {
		got = append(got, id)
	}
	sort.Strings(got)
	if !slices.Equal(got, activeRows) {
		t.Errorf("rows across RowResults and row-bound findings = %v, want exactly the active rows %v", got, activeRows)
	}
	if ev.RowAccounting.ActiveRows != len(activeRows) {
		t.Errorf("rowAccounting.activeRows = %d, want %d", ev.RowAccounting.ActiveRows, len(activeRows))
	}
	if len(ev.RowAccounting.Unaccounted) != 0 {
		t.Errorf("rowAccounting.unaccounted = %v, want none", ev.RowAccounting.Unaccounted)
	}

	if len(ev.NewFailures) != 0 {
		t.Errorf("newFailures = %v, want none: every current failure already failed at baseline", ev.NewFailures)
	}
	if len(findingsOf(ev, "red", "SC-02")) == 0 {
		t.Error("SC-02 lost its red finding; the active-row rule outranks the baseline exemption")
	}

	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want trace to have written nothing into the repository", lines)
	}

	code, stdout, stderr = runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "e2e-trace-2"))
	if code != exitOK {
		t.Fatalf("second trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	second := decodeTraceOutput(t, stdout)
	if first, again := maskVolatile(t, out.EvidencePath), maskVolatile(t, second.EvidencePath); first != again {
		t.Errorf("two trace runs on identical state produced different evidence:\n--- first ---\n%s\n--- second ---\n%s", first, again)
	}
}
