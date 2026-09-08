package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/report"
)

// traceCreditCoverageSource is the repo-relative path every structural-credit
// fixture repository writes its coverage source to.
const traceCreditCoverageSource = "docs/superpowers/coverage/2026-09-07-structural-credit-coverage.md"

// traceCreditModuleFile is the single test file each fixture module carries.
const traceCreditModuleFile = "credit_test.go"

// traceCreditDoc renders a coverage source around the caller's runner command
// and scenario rows. Every other field is the minimum trace reads; the
// baseline is deliberately absent so trace records baselineStatus "missing"
// and skips baseline comparison.
func traceCreditDoc(runnerCommand, rows string) string {
	return `# Coverage: structural credit

Spec: docs/superpowers/specs/2026-09-07-structural-credit-design.md
Baseline: 0000000 · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ` + runnerCommand + `
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-structural-credit-baseline.json
- baseline-sha256: 0000000000000000000000000000000000000000000000000000000000000000
- final-command: ["/test-design:coverage-verification", "` + traceCreditCoverageSource + `"]

## Scenarios

### Behavior: credit follows the evidence of what executed

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
` + rows
}

// buildTraceCreditRepo builds a temporary Git repository holding one Go
// module, one test file and one coverage source, and returns the repository
// root with the repo-relative coverage source path.
func buildTraceCreditRepo(t *testing.T, module, testSource, runnerCommand, rows string) (repo, coverageSource string) {
	t.Helper()
	isolateEnv(t)
	repo = t.TempDir()

	mustWriteFileAt(t, filepath.Join(repo, "go.mod"), "module "+module+"\n\ngo 1.26\n")
	mustWriteFileAt(t, filepath.Join(repo, traceCreditModuleFile), testSource)
	mustWriteFileAt(t, filepath.Join(repo, filepath.FromSlash(traceCreditCoverageSource)),
		traceCreditDoc(runnerCommand, rows))

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "structural credit fixture")

	return repo, traceCreditCoverageSource
}

// runTraceCredit runs the real trace op over repo and returns its stdout
// summary together with the decoded trace evidence and dispatch inputs.
func runTraceCredit(t *testing.T, repo, coverageSource, runID string) (traceOutput, traceEvidence, dispatchInputs) {
	t.Helper()
	requestBody, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": coverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal trace request: %v", err)
	}
	code, stdout, stderr := runVerifyOp(t, "trace", string(requestBody))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	ev := readTraceEvidence(t, out)

	data, err := os.ReadFile(out.DispatchPath)
	if err != nil {
		t.Fatalf("read dispatch inputs: %v", err)
	}
	var payload dispatchInputs
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode dispatch inputs: %v", err)
	}
	return out, ev, payload
}

// mustRowResult returns the single RowResult for rowID and fails when the row
// has none or more than one.
func mustRowResult(t *testing.T, ev traceEvidence, rowID string) traceRowResult {
	t.Helper()
	var found []traceRowResult
	for _, r := range ev.RowResults {
		if r.RowID == rowID {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: %d RowResults, want exactly 1: %+v", rowID, len(found), ev.RowResults)
	}
	return found[0]
}

// mustDispatchRow returns the dispatch row for rowID.
func mustDispatchRow(t *testing.T, payload dispatchInputs, rowID string) dispatchRow {
	t.Helper()
	for _, row := range payload.Rows {
		if row.RowID == rowID {
			return row
		}
	}
	t.Fatalf("%s: no dispatch row in %+v", rowID, payload.Rows)
	return dispatchRow{}
}

// wantPrimaryIsFirstContributor checks the invariant that a row's legacy
// scalar fields are its first contributor, whatever that contributor's
// execution status.
func wantPrimaryIsFirstContributor(t *testing.T, result traceRowResult) {
	t.Helper()
	if len(result.Contributors) == 0 {
		t.Fatalf("%s: no contributors on RowResult %+v", result.RowID, result)
	}
	primary := result.Contributors[0]
	if result.Scope != primary.Scope || result.Name != primary.Name ||
		result.File != primary.File || result.Line != primary.Line || result.EndLine != primary.EndLine {
		t.Errorf("%s: scalar fields %+v do not name the first contributor %+v", result.RowID, result, primary)
	}
}

// contributorNames lists contributor names in evidence order.
func contributorNames(cs []traceContributor) []string {
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.Name)
	}
	return names
}

