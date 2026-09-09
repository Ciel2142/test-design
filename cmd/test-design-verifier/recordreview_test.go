package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"test-design/internal/evidence"
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
		if filepath.Base(completed.EvidencePath) != "review-g1.json" {
			t.Errorf("evidencePath = %q, want it to name review-g1.json", completed.EvidencePath)
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
		wantSources := map[string][]string{}
		for _, id := range doc.Rows {
			row := dispatchRowFor(t, payload, id)
			hashes := []string{}
			if row.TestSourceSHA256 != "" {
				hashes = append(hashes, row.TestSourceSHA256)
			}
			for _, c := range row.Contributors {
				hashes = append(hashes, c.TestSourceSHA256)
			}
			wantSources[id] = hashes
		}
		if !reflect.DeepEqual(doc.Inputs.Sources, wantSources) {
			t.Errorf("inputs.sources = %v, want the dispatch inputs' hashes %v", doc.Inputs.Sources, wantSources)
		}
		if got := doc.Inputs.Sources["SC-01"]; len(got) == 0 || got[0] != dispatchRowFor(t, payload, "SC-01").TestSourceSHA256 {
			t.Errorf("inputs.sources[SC-01] = %v, want the primary source first", got)
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
