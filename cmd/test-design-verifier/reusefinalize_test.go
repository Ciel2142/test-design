package main

import (
	"encoding/json"
	"fmt"
	"io"
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
	return opRequestJSON(t, s.repo, s.coverageSource, runID, extra)
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
// reading an empty adoption. It returns the new run's ID.
func readyAdoptedRun(
	t *testing.T, s *liveSubject, prefix string, seed func(*testing.T, *liveSubject, string),
) string {
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
	readyTrace(t, s, runID)
	out := readyReuse(t, s, runID, priorRunID, policy)
	for _, decision := range slices.Concat(out.Review, out.Mutation) {
		if !decision.Reused {
			t.Fatalf("reuse refused %s %s: %s", decision.Kind, decision.ID, decision.Reason)
		}
	}
	return runID
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

// readyFinalizeRequestJSON is the finalize request over the subject: the run's
// three fields, and nothing about the layers — an adopting run and a fresh one
// both answer for those from the ledger they built.
func readyFinalizeRequestJSON(t *testing.T, s *liveSubject, runID string, body finalizeRequestBody) string {
	t.Helper()
	return finalizeRequestJSONFor(t, s.repo, s.coverageSource, runID, body)
}

func readyFinalize(t *testing.T, s *liveSubject, runID string) finalizeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, freshFinalizeBody()))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return decodeFinalizeOutput(t, stdout)
}

