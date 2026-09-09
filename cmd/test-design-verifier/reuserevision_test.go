package main

import (
	"encoding/json"
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

// revisionResume asks the real resume op what the run still owes.
func revisionResume(t *testing.T, s *liveSubject, runID string) resumeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "resume", readyRequestJSON(t, s, runID, nil))
	if code != exitOK {
		t.Fatalf("resume exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out resumeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("resume stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// revisionEnvironmentalRow leaves the run's own layer-4 answer for SC-02 —
// the row is opened by prepare-mutation and closed by record-attempt as
// environmental — and returns it as the layer-4 entry the parent pastes,
// pointing at the attempt document record-attempt actually wrote.
func revisionEnvironmentalRow(t *testing.T, s *liveSubject, runID string) report.MutationInput {
	t.Helper()
	readyEnvironmentalVersionRow(t, s, runID)
	rec := ledgerRecord(t, revisionResume(t, s, runID), runstate.KindMutationRow, readyVersionRow)
	attempt := rec.Artifacts[artifactAttempt].Path
	if attempt == "" {
		t.Fatalf("record-attempt left %s with artifacts %+v, want an attempt document", readyVersionRow, rec.Artifacts)
	}
	return report.MutationInput{
		RowID:               readyVersionRow,
		Outcome:             "environmental",
		Reason:              "the mutation runner could not start in this environment",
		AttemptEvidencePath: attempt,
	}
}

// revisionBatches is the parent's own layer-2 bookkeeping for the revision: one
// canary-free batch per review group, each answering for its single row. A run
// that adopted nothing has no batch of its own in its ledger, so this is what a
// parent pastes to answer for layer 2 at all.
func revisionBatches(runID string) []report.Batch {
	return []report.Batch{
		readyHoldingBatch(runID, readyMarkerGroup, readyMarkerRow),
		readyHoldingBatch(runID, readyVersionGroup, readyVersionRow),
	}
}

// revisionFinalizeRequestJSON is the finalize request over the subject with a
// layer-3 evidence path, which the shared ready request does not carry: the
// live decision here is required, so every finalize of this subject answers for
// the live layer with a document.
func revisionFinalizeRequestJSON(
	t *testing.T, s *liveSubject, runID, tracePath, livePath string,
	batches []report.Batch, rows []report.MutationInput,
) string {
	t.Helper()
	body, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:  evidence.SchemaVersion,
		RepoRoot:       s.repo,
		CoverageSource: s.coverageSource,
		RunID:          runID,
		finalizeRequestBody: finalizeRequestBody{
			Approval:      finalizeApproval,
			TraceEvidence: tracePath,
			Freshness:     report.Freshness{Result: "fresh"},
			Layer2:        finalizeLayer2{Batches: batches},
			Layer3:        &finalizeLayer3{EvidencePath: livePath},
			Layer4:        finalizeLayer4{Rows: rows},
		},
	})
	if err != nil {
		t.Fatalf("marshal finalize request: %v", err)
	}
	return string(body)
}

func revisionFinalize(
	t *testing.T, s *liveSubject, runID, tracePath, livePath string,
	batches []report.Batch, rows []report.MutationInput,
) finalizeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize",
		revisionFinalizeRequestJSON(t, s, runID, tracePath, livePath, batches, rows))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return decodeFinalizeOutput(t, stdout)
}

// revisionRefuseFinalize drives finalize expecting an op-level refusal, and
// holds it to publishing nothing.
func revisionRefuseFinalize(
	t *testing.T, s *liveSubject, runID, tracePath, livePath string,
	batches []report.Batch, rows []report.MutationInput,
) cliError {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize",
		revisionFinalizeRequestJSON(t, s, runID, tracePath, livePath, batches, rows))
	if code != exitError || stdout != "" {
		t.Fatalf("finalize was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
		t.Errorf("git status = %v, want a refused finalize to publish nothing", lines)
	}
	return decodeCLIError(t, stderr)
}

// revisionDecisionFor returns the decision reuse reported for id, whichever
// list it landed in, and fails when reuse said nothing about it at all.
func revisionDecisionFor(t *testing.T, out reuseOutput, id string) reuseDecision {
	t.Helper()
	for _, decision := range append(append([]reuseDecision{}, out.Review...), out.Mutation...) {
		if decision.ID == id {
			return decision
		}
	}
	t.Fatalf("reuse said nothing about %s: review=%+v mutation=%+v", id, out.Review, out.Mutation)
	return reuseDecision{}
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
	envRowA := revisionEnvironmentalRow(t, s, runA)

	outA := revisionFinalize(t, s, runA, traceA.EvidencePath, execA.EvidencePath,
		revisionBatches(runA), []report.MutationInput{envRowA})
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

		if marker := revisionDecisionFor(t, out, readyMarkerGroup); !marker.Reused || marker.Reason != "" {
			t.Errorf("group %s = %+v, want it adopted: nothing it was handed moved", readyMarkerGroup, marker)
		}
		const isolated = "isolated-change:ready/version_test.go"
		if version := revisionDecisionFor(t, out, readyVersionGroup); version.Reused || version.Reason != isolated {
			t.Errorf("group %s = %+v, want it refused %q", readyVersionGroup, version, isolated)
		}
		if row := revisionDecisionFor(t, out, readyVersionRow); row.Reused || row.Reason != isolated {
			t.Errorf("row %s = %+v, want its mutation evidence refused %q", readyVersionRow, row, isolated)
		}
		if out.Trace.Reused || out.Trace.Reason != reasonFreshRequired {
			t.Errorf("trace = %+v, want it never adopted, reason %q", out.Trace, reasonFreshRequired)
		}
		if out.Live.Reused || out.Live.Reason != reasonFreshRequired {
			t.Errorf("live = %+v, want it never adopted, reason %q", out.Live, reasonFreshRequired)
		}

		resumed := revisionResume(t, s, runB1)
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
		for _, decision := range append(append([]reuseDecision{}, out.Review...), out.Mutation...) {
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
		// still clean: finalize's staleness rule keeps another run's journey
		// out however convenient it would be here.
		if got := revisionRefuseFinalize(t, s, runB2, tr.EvidencePath, execA.EvidencePath,
			revisionBatches(runB2), []report.MutationInput{}); got.Code != "stale-evidence" {
			t.Errorf("refusal code = %q, want stale-evidence for run A's live evidence; message=%s", got.Code, got.Message)
		}

		envRow := revisionEnvironmentalRow(t, s, runB2)
		final := revisionFinalize(t, s, runB2, tr.EvidencePath, exec.EvidencePath,
			revisionBatches(runB2), []report.MutationInput{envRow})
		if final.Verdict != report.VerdictBlocked {
			t.Fatalf("verdict = %q, want %s; reasons=%+v", final.Verdict, report.VerdictBlocked, final.Reasons)
		}
		requireReason(t, final.Reasons, "1", readyMarkerRow, "red")
		requireReason(t, final.Reasons, "3", readyMarkerRow, "LIVE-FAIL")

		rep := readPublishedReport(t, s.repo, final.JSONPath)
		if rep.HeadCommit != headB2 {
			t.Errorf("published headCommit = %q, want the new revision %q", rep.HeadCommit, headB2)
		}
		for _, batch := range rep.Batches {
			if batch.Reuse != nil {
				t.Errorf("batch %s = %+v, want no adopted batch: this revision adopted nothing", batch.BatchID, batch)
			}
		}
	})
}
