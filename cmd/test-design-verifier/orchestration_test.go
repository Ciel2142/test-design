package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/runstate"
)

// This file holds the one proof that drives the whole pipeline twice over one
// subject: a mixed-outcome run that answers every obligation itself, and an
// isolated repair that adopts everything still standing. What it holds between
// them is that the published work summary is the run's own ledger and nothing
// else — every obligation the test drove appears, each named fresh or reused as
// the run actually answered it, each proved by a published transcript — and
// that adopting really is cheaper: the second run dispatches fewer critics over
// fewer bytes than the first.

// orchInvocation is one obligation the test drives, appended to the run's log
// before the op that answers it, so the log is what the run was asked for
// rather than what it reported.
type orchInvocation = struct{ kind, id string }

// orchLog collects invocations for one run.
type orchLog struct{ entries []orchInvocation }

func (l *orchLog) drive(kind runstate.Kind, id string) {
	l.entries = append(l.entries, orchInvocation{string(kind), id})
}

// orchReviewGroup opens groupID over rowID under policy, closes it with
// verdict, and returns what the dispatch cost: one started call and the bytes
// of every file its critic was handed. The sizes are read from the repository,
// because the bundle names files by their repository path.
func orchReviewGroup(
	t *testing.T, s *liveSubject, runID, groupID, rowID, verdict string, policy *reviewPolicy,
) (dispatches int, bundleBytes int64, bundleFiles int) {
	t.Helper()
	// The ready* drivers are generic over *liveSubject despite the name; this
	// subject is driven through the same ones rather than a second copy.
	started := readyRecordReview(t, s, runID, map[string]any{
		"groupId": groupID, "status": "started", "rows": []string{rowID}, "policy": policy,
	})
	for _, file := range started.Bundle {
		info, err := os.Stat(filepath.Join(s.repo, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatalf("stat bundle file %s of group %s: %v", file.Path, groupID, err)
		}
		bundleBytes += info.Size()
	}
	// The critic's reads are recorded with the completion, which is where the
	// manifest becomes complete: a group whose manifest stays incomplete is
	// never adoptable, and the repair run below would adopt nothing at all.
	completed := readyRecordReview(t, s, runID, map[string]any{
		"groupId": groupID, "status": "completed", "reads": []string{},
		"verdicts": []map[string]string{{
			"rowId": rowID, "verdict": verdict, "reason": "critic verdict for " + rowID,
		}},
	})
	if !completed.ManifestComplete {
		t.Fatalf("group %s manifest is incomplete (%v); an incomplete manifest is never adoptable",
			groupID, completed.ManifestIncomplete)
	}
	return 1, bundleBytes, len(started.Bundle)
}

// orchLivePlan is the plan the live-runner hands execute-live: one walkthrough
// step running the binary the profile's start step built, bound to SC-01 and
// settled by the greeting the deployed binary prints.
func orchLivePlan(runID string) map[string]any {
	return map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{{
			"expectation": "the deployed binary prints the approved greeting",
			"command":     []string{"./orch-" + runID},
			"predicates": []map[string]string{
				{"form": "exit-code", "value": "0"},
				{"form": "matches", "stream": "stdout", "value": orchGreeting},
			},
			"scBindings": []string{orchGreetRow},
		}},
	}
}