// readyWantRefusal drives finalize expecting the named refusal and the empty
// working tree that says nothing was published.
func readyWantRefusal(t *testing.T, s *liveSubject, runID string, body finalizeRequestBody, code string) {
	t.Helper()
	exit, stdout, stderr := runVerifyOp(t, "finalize", readyFinalizeRequestJSON(t, s, runID, body))
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

// TestFinalize_AdoptedRecordsCarryTheRunsLayer2AndLayer4 is the adopting run's
// whole answer: a parent that pastes no bookkeeping at all still publishes a
// PASS, because the records its own ledger holds are read, re-verified and
// admitted as batches and layer-4 entries, each keeping the run it came from
// and each publishing a transcript copy of the original artifact.
func TestFinalize_AdoptedRecordsCarryTheRunsLayer2AndLayer4(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID := readyAdoptedRun(t, s, "rdy-adopt", readyCaughtVersionRow)

	out := readyFinalize(t, s, runID)
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

// TestFinalize_RefusesAnyParentPastedEnvelope holds the one door evidence may
// come through. Every layer a parent used to paste is now the run's own ledger
// to answer for, so a request that still carries one of those fields is refused
// by name before finalize touches the repository at all.
func TestFinalize_RefusesAnyParentPastedEnvelope(t *testing.T) {
	for _, retired := range []struct {
		field string
		value any
	}{
		{"traceEvidence", "/runs/run-9/trace-evidence.json"},
		{"layer2", map[string]any{"batches": []map[string]any{{
			"batchId": "b1", "runId": "fin-1", "rows": []string{"SC-01"},
			"verdicts": []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}},
		}}}},
		{"layer3", map[string]any{"evidencePath": "/runs/run-9/live-evidence.json"}},
		{"layer4", map[string]any{"rows": []map[string]any{{"rowId": "SC-07", "outcome": "caught"}}}},
	} {
		t.Run(retired.field, func(t *testing.T) {
			f := newFinalizeFixture(t)
			f.seed()

			exit, stdout, stderr := runVerifyOp(t, "finalize", f.legacyRequestJSON(retired.field, retired.value))

			if exit != exitError || stdout != "" {
				t.Fatalf("finalize was accepted: exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
			}
			got := decodeCLIError(t, stderr)
			if got.Code != codeEnvelopeNotAccepted {
				t.Errorf("refusal code = %q, want %q; message=%s", got.Code, codeEnvelopeNotAccepted, got.Message)
			}
			if !strings.Contains(got.Message, retired.field) {
				t.Errorf("message = %q, want it to name the retired field %s", got.Message, retired.field)
			}
			if lines := gitStatusLines(t, f.repo); len(lines) != 0 {
				t.Errorf("git status = %v, want a refusal to publish nothing", lines)
			}
		})
	}
}

// TestFinalize_AdoptedUnappliableRowBlocksWithItsArtifactReason proves an
// adopted refusal is still a refusal: the prior run could not apply SC-02's
// mutation, and the adopting run reports that in the prior run's own words
// rather than as a caught row or a default sentence.
func TestFinalize_AdoptedUnappliableRowBlocksWithItsArtifactReason(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID := readyAdoptedRun(t, s, "rdy-unappl", readyUnappliableVersionRow)

	out := readyFinalize(t, s, runID)
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
	runID := readyAdoptedRun(t, s, "rdy-envt", readyEnvironmentalVersionRow)

	out := readyFinalize(t, s, runID)
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
	runID := readyAdoptedRun(t, s, "rdy-unread", readyEnvironmentalVersionRow)

	// A group whose shape cannot be read is an obligation resume reports as
	// still owed, so finalize refuses it in resume's words. The environmental
	// mutation row is a completed failure — an answer, not a gap — so resume
	// lists nothing about it and finalize is the one that refuses to read past
	// a record it cannot decode. Either way nothing is published.
	for kind, code := range map[runstate.Kind]string{
		runstate.KindReviewGroup: codeRunIncomplete,
		runstate.KindMutationRow: "ledger-invalid",
	} {
		t.Run(string(kind), func(t *testing.T) {
			original := readyBreakRecordInputs(t, s, runID, kind)
			t.Cleanup(func() { readyRestoreLedger(t, s, runID, original) })
			readyWantRefusal(t, s, runID, freshFinalizeBody(), code)
		})
	}
}

// readyVersionSite is the site readyCaughtVersionRow hands evaluate-mutation
// for SC-02. A run that adopts that row publishes the prior run's site list
// unchanged, so the name has to be stated here to be checked there.
const readyVersionSite = "version-constant"

// readyPriorRun is the earlier run the retained-bytes and provenance proofs
// stand on: a run that reviewed both groups, caught SC-02's mutation and
// published a passing report, with everything a later run must be able to name
// about it — the commit and document it verified, the artifacts it retained and
// the sequence numbers they sit at, the commit its published files became, and
// the critic policy it reviewed under, which a consuming run must present again
// for the groups to be adopted at all.
type readyPriorRun struct {
	runID        string
	commit       string // the commit the prior run traced and verified
	sourceSHA256 string // the coverage document it verified
	headAfter    string // HEAD once its published files were committed
	ledgerPath   string
	reportPath   string // the published report, relative to the repository
	policy       *reviewPolicy
	artifacts    map[string]string // record id -> the retained artifact the record points at
	seqs         map[string]int    // record id -> its sequence number in the prior ledger
}

// readyResume asks the real resume op what one run of the subject holds.
func readyResume(t *testing.T, s *liveSubject, runID string) resumeOutput {
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

// readyPublishedPriorRun drives the whole earlier run — trace, both review
// groups under one policy, SC-02's mutation caught through the real mutation
// ops — finalizes it to a published PASS, and commits what it published, so the
// run that stands on it traces at a commit whose only difference from the prior
// one is the publication the coverage directory exempts.
func readyPublishedPriorRun(t *testing.T, s *liveSubject, prefix string) *readyPriorRun {
	t.Helper()
	prior := &readyPriorRun{
		runID:     liveRunID(prefix),
		policy:    reusePolicy(t),
		artifacts: map[string]string{},
		seqs:      map[string]int{},
	}

	tr := readyTrace(t, s, prior.runID)
	readyGroups(t, s, prior.runID, readDispatchInputs(t, tr.DispatchPath), prior.policy)
	readyCaughtVersionRow(t, s, prior.runID)

	state := readyResume(t, s, prior.runID)
	if state.Identity == nil {
		t.Fatalf("resume %s reported no identity; the prior run must name the commit and document it verified", prior.runID)
	}
	prior.ledgerPath = state.LedgerPath
	prior.commit, prior.sourceSHA256 = state.Identity.RepoCommit, state.Identity.SourceSHA256
	for _, want := range []struct {
		kind runstate.Kind
		id   string
	}{
		{runstate.KindReviewGroup, readyMarkerGroup},
		{runstate.KindReviewGroup, readyVersionGroup},
		{runstate.KindMutationRow, readyVersionRow},
	} {
		rec := ledgerRecord(t, state, want.kind, want.id)
		artifact, ok := rec.Artifacts[artifactEvidence]
		if !ok {
			t.Fatalf("prior %s %s retained no evidence artifact", want.kind, want.id)
		}
		prior.artifacts[want.id], prior.seqs[want.id] = artifact.Path, rec.Seq
	}

	out := readyFinalize(t, s, prior.runID)
	if out.Verdict != report.VerdictPass {
		t.Fatalf("prior run verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
	}
	prior.reportPath = out.JSONPath

	// The published report is committed inside the subject so the run that
	// adopts this one traces a clean tree; the only path that moved is the
	// publication itself.
	mustGit(t, s.repo, "add", "-A")
	mustGit(t, s.repo, "commit", "-q", "-m", "publish the prior run's verification")
	prior.headAfter = subjectHead(t, s.repo)
	if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
		t.Fatalf("git status = %v, want the subject clean once the prior run's report is committed", lines)
	}
	return prior
}

// readyArtifactBytes reads a retained artifact of the prior run, which lives in
// the isolated state directory rather than the subject repository.
func readyArtifactBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read retained artifact %s: %v", path, err)
	}
	return data
}

