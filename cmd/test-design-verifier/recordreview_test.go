package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// runRecordReview drives the real record-review op and decodes its output.
func runRecordReview(t *testing.T, repo, runID string, extra map[string]any) recordReviewOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "record-review", requestWith(t, repo, runID, extra))
	if code != exitOK {
		t.Fatalf("record-review exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out recordReviewOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("record-review stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// refuseRecordReview drives record-review expecting a refusal and returns it.
func refuseRecordReview(t *testing.T, repo, runID string, extra map[string]any) cliError {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "record-review", requestWith(t, repo, runID, extra))
	if code != exitError || stdout != "" {
		t.Fatalf("record-review was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	return decodeCLIError(t, stderr)
}

func readDispatchInputs(t *testing.T, path string) dispatchInputs {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dispatch inputs %s: %v", path, err)
	}
	var payload dispatchInputs
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("dispatch inputs did not decode: %v", err)
	}
	return payload
}

func dispatchRowFor(t *testing.T, payload dispatchInputs, rowID string) dispatchRow {
	t.Helper()
	for _, row := range payload.Rows {
		if row.RowID == rowID {
			return row
		}
	}
	t.Fatalf("no %s row in the dispatch inputs", rowID)
	return dispatchRow{}
}

// TestRecordReview_RefusesAMalformedRequestBeforeTheLedger walks the request
// checks a group must clear before anything is recorded. Every one of them is
// decided from the request and the coverage document alone, which is why they
// answer before the run has a trace at all.
func TestRecordReview_RefusesAMalformedRequestBeforeTheLedger(t *testing.T) {
	repo := fixtureRepo(t)
	cases := []struct {
		code  string
		extra map[string]any
	}{
		{"missing-group-id", map[string]any{"status": "started", "rows": []string{"SC-01"}}},
		{"invalid-group-id", map[string]any{"groupId": "a/b", "status": "started", "rows": []string{"SC-01"}}},
		{"invalid-status", map[string]any{"groupId": "g1", "status": "opened", "rows": []string{"SC-01"}}},
		{"missing-rows", map[string]any{"groupId": "g1", "status": "started"}},
		{"duplicate-row", map[string]any{"groupId": "g1", "status": "started", "rows": []string{"SC-01", "SC-01"}}},
		{"unknown-row", map[string]any{"groupId": "g1", "status": "started", "rows": []string{"SC-77"}}},
		{"trace-incomplete", map[string]any{"groupId": "g1", "status": "started", "rows": []string{"SC-01"}}},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			if got := refuseRecordReview(t, repo, "review-refusals", tc.extra); got.Code != tc.code {
				t.Errorf("error code = %q, want %q (message %q)", got.Code, tc.code, got.Message)
			}
		})
	}
}

