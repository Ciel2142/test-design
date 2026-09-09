package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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

// TestResume_PendingIsSectionedByLayer locks the order a parent works through:
// pending is read top to bottom, so it is sectioned by layer — the review
// groups the run opened, then the rows no group claims, then the mutation rows
// it opened, then the ones it never did. Interleaving them by ledger position
// would hand the parent a list that jumps between layers.
func TestResume_PendingIsSectionedByLayer(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "resume-pending-sections"

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "started", "rows": []string{"SC-01", "SC-02"},
	})
	prepared := mutationRowInputs{RunnerKind: "go", RunDir: t.TempDir()}
	startLedgerRecord(t, repo, runID, runstate.KindMutationRow, "SC-10", prepared)

	out := runResume(t, repo, runID)

	want := []resumePending{
		{Kind: runstate.KindReviewGroup, ID: "A", Reason: "started-not-completed"},
		{Kind: runstate.KindReviewGroup, ID: "", Reason: "uncovered: " + strings.Join(out.Review.Uncovered, ", ")},
		{Kind: runstate.KindMutationRow, ID: "SC-10", Reason: "started-not-completed"},
		{Kind: runstate.KindMutationRow, ID: "SC-07", Reason: "never-started"},
	}
	if !reflect.DeepEqual(out.Pending, want) {
		t.Errorf("pending = %+v, want %+v", out.Pending, want)
	}
	if (out.Counts != resumeCounts{Started: 2, Completed: 1, Pending: 4}) {
		t.Errorf("counts = %+v, want two started obligations, the completed trace and four pending", out.Counts)
	}
	row := resumeRow(t, out, "SC-10")
	if row.Status != "started" {
		t.Errorf("SC-10 status = %q, want started", row.Status)
	}
	if row.Reason != "state-unreadable" {
		t.Errorf("SC-10 reason = %q, want state-unreadable: the prepared run dir holds no state document", row.Reason)
	}
}

// TestResume_ATamperedGroupArtifactIsInvalid keeps completed work from standing
// on evidence that changed under it. The group's record still says completed —
// the ledger records what happened — but resume reports the obligation invalid,
// names the artifact that no longer matches, and offers it again.
func TestResume_ATamperedGroupArtifactIsInvalid(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "resume-tampered-group"

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "started", "rows": []string{"SC-01", "SC-02"},
	})
	completed := runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "completed",
		"verdicts": []map[string]string{
			{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-02", "verdict": "HOLDS"},
		},
	})

	out := runResume(t, repo, runID)
	if group := resumeGroupFor(t, out, "A"); group.Status != "completed" || group.Reason != "" {
		t.Fatalf("group A = %+v, want a valid completed group before the artifact is touched", group)
	}

	data, err := os.ReadFile(completed.EvidencePath)
	if err != nil {
		t.Fatalf("read %s: %v", completed.EvidencePath, err)
	}
	if err := os.WriteFile(completed.EvidencePath, append(data, ' '), 0o644); err != nil {
		t.Fatalf("write %s: %v", completed.EvidencePath, err)
	}

	out = runResume(t, repo, runID)
	group := resumeGroupFor(t, out, "A")
	if group.Status != "invalid" {
		t.Errorf("group A status = %q, want invalid", group.Status)
	}
	if group.Reason != "artifact-digest-mismatch:evidence" {
		t.Errorf("group A reason = %q, want artifact-digest-mismatch:evidence", group.Reason)
	}
	if out.Counts.Invalid != 1 {
		t.Errorf("counts.invalid = %d, want 1", out.Counts.Invalid)
	}
	if rec := ledgerRecord(t, out, runstate.KindReviewGroup, "A"); rec.Status != runstate.StatusCompleted {
		t.Errorf("record status = %q, want the ledger to keep %q: what happened does not change", rec.Status, runstate.StatusCompleted)
	}
	if !slices.Contains(out.Pending, (resumePending{
		Kind: runstate.KindReviewGroup, ID: "A", Reason: "artifact-digest-mismatch:evidence",
	})) {
		t.Errorf("pending = %+v, want group A offered again", out.Pending)
	}
	if out.Review.Complete {
		t.Error("review.complete = true, want false while a group rests on changed evidence")
	}
}

// resumeGroupFor returns the review group resume reported for id.
func resumeGroupFor(t *testing.T, out resumeOutput, id string) resumeGroup {
	t.Helper()
	for _, group := range out.Review.Groups {
		if group.ID == id {
			return group
		}
	}
	t.Fatalf("no group %s in %+v", id, out.Review.Groups)
	return resumeGroup{}
}

// TestResume_AStartedRowNamesTheWorkspaceItsStateNowCarries locks the one
// place the ledger record is not the live answer: recover-mutation replaces a
// vanished worktree and records the replacement in the row's state, so a
// parent re-dispatching the mutator must be handed the path that exists, not
// the one the row was opened against.
func TestResume_AStartedRowNamesTheWorkspaceItsStateNowCarries(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "resume-recreated"
	// The mutation layer is only reported once the trace that names its rows
	// has completed.
	if code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID)); code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	prep := interruptedRow(t, repo, runID)
	if err := os.RemoveAll(prep.WorktreePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	recovered := runRecoverMutation(t, repo, runID, prep.RunDir)
	if !recovered.Recreated || recovered.WorktreePath == prep.WorktreePath {
		t.Fatalf("recover-mutation = %+v, want a recreated worktree at a fresh path", recovered)
	}

	row := resumeRow(t, runResume(t, repo, runID), "SC-09")
	if row.WorktreePath != recovered.WorktreePath {
		t.Errorf("worktreePath = %q, want the recreated workspace %q", row.WorktreePath, recovered.WorktreePath)
	}
	if row.RunDir != prep.RunDir {
		t.Errorf("runDir = %q, want %q", row.RunDir, prep.RunDir)
	}
	if !slices.Equal(row.CompletedSites, []string{"site-a"}) {
		t.Errorf("completedSites = %v, want [site-a]", row.CompletedSites)
	}
	if row.InterruptedSite != "" {
		t.Errorf("interruptedSite = %q, want none after the recovery cleared the marker", row.InterruptedSite)
	}
	if row.Reason != "" {
		t.Errorf("reason = %q, want none", row.Reason)
	}

	// With the state unreadable there is nothing live to prefer, so the row
	// falls back to the workspace its record was opened against.
	if err := os.WriteFile(filepath.Join(prep.RunDir, "mutation-state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	row = resumeRow(t, runResume(t, repo, runID), "SC-09")
	if row.Reason != reasonStateUnreadable {
		t.Errorf("reason = %q, want %q", row.Reason, reasonStateUnreadable)
	}
	if row.WorktreePath != prep.WorktreePath {
		t.Errorf("worktreePath = %q, want the record's %q when the state cannot be read", row.WorktreePath, prep.WorktreePath)
	}
}