func readyWriteArtifact(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write retained artifact %s: %v", path, err)
	}
}

func readyDigest(t *testing.T, path string) string {
	t.Helper()
	sum, err := evidence.SHA256File(path)
	if err != nil {
		t.Fatalf("digest %s: %v", path, err)
	}
	return sum
}

// readyReused asserts one prior record was adopted and returns its decision.
func readyReused(t *testing.T, decisions []reuseDecision, id string) reuseDecision {
	t.Helper()
	decision := reuseDecisionFor(t, decisions, id)
	if !decision.Reused {
		t.Errorf("%s was refused %q, want it adopted: nothing about it moved", id, decision.Reason)
	}
	return decision
}

// readyRefused asserts one prior record was refused for reason, and that the
// refusal is the whole of what happened to it.
func readyRefused(t *testing.T, decisions []reuseDecision, id, reason string) {
	t.Helper()
	decision := reuseDecisionFor(t, decisions, id)
	if decision.Reused || decision.Reason != reason {
		t.Errorf("%s = %+v, want it refused %q", id, decision, reason)
	}
}

// readyFinalizeRequest is the decoded finalize request, which is how a proof
// reaches finalizeOp's between hook: the hook exists to change the world at the
// one moment between the report being written and being published.
func readyFinalizeRequest(t *testing.T, s *liveSubject, runID string) *finalizeRequest {
	t.Helper()
	req, cliErr := decodeFinalizeRequest("finalize", []byte(readyFinalizeRequestJSON(t, s, runID, freshFinalizeBody())))
	if cliErr != nil {
		t.Fatalf("decodeFinalizeRequest: %s", cliErr.Message)
	}
	return req
}