// TestRecordReview_GroupLifecycle drives one real trace and then the whole
// group protocol over it: a row may belong to one current group, a group is
// completed once, the verdicts must cover exactly the group's rows, and the
// completion writes the evidence that carries the dispatch inputs the group was
// judged against.
func TestRecordReview_GroupLifecycle(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "review-lifecycle"

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)
	payload := readDispatchInputs(t, tr.DispatchPath)

	started := runRecordReview(t, repo, runID, map[string]any{
		"groupId": "g1", "status": "started", "rows": []string{"SC-01", "SC-02"},
	})
	if started.Status != "started" || started.GroupID != "g1" {
		t.Fatalf("record-review started = %+v, want group g1 started", started)
	}

	t.Run("a row already in a current group cannot join a second", func(t *testing.T) {
		got := refuseRecordReview(t, repo, runID, map[string]any{
			"groupId": "g2", "status": "started", "rows": []string{"SC-02", "SC-03"},
		})
		if got.Code != "row-already-grouped" {
			t.Errorf("error code = %q, want row-already-grouped (message %q)", got.Code, got.Message)
		}
	})

	t.Run("a group that was never started cannot be completed", func(t *testing.T) {
		got := refuseRecordReview(t, repo, runID, map[string]any{
			"groupId": "g9", "status": "completed",
			"verdicts": []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}},
		})
		if got.Code != "review-group-not-started" {
			t.Errorf("error code = %q, want review-group-not-started (message %q)", got.Code, got.Message)
		}
	})

	t.Run("verdicts are refused before the group's own coverage is judged", func(t *testing.T) {
		cases := []struct {
			code     string
			verdicts []map[string]string
		}{
			{"duplicate-verdict", []map[string]string{
				{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-01", "verdict": "WEAK"},
			}},
			{"verdict-outside-group", []map[string]string{
				{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-03", "verdict": "HOLDS"},
			}},
			{"invalid-verdict", []map[string]string{
				{"rowId": "SC-01", "verdict": "PASS"}, {"rowId": "SC-02", "verdict": "HOLDS"},
			}},
			{"missing-verdict", []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}}},
		}
		for _, tc := range cases {
			t.Run(tc.code, func(t *testing.T) {
				got := refuseRecordReview(t, repo, runID, map[string]any{
					"groupId": "g1", "status": "completed", "verdicts": tc.verdicts,
				})
				if got.Code != tc.code {
					t.Errorf("error code = %q, want %q (message %q)", got.Code, tc.code, got.Message)
				}
			})
		}
	})

	completed := runRecordReview(t, repo, runID, map[string]any{
		"groupId": "g1", "status": "completed",
		"verdicts": []map[string]string{
			{"rowId": "SC-01", "verdict": "HOLDS", "reason": "the assertion names the bound value"},
			{"rowId": "SC-02", "verdict": "HOLDS"},
		},
	})
	if completed.Outcome != "holds" {
		t.Fatalf("outcome = %q, want holds when every verdict holds", completed.Outcome)
	}

	t.Run("the completed group's evidence carries what it was judged against", func(t *testing.T) {
		// The trace is the ledger's seq-1 record and g1 its seq-2 one; the
		// file carries the started record's seq so a repeated group never
		// writes over the evidence its superseded record digested.
		if filepath.Base(completed.EvidencePath) != "review-g1-2.json" {
			t.Errorf("evidencePath = %q, want it to name review-g1-2.json", completed.EvidencePath)
		}
		data, err := os.ReadFile(completed.EvidencePath)
		if err != nil {
			t.Fatalf("read review evidence: %v", err)
		}
		var doc reviewEvidence
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("review evidence did not decode: %v\ndata=%s", err, data)
		}
		if doc.SchemaVersion != evidence.SchemaVersion || doc.VerifierVersion != evidence.VerifierVersion {
			t.Errorf("envelope = %+v, want the verifier's own", doc.Envelope)
		}
		if doc.RunID != runID || doc.GroupID != "g1" {
			t.Errorf("identity = %s/%s, want %s/g1", doc.RunID, doc.GroupID, runID)
		}
		if doc.RepoCommit != payload.RepoCommit || doc.Source != fixtureCoverageSource || doc.SourceSHA256 == "" {
			t.Errorf("identity = %+v, want the traced commit and source", doc)
		}
		if want := []string{"SC-01", "SC-02"}; !reflect.DeepEqual(doc.Rows, want) {
			t.Errorf("rows = %v, want %v", doc.Rows, want)
		}
		// A row's sources are the hashes the dispatch document names for it,
		// the primary first and none of them twice. The document lists the
		// primary again among the contributors, so an identity built by
		// concatenation would record it twice and describe a set of sources no
		// critic was ever handed.
		if len(doc.Inputs.Sources) != len(doc.Rows) {
			t.Errorf("inputs.sources = %v, want one entry per group row %v", doc.Inputs.Sources, doc.Rows)
		}
		for _, id := range doc.Rows {
			got := doc.Inputs.Sources[id]
			row := dispatchRowFor(t, payload, id)
			distinct := map[string]bool{}
			if row.TestSourceSHA256 != "" {
				distinct[row.TestSourceSHA256] = true
			}
			for _, c := range row.Contributors {
				if c.TestSourceSHA256 != "" {
					distinct[c.TestSourceSHA256] = true
				}
			}
			if len(got) == 0 || got[0] != row.TestSourceSHA256 {
				t.Errorf("inputs.sources[%s] = %v, want the primary source %s first", id, got, row.TestSourceSHA256)
			}
			seen := map[string]bool{}
			for _, hash := range got {
				if seen[hash] {
					t.Errorf("inputs.sources[%s] = %v, want %s listed once", id, got, hash)
				}
				if !distinct[hash] {
					t.Errorf("inputs.sources[%s] = %v, want only hashes the dispatch document names for %s", id, got, id)
				}
				seen[hash] = true
			}
			if len(seen) != len(distinct) {
				t.Errorf("inputs.sources[%s] = %v, want every distinct source the dispatch document names (%d)", id, got, len(distinct))
			}
		}
		digest, err := evidence.SHA256File(tr.DispatchPath)
		if err != nil {
			t.Fatalf("SHA256File: %v", err)
		}
		if doc.Inputs.DispatchInputsSHA256 != digest {
			t.Errorf("inputs.dispatchInputsSha256 = %q, want the digest of %s (%s)", doc.Inputs.DispatchInputsSHA256, tr.DispatchPath, digest)
		}
		want := []reviewVerdict{
			{RowID: "SC-01", Verdict: "HOLDS", Reason: "the assertion names the bound value"},
			{RowID: "SC-02", Verdict: "HOLDS"},
		}
		if !reflect.DeepEqual(doc.Verdicts, want) {
			t.Errorf("verdicts = %+v, want %+v", doc.Verdicts, want)
		}
	})

	t.Run("a completed group is not completed twice", func(t *testing.T) {
		got := refuseRecordReview(t, repo, runID, map[string]any{
			"groupId": "g1", "status": "completed",
			"verdicts": []map[string]string{
				{"rowId": "SC-01", "verdict": "WEAK"}, {"rowId": "SC-02", "verdict": "WEAK"},
			},
		})
		if got.Code != "duplicate-completion" {
			t.Errorf("error code = %q, want duplicate-completion (message %q)", got.Code, got.Message)
		}
	})

	t.Run("a completed group is not restarted without a reason", func(t *testing.T) {
		got := refuseRecordReview(t, repo, runID, map[string]any{
			"groupId": "g1", "status": "started", "rows": []string{"SC-01", "SC-02"},
		})
		if got.Code != "review-group-completed" {
			t.Errorf("error code = %q, want review-group-completed (message %q)", got.Code, got.Message)
		}
	})

	t.Run("a verdict below HOLDS blocks its group", func(t *testing.T) {
		runRecordReview(t, repo, runID, map[string]any{
			"groupId": "g2", "status": "started", "rows": []string{"SC-03"},
		})
		blocked := runRecordReview(t, repo, runID, map[string]any{
			"groupId": "g2", "status": "completed",
			"verdicts": []map[string]string{{"rowId": "SC-03", "verdict": "MISSING", "reason": "layer-1 gap"}},
		})
		if blocked.Outcome != "blocked" {
			t.Errorf("outcome = %q, want blocked when a verdict is not HOLDS", blocked.Outcome)
		}
	})

	out := runResume(t, repo, runID)
	if len(out.Review.Groups) != 2 {
		t.Fatalf("review.groups = %+v, want both groups", out.Review.Groups)
	}
	for _, group := range out.Review.Groups {
		if group.Status != "completed" || group.Reason != "" {
			t.Errorf("group %s = %+v, want a valid completed group", group.ID, group)
		}
	}
	for _, id := range []string{"SC-01", "SC-02", "SC-03"} {
		if slices.Contains(out.Review.Uncovered, id) {
			t.Errorf("review.uncovered = %v, want %s covered by its group", out.Review.Uncovered, id)
		}
	}
	if out.Review.Complete {
		t.Error("review.complete = true, want false while seven rows are ungrouped")
	}
	if out.Counts.Failed != 1 {
		t.Errorf("counts.failed = %d, want 1: the blocked group is a failed outcome", out.Counts.Failed)
	}
}

