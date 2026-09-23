package main

import (
	"encoding/json"
	"slices"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/runstate"
)

// This file holds the one proof that runs the ready subject twice, at two
// commits. Everything else in this wave asks what may be adopted when the
// repository stood still; this asks what must be observed again when it did
// not. The answer reuse owes is the same in both revisions — the trace phase
// and the live attempt are never adopted — and the answer the run owes is the
// one the new revision produces: its own suite, its own live journey, and a
// report that names the commit it describes.

// The two markers the subject's binary can print: the one the coverage
// document approved and the one the production revision prints instead. The
// live plan's predicate is written against the first, so the second is a
// journey that really fails rather than one a test declared failed.
const (
	revisionMarkerV1 = "ready-v1"
	revisionMarkerV2 = "ready-v2"
)

// revisionMarkerV2Source is the production change of the second revision: the
// marker SC-01's tagged test asserts and the live journey walks, moved.
const revisionMarkerV2Source = `package ready

// Marker is the string the deployed binary prints once it is running.
func Marker() string { return "ready-v2" }
`

// revisionCommentedVersionTestSource is the isolated change of the first
// revision: the same declarations at the same lines, one comment added below
// them, in the one file only SC-02's review group was handed.
const revisionCommentedVersionTestSource = readyVersionTestSource + `
// The revision under test only adds this comment.
`

// revisionLivePlan is the plan the live-runner hands execute-live: one
// walkthrough step running the binary the profile's start step built, bound to
// SC-01 and settled by the marker the deployed binary prints. The command is
// the profile's own declared walkthrough with the run's id substituted.
func revisionLivePlan(runID string) map[string]any {
	return map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{{
			"expectation": "the deployed binary prints the approved ready marker",
			"command":     []string{"./ready-" + runID},
			"predicates": []map[string]string{
				{"form": "exit-code", "value": "0"},
				{"form": "matches", "stream": "stdout", "value": revisionMarkerV1},
			},
			"scBindings": []string{readyMarkerRow},
		}},
	}
}

// revisionCommit moves the subject's own history forward: it stages and
// commits inside the temporary repository the subject builder made, never this
// one, and returns the commit it wrote.
func revisionCommit(t *testing.T, s *liveSubject, message string) string {
	t.Helper()
	mustGit(t, s.repo, "add", "-A")
	mustGit(t, s.repo, "commit", "-q", "-m", message)
	return subjectHead(t, s.repo)
}

// revisionEnvironmentalRow leaves the run's own layer-4 answer for SC-02 in the
// ledger: the row is opened by prepare-mutation and closed by record-attempt as
// environmental, pointing at the attempt document record-attempt wrote.
func revisionEnvironmentalRow(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	readyEnvironmentalVersionRow(t, s, runID)
	rec := ledgerRecord(t, readyResume(t, s, runID), runstate.KindMutationRow, readyVersionRow)
	if rec.Artifacts[artifactAttempt].Path == "" {
		t.Fatalf("record-attempt left %s with artifacts %+v, want an attempt document", readyVersionRow, rec.Artifacts)
	}
}

func revisionFinalize(t *testing.T, s *liveSubject, runID string, body finalizeRequestBody) finalizeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, body))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return decodeFinalizeOutput(t, stdout)
}

// revisionRefuseFinalize drives finalize expecting an op-level refusal, and
// holds it to publishing nothing.
func revisionRefuseFinalize(t *testing.T, s *liveSubject, runID string, body finalizeRequestBody) cliError {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, body))
	if code != exitError || stdout != "" {
		t.Fatalf("finalize was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
		t.Errorf("git status = %v, want a refused finalize to publish nothing", lines)
	}
	return decodeCLIError(t, stderr)
}

