package main

import (
	"encoding/json"
	"io"
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

// TestResume_AStartedRowIsOwedWhateverItsRisk holds the difference between
// what a run is required to do and what it has actually taken on. The rows the
// dispatch inputs require are the high-risk ones, and only those are offered
// when nothing has touched them. But a row the run opened is an obligation
// however it came to be opened: until it is answered or recovered, a parent
// reading pending top to bottom has to see it, or an interrupted low-risk row
// leaves a workspace standing that the report says nothing about.
func TestResume_AStartedRowIsOwedWhateverItsRisk(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "resume-started-low-risk"

	if code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID)); code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	// SC-09 is low risk, so the dispatch inputs never require a mutation of
	// it; prepare-mutation opening one is the run's own doing.
	preparedMutation(t, repo, runID, "SC-09")

	out := runResume(t, repo, runID)

	if got, want := resumeRow(t, out, "SC-09").Status, string(runstate.StatusStarted); got != want {
		t.Errorf("SC-09 status = %q, want %q", got, want)
	}
	owed := resumePending{Kind: runstate.KindMutationRow, ID: "SC-09", Reason: reasonNotCompleted}
	if !slices.Contains(out.Pending, owed) {
		t.Errorf("pending = %+v, want %+v among them: the run opened this row and has not closed it", out.Pending, owed)
	}
	// The rows the dispatch requires are still offered, and still only when
	// nothing has touched them.
	for _, required := range []string{"SC-10", "SC-07"} {
		never := resumePending{Kind: runstate.KindMutationRow, ID: required, Reason: reasonNeverStarted}
		if !slices.Contains(out.Pending, never) {
			t.Errorf("pending = %+v, want %+v among them", out.Pending, never)
		}
	}
}

// survivedGuardMutant is the equivalent mutant of
// TestEvaluateMutation_SurvivedSite: it rewrites ValidateQuantity's message on
// a branch the bound tests never take, so every targeted test still passes and
// the site survives.
const survivedGuardMutant = "\tif qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil"

// startReviewGroup opens groupID over rows through the real record-review op.
func startReviewGroup(t *testing.T, repo, runID, groupID string, rows ...string) {
	t.Helper()
	runRecordReview(t, repo, runID, map[string]any{
		"groupId": groupID, "status": "started", "rows": rows,
	})
}

// soundVerdicts answers rows the way a critic that found every bound assertion
// sound would: HOLDS, and MISSING for a row the trace reported as a layer-1
// gap, the only verdict a gap can carry.
func soundVerdicts(t *testing.T, dispatch dispatchInputs, rows ...string) []map[string]string {
	t.Helper()
	verdicts := make([]map[string]string, 0, len(rows))
	for _, id := range rows {
		verdict := "HOLDS"
		if dispatchRowFor(t, dispatch, id).Status == "gap" {
			verdict = "MISSING"
		}
		verdicts = append(verdicts, map[string]string{"rowId": id, "verdict": verdict})
	}
	return verdicts
}

// completeReviewGroup closes groupID with one sound verdict per row.
func completeReviewGroup(t *testing.T, repo, runID, groupID string, dispatch dispatchInputs, rows ...string) recordReviewOutput {
	t.Helper()
	return runRecordReview(t, repo, runID, map[string]any{
		"groupId": groupID, "status": "completed", "verdicts": soundVerdicts(t, dispatch, rows...),
	})
}

// supersededRecord returns the superseded record the ledger kept for (kind, id).
func supersededRecord(t *testing.T, out resumeOutput, kind runstate.Kind, id string) runstate.Record {
	t.Helper()
	for _, rec := range out.Records {
		if rec.Kind == kind && rec.ID == id && rec.Status == runstate.StatusSuperseded {
			return rec
		}
	}
	t.Fatalf("no superseded %s/%s record in %+v", kind, id, out.Records)
	return runstate.Record{}
}