// dispatchContributorNames lists dispatch contributor names in order.
func dispatchContributorNames(cs []dispatchContributor) []string {
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.TestName)
	}
	return names
}

// oneTestThreeRowsSource cites three active IDs from one passing test: under
// structural credit every distinct active ID a site cites binds.
const oneTestThreeRowsSource = `package creditproof

import "testing"

// covers: SC-01, SC-02, SC-03
func TestOneTestCoversThreeRows(t *testing.T) {
	if got := 2 + 2; got != 4 {
		t.Fatalf("2+2 = %d, want 4", got)
	}
}
`

const oneTestThreeRows = `| SC-01 | the shared test's first behavior | the shared test passes | unit | first assertion removed | low |
| SC-02 | the shared test's second behavior | the shared test passes | unit | second assertion removed | low |
| SC-03 | the shared test's third behavior | the shared test passes | unit | third assertion removed | low |
`

// covers: SC-02
func TestTraceCreditsOneTestForThreeRows(t *testing.T) {
	repo, coverageSource := buildTraceCreditRepo(t, "creditproof", oneTestThreeRowsSource,
		`["go", "test", "-count=1", "-json", "./..."]`, oneTestThreeRows)
	_, ev, payload := runTraceCredit(t, repo, coverageSource, "one-test-three-rows")

	if len(ev.RowResults) != 3 {
		t.Fatalf("rowResults = %+v, want one per cited row", ev.RowResults)
	}
	if len(ev.Findings) != 0 {
		t.Errorf("findings = %+v, want none: citing three IDs from one site is credit, not a finding", ev.Findings)
	}
	if ev.RowAccounting.ActiveRows != 3 || ev.RowAccounting.WithResult != 3 || len(ev.RowAccounting.Unaccounted) != 0 {
		t.Errorf("rowAccounting = %+v, want 3 active rows all credited", ev.RowAccounting)
	}

	for _, rowID := range []string{"SC-01", "SC-02", "SC-03"} {
		result := mustRowResult(t, ev, rowID)
		if result.Status != "passed" || result.Scope != "creditproof" || result.Name != "TestOneTestCoversThreeRows" {
			t.Errorf("%s: RowResult = %+v, want creditproof TestOneTestCoversThreeRows passed", rowID, result)
		}
		if result.File != traceCreditModuleFile || result.Line != 5 || result.EndLine != 10 {
			t.Errorf("%s: site = %s:%d-%d, want %s:5-10", rowID, result.File, result.Line, result.EndLine, traceCreditModuleFile)
		}
		if len(result.Contributors) != 1 {
			t.Fatalf("%s: contributors = %+v, want the one citing test", rowID, result.Contributors)
		}
		c := result.Contributors[0]
		if !c.Executed || c.Status != "passed" || c.Name != "TestOneTestCoversThreeRows" {
			t.Errorf("%s: contributor = %+v, want the executed passing test", rowID, c)
		}
		wantPrimaryIsFirstContributor(t, result)

		row := mustDispatchRow(t, payload, rowID)
		if row.Status != "passed" || row.TestName != "TestOneTestCoversThreeRows" {
			t.Errorf("%s: dispatch row = %+v, want the passing test's identity", rowID, row)
		}
		if !strings.Contains(row.TestSource, "func TestOneTestCoversThreeRows") || row.TestSourceSHA256 == "" {
			t.Errorf("%s: dispatch source = %q (sha %q), want the cited test's declaration", rowID, row.TestSource, row.TestSourceSHA256)
		}
		if len(row.Contributors) != 1 {
			t.Fatalf("%s: dispatch contributors = %+v, want the one citing test", rowID, row.Contributors)
		}
		dc := row.Contributors[0]
		if dc.TestName != row.TestName || dc.TestSource != row.TestSource || dc.TestSourceSHA256 != row.TestSourceSHA256 {
			t.Errorf("%s: dispatch contributor %+v does not match the row's primary source", rowID, dc)
		}
	}
}