// opResult is what one concurrently driven op returned.
type opResult struct {
	code   int
	stdout string
	stderr string
}

// runConcurrently drives one op once per request, every call released at the
// same instant, and returns the results in request order. The requests are
// built beforehand so that nothing inside a goroutine can fail the test.
func runConcurrently(t *testing.T, op string, requests []string) []opResult {
	t.Helper()
	results := make([]opResult, len(requests))
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-release
			code, stdout, stderr := runVerifyOp(t, op, request)
			results[i] = opResult{code: code, stdout: stdout, stderr: stderr}
		}()
	}
	close(release)
	wg.Wait()
	return results
}

// TestRecordReview_ConcurrentStartsKeepARowInOneGroup is the parallel-critic
// case the ledger lock exists for: ten parents open ten groups over one row at
// once. The row belongs to exactly one current group afterwards — the check
// that a row is ungrouped and the Start that claims it happen under one
// exclusive lock, so nine callers see the tenth's claim and are refused by
// name rather than each reading a ledger with no group in it.
func TestRecordReview_ConcurrentStartsKeepARowInOneGroup(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "review-start-race"
	if code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID)); code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	const groups = 10
	requests := make([]string, 0, groups)
	for i := range groups {
		requests = append(requests, requestWith(t, repo, runID, map[string]any{
			"groupId": fmt.Sprintf("g%d", i), "status": "started", "rows": []string{"SC-01"},
		}))
	}

	results := runConcurrently(t, "record-review", requests)

	started, refused := 0, 0
	for i, result := range results {
		switch result.code {
		case exitOK:
			started++
		case exitError:
			if got := decodeCLIError(t, result.stderr); got.Code != "row-already-grouped" {
				t.Errorf("g%d refused with %q, want row-already-grouped (message %q)", i, got.Code, got.Message)
			}
			refused++
		default:
			t.Errorf("g%d exit code = %d; stdout=%s stderr=%s", i, result.code, result.stdout, result.stderr)
		}
	}
	if started != 1 || refused != groups-1 {
		t.Errorf("started = %d, refused = %d, want exactly one group started and %d refused", started, refused, groups-1)
	}

	out := runResume(t, repo, runID)
	if len(out.Review.Groups) != 1 {
		t.Errorf("review.groups = %+v, want SC-01 in exactly one current group", out.Review.Groups)
	}
}