// covers: SC-29
func TestReuse_NewRevisionGetsFreshSuiteAndLive(t *testing.T) {
	s := readySubject(t, "required")
	policy := reusePolicy(t)

	// Run A is the complete run the later revisions are asked to stand on: its
	// own trace, its two reviewed groups, a live journey that passed against
	// the binary the profile built, and a layer-4 obligation the environment
	// refused. Its published report is committed, so the revisions that follow
	// differ from it only by the source change each one makes.
	runA := liveRunID("rev29-a")
	traceA, _, execA := runLivePipeline(t, s, runA, revisionLivePlan(runA), nil)
	if got := rowVerdict(t, execA.RowVerdicts, readyMarkerRow).Verdict; got != "LIVE-PASS" {
		t.Fatalf("live verdict for %s at the approved revision = %q, want LIVE-PASS", readyMarkerRow, got)
	}
	readyGroups(t, s, runA, readDispatchInputs(t, traceA.DispatchPath), policy)
	revisionEnvironmentalRow(t, s, runA)

	outA := revisionFinalize(t, s, runA, freshFinalizeBody())
	if outA.Verdict != report.VerdictPartial {
		t.Fatalf("run A verdict = %q, want %s; reasons=%+v", outA.Verdict, report.VerdictPartial, outA.Reasons)
	}
	requireReason(t, outA.Reasons, "4", readyVersionRow, "mutation-environmental")
	headA := revisionCommit(t, s, "publish run A's verification report")

	// An edit to the one test file a single group was handed leaves every
	// other record standing. What it must not leave standing is the run's own
	// observations: the trace phase and the live attempt belong to the run
	// asking, so this revision owes both however much it adopted.
	t.Run("isolated", func(t *testing.T) {
		runB1 := liveRunID("rev29-b1")
		writeSubjectFile(t, s.repo, "ready/version_test.go", revisionCommentedVersionTestSource)
		revisionCommit(t, s, "comment the version test")
		t.Cleanup(func() { mustGit(t, s.repo, "reset", "--hard", headA) })

		readyTrace(t, s, runB1)
		out := readyReuse(t, s, runB1, runA, policy)

		if marker := reuseDecisionFor(t, out.Review, readyMarkerGroup); !marker.Reused || marker.Reason != "" {
			t.Errorf("group %s = %+v, want it adopted: nothing it was handed moved", readyMarkerGroup, marker)
		}
		const isolated = "isolated-change:ready/version_test.go"
		readyRefused(t, out.Review, readyVersionGroup, isolated)
		readyRefused(t, out.Mutation, readyVersionRow, isolated)
		if want := (reuseCounts{Reused: 1, Refused: 2}); out.Counts != want {
			t.Errorf("counts = %+v, want %+v: one group adopted, the edited group and its row refused",
				out.Counts, want)
		}
		if out.Trace.Reused || out.Trace.Reason != reasonFreshRequired {
			t.Errorf("trace = %+v, want it never adopted, reason %q", out.Trace, reasonFreshRequired)
		}
		if out.Live.Reused || out.Live.Reason != reasonFreshRequired {
			t.Errorf("live = %+v, want it never adopted, reason %q", out.Live, reasonFreshRequired)
		}

		resumed := readyResume(t, s, runB1)
		if !resumed.Live.Required || resumed.Live.Status != statusNone {
			t.Errorf("resumed live = %+v, want a required attempt with status %q", resumed.Live, statusNone)
		}
		want := resumePending{Kind: runstate.KindLiveAttempt, ID: liveAttemptID, Reason: reasonNeverStarted}
		found := false
		for _, pending := range resumed.Pending {
			if pending == want {
				found = true
			}
		}
		if !found {
			t.Errorf("pending = %+v, want %+v however much the revision adopted", resumed.Pending, want)
		}
	})

	// A production change moves what every record was judged against, and it
	// moves what the suite and the live journey observe. Nothing reuse can do
	// hides either: the run traces the new commit itself, walks the new binary
	// itself, and its report is blocked by the failures both found.
	t.Run("production", func(t *testing.T) {
		runB2 := liveRunID("rev29-b2")
		writeSubjectFile(t, s.repo, "ready/ready.go", revisionMarkerV2Source)
		headB2 := revisionCommit(t, s, "move the ready marker to "+revisionMarkerV2)

		tr, _, exec := runLivePipeline(t, s, runB2, revisionLivePlan(runB2), nil)
		if tr.RepoCommit != headB2 {
			t.Errorf("trace repoCommit = %q, want the new revision %q", tr.RepoCommit, headB2)
		}
		traceDoc, err := report.LoadTrace(tr.EvidencePath)
		if err != nil {
			t.Fatalf("LoadTrace: %v", err)
		}
		named := false
		for _, failure := range traceDoc.NewFailures {
			if failure.Name == "TestMarker" {
				named = true
			}
		}
		if !named {
			t.Errorf("newFailures = %+v, want the suite's own report of TestMarker failing here", traceDoc.NewFailures)
		}

		out := readyReuse(t, s, runB2, runA, policy)
		const changed = "module-changed:ready/ready.go"
		// Each record the prior run holds is named before the sweep, and the
		// counts pin how many there were: a sweep over a list reuse never
		// filled would pass over nothing at all.
		readyRefused(t, out.Review, readyMarkerGroup, changed)
		readyRefused(t, out.Review, readyVersionGroup, changed)
		readyRefused(t, out.Mutation, readyVersionRow, changed)
		if want := (reuseCounts{Refused: 3}); out.Counts != want {
			t.Errorf("counts = %+v, want %+v: a production change leaves nothing to adopt", out.Counts, want)
		}
		for _, decision := range slices.Concat(out.Review, out.Mutation) {
			if decision.Reused || decision.Reason != changed {
				t.Errorf("%s %s = %+v, want every record refused %q", decision.Kind, decision.ID, decision, changed)
			}
		}

		if got := rowVerdict(t, exec.RowVerdicts, readyMarkerRow).Verdict; got != "LIVE-FAIL" {
			t.Errorf("live verdict for %s = %q, want LIVE-FAIL against the new binary", readyMarkerRow, got)
		}
		liveDoc := readLiveEvidence(t, exec.EvidencePath)
		if liveDoc.RepoCommit != headB2 {
			t.Errorf("live evidence repoCommit = %q, want the new revision %q", liveDoc.RepoCommit, headB2)
		}

		// The prior run's live evidence is asked for first, while the tree is
		// still clean: a request may no longer name a journey at all, so run A's
		// is refused one gate earlier than the staleness rule that used to
		// answer for it, and the report still reads this run's own record.
		pasted := freshFinalizeBody()
		pasted.Layer3 = json.RawMessage(`{"evidencePath":"` + execA.EvidencePath + `"}`)
		if got := revisionRefuseFinalize(t, s, runB2, pasted); got.Code != codeEnvelopeNotAccepted {
			t.Errorf("refusal code = %q, want %q for run A's live evidence; message=%s",
				got.Code, codeEnvelopeNotAccepted, got.Message)
		}

		// This revision adopted nothing, so it answers for every layer itself.
		readyGroups(t, s, runB2, readDispatchInputs(t, tr.DispatchPath), policy)
		revisionEnvironmentalRow(t, s, runB2)
		final := revisionFinalize(t, s, runB2, freshFinalizeBodyWithPaths("ready/ready.go"))
		if final.Verdict != report.VerdictBlocked {
			t.Fatalf("verdict = %q, want %s; reasons=%+v", final.Verdict, report.VerdictBlocked, final.Reasons)
		}
		requireReason(t, final.Reasons, "1", readyMarkerRow, "red")
		requireReason(t, final.Reasons, "3", readyMarkerRow, "LIVE-FAIL")

		rep := readPublishedReport(t, s.repo, final.JSONPath)
		if rep.HeadCommit != headB2 {
			t.Errorf("published headCommit = %q, want the new revision %q", rep.HeadCommit, headB2)
		}
		if len(rep.Batches) != 2 {
			t.Fatalf("published batches = %+v, want the run's own two review groups", rep.Batches)
		}
		for _, batch := range rep.Batches {
			if batch.Reuse != nil {
				t.Errorf("batch %s = %+v, want no adopted batch: this revision adopted nothing", batch.BatchID, batch)
			}
		}
	})
}