// twoTestsOneRowSource binds two tests to each of two rows: SC-01's pair both
// pass, while SC-02's second contributor fails after its first passes.
const twoTestsOneRowSource = `package combineproof

import "testing"

// covers: SC-01
func TestFirstContributorPasses(t *testing.T) {
	if got := 1 + 1; got != 2 {
		t.Fatalf("1+1 = %d, want 2", got)
	}
}

// covers: SC-01
func TestSecondContributorPasses(t *testing.T) {
	if got := 2 + 3; got != 5 {
		t.Fatalf("2+3 = %d, want 5", got)
	}
}

// covers: SC-02
func TestPassingSibling(t *testing.T) {
	if got := 3 + 4; got != 7 {
		t.Fatalf("3+4 = %d, want 7", got)
	}
}

// covers: SC-02
func TestFailingContributor(t *testing.T) {
	t.Fatal("seeded failure: SC-02's second contributor fails")
}
`

const twoTestsOneRow = `| SC-01 | two tests exercise one row | both cited tests pass | unit | one arm's guard removed | low |
| SC-02 | a failing contributor is not hidden by a passing sibling | the failing test's identity survives | unit | the failing arm's assertion removed | low |
`

// covers: SC-03
func TestTraceCombinesTwoTestsForOneRow(t *testing.T) {
	repo, coverageSource := buildTraceCreditRepo(t, "combineproof", twoTestsOneRowSource,
		`["go", "test", "-count=1", "-json", "./..."]`, twoTestsOneRow)
	_, ev, payload := runTraceCredit(t, repo, coverageSource, "two-tests-one-row")

	for _, f := range ev.Findings {
		if f.Kind == "ambiguity" {
			t.Errorf("finding %+v: two sites binding one row is credit, not ambiguity", f)
		}
	}

	passing := mustRowResult(t, ev, "SC-01")
	if passing.Status != "passed" {
		t.Errorf("SC-01: status = %q, want passed", passing.Status)
	}
	if got := contributorNames(passing.Contributors); len(got) != 2 ||
		got[0] != "TestFirstContributorPasses" || got[1] != "TestSecondContributorPasses" {
		t.Errorf("SC-01: contributors = %v, want both citing tests in file order", got)
	}
	for _, c := range passing.Contributors {
		if !c.Executed || c.Status != "passed" {
			t.Errorf("SC-01: contributor %+v, want executed and passed", c)
		}
	}
	wantPrimaryIsFirstContributor(t, passing)

	failing := mustRowResult(t, ev, "SC-02")
	if failing.Status != "failed" {
		t.Errorf("SC-02: status = %q, want failed: a failing contributor dominates a passing one", failing.Status)
	}
	if got := contributorNames(failing.Contributors); len(got) != 2 ||
		got[0] != "TestPassingSibling" || got[1] != "TestFailingContributor" {
		t.Errorf("SC-02: contributors = %v, want both identities retained in file order", got)
	}
	if got := failing.Contributors[0].Status; got != "passed" {
		t.Errorf("SC-02: first contributor status = %q, want passed", got)
	}
	if got := failing.Contributors[1].Status; got != "failed" {
		t.Errorf("SC-02: second contributor status = %q, want failed", got)
	}
	// The primary is the first contributor by site, not the one whose status
	// the row took: SC-02 is failed while its scalar fields name the sibling
	// that passed.
	wantPrimaryIsFirstContributor(t, failing)
	if failing.Name != "TestPassingSibling" {
		t.Errorf("SC-02: name = %q, want the first contributor by site", failing.Name)
	}
	wantFinding(t, ev, "red", "SC-02", "TestFailingContributor")

	passingRow := mustDispatchRow(t, payload, "SC-01")
	if passingRow.Status != "passed" {
		t.Errorf("SC-01: dispatch status = %q, want passed", passingRow.Status)
	}
	if got := dispatchContributorNames(passingRow.Contributors); len(got) != 2 ||
		got[0] != "TestFirstContributorPasses" || got[1] != "TestSecondContributorPasses" {
		t.Errorf("SC-01: dispatch contributors = %v, want both citing tests", got)
	}
	first, second := passingRow.Contributors[0], passingRow.Contributors[1]
	if !strings.Contains(first.TestSource, "func TestFirstContributorPasses") ||
		!strings.Contains(second.TestSource, "func TestSecondContributorPasses") {
		t.Errorf("SC-01: dispatch contributors carry the wrong sources: %q / %q", first.TestSource, second.TestSource)
	}
	if first.TestSourceSHA256 == "" || first.TestSourceSHA256 == second.TestSourceSHA256 {
		t.Errorf("SC-01: contributor hashes = %q / %q, want one per distinct range", first.TestSourceSHA256, second.TestSourceSHA256)
	}

	failingRow := mustDispatchRow(t, payload, "SC-02")
	if failingRow.Status != "failed" || failingRow.TestName != "TestPassingSibling" {
		t.Errorf("SC-02: dispatch row = %+v, want failed status on the primary contributor's identity", failingRow)
	}
	if got := dispatchContributorNames(failingRow.Contributors); len(got) != 2 ||
		got[0] != "TestPassingSibling" || got[1] != "TestFailingContributor" {
		t.Errorf("SC-02: dispatch contributors = %v, want both identities retained", got)
	}
	if failingRow.TestSource != failingRow.Contributors[0].TestSource ||
		failingRow.TestSourceSHA256 != failingRow.Contributors[0].TestSourceSHA256 {
		t.Errorf("SC-02: dispatch row source does not come from the primary contributor: %+v", failingRow)
	}
	if !strings.Contains(failingRow.Contributors[1].TestSource, "seeded failure") {
		t.Errorf("SC-02: failing contributor source = %q, want its own declaration", failingRow.Contributors[1].TestSource)
	}
}