// TestResume_SelectsOnlyValidatedCompletions holds the line between work a run
// may stand on and work it must do again. Only a completed record whose
// artifacts still digest to what it recorded is reusable: an open group is not,
// a group that never claimed a row does not answer for it, and a completed
// group whose evidence changed under it stops standing the moment it does. The
// protocol around that line is proved with it — a completed group is completed
// once, verdicts belong to the group that owns their rows, and a group that was
// never opened cannot be closed, a completion that leaves a row unanswered
// does not close a group, and a group judged against dispatch inputs the run
// no longer hands out stops standing. The mutation layer holds the same line:
// a required row whose evidence changed under it is not a completion the
// layer may stand on.
// covers: SC-20
func TestResume_SelectsOnlyValidatedCompletions(t *testing.T) {
	// SC-01's test is gated behind fixtureGateVar so a repeated trace can hand
	// out different dispatch inputs at the same commit; with the gate unset the
	// fixture behaves as the plain one.
	repo := fixtureRepoWith(t, gatedFixtureEdit(t))
	const runID = "resume-validated-completions"

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)
	dispatch := readDispatchInputs(t, tr.DispatchPath)

	startReviewGroup(t, repo, runID, "A", "SC-01", "SC-02")
	groupA := completeReviewGroup(t, repo, runID, "A", dispatch, "SC-01", "SC-02")
	startReviewGroup(t, repo, runID, "B", "SC-03", "SC-04")
	startReviewGroup(t, repo, runID, "C", "SC-05")
	// C closes while B is still open. Completing groups out of order is
	// ordinary: neither group's standing depends on the other's.
	completeReviewGroup(t, repo, runID, "C", dispatch, "SC-05")

	prep := preparedMutation(t, repo, runID, "SC-10")
	mutateOrdersGuard(t, prep.WorktreePath, survivedGuardMutant)
	if code, _, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", false)); code != exitOK {
		t.Fatalf("evaluate-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	out := runResume(t, repo, runID)

	if out.Trace.Status != string(runstate.StatusCompleted) {
		t.Errorf("trace.status = %q, want completed", out.Trace.Status)
	}
	if out.Trace.EvidencePath != tr.EvidencePath {
		t.Errorf("trace.evidencePath = %q, want the trace op's own %q", out.Trace.EvidencePath, tr.EvidencePath)
	}
	if out.Trace.DispatchPath != tr.DispatchPath {
		t.Errorf("trace.dispatchPath = %q, want the trace op's own %q", out.Trace.DispatchPath, tr.DispatchPath)
	}

	if out.Review.Complete {
		t.Error("review.complete = true, want false while B is open and five rows belong to no group")
	}
	for _, id := range []string{"A", "C"} {
		if group := resumeGroupFor(t, out, id); group.Status != string(runstate.StatusCompleted) || group.Reason != "" {
			t.Errorf("group %s = %+v, want a valid completed group", id, group)
		}
	}
	if group := resumeGroupFor(t, out, "B"); group.Status != string(runstate.StatusStarted) {
		t.Errorf("group B = %+v, want started: an open group never satisfies its obligation", group)
	}
	// The rows in no group, compared as a set: resume lists them in dispatch
	// order, which is not the order the coverage row names them in.
	wantUncovered := []string{"SC-06", "SC-07", "SC-08", "SC-09", "SC-10"}
	if got := slices.Sorted(slices.Values(out.Review.Uncovered)); !slices.Equal(got, wantUncovered) {
		t.Errorf("review.uncovered = %v, want the active rows no group claims %v", out.Review.Uncovered, wantUncovered)
	}

	row := resumeRow(t, out, "SC-10")
	if row.Status != string(runstate.StatusStarted) {
		t.Errorf("SC-10 status = %q, want started: sites without evidence are not an answer", row.Status)
	}
	if !slices.Equal(row.CompletedSites, []string{"site-a"}) {
		t.Errorf("SC-10 completedSites = %v, want [site-a]", row.CompletedSites)
	}
	if untouched := resumeRow(t, out, "SC-07"); untouched.Status != statusNone {
		t.Errorf("SC-07 = %+v, want status none", untouched)
	}
	if out.Live.Required || out.Live.Status != statusNone {
		t.Errorf("live = %+v, want a layer the fixture document never asks for", out.Live)
	}

	wantPending := []resumePending{
		{Kind: runstate.KindReviewGroup, ID: "B", Reason: reasonNotCompleted},
		{Kind: runstate.KindReviewGroup, Reason: "uncovered: " + strings.Join(out.Review.Uncovered, ", ")},
		{Kind: runstate.KindMutationRow, ID: "SC-10", Reason: reasonNotCompleted},
		{Kind: runstate.KindMutationRow, ID: "SC-07", Reason: reasonNeverStarted},
	}
	if !reflect.DeepEqual(out.Pending, wantPending) {
		t.Errorf("pending = %+v, want %+v", out.Pending, wantPending)
	}
	if want := (resumeCounts{Started: 2, Completed: 3, Pending: 4}); out.Counts != want {
		t.Errorf("counts = %+v, want %+v", out.Counts, want)
	}

	verdictsA := soundVerdicts(t, dispatch, "SC-01", "SC-02")
	// A second completion of A carries a different answer, so anything it
	// wrote would be visible: the refusal has to land before A's evidence is
	// touched, and the first result stands in what resume reports.
	if got := refuseRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "completed",
		"verdicts": []map[string]string{
			{"rowId": "SC-01", "verdict": "WEAK", "reason": "a second critic disagreed"},
			{"rowId": "SC-02", "verdict": "HOLDS"},
		},
	}); got.Code != "duplicate-completion" {
		t.Errorf("completing A twice = %q, want duplicate-completion (message %q)", got.Code, got.Message)
	}
	out = runResume(t, repo, runID)
	if group := resumeGroupFor(t, out, "A"); group.Status != string(runstate.StatusCompleted) || group.Reason != "" || group.Outcome != "holds" {
		t.Errorf("group A after the refused duplicate = %+v, want the first completion standing: completed holds with nothing against it", group)
	}
	if got := reviewVerdictsAt(t, groupA.EvidencePath); !reflect.DeepEqual(got, []reviewVerdict{{RowID: "SC-01", Verdict: "HOLDS"}, {RowID: "SC-02", Verdict: "HOLDS"}}) {
		t.Errorf("A's evidence verdicts = %+v, want the first result's HOLDS verdicts untouched by the refused duplicate", got)
	}
	if got := refuseRecordReview(t, repo, runID, map[string]any{
		"groupId": "B", "status": "completed", "verdicts": verdictsA,
	}); got.Code != "verdict-outside-group" {
		t.Errorf("completing B with A's verdicts = %q, want verdict-outside-group (message %q)", got.Code, got.Message)
	}
	if got := refuseRecordReview(t, repo, runID, map[string]any{
		"groupId": "D", "status": "completed", "verdicts": soundVerdicts(t, dispatch, "SC-06"),
	}); got.Code != "review-group-not-started" {
		t.Errorf("completing an unopened D = %q, want review-group-not-started (message %q)", got.Code, got.Message)
	}
	// A completion that answers only one of B's rows is not B's answer: it is
	// refused by name and B stays the open group it was.
	if got := refuseRecordReview(t, repo, runID, map[string]any{
		"groupId": "B", "status": "completed", "verdicts": soundVerdicts(t, dispatch, "SC-03"),
	}); got.Code != "missing-verdict" {
		t.Errorf("completing B without SC-04's verdict = %q, want missing-verdict (message %q)", got.Code, got.Message)
	}
	if group := resumeGroupFor(t, runResume(t, repo, runID), "B"); group.Status != string(runstate.StatusStarted) || group.Reason != "" || group.Outcome != "" {
		t.Errorf("group B after the refused partial completion = %+v, want it still open with no outcome", group)
	}

	completeReviewGroup(t, repo, runID, "B", dispatch, "SC-03", "SC-04")
	remaining := []string{"SC-06", "SC-07", "SC-08", "SC-09", "SC-10"}
	startReviewGroup(t, repo, runID, "E", remaining...)
	completeReviewGroup(t, repo, runID, "E", dispatch, remaining...)

	out = runResume(t, repo, runID)
	if !out.Review.Complete {
		t.Errorf("review.complete = false, want true once every active row sits in a completed group (uncovered %v)", out.Review.Uncovered)
	}
	if len(out.Review.Uncovered) != 0 {
		t.Errorf("review.uncovered = %v, want none", out.Review.Uncovered)
	}

	if base := filepath.Base(groupA.EvidencePath); base != "review-A-2.json" {
		t.Fatalf("group A evidence = %q, want review-A-2.json (the trace is seq 1, A seq 2)", base)
	}
	data, err := os.ReadFile(groupA.EvidencePath)
	if err != nil {
		t.Fatalf("read %s: %v", groupA.EvidencePath, err)
	}
	data[len(data)-1] = ' '
	if err := os.WriteFile(groupA.EvidencePath, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", groupA.EvidencePath, err)
	}

	out = runResume(t, repo, runID)
	group := resumeGroupFor(t, out, "A")
	if group.Status != statusInvalid {
		t.Errorf("group A status = %q, want %q", group.Status, statusInvalid)
	}
	if group.Reason != "artifact-digest-mismatch:evidence" {
		t.Errorf("group A reason = %q, want artifact-digest-mismatch:evidence", group.Reason)
	}
	offered := resumePending{Kind: runstate.KindReviewGroup, ID: "A", Reason: "artifact-digest-mismatch:evidence"}
	if !slices.Contains(out.Pending, offered) {
		t.Errorf("pending = %+v, want %+v offered again", out.Pending, offered)
	}
	if out.Counts.Invalid != 1 {
		t.Errorf("counts.invalid = %d, want 1", out.Counts.Invalid)
	}
	if out.Review.Complete {
		t.Error("review.complete = true, want false: a group resting on changed evidence is not a validated completion")
	}

	// The mutation layer's counterpart. SC-10 is finished on a second site and
	// SC-07 is answered by a refused attempt: both rows the dispatch requires
	// are completed, so the layer is complete — until one of those completions
	// stops standing, when a completed record is no longer enough.
	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, stdout, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-b", true))
	if code != exitOK {
		t.Fatalf("evaluate-mutation site-b exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var evaluated evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &evaluated); err != nil {
		t.Fatalf("evaluate-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if code, _, stderr := runVerifyOp(t, "record-attempt", requestWith(t, repo, runID, map[string]any{
		"layer": 4, "rowID": "SC-07", "refusal": "mutation runner unavailable on this machine",
	})); code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out = runResume(t, repo, runID)
	if !out.Mutation.Complete {
		t.Errorf("mutation.complete = false, want true once both required rows are validated completions (rows %+v)", out.Mutation.Rows)
	}
	for id, outcome := range map[string]string{"SC-10": "survived", "SC-07": "environmental"} {
		if row := resumeRow(t, out, id); row.Status != string(runstate.StatusCompleted) || row.Reason != "" || row.Outcome != outcome {
			t.Errorf("%s = %+v, want a valid completed row with outcome %s", id, row, outcome)
		}
	}

	if filepath.Dir(evaluated.EvidencePath) != prep.RunDir {
		t.Fatalf("SC-10 evidence = %q, want it inside the row's run dir %q", evaluated.EvidencePath, prep.RunDir)
	}
	data, err = os.ReadFile(evaluated.EvidencePath)
	if err != nil {
		t.Fatalf("read %s: %v", evaluated.EvidencePath, err)
	}
	if err := os.WriteFile(evaluated.EvidencePath, append(data, ' '), 0o644); err != nil {
		t.Fatalf("write %s: %v", evaluated.EvidencePath, err)
	}
	out = runResume(t, repo, runID)
	if row := resumeRow(t, out, "SC-10"); row.Status != statusInvalid || row.Reason != "artifact-digest-mismatch:evidence" {
		t.Errorf("SC-10 = %+v, want status %s reason artifact-digest-mismatch:evidence", row, statusInvalid)
	}
	if rec := ledgerRecord(t, out, runstate.KindMutationRow, "SC-10"); rec.Status != runstate.StatusCompleted {
		t.Errorf("SC-10 record status = %q, want the ledger to keep %q", rec.Status, runstate.StatusCompleted)
	}
	if out.Mutation.Complete {
		t.Error("mutation.complete = true, want false: a required row resting on changed evidence is a completed record, not a validated completion")
	}
	if out.Counts.Invalid != 2 {
		t.Errorf("counts.invalid = %d, want 2: group A and row SC-10", out.Counts.Invalid)
	}

	// A repeated trace that hands out different dispatch inputs — SC-01's test
	// gated off at the same commit — leaves every completed group judged
	// against sources the run no longer hands out. Each is reported invalid
	// for that reason and offered again, and the review layer is not settled
	// by groups that merely exist as completed records.
	t.Setenv(fixtureGateVar, "1")
	gated := repeatTrace(t, repo, runID, "SC-01's test gated off")
	if got := dispatchRowFor(t, readDispatchInputs(t, gated.DispatchPath), "SC-01").Status; got != "gap" {
		t.Fatalf("SC-01 status = %q under the gate, want gap: the repeated trace did not change the dispatch inputs", got)
	}
	out = runResume(t, repo, runID)
	if out.Trace.Status != string(runstate.StatusCompleted) || out.Trace.Reason != "" {
		t.Errorf("trace = %+v, want the repeated phase completed and valid", out.Trace)
	}
	for _, id := range []string{"B", "C", "E"} {
		if group := resumeGroupFor(t, out, id); group.Status != statusInvalid || group.Reason != reasonInputsChanged {
			t.Errorf("group %s = %+v, want status %s reason %s after the dispatch inputs changed", id, group, statusInvalid, reasonInputsChanged)
		}
	}
	// C held, so it is owed again; B and E are blocked by their gap rows'
	// MISSING verdicts, and a completed failure is an answer that is never
	// offered again, whatever its standing.
	owed := resumePending{Kind: runstate.KindReviewGroup, ID: "C", Reason: reasonInputsChanged}
	if !slices.Contains(out.Pending, owed) {
		t.Errorf("pending = %+v, want %+v offered again", out.Pending, owed)
	}
	if group := resumeGroupFor(t, out, "A"); group.Status != statusInvalid {
		t.Errorf("group A = %+v, want it still invalid", group)
	}
	if out.Review.Complete {
		t.Error("review.complete = true, want false: groups judged against inputs the run no longer hands out are not validated completions")
	}
	if out.Counts.Invalid != 5 {
		t.Errorf("counts.invalid = %d, want 5: the four groups and row SC-10", out.Counts.Invalid)
	}
}