// covers: SC-27
func TestReuse_RejectsAlteredRetainedBytes(t *testing.T) {
	s := readySubject(t, "not-applicable")
	prior := readyPublishedPriorRun(t, s, "rdy-altered")
	reviewPath, mutationPath := prior.artifacts[readyMarkerGroup], prior.artifacts[readyVersionRow]

	// Reuse trusts nothing it did not re-hash. A record whose retained bytes no
	// longer answer to the digest the prior run recorded is refused by name,
	// and the refusal is that record's alone: every other record of the same
	// prior run is still adopted in the same answer.
	t.Run("tampered-at-reuse", func(t *testing.T) {
		truncated := liveRunID("rdy-alt-trunc")
		readyTrace(t, s, truncated)
		original := readyArtifactBytes(t, reviewPath)
		readyWriteArtifact(t, reviewPath, original[:len(original)-1])

		out := readyReuse(t, s, truncated, prior.runID, prior.policy)
		readyRefused(t, out.Review, readyMarkerGroup, "artifact-digest-mismatch:"+artifactEvidence)
		readyReused(t, out.Review, readyVersionGroup)
		readyReused(t, out.Mutation, readyVersionRow)
		readyWriteArtifact(t, reviewPath, original)

		deleted := liveRunID("rdy-alt-del")
		readyTrace(t, s, deleted)
		evidenceBytes := readyArtifactBytes(t, mutationPath)
		if err := os.Remove(mutationPath); err != nil {
			t.Fatalf("remove retained mutation evidence: %v", err)
		}

		out = readyReuse(t, s, deleted, prior.runID, prior.policy)
		readyRefused(t, out.Mutation, readyVersionRow, "artifact-missing:"+artifactEvidence)
		readyReused(t, out.Review, readyMarkerGroup)
		readyReused(t, out.Review, readyVersionGroup)
		readyWriteArtifact(t, mutationPath, evidenceBytes)
	})

	// Adoption is not the last time the bytes are read. Finalize re-hashes every
	// adopted artifact immediately before it publishes, so bytes that changed
	// after the ledger was read leave nothing published at all; with them
	// restored the same request publishes, and the transcript copy it names
	// digests to what the reuse reference recorded for the original.
	t.Run("tampered-at-finalize", func(t *testing.T) {
		runID := liveRunID("rdy-alt-fin")
		readyTrace(t, s, runID)
		out := readyReuse(t, s, runID, prior.runID, prior.policy)
		readyReused(t, out.Review, readyMarkerGroup)
		readyReused(t, out.Review, readyVersionGroup)
		readyReused(t, out.Mutation, readyVersionRow)

		original := readyArtifactBytes(t, reviewPath)
		cliErr := finalizeOp(readyFinalizeRequest(t, s, runID), io.Discard, func() {
			readyWriteArtifact(t, reviewPath, append(slices.Clone(original), '\n'))
		})
		if cliErr == nil {
			t.Fatalf("finalizeOp published a report over an artifact that changed under it")
		}
		if cliErr.Code != codeReusedArtifactChanged {
			t.Errorf("refusal code = %q, want %q; message=%s", cliErr.Code, codeReusedArtifactChanged, cliErr.Message)
		}
		if lines := gitStatusLines(t, s.repo); len(lines) != 0 {
			t.Errorf("git status = %v, want the refusal to have published nothing", lines)
		}
		readyWriteArtifact(t, reviewPath, original)

		// The one remaining door into a report the ledger already answers for:
		// a parent that pastes a layer at all is refused before finalize reads
		// anything, adopted or not.
		pasted := freshFinalizeBody()
		pasted.Layer2 = json.RawMessage(`{"batches":[{"batchId":"b1","runId":"` + runID +
			`","rows":["` + readyMarkerRow + `"],"verdicts":[{"rowId":"` + readyMarkerRow + `","verdict":"HOLDS"}]}]}`)
		readyWantRefusal(t, s, runID, pasted, codeEnvelopeNotAccepted)

		published := readyFinalize(t, s, runID)
		if published.Verdict != report.VerdictPass {
			t.Fatalf("verdict = %q, want %s; reasons=%+v", published.Verdict, report.VerdictPass, published.Reasons)
		}
		rep := readPublishedReport(t, s.repo, published.JSONPath)
		copyPath := readyTranscriptPath(s, runID, "reused-review-"+readyMarkerGroup+".json")
		ref := publishedTranscript(t, rep, copyPath)
		batch := readyBatch(t, rep, "reuse:"+readyMarkerGroup)
		if batch.Reuse == nil {
			t.Fatalf("batch %s carries no reuse reference", batch.BatchID)
		}
		if ref.SHA256 != batch.Reuse.Artifact.SHA256 {
			t.Errorf("transcript %s sha256 = %s, want the recorded %s", copyPath, ref.SHA256, batch.Reuse.Artifact.SHA256)
		}
		if got := readyDigest(t, reviewPath); ref.SHA256 != got {
			t.Errorf("transcript %s sha256 = %s, want the original's %s", copyPath, ref.SHA256, got)
		}
	})
}