// withheldCreditSource pairs each row with one contributor that does not earn
// credit: SC-01's first test skips, and SC-02's first test is excluded from
// the run by the runner command's -run filter.
const withheldCreditSource = `package withholdproof

import "testing"

// covers: SC-01
func TestSkippedContributor(t *testing.T) {
	t.Skip("the behavior this contributor would prove is not implemented yet")
}

// covers: SC-01
func TestPassingContributor(t *testing.T) {
	if got := 5 + 5; got != 10 {
		t.Fatalf("5+5 = %d, want 10", got)
	}
}

// covers: SC-02
func TestExcludedFromRun(t *testing.T) {
	if got := 6 + 6; got != 12 {
		t.Fatalf("6+6 = %d, want 12", got)
	}
}

// covers: SC-02
func TestExecutedContributor(t *testing.T) {
	if got := 7 + 7; got != 14 {
		t.Fatalf("7+7 = %d, want 14", got)
	}
}
`

const withheldCreditRows = `| SC-01 | a skipped contributor earns no credit | the skipped test is named as a gap | unit | the skip removed | low |
| SC-02 | a contributor the run never reached earns no credit | the absent test is named as a gap | unit | the filter widened | low |
`

// covers: SC-04
func TestTraceWithholdsCreditForSkippedContributor(t *testing.T) {
	// -run Contributor reaches TestSkippedContributor, TestPassingContributor
	// and TestExecutedContributor, and never reaches TestExcludedFromRun.
	repo, coverageSource := buildTraceCreditRepo(t, "withholdproof", withheldCreditSource,
		`["go", "test", "-count=1", "-json", "-run", "Contributor", "./..."]`, withheldCreditRows)
	out, ev, payload := runTraceCredit(t, repo, coverageSource, "withheld-credit")

	// A skipped contributor beside a passing one: the row is credited by what
	// executed, and the skipped site earns its own gap.
	skipped := mustRowResult(t, ev, "SC-01")
	if skipped.Status != "skipped" {
		t.Errorf("SC-01: status = %q, want skipped: a skipped contributor dominates a passing one", skipped.Status)
	}
	if got := contributorNames(skipped.Contributors); len(got) != 2 ||
		got[0] != "TestSkippedContributor" || got[1] != "TestPassingContributor" {
		t.Fatalf("SC-01: contributors = %v, want both citing tests in file order", got)
	}
	if c := skipped.Contributors[0]; !c.Executed || c.Status != "skipped" {
		t.Errorf("SC-01: first contributor = %+v, want executed and skipped", c)
	}
	if c := skipped.Contributors[1]; !c.Executed || c.Status != "passed" {
		t.Errorf("SC-01: second contributor = %+v, want executed and passed", c)
	}
	wantPrimaryIsFirstContributor(t, skipped)
	wantFinding(t, ev, "gap", "SC-01", "TestSkippedContributor")

	// A contributor the run never reached: the executed sibling still credits
	// the row, and the primary stays the first contributor by site even
	// though it never executed.
	absent := mustRowResult(t, ev, "SC-02")
	if absent.Status != "passed" {
		t.Errorf("SC-02: status = %q, want passed: the row collapses over executed contributors only", absent.Status)
	}
	if got := contributorNames(absent.Contributors); len(got) != 2 ||
		got[0] != "TestExcludedFromRun" || got[1] != "TestExecutedContributor" {
		t.Fatalf("SC-02: contributors = %v, want both citing tests in file order", got)
	}
	unexecuted := absent.Contributors[0]
	if unexecuted.Executed || unexecuted.Status != "" {
		t.Errorf("SC-02: first contributor = %+v, want unexecuted with no status", unexecuted)
	}
	if c := absent.Contributors[1]; !c.Executed || c.Status != "passed" {
		t.Errorf("SC-02: second contributor = %+v, want executed and passed", c)
	}
	wantPrimaryIsFirstContributor(t, absent)
	if absent.Name != "TestExcludedFromRun" {
		t.Errorf("SC-02: name = %q, want the first contributor by site regardless of execution", absent.Name)
	}
	wantFinding(t, ev, "gap", "SC-02", fmt.Sprintf("%s:%d", unexecuted.File, unexecuted.Line))

	// Both rows keep their contributors and their primary's source in the
	// dispatch inputs even though each row's status is a finding kind.
	for _, rowID := range []string{"SC-01", "SC-02"} {
		row := mustDispatchRow(t, payload, rowID)
		if row.Status != "gap" {
			t.Errorf("%s: dispatch status = %q, want gap", rowID, row.Status)
		}
		if len(row.Contributors) != 2 {
			t.Errorf("%s: dispatch contributors = %+v, want both citing tests", rowID, row.Contributors)
			continue
		}
		if row.TestSource == "" || row.TestSource != row.Contributors[0].TestSource {
			t.Errorf("%s: dispatch row source does not come from the primary contributor: %+v", rowID, row)
		}
	}

	// Downstream, neither row counts as bound.
	src, cliErr := loadVerifiedSource("trace", repo, coverageSource)
	if cliErr != nil {
		t.Fatalf("load coverage source: %s", cliErr.Message)
	}
	head, cliErr := headCommit("trace", repo)
	if cliErr != nil {
		t.Fatalf("head commit: %s", cliErr.Message)
	}
	tr, err := report.LoadTrace(out.EvidencePath)
	if err != nil {
		t.Fatalf("load trace evidence: %v", err)
	}
	computed, err := report.Compute(&report.Inputs{
		Source:     src,
		RunID:      ev.RunID,
		HeadCommit: head,
		Trace:      tr,
		Freshness:  report.Freshness{Result: "fresh"},
		Batches: []report.Batch{{
			BatchID: "batch-1", DispatchID: "dispatch-1", RunID: ev.RunID,
			Rows:                    []string{"SC-01", "SC-02"},
			CanaryUnavailableReason: "no critic runs in this layer-1 proof",
			Verdicts: []report.CriticVerdict{
				{RowID: "SC-01", Verdict: "MISSING", Reason: "layer-1 gap"},
				{RowID: "SC-02", Verdict: "MISSING", Reason: "layer-1 gap"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("compute report: %v", err)
	}
	for _, row := range computed.Rows {
		if row.Layer1.Status == "bound" {
			t.Errorf("%s: layer-1 status = bound, want a gap: one contributor earned no credit", row.RowID)
		}
	}
}

// redOverGapSource binds one row to two contributors that each withhold
// credit differently: the first fails, the second skips. The row collapses to
// failed while both a red and a gap finding name it.
const redOverGapSource = `package precedenceproof

import "testing"

// covers: SC-01
func TestFailingArm(t *testing.T) {
	t.Fatal("seeded failure: SC-01's failing contributor")
}

// covers: SC-01
func TestSkippedArm(t *testing.T) {
	t.Skip("SC-01's other contributor never reaches its assertion")
}
`

const redOverGapRows = `| SC-01 | a failed contributor outranks a skipped one | the row is named red everywhere | unit | the failing arm's assertion removed | low |
`

func TestTraceDispatchKeepsFailedRowsRedBesideAGap(t *testing.T) {
	repo, coverageSource := buildTraceCreditRepo(t, "precedenceproof", redOverGapSource,
		`["go", "test", "-count=1", "-json", "./..."]`, redOverGapRows)
	out, ev, payload := runTraceCredit(t, repo, coverageSource, "red-over-gap")

	result := mustRowResult(t, ev, "SC-01")
	if result.Status != "failed" {
		t.Errorf("SC-01: status = %q, want failed: a failing contributor dominates a skipped one", result.Status)
	}
	if got := contributorNames(result.Contributors); len(got) != 2 ||
		got[0] != "TestFailingArm" || got[1] != "TestSkippedArm" {
		t.Fatalf("SC-01: contributors = %v, want both citing tests in file order", got)
	}
	wantFinding(t, ev, "red", "SC-01", "TestFailingArm")
	wantFinding(t, ev, "gap", "SC-01", "TestSkippedArm")

	// The row carries both findings, and the red one settles its status: the
	// gap must not talk the dispatch inputs down from failed.
	row := mustDispatchRow(t, payload, "SC-01")
	if row.Status != "failed" {
		t.Errorf("SC-01: dispatch status = %q, want failed: red outranks the gap beside it", row.Status)
	}

	// The report reads the same findings and must name the same state.
	src, cliErr := loadVerifiedSource("trace", repo, coverageSource)
	if cliErr != nil {
		t.Fatalf("load coverage source: %s", cliErr.Message)
	}
	head, cliErr := headCommit("trace", repo)
	if cliErr != nil {
		t.Fatalf("head commit: %s", cliErr.Message)
	}
	tr, err := report.LoadTrace(out.EvidencePath)
	if err != nil {
		t.Fatalf("load trace evidence: %v", err)
	}
	computed, err := report.Compute(&report.Inputs{
		Source:     src,
		RunID:      ev.RunID,
		HeadCommit: head,
		Trace:      tr,
		Freshness:  report.Freshness{Result: "fresh"},
		Batches: []report.Batch{{
			BatchID: "batch-1", DispatchID: "dispatch-1", RunID: ev.RunID,
			Rows:                    []string{"SC-01"},
			CanaryUnavailableReason: "no critic runs in this layer-1 proof",
			Verdicts: []report.CriticVerdict{
				{RowID: "SC-01", Verdict: "MISSING", Reason: "layer-1 red"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("compute report: %v", err)
	}
	if len(computed.Rows) != 1 {
		t.Fatalf("report rows = %+v, want the single active row", computed.Rows)
	}
	if got := computed.Rows[0].Layer1.Status; got != "red" {
		t.Errorf("SC-01: layer-1 status = %q, want red", got)
	}
}
