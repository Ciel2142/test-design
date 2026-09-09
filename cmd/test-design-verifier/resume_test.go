package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"test-design/internal/runstate"
)

// runResume drives the real resume op over repo's run and decodes its report.
func runResume(t *testing.T, repo, runID string) resumeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "resume", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("resume exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out resumeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("resume stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// resumeRow returns the mutation row resume reported for rowID.
func resumeRow(t *testing.T, out resumeOutput, rowID string) resumeMutationRow {
	t.Helper()
	for _, row := range out.Mutation.Rows {
		if row.RowID == rowID {
			return row
		}
	}
	t.Fatalf("no %s row in %+v", rowID, out.Mutation.Rows)
	return resumeMutationRow{}
}

// resumeRowIDs names the mutation rows resume reported, in the order it did.
func resumeRowIDs(out resumeOutput) []string {
	ids := make([]string, 0, len(out.Mutation.Rows))
	for _, row := range out.Mutation.Rows {
		ids = append(ids, row.RowID)
	}
	return ids
}

// TestResume_BeforeAnyOpOffersOnlyTheTracePhase locks what a parent sees when
// it asks about a run nothing has started: no ledger, one obligation, and the
// two later layers named as waiting on the trace rather than as satisfied.
func TestResume_BeforeAnyOpOffersOnlyTheTracePhase(t *testing.T) {
	repo := fixtureRepo(t)

	out := runResume(t, repo, "never-started")

	if out.Initialized {
		t.Errorf("initialized = true, want false before any op wrote a ledger")
	}
	if out.Identity != nil {
		t.Errorf("identity = %+v, want none before the ledger exists", out.Identity)
	}
	if out.Stale {
		t.Errorf("stale = true, want false: a run with no ledger has no identity to differ from")
	}
	if out.Trace.Status != "none" {
		t.Errorf("trace.status = %q, want none", out.Trace.Status)
	}
	want := []resumePending{{Kind: runstate.KindPhase, ID: "trace", Reason: "never-started"}}
	if !reflect.DeepEqual(out.Pending, want) {
		t.Errorf("pending = %+v, want %+v", out.Pending, want)
	}
	if out.Review.Reason != "trace-incomplete" {
		t.Errorf("review.reason = %q, want trace-incomplete", out.Review.Reason)
	}
	if out.Mutation.Reason != "trace-incomplete" {
		t.Errorf("mutation.reason = %q, want trace-incomplete", out.Mutation.Reason)
	}
	if out.Review.Complete || out.Mutation.Complete {
		t.Errorf("review.complete = %v, mutation.complete = %v, want both false", out.Review.Complete, out.Mutation.Complete)
	}
	if len(out.Records) != 0 {
		t.Errorf("records = %+v, want none", out.Records)
	}
	if (out.Counts != resumeCounts{Pending: 1}) {
		t.Errorf("counts = %+v, want only one pending obligation", out.Counts)
	}
	if filepath.Base(out.LedgerPath) != runstate.LedgerFile {
		t.Errorf("ledgerPath = %q, want it to name %s", out.LedgerPath, runstate.LedgerFile)
	}
	if _, err := os.Stat(out.LedgerPath); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) err = %v, want resume to have written no ledger", out.LedgerPath, err)
	}
}

// TestResume_AfterTraceReportsThePhaseAndTheRowsItOwes reads the run back from
// the ledger alone: the completed phase with the paths its evidence landed on,
// every active row still ungrouped, and the two high-risk rows the mutation
// layer owes, none of them started.
func TestResume_AfterTraceReportsThePhaseAndTheRowsItOwes(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "resume-after-trace"

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)

	out := runResume(t, repo, runID)

	if !out.Initialized {
		t.Fatalf("initialized = false, want true after trace wrote a ledger")
	}
	if out.Stale {
		t.Errorf("stale = true (%s), want false against the commit and source the trace ran on", out.StaleReason)
	}
	if out.Identity == nil || out.Identity.RunID != runID {
		t.Fatalf("identity = %+v, want the run's own identity", out.Identity)
	}
	if out.Trace.Status != "completed" || out.Trace.Outcome != "ok" {
		t.Errorf("trace = %+v, want completed ok", out.Trace)
	}
	if out.Trace.EvidencePath != tr.EvidencePath {
		t.Errorf("trace.evidencePath = %q, want %q", out.Trace.EvidencePath, tr.EvidencePath)
	}
	if out.Trace.DispatchPath != tr.DispatchPath {
		t.Errorf("trace.dispatchPath = %q, want %q", out.Trace.DispatchPath, tr.DispatchPath)
	}
	if out.Trace.RunDir != tr.RunDir {
		t.Errorf("trace.runDir = %q, want %q", out.Trace.RunDir, tr.RunDir)
	}
	if out.Trace.Reason != "" {
		t.Errorf("trace.reason = %q, want none: the retained evidence still verifies", out.Trace.Reason)
	}

	wantRows := []string{"SC-10", "SC-07"}
	if got := resumeRowIDs(out); !reflect.DeepEqual(got, wantRows) {
		t.Errorf("mutation rows = %v, want the high-risk rows in dispatch order %v", got, wantRows)
	}
	for _, id := range wantRows {
		if row := resumeRow(t, out, id); row.Status != "none" {
			t.Errorf("%s = %+v, want status none", id, row)
		}
	}
	if out.Mutation.Complete {
		t.Error("mutation.complete = true, want false while two rows are owed")
	}
	if out.Mutation.Reason != "" {
		t.Errorf("mutation.reason = %q, want none once the trace is complete", out.Mutation.Reason)
	}

	wantUncovered := []string{"SC-01", "SC-02", "SC-03", "SC-04", "SC-09", "SC-10", "SC-05", "SC-06", "SC-07", "SC-08"}
	if !reflect.DeepEqual(out.Review.Uncovered, wantUncovered) {
		t.Errorf("review.uncovered = %v, want every active row in dispatch order %v", out.Review.Uncovered, wantUncovered)
	}
	if out.Review.Complete || out.Review.Reason != "" {
		t.Errorf("review = %+v, want incomplete with no trace-incomplete reason", out.Review)
	}
	if len(out.Review.Groups) != 0 {
		t.Errorf("review.groups = %+v, want none", out.Review.Groups)
	}

	if out.Live.Required {
		t.Errorf("live.required = %v, want false: the fixture document has no live layer", out.Live.Required)
	}
	if out.Live.RecoveryRequired {
		t.Error("live.recoveryRequired = true, want false with no live attempt started")
	}

	wantPending := []resumePending{
		{Kind: runstate.KindReviewGroup, ID: "", Reason: "uncovered: " + "SC-01, SC-02, SC-03, SC-04, SC-09, SC-10, SC-05, SC-06, SC-07, SC-08"},
		{Kind: runstate.KindMutationRow, ID: "SC-10", Reason: "never-started"},
		{Kind: runstate.KindMutationRow, ID: "SC-07", Reason: "never-started"},
	}
	if !reflect.DeepEqual(out.Pending, wantPending) {
		t.Errorf("pending = %+v, want %+v", out.Pending, wantPending)
	}
	if (out.Counts != resumeCounts{Completed: 1, Pending: 3}) {
		t.Errorf("counts = %+v, want one completed record and three pending obligations", out.Counts)
	}
}