// covers: SC-28
func TestReuse_PreservesOriginalProvenanceInReport(t *testing.T) {
	s := readySubject(t, "not-applicable")
	prior := readyPublishedPriorRun(t, s, "rdy-prov")

	retained := []string{
		prior.ledgerPath,
		prior.artifacts[readyMarkerGroup],
		prior.artifacts[readyVersionGroup],
		prior.artifacts[readyVersionRow],
	}
	before := make([]string, len(retained))
	for i, path := range retained {
		before[i] = readyDigest(t, path)
	}

	runID := liveRunID("rdy-prov-now")
	readyTrace(t, s, runID)
	adopted := readyReuse(t, s, runID, prior.runID, prior.policy)
	readyReused(t, adopted.Review, readyMarkerGroup)
	readyReused(t, adopted.Review, readyVersionGroup)
	readyReused(t, adopted.Mutation, readyVersionRow)

	// The publication is the only thing that moved between the two commits, and
	// the coverage directory is the one place a run writes: exempt, and so
	// absent from every adopted record's basis.
	if len(adopted.ChangedPaths) == 0 {
		t.Errorf("changedPaths = %+v, want the prior run's committed report", adopted.ChangedPaths)
	}
	for _, changed := range adopted.ChangedPaths {
		if changed.Class != "exempt" {
			t.Errorf("changed path %s class = %q, want exempt", changed.Path, changed.Class)
		}
	}
	reuseChangedPathFor(t, adopted, prior.reportPath)

	out := readyFinalize(t, s, runID)
	if out.Verdict != report.VerdictPass {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
	}

	// Adoption reads the prior run; it never writes to it.
	for i, path := range retained {
		if got := readyDigest(t, path); got != before[i] {
			t.Errorf("%s digests to %s after the adopting run finalized, want the untouched %s", path, got, before[i])
		}
	}

	rep := readPublishedReport(t, s.repo, out.JSONPath)
	if rep.RunID != runID {
		t.Errorf("report runId = %q, want the consuming run %q", rep.RunID, runID)
	}
	if rep.HeadCommit != prior.headAfter {
		t.Errorf("report headCommit = %q, want the commit this run verified %q", rep.HeadCommit, prior.headAfter)
	}

	for _, group := range []string{readyMarkerGroup, readyVersionGroup} {
		batch := readyBatch(t, rep, "reuse:"+group)
		if batch.RunID != prior.runID {
			t.Errorf("batch %s runId = %q, want the run it was judged in %q", batch.BatchID, batch.RunID, prior.runID)
		}
		ref := batch.Reuse
		if ref == nil {
			t.Errorf("batch %s carries no reuse reference", batch.BatchID)
			continue
		}
		if ref.PriorRunID != prior.runID || ref.PriorRepoCommit != prior.commit || ref.PriorSourceSHA256 != prior.sourceSHA256 {
			t.Errorf("batch %s reference = %+v, want run %s at %s over %s",
				batch.BatchID, ref, prior.runID, prior.commit, prior.sourceSHA256)
		}
		if ref.PriorSeq != prior.seqs[group] {
			t.Errorf("batch %s priorSeq = %d, want the prior record's %d", batch.BatchID, ref.PriorSeq, prior.seqs[group])
		}
		if ref.Outcome != "holds" {
			t.Errorf("batch %s reference outcome = %q, want holds", batch.BatchID, ref.Outcome)
		}
		if ref.Artifact.Path != prior.artifacts[group] {
			t.Errorf("batch %s artifact path = %q, want the original %q", batch.BatchID, ref.Artifact.Path, prior.artifacts[group])
		}
		if ref.Basis.PriorCommit != prior.commit {
			t.Errorf("batch %s basis priorCommit = %q, want %q", batch.BatchID, ref.Basis.PriorCommit, prior.commit)
		}
		if len(ref.Basis.ChangedPaths) != 0 {
			t.Errorf("batch %s basis changedPaths = %v, want none outside the exempt directory",
				batch.BatchID, ref.Basis.ChangedPaths)
		}
	}

	if got := publishedRow(t, rep, readyMarkerRow).Layer2; got.Verdict != "HOLDS" || got.BatchID != "reuse:"+readyMarkerGroup {
		t.Errorf("%s layer2 = %+v, want HOLDS from reuse:%s", readyMarkerRow, got, readyMarkerGroup)
	}
	// A row report's sites are the sites that survived, so a caught row names
	// none; the site the prior run mutated is preserved in the transcript copy
	// of its evidence, which is asserted below.
	layer4 := publishedRow(t, rep, readyVersionRow).Layer4
	if layer4.Status != "MUTATION-CAUGHT" || len(layer4.Sites) != 0 {
		t.Errorf("%s layer4 = %+v, want MUTATION-CAUGHT with no surviving site", readyVersionRow, layer4)
	}
	if layer4.Reuse == nil || layer4.Reuse.PriorRunID != prior.runID || layer4.Reuse.Outcome != "caught" {
		t.Errorf("%s layer4 reuse = %+v, want the caught row adopted from %s", readyVersionRow, layer4.Reuse, prior.runID)
	}

	// The published copies are the prior run's documents, not restamped ones.
	reviewCopy := filepath.Join(s.repo,
		filepath.FromSlash(readyTranscriptPath(s, runID, "reused-review-"+readyMarkerGroup+".json")))
	art, err := report.LoadReviewArtifact(reviewCopy, readyBatch(t, rep, "reuse:"+readyMarkerGroup).Reuse.Artifact.SHA256)
	if err != nil {
		t.Fatalf("load published review copy: %v", err)
	}
	if art.RunID != prior.runID || art.RepoCommit != prior.commit || art.SourceSHA256 != prior.sourceSHA256 {
		t.Errorf("review copy identity = %s/%s/%s, want the prior run %s/%s/%s",
			art.RunID, art.RepoCommit, art.SourceSHA256, prior.runID, prior.commit, prior.sourceSHA256)
	}
	mutationCopy := filepath.Join(s.repo,
		filepath.FromSlash(readyTranscriptPath(s, runID, "reused-mutation-"+readyVersionRow+".json")))
	ev, err := report.LoadMutation(mutationCopy)
	if err != nil {
		t.Fatalf("load published mutation copy: %v", err)
	}
	if ev.RunID != prior.runID {
		t.Errorf("mutation copy runId = %q, want the prior run %q", ev.RunID, prior.runID)
	}
	if len(ev.Sites) != 1 || ev.Sites[0].Site != readyVersionSite || ev.Sites[0].Outcome != "caught" {
		t.Errorf("mutation copy sites = %+v, want the prior run's caught %s", ev.Sites, readyVersionSite)
	}

	// The consuming run's own ledger says where each answer came from.
	state := readyResume(t, s, runID)
	for _, want := range []struct {
		kind runstate.Kind
		id   string
	}{
		{runstate.KindReviewGroup, readyMarkerGroup},
		{runstate.KindReviewGroup, readyVersionGroup},
		{runstate.KindMutationRow, readyVersionRow},
	} {
		rec := ledgerRecord(t, state, want.kind, want.id)
		detail := fmt.Sprintf("reused from %s seq %d", prior.runID, prior.seqs[want.id])
		if rec.Status != runstate.StatusCompleted || rec.Detail != detail {
			t.Errorf("%s %s = %s %q, want completed %q", want.kind, want.id, rec.Status, rec.Detail, detail)
		}
	}
}