// reviewVerdictsAt reads the verdicts a group's evidence file carries.
func reviewVerdictsAt(t *testing.T, path string) []reviewVerdict {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc reviewEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("review evidence %s did not decode: %v", path, err)
	}
	return doc.Verdicts
}

// TestResume_PreservesFailedOutcomes holds a run to the answers it already
// has. A blocked group, a surviving mutant and a refused mutation runner are
// results, not gaps: resume counts them, never offers them again, and the ops
// that would overwrite them refuse by name. Repeating one is possible but
// deliberate — it costs a reason, which the replaced record keeps alongside the
// outcome it measured. And none of it reaches a different run ID.
// covers: SC-22
func TestResume_PreservesFailedOutcomes(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "resume-failed-outcomes"

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	startReviewGroup(t, repo, runID, "A", "SC-01", "SC-02")
	blocked := runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "completed",
		"verdicts": []map[string]string{
			{"rowId": "SC-01", "verdict": "HOLDS"},
			{"rowId": "SC-02", "verdict": "WEAK", "reason": "the assertion does not name the value it rejects"},
		},
	})
	if blocked.Outcome != "blocked" {
		t.Fatalf("group A outcome = %q, want blocked when a verdict is below HOLDS", blocked.Outcome)
	}

	prep := preparedMutation(t, repo, runID, "SC-09")
	mutateOrdersGuard(t, prep.WorktreePath, survivedGuardMutant)
	code, stdout, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", true))
	if code != exitOK {
		t.Fatalf("evaluate-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var evaluated evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &evaluated); err != nil {
		t.Fatalf("evaluate-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if evaluated.Outcome != "survived" {
		t.Fatalf("SC-09 outcome = %q, want survived", evaluated.Outcome)
	}

	attempt := requestWith(t, repo, runID, map[string]any{
		"layer": 4, "rowID": "SC-10", "refusal": "mutation runner unavailable on this machine",
	})
	if code, _, stderr := runVerifyOp(t, "record-attempt", attempt); code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	// The restart: everything below is read from the ledger alone.
	out := runResume(t, repo, runID)
	answered := []struct {
		kind    runstate.Kind
		id      string
		outcome string
	}{
		{runstate.KindReviewGroup, "A", "blocked"},
		{runstate.KindMutationRow, "SC-09", "survived"},
		{runstate.KindMutationRow, "SC-10", "environmental"},
	}
	for _, want := range answered {
		rec := ledgerRecord(t, out, want.kind, want.id)
		if rec.Status != runstate.StatusCompleted || rec.Outcome != want.outcome {
			t.Errorf("%s/%s = %s/%s, want completed %s", want.kind, want.id, rec.Status, rec.Outcome, want.outcome)
		}
	}
	wantPending := []resumePending{
		{Kind: runstate.KindReviewGroup, Reason: "uncovered: " + strings.Join(out.Review.Uncovered, ", ")},
		{Kind: runstate.KindMutationRow, ID: "SC-07", Reason: reasonNeverStarted},
	}
	if !reflect.DeepEqual(out.Pending, wantPending) {
		t.Errorf("pending = %+v, want %+v: a completed failure is an answer, not a gap", out.Pending, wantPending)
	}
	if out.Counts.Failed != 3 {
		t.Errorf("counts.failed = %d, want 3", out.Counts.Failed)
	}
	if out.Counts.Completed != 4 {
		t.Errorf("counts.completed = %d, want 4: the trace and the three answers", out.Counts.Completed)
	}

	restartA := map[string]any{"groupId": "A", "status": "started", "rows": []string{"SC-01", "SC-02"}}
	if got := refuseRecordReview(t, repo, runID, restartA); got.Code != "review-group-completed" {
		t.Errorf("reopening A = %q, want review-group-completed (message %q)", got.Code, got.Message)
	}
	code, stdout, stderr = runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, runID, "SC-09"))
	if code != exitError || stdout != "" {
		t.Fatalf("an answered row was measured again: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "mutation-row-completed" {
		t.Errorf("re-preparing SC-09 = %q, want mutation-row-completed (message %q)", got.Code, got.Message)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees = %v, want the refusal to land before any workspace was made", left)
	}

	const reason = "environmental recovery: runner cache repaired"
	repeat := map[string]any{"reason": reason}
	runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "started", "rows": []string{"SC-01", "SC-02"}, "repeat": repeat,
	})
	if code, _, stderr := runVerifyOp(t, "prepare-mutation", requestWith(t, repo, runID, map[string]any{
		"rowID": "SC-09", "repeat": repeat,
	})); code != exitOK {
		t.Fatalf("repeat prepare-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	out = runResume(t, repo, runID)
	for _, want := range answered[:2] {
		replaced := supersededRecord(t, out, want.kind, want.id)
		if replaced.Outcome != want.outcome {
			t.Errorf("superseded %s/%s outcome = %q, want the outcome it kept %q", want.kind, want.id, replaced.Outcome, want.outcome)
		}
		if replaced.Detail != reason {
			t.Errorf("superseded %s/%s detail = %q, want the retained repeat reason %q", want.kind, want.id, replaced.Detail, reason)
		}
		if rec := ledgerRecord(t, out, want.kind, want.id); rec.Status != runstate.StatusStarted {
			t.Errorf("current %s/%s status = %q, want %q", want.kind, want.id, rec.Status, runstate.StatusStarted)
		}
	}
	if out.Counts.Superseded != 2 {
		t.Errorf("counts.superseded = %d, want 2", out.Counts.Superseded)
	}

	// A ledger is keyed by run ID, so a changed input is a new run that
	// inherits none of this one's answers.
	fresh := runResume(t, repo, runID+"-2")
	if fresh.Initialized {
		t.Errorf("initialized = true for %s-2, want a run with no ledger of its own", runID)
	}
	wantFresh := []resumePending{{Kind: runstate.KindPhase, ID: tracePhaseID, Reason: reasonNeverStarted}}
	if !reflect.DeepEqual(fresh.Pending, wantFresh) {
		t.Errorf("pending = %+v, want only the trace phase %+v", fresh.Pending, wantFresh)
	}
	if fresh.Counts.Completed != 0 {
		t.Errorf("counts.completed = %d, want 0", fresh.Counts.Completed)
	}

	// The live layer's answer, over a subject whose live decision is required:
	// a refused live attempt completes the layer unavailable, and the restart
	// keeps it a failure — counted as one, never reported as a verified layer.
	t.Run("a refused live attempt stays unavailable", func(t *testing.T) {
		subject := newRecordAttemptLiveSubject(t)
		liveRun := liveRunID("resume-failed-live")
		const refusal = "no live environment is configured for this run"
		if code, _, stderr := runVerifyOp(t, "record-attempt", opRequestJSON(t, subject.repo, subject.coverageSource, liveRun, map[string]any{
			"layer": 3, "refusal": refusal,
		})); code != exitOK {
			t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}

		code, stdout, stderr := runVerifyOp(t, "resume", liveRequestJSON(t, subject.repo, subject.coverageSource, liveRun))
		if code != exitOK {
			t.Fatalf("resume exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		var live resumeOutput
		if err := json.Unmarshal([]byte(stdout), &live); err != nil {
			t.Fatalf("resume stdout did not decode: %v\nstdout=%s", err, stdout)
		}
		if !live.Live.Required {
			t.Fatalf("live.required = false, want true for a document whose live decision is required")
		}
		if live.Live.Status != string(runstate.StatusCompleted) || live.Live.Outcome != "unavailable" || live.Live.Reason != "" {
			t.Errorf("live = %+v, want completed unavailable with nothing against it", live.Live)
		}
		rec := ledgerRecord(t, live, runstate.KindLiveAttempt, liveAttemptID)
		if rec.Status != runstate.StatusCompleted || rec.Outcome != "unavailable" {
			t.Errorf("live-attempt record = %s/%s, want completed unavailable", rec.Status, rec.Outcome)
		}
		if !runstate.IsFailedOutcome(rec.Kind, rec.Outcome) {
			t.Errorf("live-attempt outcome %q is not a failed outcome, want the refused attempt kept as a failure", rec.Outcome)
		}
		if live.Counts.Failed != 1 || live.Counts.Completed != 1 {
			t.Errorf("counts = %+v, want the one completed record counted as the one failure", live.Counts)
		}
		for _, pending := range live.Pending {
			if pending.Kind == runstate.KindLiveAttempt {
				t.Errorf("pending = %+v, want the answered live layer not offered again", live.Pending)
			}
		}
	})
}

// TestResume_AnUnfinishedPublicationIsStillOwed reads the run's publication
// back out of the ledger: what it measured, what it meant to write, and that a
// record left open by a failed publication is an obligation the run still
// carries.
func TestResume_AnUnfinishedPublicationIsStillOwed(t *testing.T) {
	f := newFinalizeFixture(t)
	planted := filepath.Join(f.repo, "docs", "superpowers", "coverage", "transcripts", f.runID, traceEvidenceFile)

	cliErr := finalizeOp(f.request(), io.Discard, func() {
		if err := os.MkdirAll(planted, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", planted, err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(planted) })
	})
	if cliErr == nil || cliErr.Code != "publish-failed" {
		t.Fatalf("finalizeOp = %+v, want the publish-failed refusal", cliErr)
	}

	out := runResume(t, f.repo, f.runID)

	pub := out.Publication
	if pub.Status != string(runstate.StatusStarted) {
		t.Errorf("publication.status = %q, want started", pub.Status)
	}
	if pub.Outcome != "" {
		t.Errorf("publication.outcome = %q, want none while the record is open", pub.Outcome)
	}
	if pub.Reason != reasonNotCompleted {
		t.Errorf("publication.reason = %q, want %q", pub.Reason, reasonNotCompleted)
	}
	if pub.MeasuredCommit != f.head {
		t.Errorf("publication.measuredCommit = %q, want the commit the run was opened at %q", pub.MeasuredCommit, f.head)
	}
	if pub.Acknowledged {
		t.Error("publication.acknowledged = true, want false: the request carried no acknowledgement")
	}
	if len(pub.Files) != 6 {
		t.Fatalf("publication.files = %+v, want the report, the Markdown and four transcripts", pub.Files)
	}
	for i, ref := range pub.Files {
		if i > 0 && pub.Files[i-1].Path >= ref.Path {
			t.Errorf("publication.files are not sorted by path: %+v", pub.Files)
		}
		if ref.SHA256 == "" {
			t.Errorf("publication.files[%d] = %+v, want the digest of the bytes it writes", i, ref)
		}
	}

	owed := resumePending{Kind: runstate.KindPhase, ID: publishPhaseID, Reason: reasonNotCompleted}
	if !slices.Contains(out.Pending, owed) {
		t.Errorf("pending = %+v, want it to carry %+v", out.Pending, owed)
	}
	// The publication is the last section, after everything the report speaks
	// for: a parent reads the layers it owes before the report it owes.
	if got := out.Pending[len(out.Pending)-1]; got != owed {
		t.Errorf("pending ends with %+v, want the publication %+v", got, owed)
	}
	if out.HeadCommit != f.head {
		t.Errorf("headCommit = %q, want %q", out.HeadCommit, f.head)
	}
}

// TestResume_CountsAdoptedRecordsAsReused separates the run's own work from
// what it stands on: the count is read from each record's captured inputs, so a
// fresh trace beside three adopted records is one run with three reused answers.
func TestResume_CountsAdoptedRecordsAsReused(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID := readyAdoptedRun(t, s, "rsm-reused", readyCaughtVersionRow)

	out := readyResume(t, s, runID)

	if out.Counts.Reused != 3 {
		t.Errorf("counts.reused = %d, want the two adopted groups and the adopted row; counts=%+v", out.Counts.Reused, out.Counts)
	}
	if out.Counts.Completed != 4 {
		t.Errorf("counts.completed = %d, want the adopted three plus this run's own trace", out.Counts.Completed)
	}
	if out.Publication.Status != statusNone {
		t.Errorf("publication = %+v, want none before finalize runs", out.Publication)
	}

	// The run's own trace is not reused, whatever the records beside it say.
	if _, err := os.Stat(out.Trace.EvidencePath); err != nil {
		t.Errorf("stat trace evidence: %v", err)
	}
}

// TestResume_APublicationCommitDoesNotMakeTheRunStale is the measured
// revision seen from the accounting side: committing what finalize published
// moves HEAD without moving the subject, and resume keeps standing on the
// commit the run was opened at until something else changes.
func TestResume_APublicationCommitDoesNotMakeTheRunStale(t *testing.T) {
	f := newFinalizeFixture(t)
	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("finalize: %s", stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	rep := f.publishedReport(out.JSONPath)

	published := []string{out.JSONPath, out.MarkdownPath}
	for _, ref := range rep.Transcripts {
		published = append(published, ref.Path)
	}
	mustGit(t, f.repo, append([]string{"add"}, published...)...)
	mustGit(t, f.repo, "commit", "-q", "-m", "publish the verification report")
	afterPublish := strings.TrimSpace(mustGit(t, f.repo, "rev-parse", "HEAD"))

	state := runResume(t, f.repo, f.runID)

	if state.Stale {
		t.Errorf("stale = true (%s), want a publication-only commit to leave the run standing", state.StaleReason)
	}
	if state.HeadCommit != afterPublish {
		t.Errorf("headCommit = %q, want %q", state.HeadCommit, afterPublish)
	}
	if state.Identity.RepoCommit != f.head {
		t.Errorf("identity.repoCommit = %q, want the commit the run was opened at %q", state.Identity.RepoCommit, f.head)
	}
	if state.Publication.MeasuredCommit != f.head {
		t.Errorf("publication.measuredCommit = %q, want %q", state.Publication.MeasuredCommit, f.head)
	}

	// Anything else in the same repository is another subject.
	source := filepath.Join(f.repo, "orders", "orders.go")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read orders.go: %v", err)
	}
	if err := os.WriteFile(source, append(data, []byte("\n// a later thought\n")...), 0o644); err != nil {
		t.Fatalf("write orders.go: %v", err)
	}
	mustGit(t, f.repo, "add", "orders/orders.go")
	mustGit(t, f.repo, "commit", "-q", "-m", "production change")

	moved := runResume(t, f.repo, f.runID)
	if !moved.Stale {
		t.Errorf("stale = false after a production commit, want the run stale")
	}
}