// TestRecordReview_ConcurrentCompletionsRecordOneResult: two parents complete
// one group at the same instant with different verdicts. Exactly one result
// stands, and it is whole — the recorded outcome was computed from the verdicts
// the evidence file holds, and the record's digest is of that file. The evidence
// is written and the completion recorded under one exclusive lock, so the loser
// sees a completed group and writes nothing.
func TestRecordReview_ConcurrentCompletionsRecordOneResult(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "review-complete-race"
	if code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID)); code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	startReviewGroup(t, repo, runID, "A", "SC-01", "SC-02")

	requests := []string{
		requestWith(t, repo, runID, map[string]any{
			"groupId": "A", "status": "completed",
			"verdicts": []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-02", "verdict": "HOLDS"}},
		}),
		requestWith(t, repo, runID, map[string]any{
			"groupId": "A", "status": "completed",
			"verdicts": []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-02", "verdict": "WEAK", "reason": "asserts nothing about the value"}},
		}),
	}

	results := runConcurrently(t, "record-review", requests)

	var accepted []recordReviewOutput
	for i, result := range results {
		switch result.code {
		case exitOK:
			var out recordReviewOutput
			if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
				t.Fatalf("completion %d stdout did not decode: %v\nstdout=%s", i, err, result.stdout)
			}
			accepted = append(accepted, out)
		case exitError:
			if got := decodeCLIError(t, result.stderr); got.Code != "duplicate-completion" {
				t.Errorf("completion %d refused with %q, want duplicate-completion (message %q)", i, got.Code, got.Message)
			}
		default:
			t.Errorf("completion %d exit code = %d; stdout=%s stderr=%s", i, result.code, result.stdout, result.stderr)
		}
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted completions = %+v, want exactly one", accepted)
	}
	won := accepted[0]

	rec := ledgerRecord(t, runResume(t, repo, runID), runstate.KindReviewGroup, "A")
	if rec.Status != runstate.StatusCompleted || rec.Outcome != won.Outcome {
		t.Errorf("record A = %s/%s, want completed with the accepted outcome %q", rec.Status, rec.Outcome, won.Outcome)
	}
	if got := rec.Artifacts[artifactEvidence].Path; got != won.EvidencePath {
		t.Errorf("record A evidence path = %q, want the accepted completion's %q", got, won.EvidencePath)
	}
	if reasons := runstate.Verify(&rec); len(reasons) != 0 {
		t.Errorf("Verify(A) = %v, want the recorded digest to match the evidence on disk", reasons)
	}
	data, err := os.ReadFile(won.EvidencePath)
	if err != nil {
		t.Fatalf("read review evidence: %v", err)
	}
	var doc reviewEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("review evidence did not decode: %v\ndata=%s", err, data)
	}
	if got := reviewOutcome(doc.Verdicts); got != rec.Outcome {
		t.Errorf("evidence verdicts %+v imply %q, want the recorded outcome %q computed from them", doc.Verdicts, got, rec.Outcome)
	}
}