// TestFinalize_PublishesAdoptedArtifactsFromVerifiedStagedCopies closes the
// window between the last digest check and the publish that reopens the
// original paths: every adopted artifact is copied into the run directory, the
// copy is what is verified against the reference, and the copy is what is
// published, so the bytes the report names are the bytes that land.
func TestFinalize_PublishesAdoptedArtifactsFromVerifiedStagedCopies(t *testing.T) {
	s := readySubject(t, "not-applicable")
	runID := readyAdoptedRun(t, s, "rdy-stage", readyCaughtVersionRow)

	var stdout strings.Builder
	if cliErr := finalizeOp(readyFinalizeRequest(t, s, runID), &stdout, nil); cliErr != nil {
		t.Fatalf("finalizeOp refused %s: %s", cliErr.Code, cliErr.Message)
	}
	out := decodeFinalizeOutput(t, stdout.String())
	rep := readPublishedReport(t, s.repo, out.JSONPath)

	for _, name := range []string{
		"reused-review-" + readyMarkerGroup + ".json",
		"reused-review-" + readyVersionGroup + ".json",
		"reused-mutation-" + readyVersionRow + ".json",
	} {
		published := readyTranscriptPath(s, runID, name)
		ref := publishedTranscript(t, rep, published)
		staged := filepath.Join(out.RunDir, "transcripts", runID, name)
		sum, err := evidence.SHA256File(staged)
		if err != nil {
			t.Errorf("staged copy %s: %v, want finalize to publish from a copy it verified in its own run directory", staged, err)
			continue
		}
		if sum != ref.SHA256 {
			t.Errorf("staged copy %s digests to %s, want the reference digest %s the report names", staged, sum, ref.SHA256)
		}
	}
	batch := readyBatch(t, rep, "reuse:"+readyMarkerGroup)
	if batch.Reuse == nil {
		t.Fatalf("batch %s carries no reuse reference", batch.BatchID)
	}
	if got := publishedTranscript(t, rep, readyTranscriptPath(s, runID, "reused-review-"+readyMarkerGroup+".json")); got.SHA256 != batch.Reuse.Artifact.SHA256 {
		t.Errorf("transcript sha256 = %s, want the reference digest %s", got.SHA256, batch.Reuse.Artifact.SHA256)
	}
}

