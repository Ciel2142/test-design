package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/runstate"
)

// The proofs in this file drive the real ops — baseline, trace, record-review,
// the mutation ops, reuse and finalize — over the ready subject, and read only
// what those ops wrote. What they hold between them is the line finalize draws
// around adopted evidence: the run's own ledger is the only way a prior run's
// verdict reaches a report, and a parent that pastes what was adopted is
// refused rather than counted twice.

// readyRequestJSON is a verify request over the ready subject plus whatever
// fields the op being driven adds.
func readyRequestJSON(t *testing.T, s *liveSubject, runID string, extra map[string]any) string {
	t.Helper()
	body := map[string]any{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       s.repo,
		"coverageSource": s.coverageSource,
		"runID":          runID,
	}
	for name, value := range extra {
		body[name] = value
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(data)
}

// readyTrace runs the real trace op for runID and returns its output, failing
// on a baseline the subject should never present as anything but ok.
func readyTrace(t *testing.T, s *liveSubject, runID string) traceOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "trace", readyRequestJSON(t, s, runID, nil))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)
	if tr.BaselineStatus != "ok" {
		t.Fatalf("trace baselineStatus = %q, want ok", tr.BaselineStatus)
	}
	return tr
}

// readyGroups opens and closes the subject's two review groups, one row each,
// under policy and with the critic's reads recorded: a group whose manifest is
// incomplete is not reusable at all, so an unrecorded read list would leave the
// proofs adopting nothing.
func readyGroups(t *testing.T, s *liveSubject, runID string, dispatch dispatchInputs, policy *reviewPolicy) {
	t.Helper()
	for _, group := range []struct{ id, row string }{
		{readyMarkerGroup, readyMarkerRow},
		{readyVersionGroup, readyVersionRow},
	} {
		readyRecordReview(t, s, runID, map[string]any{
			"groupId": group.id, "status": "started", "rows": []string{group.row}, "policy": policy,
		})
		readyRecordReview(t, s, runID, map[string]any{
			"groupId": group.id, "status": "completed",
			"verdicts": soundVerdicts(t, dispatch, group.row), "reads": []string{},
		})
	}
}