// TestRecordReview_ARepeatedGroupKeepsTheSupersededEvidence: a superseded
// record keeps its outcome, artifacts and timestamps, and the artifact it
// keeps must still be the file it digested. The repeated group's completion
// therefore lands in a file of its own, named by its record's seq, and the
// superseded record verifies against the verdicts it was completed from.
func TestRecordReview_ARepeatedGroupKeepsTheSupersededEvidence(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "review-repeat-evidence"
	if code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID)); code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	startReviewGroup(t, repo, runID, "A", "SC-01", "SC-02")
	first := runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "completed",
		"verdicts": []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-02", "verdict": "HOLDS"}},
	})
	if first.Outcome != "holds" {
		t.Fatalf("first outcome = %q, want holds", first.Outcome)
	}

	runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "started", "rows": []string{"SC-01", "SC-02"},
		"repeat": map[string]string{"reason": "critic environment repaired"},
	})
	second := runRecordReview(t, repo, runID, map[string]any{
		"groupId": "A", "status": "completed",
		"verdicts": []map[string]string{{"rowId": "SC-01", "verdict": "HOLDS"}, {"rowId": "SC-02", "verdict": "WEAK", "reason": "asserts nothing about the value"}},
	})
	if second.Outcome != "blocked" {
		t.Fatalf("second outcome = %q, want blocked", second.Outcome)
	}
	if second.EvidencePath == first.EvidencePath {
		t.Fatalf("repeated completion wrote %q, want a file of its own beside the superseded %q", second.EvidencePath, first.EvidencePath)
	}

	out := runResume(t, repo, runID)
	replaced := supersededRecord(t, out, runstate.KindReviewGroup, "A")
	if replaced.Outcome != "holds" || replaced.Artifacts[artifactEvidence].Path != first.EvidencePath {
		t.Errorf("superseded A = %+v, want outcome holds and its own evidence path %q", replaced, first.EvidencePath)
	}
	if reasons := runstate.Verify(&replaced); len(reasons) != 0 {
		t.Errorf("Verify(superseded A) = %v, want the retained evidence to still digest to what the record carries", reasons)
	}
	current := ledgerRecord(t, out, runstate.KindReviewGroup, "A")
	if current.Outcome != "blocked" || current.Artifacts[artifactEvidence].Path != second.EvidencePath {
		t.Errorf("current A = %+v, want outcome blocked and evidence path %q", current, second.EvidencePath)
	}
	if reasons := runstate.Verify(&current); len(reasons) != 0 {
		t.Errorf("Verify(current A) = %v, want none", reasons)
	}
	if want := fmt.Sprintf("review-A-%d.json", current.Seq); filepath.Base(second.EvidencePath) != want {
		t.Errorf("current evidence = %q, want it named %s after the record that completed it", second.EvidencePath, want)
	}

	data, err := os.ReadFile(first.EvidencePath)
	if err != nil {
		t.Fatalf("read the superseded evidence: %v", err)
	}
	var doc reviewEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("superseded evidence did not decode: %v\ndata=%s", err, data)
	}
	if got := reviewOutcome(doc.Verdicts); got != "holds" {
		t.Errorf("superseded evidence verdicts %+v imply %q, want the holds it was completed from", doc.Verdicts, got)
	}
}

// fixtureGateVar, when set in the environment the suite inherits, makes the
// gated fixture's SC-01 test skip. It is the one lever a test has to make a
// trace of an unchanged commit hand out different dispatch inputs: trace
// refuses a dirty tree, and a commit would move the run's identity.
const fixtureGateVar = "TEST_DESIGN_FIXTURE_GATE_SC01"

// gatedFixtureEdit rewrites the fixture's SC-01 test to skip under
// fixtureGateVar, applied before the fixture's first commit so the run's
// commit never moves.
func gatedFixtureEdit(t *testing.T) func(repo string) {
	t.Helper()
	return func(repo string) {
		path := filepath.Join(repo, "orders", "orders_test.go")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the fixture's orders test: %v", err)
		}
		src := replaceExactlyOnce(t, string(data), `import "testing"`, "import (\n\t\"os\"\n\t\"testing\"\n)")
		src = replaceExactlyOnce(t, src, "func TestValidateQuantityAccepts(t *testing.T) {\n",
			"func TestValidateQuantityAccepts(t *testing.T) {\n\tif os.Getenv(\""+fixtureGateVar+"\") != \"\" {\n\t\tt.Skip(\"gated off for this trace\")\n\t}\n")
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write the fixture's orders test: %v", err)
		}
	}
}