// TestReuse_AReadoptedRecordKeepsItsOriginalProducer follows evidence across
// three runs. The bytes were produced by the first; the second adopted them and
// the third adopts them from the second. The third's reference must still name
// the first as the producer — the artifact says so, and finalize holds the two
// to each other — while its ledger says which run it took the record from.
func TestReuse_AReadoptedRecordKeepsItsOriginalProducer(t *testing.T) {
	s := readySubject(t, "not-applicable")
	policy := reusePolicy(t)

	runA := liveRunID("rdy-chain-a")
	trA := readyTrace(t, s, runA)
	readyGroups(t, s, runA, readDispatchInputs(t, trA.DispatchPath), policy)
	readyCaughtVersionRow(t, s, runA)
	stateA := readyResume(t, s, runA)
	seqA := map[string]int{}
	for _, want := range []struct {
		kind runstate.Kind
		id   string
	}{
		{runstate.KindReviewGroup, readyMarkerGroup},
		{runstate.KindReviewGroup, readyVersionGroup},
		{runstate.KindMutationRow, readyVersionRow},
	} {
		seqA[want.id] = ledgerRecord(t, stateA, want.kind, want.id).Seq
	}

	runB := liveRunID("rdy-chain-b")
	readyTrace(t, s, runB)
	for _, decision := range slices.Concat(readyReuse(t, s, runB, runA, policy).Review, readyReuse(t, s, runB, runA, policy).Mutation) {
		if !decision.Reused && decision.Reason != "already-recorded" {
			t.Fatalf("run B refused %s %s: %s", decision.Kind, decision.ID, decision.Reason)
		}
	}
	stateB := readyResume(t, s, runB)

	runC := liveRunID("rdy-chain-c")
	readyTrace(t, s, runC)
	outC := readyReuse(t, s, runC, runB, policy)
	for _, decision := range slices.Concat(outC.Review, outC.Mutation) {
		if !decision.Reused {
			t.Fatalf("run C refused %s %s: %s", decision.Kind, decision.ID, decision.Reason)
		}
		ref := decision.Reference
		if ref.PriorRunID != runA || ref.PriorRepoCommit != stateA.Identity.RepoCommit ||
			ref.PriorSourceSHA256 != stateA.Identity.SourceSHA256 || ref.PriorSeq != seqA[decision.ID] {
			t.Errorf("%s %s reference = %+v, want the original producer %s at %s seq %d",
				decision.Kind, decision.ID, ref, runA, stateA.Identity.RepoCommit, seqA[decision.ID])
		}
		if ref.Basis.PriorCommit != stateB.Identity.RepoCommit {
			t.Errorf("%s %s basis priorCommit = %q, want the commit of the run it was compared against, %q",
				decision.Kind, decision.ID, ref.Basis.PriorCommit, stateB.Identity.RepoCommit)
		}
		rec := ledgerRecord(t, readyResume(t, s, runC), decision.Kind, decision.ID)
		want := fmt.Sprintf("reused from %s seq %d", runB, ledgerRecord(t, stateB, decision.Kind, decision.ID).Seq)
		if rec.Detail != want {
			t.Errorf("%s %s detail = %q, want %q", decision.Kind, decision.ID, rec.Detail, want)
		}
	}

	out := readyFinalize(t, s, runC)
	if out.Verdict != report.VerdictPass {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
	}
	rep := readPublishedReport(t, s.repo, out.JSONPath)
	batch := readyBatch(t, rep, "reuse:"+readyMarkerGroup)
	if batch.Reuse == nil || batch.Reuse.PriorRunID != runA || batch.RunID != runA {
		t.Errorf("batch %s = run %q reuse %+v, want the producer %s", batch.BatchID, batch.RunID, batch.Reuse, runA)
	}
	if got := publishedRow(t, rep, readyVersionRow).Layer4.Reuse; got == nil || got.PriorRunID != runA {
		t.Errorf("%s layer4 reuse = %+v, want the producer %s", readyVersionRow, got, runA)
	}
}

// / TestFinalize_WithoutALedgerIsRefusedTraceIncomplete answers a run that never
// opened a ledger from disk: there is no trace phase to assemble a report from,
// and the refusal acquires no state root of its own on the way out.
func TestFinalize_WithoutALedgerIsRefusedTraceIncomplete(t *testing.T) {
	f := newFinalizeFixture(t)
	f.unseeded = true
	stateRoot := os.Getenv(runstate.EnvStateDir)

	f.wantRefusal(codeTraceIncomplete)

	entries, err := os.ReadDir(stateRoot)
	if err != nil {
		t.Fatalf("read state root %s: %v", stateRoot, err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("state root holds %v, want a ledgerless finalize to create nothing under it", names)
	}
}