// orchLiveJourney drives prepare-live and execute-live for a run whose trace
// already stands, the way the workflow does once the review groups are out. It
// is the tail of runLivePipeline: that helper traces first, which a run that
// already traced would be refused for.
func orchLiveJourney(t *testing.T, s *liveSubject, runID, tracePath string) executeLiveOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "prepare-live", liveRequestJSON(t, s.repo, s.coverageSource, runID))
	if code != exitOK {
		t.Fatalf("prepare-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var prep prepareLiveOutput
	if err := json.Unmarshal([]byte(stdout), &prep); err != nil {
		t.Fatalf("prepare-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	code, stdout, stderr = runVerifyOp(t, "execute-live", liveExecuteRequestJSON(
		t, s.repo, s.coverageSource, prep.RunDir, writePlanFile(t, orchLivePlan(runID)), tracePath))
	if code != exitOK {
		t.Fatalf("execute-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var exec executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &exec); err != nil {
		t.Fatalf("execute-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if got := rowVerdict(t, exec.RowVerdicts, orchGreetRow).Verdict; got != "LIVE-PASS" {
		t.Fatalf("live verdict for %s = %q, want LIVE-PASS against the deployed binary", orchGreetRow, got)
	}
	return exec
}

// orchCaughtLimitRow answers SC-04 the way a run that really mutated does:
// prepare-mutation opens the row against its bound test, the guard in the
// prepared worktree is rewritten to accept zero, and evaluate-mutation runs the
// row's targeted family over the mutant.
func orchCaughtLimitRow(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	prep := preparedMutationFor(t, s.repo, s.coverageSource, runID, orchLimitRow)
	writeSubjectFile(t, prep.WorktreePath, "delta/delta.go", orchMutatedDeltaSource)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSONFor(t, s.repo, s.coverageSource, runID, prep.RunDir, "site-a", true))
	if code != exitOK {
		t.Fatalf("evaluate-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("evaluate-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "caught" {
		t.Fatalf("mutation outcome for %s = %q, want caught", orchLimitRow, out.Outcome)
	}
}

// orchEnvironmentalTagRow answers SC-05 the way a run whose mutation runner
// never started does. The row is opened by prepare-mutation first so the record
// carries the bound test's identity: a row answered without ever being prepared
// records no identity at all, and reuse refuses such a row identity-unrecorded
// rather than judging it.
func orchEnvironmentalTagRow(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	preparedMutationFor(t, s.repo, s.coverageSource, runID, orchTagRow)
	code, _, stderr := runVerifyOp(t, "record-attempt", readyRequestJSON(t, s, runID, map[string]any{
		"layer": 4, "rowID": orchTagRow,
		"refusal": "the mutation runner could not start in this environment",
	}))
	if code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
}

// orchWorkItem returns the published work item for (kind, id).
func orchWorkItem(t *testing.T, rep *report.Report, kind runstate.Kind, id string) report.WorkItem {
	t.Helper()
	for _, item := range rep.Work.Items {
		if item.Kind == string(kind) && item.ID == id {
			return item
		}
	}
	t.Fatalf("work items = %+v, want an entry for %s %s", rep.Work.Items, kind, id)
	return report.WorkItem{}
}

// orchInvocationsOf is the published summary read back as the log the test
// keeps: one entry per item, in the order the report lists them.
func orchInvocationsOf(rep *report.Report) []orchInvocation {
	entries := make([]orchInvocation, 0, len(rep.Work.Items))
	for _, item := range rep.Work.Items {
		entries = append(entries, orchInvocation{item.Kind, item.ID})
	}
	return entries
}

// covers: SC-36
func TestOrchestration_SummaryMatchesRetainedWork(t *testing.T) {
	s := orchestrationSubject(t)
	// One policy identity for both runs, written once: reuse compares the
	// critic's instructions by path and digest, so a second file would read as
	// a changed policy rather than the same one.
	policy := reusePolicy(t)
	headA := s.head

	// Run A answers every obligation itself, and two of them badly: one
	// group's assertion is weak, and the environment refuses one high-risk
	// row's mutation. The report it publishes is what the run actually did.
	runA := liveRunID("orch-a")
	logA := &orchLog{}
	dispatchesA, bundleBytesA := 0, int64(0)

	logA.drive(runstate.KindPhase, tracePhaseID)
	traceA := readyTrace(t, s, runA)

	for _, group := range []struct{ id, row, verdict string }{
		{orchGreetGroup, orchGreetRow, "HOLDS"},
		{orchCountGroup, orchCountRow, "HOLDS"},
		{orchFlagGroup, orchFlagRow, "WEAK"},
		{orchLimitGroup, orchLimitRow, "HOLDS"},
		{orchTagGroup, orchTagRow, "HOLDS"},
	} {
		logA.drive(runstate.KindReviewGroup, group.id)
		dispatched, bytes, _ := orchReviewGroup(t, s, runA, group.id, group.row, group.verdict, policy)
		dispatchesA += dispatched
		bundleBytesA += bytes
	}

	logA.drive(runstate.KindLiveAttempt, liveAttemptID)
	orchLiveJourney(t, s, runA, traceA.EvidencePath)

	logA.drive(runstate.KindMutationRow, orchLimitRow)
	orchCaughtLimitRow(t, s, runA)

	logA.drive(runstate.KindMutationRow, orchTagRow)
	orchEnvironmentalTagRow(t, s, runA)

	outA := readyFinalize(t, s, runA)
	if outA.Verdict != report.VerdictBlocked {
		t.Fatalf("run A verdict = %q, want %s; reasons=%+v", outA.Verdict, report.VerdictBlocked, outA.Reasons)
	}
	requireReason(t, outA.Reasons, "2", orchFlagRow, "WEAK")
	requireReason(t, outA.Reasons, "4", orchTagRow, "mutation-environmental")

	repA := readPublishedReport(t, s.repo, outA.JSONPath)
	if got := orchInvocationsOf(repA); !reflect.DeepEqual(got, logA.entries) {
		t.Errorf("work items = %+v, want exactly the obligations the run drove, in order: %+v", got, logA.entries)
	}
	wantA := map[string]struct{ outcome, standing string }{
		tracePhaseID:   {"ok", report.StandingOK},
		orchGreetGroup: {"holds", report.StandingOK},
		orchCountGroup: {"holds", report.StandingOK},
		orchFlagGroup:  {"blocked", report.StandingFailed},
		orchLimitGroup: {"holds", report.StandingOK},
		orchTagGroup:   {"holds", report.StandingOK},
		liveAttemptID:  {"pass", report.StandingOK},
		orchLimitRow:   {"caught", report.StandingOK},
		orchTagRow:     {"environmental", report.StandingIncomplete},
	}
	for _, item := range repA.Work.Items {
		want, known := wantA[item.ID]
		if !known {
			t.Errorf("work item %s %s is not an obligation this run drove", item.Kind, item.ID)
			continue
		}
		if item.Outcome != want.outcome || item.Standing != want.standing {
			t.Errorf("work item %s %s = outcome %q standing %q, want %q and %q",
				item.Kind, item.ID, item.Outcome, item.Standing, want.outcome, want.standing)
		}
		if item.Source != report.WorkFresh {
			t.Errorf("work item %s %s source = %q, want %q: run A adopted nothing",
				item.Kind, item.ID, item.Source, report.WorkFresh)
		}
		// The item is only as good as the file that proves it: the path has to
		// be in the repository, hold the bytes the item names, and be one of
		// the transcripts the report published.
		sum, err := evidence.SHA256File(filepath.Join(s.repo, filepath.FromSlash(item.Artifact.Path)))
		if err != nil {
			t.Errorf("work item %s %s artifact %s: %v", item.Kind, item.ID, item.Artifact.Path, err)
			continue
		}
		if sum != item.Artifact.SHA256 {
			t.Errorf("work item %s %s artifact %s digests to %s, want the listed %s",
				item.Kind, item.ID, item.Artifact.Path, sum, item.Artifact.SHA256)
		}
		if ref := publishedTranscript(t, repA, item.Artifact.Path); ref.SHA256 != item.Artifact.SHA256 {
			t.Errorf("transcript %s digest %s does not match work item %s %s digest %s",
				ref.Path, ref.SHA256, item.Kind, item.ID, item.Artifact.SHA256)
		}
	}
	if want := (report.WorkCounts{Fresh: 9, Reused: 0, Failed: 1, Incomplete: 1}); repA.Work.Counts != want {
		t.Errorf("run A work counts = %+v, want %+v", repA.Work.Counts, want)
	}

	resumedA := resumeFor(t, s.repo, s.coverageSource, runA)
	if resumedA.Counts.Reused != 0 {
		t.Errorf("resume A counts.reused = %d, want 0: run A adopted nothing", resumedA.Counts.Reused)
	}
	if resumedA.Publication.Status != string(runstate.StatusCompleted) {
		t.Errorf("resume A publication = %+v, want a completed publication", resumedA.Publication)
	}

	// The repair: run A's report is committed, and then one test file is made
	// to assert more than it did. Nothing a critic read for the other four
	// groups moved, so the second run has to buy only what really changed.
	revisionCommit(t, s, "publish run A's verification report")
	writeSubjectFile(t, s.repo, "gamma/gamma_test.go", orchRepairedGammaTestSource)
	headA2 := revisionCommit(t, s, "strengthen the flag assertion")

	runB := liveRunID("orch-b")
	logB := &orchLog{}
	dispatchesB, bundleBytesB := 0, int64(0)

	logB.drive(runstate.KindPhase, tracePhaseID)
	traceB := readyTrace(t, s, runB)
	if traceB.RepoCommit != headA2 {
		t.Fatalf("run B trace repoCommit = %q, want the repaired revision %q", traceB.RepoCommit, headA2)
	}

	adopting := readyReuse(t, s, runB, runA, policy)
	const isolated = "isolated-change:gamma/gamma_test.go"
	// Every record run A left is named before the sweep, so a sweep over a
	// list reuse never filled would pass over nothing at all.
	decided := slices.Concat(adopting.Review, adopting.Mutation)
	gotDecided := make([]string, 0, len(decided))
	for _, decision := range decided {
		gotDecided = append(gotDecided, decision.ID)
	}
	wantDecided := []string{
		orchGreetGroup, orchCountGroup, orchFlagGroup, orchLimitGroup, orchTagGroup,
		orchLimitRow, orchTagRow,
	}
	if !slices.Equal(gotDecided, wantDecided) {
		t.Fatalf("reuse judged %v, want every record run A left: %v", gotDecided, wantDecided)
	}
	for _, decision := range decided {
		wantReused := decision.ID != orchFlagGroup
		if decision.Reused != wantReused {
			t.Errorf("reuse %s %s = %+v, want reused=%v", decision.Kind, decision.ID, decision, wantReused)
			continue
		}
		if !wantReused && decision.Reason != isolated {
			t.Errorf("reuse refused %s %s for %q, want %q", decision.Kind, decision.ID, decision.Reason, isolated)
		}
	}

	logB.drive(runstate.KindReviewGroup, orchFlagGroup)
	dispatched, bytes, files := orchReviewGroup(t, s, runB, orchFlagGroup, orchFlagRow, "HOLDS", policy)
	dispatchesB += dispatched
	bundleBytesB += bytes
	if files != 1 {
		t.Errorf("group %s was handed %d files, want the one file the repair moved", orchFlagGroup, files)
	}

	logB.drive(runstate.KindLiveAttempt, liveAttemptID)
	orchLiveJourney(t, s, runB, traceB.EvidencePath)

	outB := readyFinalize(t, s, runB)
	if outB.Verdict != report.VerdictPartial {
		t.Fatalf("run B verdict = %q, want %s; reasons=%+v", outB.Verdict, report.VerdictPartial, outB.Reasons)
	}
	requireReason(t, outB.Reasons, "4", orchTagRow, "mutation-environmental")

	repB := readPublishedReport(t, s.repo, outB.JSONPath)
	if want := (report.WorkCounts{Fresh: 3, Reused: 6, Failed: 0, Incomplete: 1}); repB.Work.Counts != want {
		t.Errorf("run B work counts = %+v, want %+v", repB.Work.Counts, want)
	}
	// What run B did itself is exactly what it drove; everything else in its
	// summary came from run A's ledger.
	freshB := make([]orchInvocation, 0, len(repB.Work.Items))
	for _, item := range repB.Work.Items {
		if item.Source == report.WorkFresh {
			freshB = append(freshB, orchInvocation{item.Kind, item.ID})
		}
	}
	if !reflect.DeepEqual(freshB, logB.entries) {
		t.Errorf("run B fresh work items = %+v, want exactly the obligations it drove: %+v", freshB, logB.entries)
	}
	greet := orchWorkItem(t, repB, runstate.KindReviewGroup, orchGreetGroup)
	if greet.Source != report.WorkReused {
		t.Fatalf("work item %s source = %q, want %q", orchGreetGroup, greet.Source, report.WorkReused)
	}
	if greet.Reuse == nil {
		t.Fatalf("work item %s is reused but carries no reuse reference", orchGreetGroup)
	}
	if greet.Reuse.PriorRunID != runA {
		t.Errorf("work item %s reuse priorRunId = %q, want run A %q", orchGreetGroup, greet.Reuse.PriorRunID, runA)
	}
	if greet.Reuse.Basis.PriorCommit != headA {
		t.Errorf("work item %s reuse basis priorCommit = %q, want run A's commit %q",
			orchGreetGroup, greet.Reuse.Basis.PriorCommit, headA)
	}
	if want := []string{orchGreetRow}; !slices.Equal(greet.Reuse.Basis.ClaimRows, want) {
		t.Errorf("work item %s reuse basis claimRows = %v, want %v", orchGreetGroup, greet.Reuse.Basis.ClaimRows, want)
	}

	resumedB := resumeFor(t, s.repo, s.coverageSource, runB)
	if resumedB.Counts.Reused != 6 {
		t.Errorf("resume B counts.reused = %d, want 6", resumedB.Counts.Reused)
	}

	markdown, err := os.ReadFile(filepath.Join(s.repo, filepath.FromSlash(outB.MarkdownPath)))
	if err != nil {
		t.Fatalf("read published markdown: %v", err)
	}
	if !strings.Contains(string(markdown), "## Work") {
		t.Errorf("published markdown carries no ## Work section:\n%s", markdown)
	}
	ref := greet.Reuse
	bullet := fmt.Sprintf(
		"- %s %s: reused from run %s seq %d at %s; basis: prior commit %s, %d changed paths, claims %s, %d context files, policy %s, toolchain %s",
		greet.Kind, greet.ID, ref.PriorRunID, ref.PriorSeq, ref.PriorRepoCommit,
		ref.Basis.PriorCommit, len(ref.Basis.ChangedPaths), strings.Join(ref.Basis.ClaimRows, ","),
		ref.Basis.ContextFiles, ref.Basis.Policy, ref.Basis.Toolchain)
	if !strings.Contains(string(markdown), bullet) {
		t.Errorf("published markdown carries no reused bullet for %s; want a line %q", orchGreetGroup, bullet)
	}

	// What the repair bought: the second run dispatched fewer critics, over
	// fewer bytes, and still published a report for every obligation.
	if dispatchesB >= dispatchesA {
		t.Errorf("dispatches A=%d B=%d, want the repair to dispatch strictly fewer critics", dispatchesA, dispatchesB)
	}
	if bundleBytesB >= bundleBytesA {
		t.Errorf("bundleBytes A=%d B=%d, want the repair to hand its critics strictly fewer bytes",
			bundleBytesA, bundleBytesB)
	}
	t.Logf("dispatches A=%d B=%d bundleBytes A=%d B=%d", dispatchesA, dispatchesB, bundleBytesA, bundleBytesB)
}