// repeatTrace runs trace again for the run under a repeat reason and returns
// what it produced.
func repeatTrace(t *testing.T, repo, runID, reason string) traceOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "trace", requestWith(t, repo, runID, map[string]any{
		"repeat": map[string]string{"reason": reason},
	}))
	if code != exitOK {
		t.Fatalf("repeated trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return decodeTraceOutput(t, stdout)
}

// TestRecordReview_DispatchInputsAreComparedByContent drives the rule the
// review layer stands on through the real ops: a group is judged against the
// content of the dispatch inputs it was opened under, never their path. A
// repeated trace that hands out byte-identical inputs leaves every group as it
// was — a completed one valid, an open one completable. One that hands out
// different inputs, here because the suite's own result changed at the same
// commit, invalidates the completed groups (resume: inputs-changed) and refuses
// to close an open one (dispatch-inputs-changed). The trace itself stays valid
// throughout: what changed is what it hands out, not whether it stands.
func TestRecordReview_DispatchInputsAreComparedByContent(t *testing.T) {
	repo := fixtureRepoWith(t, gatedFixtureEdit(t))
	const runID = "review-dispatch-content"

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	first := decodeTraceOutput(t, stdout)
	dispatch := readDispatchInputs(t, first.DispatchPath)
	if got := dispatchRowFor(t, dispatch, "SC-01").Status; got == "gap" {
		t.Fatalf("SC-01 status = %q with the gate unset, want the bound test run", got)
	}
	firstDigest, err := evidence.SHA256File(first.DispatchPath)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}

	startReviewGroup(t, repo, runID, "A", "SC-01", "SC-02")
	completeReviewGroup(t, repo, runID, "A", dispatch, "SC-01", "SC-02")
	startReviewGroup(t, repo, runID, "B", "SC-03")
	startReviewGroup(t, repo, runID, "C", "SC-09")

	t.Run("byte-identical repeated inputs keep every group as it was", func(t *testing.T) {
		repeated := repeatTrace(t, repo, runID, "trace repeated with nothing changed")
		if repeated.DispatchPath == first.DispatchPath {
			t.Fatalf("repeated trace reused %q, want a document of its own so the comparison is of content", repeated.DispatchPath)
		}
		digest, err := evidence.SHA256File(repeated.DispatchPath)
		if err != nil {
			t.Fatalf("SHA256File: %v", err)
		}
		if digest != firstDigest {
			t.Fatalf("repeated dispatch inputs digest %s != %s, want the unchanged commit to hand out identical inputs", digest, firstDigest)
		}

		out := runResume(t, repo, runID)
		if out.Trace.Status != string(runstate.StatusCompleted) || out.Trace.Reason != "" {
			t.Errorf("trace = %+v, want the repeated phase completed and valid", out.Trace)
		}
		if group := resumeGroupFor(t, out, "A"); group.Status != string(runstate.StatusCompleted) || group.Reason != "" {
			t.Errorf("group A = %+v, want it still a valid completion", group)
		}
		for _, id := range []string{"B", "C"} {
			if group := resumeGroupFor(t, out, id); group.Status != string(runstate.StatusStarted) || group.Reason != "" {
				t.Errorf("group %s = %+v, want it still an open group with nothing against it", id, group)
			}
		}
		if closed := completeReviewGroup(t, repo, runID, "C", dispatch, "SC-09"); closed.Status != reviewStatusCompleted || closed.Outcome != "holds" {
			t.Errorf("completing C after the identical repeat = %+v, want it accepted and holds", closed)
		}
	})

	t.Run("changed inputs invalidate completed groups and refuse to close open ones", func(t *testing.T) {
		t.Setenv(fixtureGateVar, "1")
		gated := repeatTrace(t, repo, runID, "SC-01's test gated off")
		changed := readDispatchInputs(t, gated.DispatchPath)
		if got := dispatchRowFor(t, changed, "SC-01").Status; got != "gap" {
			t.Fatalf("SC-01 status = %q under the gate, want gap: the suite did not see the gate", got)
		}
		digest, err := evidence.SHA256File(gated.DispatchPath)
		if err != nil {
			t.Fatalf("SHA256File: %v", err)
		}
		if digest == firstDigest {
			t.Fatalf("gated dispatch inputs digest %s equals the first, want different content", digest)
		}

		got := refuseRecordReview(t, repo, runID, map[string]any{
			"groupId": "B", "status": "completed",
			"verdicts": []map[string]string{{"rowId": "SC-03", "verdict": "HOLDS"}},
		})
		if got.Code != "dispatch-inputs-changed" {
			t.Errorf("completing B = %q, want dispatch-inputs-changed (message %q)", got.Code, got.Message)
		}

		out := runResume(t, repo, runID)
		if out.Trace.Status != string(runstate.StatusCompleted) || out.Trace.Reason != "" {
			t.Errorf("trace = %+v, want the gated phase completed and valid: the inputs changed, not the trace's standing", out.Trace)
		}
		for _, id := range []string{"A", "C"} {
			group := resumeGroupFor(t, out, id)
			if group.Status != statusInvalid || group.Reason != reasonInputsChanged {
				t.Errorf("group %s = %+v, want status %s reason %s", id, group, statusInvalid, reasonInputsChanged)
			}
			owed := resumePending{Kind: runstate.KindReviewGroup, ID: id, Reason: reasonInputsChanged}
			if !slices.Contains(out.Pending, owed) {
				t.Errorf("pending = %+v, want %+v offered again", out.Pending, owed)
			}
		}
		if group := resumeGroupFor(t, out, "B"); group.Status != string(runstate.StatusStarted) || group.Reason != reasonInputsChanged {
			t.Errorf("group B = %+v, want it open with reason %s", group, reasonInputsChanged)
		}
		if out.Counts.Invalid != 2 {
			t.Errorf("counts.invalid = %d, want 2", out.Counts.Invalid)
		}
		if out.Review.Complete {
			t.Error("review.complete = true, want false over groups judged against inputs the run no longer hands out")
		}
		if !strings.Contains(strings.Join(out.Review.Uncovered, ","), "SC-06") {
			t.Errorf("review.uncovered = %v, want the never-grouped rows still listed", out.Review.Uncovered)
		}
	})
}