func readyRecordReview(t *testing.T, s *liveSubject, runID string, extra map[string]any) recordReviewOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "record-review", readyRequestJSON(t, s, runID, extra))
	if code != exitOK {
		t.Fatalf("record-review exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out recordReviewOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("record-review stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// readyAdoptedRun drives a whole prior run over the subject — trace, both
// review groups, and whatever layer-4 record seed leaves behind — then traces a
// fresh run and adopts every record the prior run proved. A refused record
// fails here rather than later, so a proof reading the finalize report is never
// reading an empty adoption. It returns the new run's ID and the trace evidence
// finalize must be handed.
func readyAdoptedRun(
	t *testing.T, s *liveSubject, prefix string, seed func(*testing.T, *liveSubject, string),
) (string, string) {
	t.Helper()
	// One policy identity, written once: reuse compares the critic's
	// instructions by path and digest, so a second file would read as a
	// changed policy rather than the same one.
	policy := reusePolicy(t)

	priorRunID := liveRunID(prefix + "-was")
	prior := readyTrace(t, s, priorRunID)
	readyGroups(t, s, priorRunID, readDispatchInputs(t, prior.DispatchPath), policy)
	if seed != nil {
		seed(t, s, priorRunID)
	}

	runID := liveRunID(prefix + "-now")
	now := readyTrace(t, s, runID)
	out := readyReuse(t, s, runID, priorRunID, policy)
	for _, decision := range slices.Concat(out.Review, out.Mutation) {
		if !decision.Reused {
			t.Fatalf("reuse refused %s %s: %s", decision.Kind, decision.ID, decision.Reason)
		}
	}
	return runID, now.EvidencePath
}

func readyReuse(t *testing.T, s *liveSubject, runID, priorRunID string, policy *reviewPolicy) reuseOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "reuse", readyRequestJSON(t, s, runID, map[string]any{
		"priorRunId": priorRunID, "policy": policy,
	}))
	if code != exitOK {
		t.Fatalf("reuse exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out reuseOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("reuse stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// readyMutatedVersionSource is the subject's version file with the constant
// bumped: the edit SC-02's tagged test is supposed to catch.
const readyMutatedVersionSource = `package ready

// Version is the subject's schema version.
func Version() int { return 2 }
`

// readyCaughtVersionRow leaves a caught mutation-row record for SC-02 behind,
// evidence and all, through the real prepare-mutation and evaluate-mutation ops.
func readyCaughtVersionRow(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	prep := preparedMutationFor(t, s.repo, s.coverageSource, runID, readyVersionRow)
	writeSubjectFile(t, prep.WorktreePath, "ready/version.go", readyMutatedVersionSource)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSONFor(t, s.repo, s.coverageSource, runID, prep.RunDir, "version-constant", true))
	if code != exitOK {
		t.Fatalf("evaluate-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("evaluate-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "caught" {
		t.Fatalf("mutation outcome = %q, want caught", out.Outcome)
	}
}

// readyUnappliableReason is the prior run's own words for why SC-02's mutation
// could not be applied; the report has to quote them rather than a default.
const readyUnappliableReason = "the version constant has no site this mutation applies to"

func readyUnappliableVersionRow(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	preparedMutationFor(t, s.repo, s.coverageSource, runID, readyVersionRow)
	code, _, stderr := runVerifyOp(t, "record-mutation", readyRequestJSON(t, s, runID, map[string]any{
		"rowId": readyVersionRow, "outcome": "unappliable", "reason": readyUnappliableReason,
	}))
	if code != exitOK {
		t.Fatalf("record-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
}

func readyEnvironmentalVersionRow(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	preparedMutationFor(t, s.repo, s.coverageSource, runID, readyVersionRow)
	code, _, stderr := runVerifyOp(t, "record-attempt", readyRequestJSON(t, s, runID, map[string]any{
		"layer": 4, "rowID": readyVersionRow, "refusal": "the mutation runner could not start in this environment",
	}))
	if code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
}

// readyFinalizeRequestJSON is the finalize request over the subject: the
// approval, the run's own trace, a fresh freshness answer, and whatever layer-2
// and layer-4 bookkeeping the parent pastes — which for an adopting run is
// usually nothing at all.
func readyFinalizeRequestJSON(
	t *testing.T, s *liveSubject, runID, tracePath string, batches []report.Batch, rows []report.MutationInput,
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
			Layer4:        finalizeLayer4{Rows: rows},
		},
	})
	if err != nil {
		t.Fatalf("marshal finalize request: %v", err)
	}
	return string(body)
}

func readyFinalize(
	t *testing.T, s *liveSubject, runID, tracePath string, batches []report.Batch, rows []report.MutationInput,
) finalizeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, tracePath, batches, rows))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return decodeFinalizeOutput(t, stdout)
}

// readyWantRefusal drives finalize expecting the named refusal and the empty
// working tree that says nothing was published.
func readyWantRefusal(
	t *testing.T, s *liveSubject, runID, tracePath string,
	batches []report.Batch, rows []report.MutationInput, code string,
) {
	t.Helper()
	exit, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, tracePath, batches, rows))
	if exit != exitError || stdout != "" {
		t.Fatalf("finalize was accepted: exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != code {
		t.Errorf("refusal code = %q, want %q; message=%s", got.Code, code, got.Message)
	}
	if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
		t.Errorf("git status = %v, want a refusal to publish nothing", lines)
	}
}

// readyTranscriptPath is where finalize publishes one of the run's transcripts:
// beside the coverage document, under the run's own directory.
func readyTranscriptPath(s *liveSubject, runID, name string) string {
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(s.coverageSource)))
	return repoPath(dir, "transcripts", runID, name)
}

func readyBatch(t *testing.T, rep *report.Report, batchID string) report.BatchReport {
	t.Helper()
	for _, batch := range rep.Batches {
		if batch.BatchID == batchID {
			return batch
		}
	}
	t.Fatalf("report batches = %+v, want an entry for %s", rep.Batches, batchID)
	return report.BatchReport{}
}

