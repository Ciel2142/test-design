package report

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// covers: SC-34
func TestLayer2_CompleteReviewFinalizesWithoutCanaryBookkeeping(t *testing.T) {
	rep := mustCompute(t, greenInputs())

	wantVerdict(t, rep, "PASS")
	if rep.Layers.Layer2 != (Layer2Summary{Status: "pass", Applicable: 3, Holds: 3}) {
		t.Errorf("Layer2 summary = %+v, want {pass 3 3 0 0 0}", rep.Layers.Layer2)
	}
	if got := rowOf(t, rep, "SC-01").Layer2; got != (CriticRef{Verdict: "HOLDS", BatchID: "batch-1"}) {
		t.Errorf("SC-01 Layer2 = %+v, want {HOLDS batch-1}", got)
	}

	wantBatches := []BatchReport{
		{
			BatchID:  "batch-1",
			RunID:    "run-1",
			Rows:     []string{"SC-01"},
			Verdicts: []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
		},
		{
			BatchID: "batch-2",
			RunID:   "run-1",
			Rows:    []string{"SC-02", "SC-03"},
			Verdicts: []CriticVerdict{
				{RowID: "SC-02", Verdict: "HOLDS"},
				{RowID: "SC-03", Verdict: "HOLDS"},
			},
		},
	}
	if !reflect.DeepEqual(rep.Batches, wantBatches) {
		t.Errorf("Batches = %+v, want %+v", rep.Batches, wantBatches)
	}

	// An old-shape document (canary, inheritsFrom, voided,
	// canaryUnavailableReason) must still decode, with the retired fields
	// silently ignored.
	const oldShapeJSON = `{"batchId":"b","runId":"run-1","rows":["SC-01"],"canary":{"rowId":"SC-01","kind":"synthesized","weakening":"x","caught":true},"inheritsFrom":"c","voided":true,"canaryUnavailableReason":"r","verdicts":[{"rowId":"SC-01","verdict":"HOLDS"}]}`
	var decoded Batch
	if err := json.Unmarshal([]byte(oldShapeJSON), &decoded); err != nil {
		t.Fatalf("json.Unmarshal old-shape batch: %v", err)
	}
	wantDecoded := Batch{BatchID: "b", RunID: "run-1", Rows: []string{"SC-01"}, Verdicts: []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}}}
	if !reflect.DeepEqual(decoded, wantDecoded) {
		t.Errorf("decoded old-shape batch = %+v, want %+v (retired fields ignored)", decoded, wantDecoded)
	}

	md := mustRender(t, rep)
	if !strings.Contains(md, "| batch-1 | run-1 | SC-01 | - |") {
		t.Errorf("markdown missing canary-free batch-1 row; got:\n%s", md)
	}
	if strings.Contains(md, "Canary") {
		t.Errorf("markdown should carry no Canary token; got:\n%s", md)
	}
}

// covers: SC-35
func TestLayer4_HighRiskObligationSurvivesCanaryRemoval(t *testing.T) {
	active := ActiveRows(greenSource())
	if got := layer4Applicable(active, nil); len(got) != 1 || got[0].ID != "SC-02" {
		t.Fatalf("layer4Applicable(active, nil) = %+v, want exactly the high-risk row SC-02", got)
	}

	t.Run("missing", func(t *testing.T) {
		in := greenInputs()
		in.Mutations = nil

		_, err := Compute(in)
		var re *RefusalError
		if !errors.As(err, &re) {
			t.Fatalf("Compute error = %v (%T), want *RefusalError", err, err)
		}
		if re.Code != CodeMissingVerdict {
			t.Fatalf("refusal code = %q, want %q (detail: %s)", re.Code, CodeMissingVerdict, re.Detail)
		}
		if !strings.Contains(re.Detail, "SC-02") {
			t.Errorf("refusal detail = %q, want it to name SC-02", re.Detail)
		}
	})

	t.Run("survived", func(t *testing.T) {
		in := greenInputs()
		in.Mutations[0].Evidence.Sites[1].Outcome = "survived"

		r := mustCompute(t, in)
		wantVerdict(t, r, "BLOCKED")
		wantReason(t, r, "4", "SC-02", "MUTATION-SURVIVED")
		if r.Layers.Layer4.Survived != 1 {
			t.Errorf("Layer4 summary = %+v, want 1 survived", r.Layers.Layer4)
		}
	})

	t.Run("caught", func(t *testing.T) {
		r := mustCompute(t, greenInputs())
		wantVerdict(t, r, "PASS")
		if got := rowOf(t, r, "SC-02").Layer4.Status; got != "MUTATION-CAUGHT" {
			t.Errorf("SC-02 Layer4.Status = %q, want MUTATION-CAUGHT", got)
		}
	})
}