// startReviewGroupWithPolicy opens groupID over rows through the real
// record-review op under the critic policy the parent names.
func startReviewGroupWithPolicy(t *testing.T, repo, runID, groupID string, policy map[string]any, rows ...string) recordReviewOutput {
	t.Helper()
	return runRecordReview(t, repo, runID, map[string]any{
		"groupId": groupID, "status": "started", "rows": rows, "policy": policy,
	})
}

// fixturePolicy is a critic policy over one absolute instruction file, the
// shape a parent hands record-review.
func fixturePolicy(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"critic":       "coverage-critic",
		"instructions": []string{writePolicyFile(t, "critic.md", "judge the assertion, not the name\n")},
	}
}

// TestRecordReview_StartedCapturesTheBundleAndTheManifest: opening a group
// tells the parent exactly which whole files its critic is being handed and
// which rows each of them serves, and records that same input identity in the
// ledger. The manifest is captured from the files on disk — the parent supplies
// only the policy paths — and is incomplete until the critic's reads are known.
func TestRecordReview_StartedCapturesTheBundleAndTheManifest(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "review-manifest-started"
	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	payload := readDispatchInputs(t, decodeTraceOutput(t, stdout).DispatchPath)
	rows := []string{"SC-01", "SC-02"}

	started := startReviewGroupWithPolicy(t, repo, runID, "g1", fixturePolicy(t), rows...)

	// One entry per file the group's rows were handed, sorted by path, each
	// naming every row it serves.
	wantRows := map[string][]string{}
	for _, id := range rows {
		row := dispatchRowFor(t, payload, id)
		for _, file := range append([]string{row.File}, contributorFiles(row)...) {
			if file == "" || slices.Contains(wantRows[file], id) {
				continue
			}
			wantRows[file] = append(wantRows[file], id)
		}
	}
	if len(started.Bundle) != len(wantRows) {
		t.Fatalf("bundle = %+v, want one entry per file the rows were handed (%v)", started.Bundle, wantRows)
	}
	var gotPaths []string
	for _, file := range started.Bundle {
		gotPaths = append(gotPaths, file.Path)
		want, err := evidence.SHA256File(filepath.Join(repo, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatalf("SHA256File %s: %v", file.Path, err)
		}
		if file.SHA256 != want {
			t.Errorf("bundle[%s].sha256 = %q, want the whole file's digest %q", file.Path, file.SHA256, want)
		}
		if !reflect.DeepEqual(file.Rows, wantRows[file.Path]) {
			t.Errorf("bundle[%s].rows = %v, want %v", file.Path, file.Rows, wantRows[file.Path])
		}
	}
	if !slices.IsSorted(gotPaths) {
		t.Errorf("bundle paths = %v, want them sorted", gotPaths)
	}
	if started.ManifestComplete {
		t.Error("manifestComplete = true, want false while the critic's reads are unrecorded")
	}
	if want := []string{"reads-unrecorded"}; !reflect.DeepEqual(started.ManifestIncomplete, want) {
		t.Errorf("manifestIncomplete = %v, want %v", started.ManifestIncomplete, want)
	}

	rec := ledgerRecord(t, runResume(t, repo, runID), runstate.KindReviewGroup, "g1")
	var recorded reviewGroupInputs
	if err := json.Unmarshal(rec.Inputs, &recorded); err != nil {
		t.Fatalf("record inputs did not decode: %v\ninputs=%s", err, rec.Inputs)
	}
	if recorded.Manifest == nil {
		t.Fatalf("record inputs = %s, want them to carry the group's manifest", rec.Inputs)
	}
	if recorded.Manifest.Version != evidence.ManifestVersion {
		t.Errorf("manifest.version = %q, want %q", recorded.Manifest.Version, evidence.ManifestVersion)
	}
	wantClaims := map[string]string{}
	for _, id := range rows {
		wantClaims[id] = dispatchRowFor(t, payload, id).ClaimSHA256
	}
	if !reflect.DeepEqual(recorded.Manifest.Claims, wantClaims) {
		t.Errorf("manifest.claims = %v, want the dispatch document's claim digests %v", recorded.Manifest.Claims, wantClaims)
	}
	if recorded.Manifest.Policy == nil || recorded.Manifest.Policy.Critic != "coverage-critic" {
		t.Errorf("manifest.policy = %+v, want the critic the parent named", recorded.Manifest.Policy)
	}
	if len(recorded.Manifest.Bundle) != len(wantRows) {
		t.Errorf("manifest.bundle = %v, want a digest per file the rows were handed (%v)", recorded.Manifest.Bundle, wantRows)
	}
}

// TestRecordReview_CompletedRecordsTheCriticsReads: what the critic read
// beyond its bundle is a claim the parent relays at completion, and the
// artifact is the authoritative complete manifest. An explicitly empty list
// completes the manifest; an absent one leaves it incomplete, which costs this
// run nothing and only makes the group ineligible for reuse later.
func TestRecordReview_CompletedRecordsTheCriticsReads(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "review-manifest-reads"
	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	payload := readDispatchInputs(t, decodeTraceOutput(t, stdout).DispatchPath)
	policy := fixturePolicy(t)

	completeWith := func(t *testing.T, groupID string, extra map[string]any, rows ...string) (recordReviewOutput, evidence.Manifest) {
		t.Helper()
		startReviewGroupWithPolicy(t, repo, runID, groupID, policy, rows...)
		request := map[string]any{
			"groupId": groupID, "status": "completed", "verdicts": soundVerdicts(t, payload, rows...),
		}
		for name, value := range extra {
			request[name] = value
		}
		out := runRecordReview(t, repo, runID, request)
		data, err := os.ReadFile(out.EvidencePath)
		if err != nil {
			t.Fatalf("read review evidence: %v", err)
		}
		var doc reviewEvidence
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("review evidence did not decode: %v\ndata=%s", err, data)
		}
		if doc.Inputs.Manifest == nil {
			t.Fatalf("evidence inputs = %+v, want them to carry the completed manifest", doc.Inputs)
		}
		return out, *doc.Inputs.Manifest
	}

	t.Run("the critic read nothing beyond its bundle", func(t *testing.T) {
		out, m := completeWith(t, "A", map[string]any{"reads": []string{}}, "SC-01", "SC-02")
		if !m.Complete || len(m.Incomplete) != 0 {
			t.Fatalf("manifest = complete %v incomplete %v, want a complete manifest", m.Complete, m.Incomplete)
		}
		if m.Reads == nil || len(m.Reads) != 0 {
			t.Errorf("manifest.reads = %v, want an empty record of reads rather than none", m.Reads)
		}
		if !out.ManifestComplete || len(out.ManifestIncomplete) != 0 {
			t.Errorf("output = complete %v incomplete %v, want the manifest it produced reported complete", out.ManifestComplete, out.ManifestIncomplete)
		}
	})

	t.Run("the critic read a tracked file", func(t *testing.T) {
		out, m := completeWith(t, "B", map[string]any{"reads": []string{"orders/orders.go"}}, "SC-05")
		if !m.Complete {
			t.Fatalf("manifest = incomplete %v, want a complete manifest", m.Incomplete)
		}
		want, err := evidence.SHA256File(filepath.Join(repo, "orders", "orders.go"))
		if err != nil {
			t.Fatalf("SHA256File: %v", err)
		}
		if got := m.Reads["orders/orders.go"]; got != want {
			t.Errorf("manifest.reads[orders/orders.go] = %q, want the file's digest %q", got, want)
		}
		if !out.ManifestComplete {
			t.Errorf("manifestComplete = false (%v), want true", out.ManifestIncomplete)
		}
	})

	t.Run("the parent recorded no reads at all", func(t *testing.T) {
		out, m := completeWith(t, "C", nil, "SC-04")
		if m.Complete {
			t.Errorf("manifest.complete = true, want false while the critic's reads are unrecorded")
		}
		if want := []string{"reads-unrecorded"}; !reflect.DeepEqual(m.Incomplete, want) {
			t.Errorf("manifest.incomplete = %v, want %v", m.Incomplete, want)
		}
		if out.ManifestComplete || !reflect.DeepEqual(out.ManifestIncomplete, []string{"reads-unrecorded"}) {
			t.Errorf("output = complete %v incomplete %v, want the incomplete manifest reported", out.ManifestComplete, out.ManifestIncomplete)
		}
		if out.Outcome == "" {
			t.Error("outcome = \"\", want the completion recorded as before: the manifest never refuses a review")
		}
	})
}