// readyHoldingBatch is the parent's own layer-2 bookkeeping for one row, in the
// canary-free shape a parent may paste: enough to be admitted on its own, so a
// refusal over it is about reuse and nothing else.
func readyHoldingBatch(runID, batchID, row string) report.Batch {
	return report.Batch{
		BatchID: batchID, DispatchID: "d-" + batchID, RunID: runID, Rows: []string{row},
		CanaryUnavailableReason: "the subject carries one assertion per row",
		Verdicts:                []report.CriticVerdict{{RowID: row, Verdict: "HOLDS"}},
	}
}

// TestFinalize_AdoptedRecordsCarryTheRunsLayer2AndLayer4 is the adopting run's
// whole answer: a parent that pastes no bookkeeping at all still publishes a
// PASS, because the records its own ledger holds are read, re-verified and
// admitted as batches and layer-4 entries, each keeping the run it came from
// and each publishing a transcript copy of the original artifact.
func TestFinalize_AdoptedRecordsCarryTheRunsLayer2AndLayer4(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID, tracePath := readyAdoptedRun(t, s, "rdy-adopt", readyCaughtVersionRow)

	out := readyFinalize(t, s, runID, tracePath, nil, nil)
	if out.Verdict != report.VerdictPass {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
	}
	rep := readPublishedReport(t, s.repo, out.JSONPath)

	for _, group := range []string{readyMarkerGroup, readyVersionGroup} {
		batch := readyBatch(t, rep, "reuse:"+group)
		if batch.Reuse == nil {
			t.Errorf("batch %s carries no reuse reference", batch.BatchID)
			continue
		}
		if batch.RunID != batch.Reuse.PriorRunID {
			t.Errorf("batch %s runId = %q, want the prior run %q", batch.BatchID, batch.RunID, batch.Reuse.PriorRunID)
		}
		if len(batch.Verdicts) != 1 {
			t.Errorf("batch %s verdicts = %+v, want the one verdict its artifact holds", batch.BatchID, batch.Verdicts)
		}
	}

	if got := publishedRow(t, rep, readyVersionRow).Layer4; got.Status != "MUTATION-CAUGHT" || got.Reuse == nil {
		t.Errorf("%s layer4 = %+v, want a caught mutation carrying its reuse reference", readyVersionRow, got)
	}

	for _, name := range []string{
		"reused-review-" + readyMarkerGroup + ".json",
		"reused-review-" + readyVersionGroup + ".json",
		"reused-mutation-" + readyVersionRow + ".json",
	} {
		path := readyTranscriptPath(s, runID, name)
		ref := publishedTranscript(t, rep, path)
		sum, err := evidence.SHA256File(filepath.Join(s.repo, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("published transcript %s: %v", path, err)
			continue
		}
		if sum != ref.SHA256 {
			t.Errorf("transcript %s digests to %s, want the listed %s", path, sum, ref.SHA256)
		}
	}
}

// TestFinalize_RefusesParentPastedEvidenceCarryingReuse holds the one door
// adopted evidence may come through. A reuse reference is written by the reuse
// op into the run's ledger; a parent that pastes one has bypassed the
// staleness and canary rules a fresh batch answers to, so it is refused before
// the report is computed at all.
func TestFinalize_RefusesParentPastedEvidenceCarryingReuse(t *testing.T) {
	pasted := &evidence.ReuseReference{
		PriorRunID: "an-earlier-run", PriorRepoCommit: "0123456789abcdef", PriorSeq: 2, Outcome: "pass",
	}

	t.Run("batch", func(t *testing.T) {
		f := newFinalizeFixture(t)
		named := slices.IndexFunc(f.body.Layer2.Batches, func(b report.Batch) bool { return b.BatchID == "b1" })
		f.body.Layer2.Batches[named].Reuse = pasted
		f.wantRefusal(report.CodeReuseNotAdmitted)
	})

	t.Run("layer-4 row", func(t *testing.T) {
		f := newFinalizeFixture(t)
		f.body.Layer4.Rows[0].Reuse = pasted
		f.wantRefusal(report.CodeReuseNotAdmitted)
	})
}

// TestFinalize_RefusesRepastingARowAnAdoptedRecordCovers keeps one row from
// being answered twice. The run's ledger already answers for both rows; a
// parent that also pastes an answer for one of them is describing the same row
// from two sources, and finalize refuses rather than choosing between them.
func TestFinalize_RefusesRepastingARowAnAdoptedRecordCovers(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID, tracePath := readyAdoptedRun(t, s, "rdy-repaste", readyEnvironmentalVersionRow)

	readyWantRefusal(t, s, runID, tracePath,
		[]report.Batch{readyHoldingBatch(runID, "b1", readyMarkerRow)}, nil, "reused-row-repasted")

	readyWantRefusal(t, s, runID, tracePath, nil,
		[]report.MutationInput{{RowID: readyVersionRow, Outcome: "unappliable", Reason: "pasted after the fact"}},
		"reused-row-repasted")
}

// TestFinalize_AdoptedUnappliableRowBlocksWithItsArtifactReason proves an
// adopted refusal is still a refusal: the prior run could not apply SC-02's
// mutation, and the adopting run reports that in the prior run's own words
// rather than as a caught row or a default sentence.
func TestFinalize_AdoptedUnappliableRowBlocksWithItsArtifactReason(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID, tracePath := readyAdoptedRun(t, s, "rdy-unappl", readyUnappliableVersionRow)

	out := readyFinalize(t, s, runID, tracePath, nil, nil)
	rep := readPublishedReport(t, s.repo, out.JSONPath)

	if got := publishedRow(t, rep, readyVersionRow).Layer4.Status; got != "MUTATION-UNAPPLIABLE" {
		t.Errorf("%s layer4 status = %q, want MUTATION-UNAPPLIABLE", readyVersionRow, got)
	}
	requireReason(t, rep.Reasons, "4", readyVersionRow, "MUTATION-UNAPPLIABLE")
	found := false
	for _, reason := range rep.Reasons {
		if reason.Code == "MUTATION-UNAPPLIABLE" && strings.Contains(reason.Detail, readyUnappliableReason) {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %+v, want the prior run's own reason %q", rep.Reasons, readyUnappliableReason)
	}
	if _, err := evidence.SHA256File(filepath.Join(s.repo,
		filepath.FromSlash(readyTranscriptPath(s, runID, "reused-mutation-unappliable-"+readyVersionRow+".json")))); err != nil {
		t.Errorf("published unappliable transcript: %v", err)
	}
}

// TestFinalize_AdoptedEnvironmentalRowReportsEnvironmental proves the third
// shape a layer-4 record can carry survives adoption: an attempt document, no
// mutation evidence, and the partial reason the environment earned.
func TestFinalize_AdoptedEnvironmentalRowReportsEnvironmental(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID, tracePath := readyAdoptedRun(t, s, "rdy-envt", readyEnvironmentalVersionRow)

	out := readyFinalize(t, s, runID, tracePath, nil, nil)
	rep := readPublishedReport(t, s.repo, out.JSONPath)

	if got := publishedRow(t, rep, readyVersionRow).Layer4.Status; got != "MUTATION-N/A (environmental)" {
		t.Errorf("%s layer4 status = %q, want MUTATION-N/A (environmental)", readyVersionRow, got)
	}
	requireReason(t, rep.Reasons, "4", readyVersionRow, "mutation-environmental")
	if _, err := evidence.SHA256File(filepath.Join(s.repo,
		filepath.FromSlash(readyTranscriptPath(s, runID, "reused-mutation-attempt-"+readyVersionRow+".json")))); err != nil {
		t.Errorf("published attempt transcript: %v", err)
	}
}

// TestFinalize_WithoutALedgerPublishesTheParentsBookkeepingAlone keeps the
// unchanged path unchanged: a run that never opened a ledger has nothing to
// adopt, and its report carries exactly what the parent pasted — no reuse
// reference and no reused transcript.
func TestFinalize_WithoutALedgerPublishesTheParentsBookkeepingAlone(t *testing.T) {
	f := newFinalizeFixture(t)

	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictPass {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
	}
	rep := f.publishedReport(out.JSONPath)
	for _, batch := range rep.Batches {
		if batch.Reuse != nil {
			t.Errorf("batch %s carries a reuse reference, want none without a ledger", batch.BatchID)
		}
	}
	for _, transcript := range rep.Transcripts {
		if strings.Contains(transcript.Path, "reused-") {
			t.Errorf("transcripts = %+v, want no reused copy without a ledger", rep.Transcripts)
			break
		}
	}
}

// readyLedgerPath is where this run's checkpoint lives, under the verifier's
// own state root.
func readyLedgerPath(t *testing.T, s *liveSubject, runID string) string {
	t.Helper()
	store, cliErr := openLedger("test", &verifyRequest{
		SchemaVersion: evidence.SchemaVersion, RepoRoot: s.repo, CoverageSource: s.coverageSource, RunID: runID,
	})
	if cliErr != nil {
		t.Fatalf("openLedger: %v", cliErr.Message)
	}
	return filepath.Join(store.Dir(), runstate.LedgerFile)
}

// readyBreakRecordInputs damages one field of the run's kind record's captured
// inputs, leaving the checkpoint itself valid JSON and the record's reuse
// reference in place: the record is still an adopted one, but the shape it
// captured can no longer be read. It returns the checkpoint's original bytes so
// the next case can put them back.
func readyBreakRecordInputs(t *testing.T, s *liveSubject, runID string, kind runstate.Kind) []byte {
	t.Helper()
	// Each kind is damaged in a field only its own shape declares, so what
	// fails is the decode of that record and nothing around it.
	damage := map[runstate.Kind]func(map[string]any){
		runstate.KindReviewGroup: func(inputs map[string]any) { inputs["rows"] = readyMarkerRow },
		runstate.KindMutationRow: func(inputs map[string]any) { inputs["runnerKind"] = 7 },
	}[kind]
	if damage == nil {
		t.Fatalf("readyBreakRecordInputs: no damage is defined for kind %s", kind)
	}

	path := readyLedgerPath(t, s, runID)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(original, &doc); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	records, _ := doc["records"].([]any)
	broken := 0
	for _, entry := range records {
		rec, _ := entry.(map[string]any)
		if rec["kind"] != string(kind) {
			continue
		}
		inputs, ok := rec["inputs"].(map[string]any)
		if !ok {
			continue
		}
		damage(inputs)
		broken++
	}
	if broken == 0 {
		t.Fatalf("ledger %s holds no %s record with captured inputs", path, kind)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode ledger: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
	return original
}

func readyRestoreLedger(t *testing.T, s *liveSubject, runID string, original []byte) {
	t.Helper()
	if err := os.WriteFile(readyLedgerPath(t, s, runID), original, 0o644); err != nil {
		t.Fatalf("restore ledger: %v", err)
	}
}

// TestFinalize_RefusesARecordWhoseCapturedInputsCannotBeRead keeps an
// unreadable record from passing for an absent one. A record whose captured
// shape cannot be decoded is not the same as a record that was never adopted:
// skipping it would drop its rows from what the ledger is known to answer for,
// and the parent could then paste an answer for a row that is already
// answered. Finalize refuses instead, and publishes nothing.
func TestFinalize_RefusesARecordWhoseCapturedInputsCannotBeRead(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID, tracePath := readyAdoptedRun(t, s, "rdy-unread", readyEnvironmentalVersionRow)

	for _, kind := range []runstate.Kind{runstate.KindReviewGroup, runstate.KindMutationRow} {
		t.Run(string(kind), func(t *testing.T) {
			original := readyBreakRecordInputs(t, s, runID, kind)
			t.Cleanup(func() { readyRestoreLedger(t, s, runID, original) })
			readyWantRefusal(t, s, runID, tracePath, nil, nil, "ledger-invalid")
		})
	}
}
