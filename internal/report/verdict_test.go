package report

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

const greenSourcePath = "docs/superpowers/coverage/thing-coverage.md"

func greenSource() *coverage.Source {
	return &coverage.Source{
		Path:           greenSourcePath,
		SHA256:         "sha-source-1",
		BaselineCommit: "base-1",
		Rows: []coverage.Row{
			{ID: "SC-01", Level: "unit", Risk: "low"},
			{ID: "SC-02", Level: "smoke", Risk: "high"},
			{ID: "SC-03", Level: "e2e", Risk: "med"},
			{ID: "SC-04", Level: "unit", Risk: "low"},
		},
		Retired: []string{"SC-04"},
	}
}

func greenTrace() *TraceEvidence {
	return &TraceEvidence{
		RunID:          "run-1",
		Source:         greenSourcePath,
		SourceSHA256:   "sha-source-1",
		RepoCommit:     "head-1",
		BaselineCommit: "base-1",
		BaselineStatus: "ok",
		ExitStatus:     0,
		Counts:         SuiteCounts{Passed: 3},
		RowAccounting:  RowAccounting{ActiveRows: 3, WithResult: 3},
		FindingCounts:  map[string]int{},
		RowResults: []TraceRowResult{
			{RowID: "SC-01", Scope: "pkg/a", Name: "TestA", Status: "passed"},
			{RowID: "SC-02", Scope: "pkg/b", Name: "TestB", Status: "passed"},
			{RowID: "SC-03", Scope: "pkg/c", Name: "TestC", Status: "passed"},
		},
	}
}

func greenBatches() []Batch {
	return []Batch{
		{
			BatchID:    "cb1",
			DispatchID: "d1",
			RunID:      "run-1",
			Rows:       []string{"SC-01"},
			Canary:     &Canary{RowID: "SC-02", Kind: "natural", Caught: true},
			Verdicts:   []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
		},
		{
			BatchID:      "b1",
			DispatchID:   "d2",
			RunID:        "run-1",
			Rows:         []string{"SC-02", "SC-03"},
			InheritsFrom: "cb1",
			Verdicts: []CriticVerdict{
				{RowID: "SC-02", Verdict: "HOLDS"},
				{RowID: "SC-03", Verdict: "HOLDS"},
			},
		},
	}
}

func greenLive() *LiveEvidence {
	return &LiveEvidence{
		RunID:        "run-1",
		Source:       greenSourcePath,
		SourceSHA256: "sha-source-1",
		RowVerdicts: []LiveRowVerdict{
			{RowID: "SC-02", Verdict: "LIVE-PASS"},
			{RowID: "SC-03", Verdict: "LIVE-PASS"},
		},
		CleanupOK:     true,
		SnapshotEqual: true,
	}
}

func greenMutations() []MutationInput {
	return []MutationInput{{
		RowID:        "SC-02",
		EvidencePath: "run/mutation-SC-02.json",
		Evidence: &MutationEvidence{
			RunID: "run-1",
			RowID: "SC-02",
			Sites: []MutationSiteOutcome{
				{Site: "pkg/b.go:12", Outcome: "caught"},
				{Site: "pkg/b.go:20", Outcome: "caught"},
			},
			SnapshotEqual: true,
		},
	}}
}

func greenInputs() *Inputs {
	return &Inputs{
		Source:     greenSource(),
		RunID:      "run-1",
		HeadCommit: "head-1",
		Trace:      greenTrace(),
		Freshness:  Freshness{Result: "fresh"},
		Batches:    greenBatches(),
		Live:       greenLive(),
		Mutations:  greenMutations(),
	}
}

func mustCompute(t *testing.T, in *Inputs) *Report {
	t.Helper()
	r, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return r
}

func rowOf(t *testing.T, r *Report, rowID string) RowReport {
	t.Helper()
	for _, row := range r.Rows {
		if row.RowID == rowID {
			return row
		}
	}
	t.Fatalf("report has no row %s", rowID)
	return RowReport{}
}

type reasonKey struct{ layer, rowID, code string }

func reasonKeys(r *Report) []reasonKey {
	keys := make([]reasonKey, 0, len(r.Reasons))
	for _, rn := range r.Reasons {
		keys = append(keys, reasonKey{rn.Layer, rn.RowID, rn.Code})
	}
	return keys
}

func wantReason(t *testing.T, r *Report, layer, rowID, code string) Reason {
	t.Helper()
	for _, rn := range r.Reasons {
		if rn.Layer == layer && rn.RowID == rowID && rn.Code == code {
			return rn
		}
	}
	t.Fatalf("report has no reason {layer:%s row:%s code:%s}; got %+v", layer, rowID, code, reasonKeys(r))
	return Reason{}
}

func wantNoReasonCode(t *testing.T, r *Report, code string) {
	t.Helper()
	for _, rn := range r.Reasons {
		if rn.Code == code {
			t.Fatalf("report carries unwanted reason code %q (%+v)", code, rn)
		}
	}
}

func wantVerdict(t *testing.T, r *Report, verdict string) {
	t.Helper()
	if r.Verdict != verdict {
		t.Fatalf("Verdict = %q, want %q (reasons: %+v)", r.Verdict, verdict, reasonKeys(r))
	}
}

func wantComputeRefusal(t *testing.T, in *Inputs, code string) {
	t.Helper()
	r, err := Compute(in)
	if r != nil {
		t.Errorf("Compute returned a report alongside the refusal")
	}
	var re *RefusalError
	if !errors.As(err, &re) {
		t.Fatalf("Compute returned %v (%T), want *RefusalError", err, err)
	}
	if re.Code != code {
		t.Fatalf("refusal code = %q, want %q (detail: %s)", re.Code, code, re.Detail)
	}
}

func TestComputeAllGreenIsPass(t *testing.T) {
	r := mustCompute(t, greenInputs())

	wantVerdict(t, r, "PASS")
	if len(r.Reasons) != 0 {
		t.Errorf("Reasons = %+v, want empty on PASS", r.Reasons)
	}
	if r.Reasons == nil {
		t.Errorf("Reasons is nil; want an empty slice so the canonical JSON renders []")
	}
	if r.Acknowledged {
		t.Errorf("Acknowledged = true on PASS")
	}

	if r.SchemaVersion != evidence.SchemaVersion || r.VerifierVersion != evidence.VerifierVersion {
		t.Errorf("envelope = %+v, want the current schema/verifier versions", r.Envelope)
	}
	if r.RunID != "run-1" || r.Source != greenSourcePath || r.SourceSHA256 != "sha-source-1" {
		t.Errorf("identity = %q/%q/%q, want run-1/%s/sha-source-1", r.RunID, r.Source, r.SourceSHA256, greenSourcePath)
	}
	if r.HeadCommit != "head-1" || r.BaselineCommit != "base-1" || r.BaselineStatus != "ok" {
		t.Errorf("commits = %q/%q/%q, want head-1/base-1/ok", r.HeadCommit, r.BaselineCommit, r.BaselineStatus)
	}
	if r.Approval != "" || r.FinishedAt != "" || len(r.Transcripts) != 0 || len(r.ToolVersions) != 0 {
		t.Errorf("Compute stamped an op-owned field: approval=%q finishedAt=%q transcripts=%v toolVersions=%v",
			r.Approval, r.FinishedAt, r.Transcripts, r.ToolVersions)
	}

	wantLayerStatus(t, r, "pass", "pass", "pass", "pass")

	if got := []string{r.Rows[0].RowID, r.Rows[1].RowID, r.Rows[2].RowID}; !reflect.DeepEqual(got, []string{"SC-01", "SC-02", "SC-03"}) {
		t.Fatalf("Rows = %v, want active rows in source order", got)
	}

	sc01 := rowOf(t, r, "SC-01")
	if sc01.Layer1 != (LayerVerdict{Status: "bound", Scope: "pkg/a", Name: "TestA"}) {
		t.Errorf("SC-01 Layer1 = %+v, want bound with scope/name", sc01.Layer1)
	}
	if sc01.Layer2 != (CriticRef{Verdict: "HOLDS", BatchID: "cb1", DispatchID: "d1"}) {
		t.Errorf("SC-01 Layer2 = %+v, want HOLDS from cb1/d1", sc01.Layer2)
	}
	if sc01.Layer3 != "N/A" {
		t.Errorf("SC-01 Layer3 = %q, want N/A (unit row)", sc01.Layer3)
	}
	if sc01.Layer4.Status != "N/A" {
		t.Errorf("SC-01 Layer4 = %+v, want N/A (low risk)", sc01.Layer4)
	}
	if sc01.Level != "unit" || sc01.Risk != "low" {
		t.Errorf("SC-01 level/risk = %q/%q, want unit/low", sc01.Level, sc01.Risk)
	}

	sc02 := rowOf(t, r, "SC-02")
	if sc02.Layer3 != "LIVE-PASS" {
		t.Errorf("SC-02 Layer3 = %q, want LIVE-PASS", sc02.Layer3)
	}
	if sc02.Layer4.Status != "MUTATION-CAUGHT" || len(sc02.Layer4.Sites) != 0 {
		t.Errorf("SC-02 Layer4 = %+v, want MUTATION-CAUGHT with no named sites", sc02.Layer4)
	}
	if sc02.Layer2 != (CriticRef{Verdict: "HOLDS", BatchID: "b1", DispatchID: "d2"}) {
		t.Errorf("SC-02 Layer2 = %+v, want HOLDS from b1/d2", sc02.Layer2)
	}

	sc03 := rowOf(t, r, "SC-03")
	if sc03.Layer3 != "LIVE-PASS" {
		t.Errorf("SC-03 Layer3 = %q, want LIVE-PASS", sc03.Layer3)
	}
	if sc03.Layer4.Status != "N/A" {
		t.Errorf("SC-03 Layer4 = %+v, want N/A (risk med)", sc03.Layer4)
	}

	if r.Layers.Layer1.Suite != (SuiteCounts{Passed: 3}) || r.Layers.Layer1.NewFailures != 0 {
		t.Errorf("Layer1 summary = %+v, want the trace suite counts and no new failures", r.Layers.Layer1)
	}
	if r.Layers.Layer2 != (Layer2Summary{Status: "pass", Applicable: 3, Holds: 3}) {
		t.Errorf("Layer2 summary = %+v, want 3 applicable / 3 holds", r.Layers.Layer2)
	}
	if r.Layers.Layer3 != (Layer3Summary{Status: "pass", Applicable: 2, Pass: 2}) {
		t.Errorf("Layer3 summary = %+v, want 2 applicable / 2 pass", r.Layers.Layer3)
	}
	if r.Layers.Layer4 != (Layer4Summary{Status: "pass", Applicable: 1, Caught: 1}) {
		t.Errorf("Layer4 summary = %+v, want 1 applicable / 1 caught", r.Layers.Layer4)
	}

	if len(r.Batches) != 2 || r.Batches[0].BatchID != "cb1" || r.Batches[0].Canary == nil {
		t.Errorf("Batches = %+v, want both dispatches with the canary preserved", r.Batches)
	}
}

func wantLayerStatus(t *testing.T, r *Report, l1, l2, l3, l4 string) {
	t.Helper()
	got := []string{r.Layers.Layer1.Status, r.Layers.Layer2.Status, r.Layers.Layer3.Status, r.Layers.Layer4.Status}
	want := []string{l1, l2, l3, l4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("layer statuses = %v, want %v", got, want)
	}
}

func gapInputs() *Inputs {
	in := greenInputs()
	in.Trace.RowResults = []TraceRowResult{
		{RowID: "SC-01", Scope: "pkg/a", Name: "TestA", Status: "passed"},
		{RowID: "SC-03", Scope: "pkg/c", Name: "TestC", Status: "passed"},
	}
	in.Trace.Counts = SuiteCounts{Passed: 2}
	in.Trace.RowAccounting = RowAccounting{ActiveRows: 3, WithResult: 2}
	in.Trace.Findings = []TraceFinding{{Kind: "gap", RowID: "SC-02", Detail: "no test cites SC-02"}}
	in.Trace.FindingCounts = map[string]int{"gap": 1}
	in.Batches[1].Verdicts[0].Verdict = "MISSING"
	in.Live.RowVerdicts[0].Verdict = "LIVE-N/A (layer 1 gap)"
	in.Mutations = nil
	return in
}

func TestComputeGapRowIsBlocked(t *testing.T) {
	r := mustCompute(t, gapInputs())

	wantVerdict(t, r, "BLOCKED")

	sc02 := rowOf(t, r, "SC-02")
	if sc02.Layer1 != (LayerVerdict{Status: "gap"}) {
		t.Errorf("SC-02 Layer1 = %+v, want gap", sc02.Layer1)
	}
	if sc02.Layer2.Verdict != "MISSING" {
		t.Errorf("SC-02 Layer2 = %+v, want MISSING", sc02.Layer2)
	}
	if sc02.Layer3 != "LIVE-N/A (layer 1 gap)" {
		t.Errorf("SC-02 Layer3 = %q, want LIVE-N/A (layer 1 gap)", sc02.Layer3)
	}
	if sc02.Layer4.Status != "MUTATION-N/A (layer 1 gap)" {
		t.Errorf("SC-02 Layer4 = %+v, want MUTATION-N/A (layer 1 gap)", sc02.Layer4)
	}

	gap := wantReason(t, r, "1", "SC-02", "gap")
	if !strings.Contains(gap.Detail, "SC-02") {
		t.Errorf("gap reason detail = %q, want it to carry the trace detail", gap.Detail)
	}
	wantReason(t, r, "2", "SC-02", "MISSING")

	wantLayerStatus(t, r, "blocked", "blocked", "pass", "n/a")
	if r.Layers.Layer3.NA != 1 || r.Layers.Layer3.Pass != 1 {
		t.Errorf("Layer3 summary = %+v, want 1 pass and 1 n/a", r.Layers.Layer3)
	}
	if r.Layers.Layer4.NA != 1 {
		t.Errorf("Layer4 summary = %+v, want the gap row counted as n/a", r.Layers.Layer4)
	}
}

func TestComputeSurvivedSiteIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Mutations[0].Evidence.Sites[1].Outcome = "survived"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	sc02 := rowOf(t, r, "SC-02")
	if sc02.Layer4.Status != "MUTATION-SURVIVED" || !reflect.DeepEqual(sc02.Layer4.Sites, []string{"pkg/b.go:20"}) {
		t.Errorf("SC-02 Layer4 = %+v, want MUTATION-SURVIVED naming pkg/b.go:20", sc02.Layer4)
	}
	rn := wantReason(t, r, "4", "SC-02", "MUTATION-SURVIVED")
	if !strings.Contains(rn.Detail, "pkg/b.go:20") {
		t.Errorf("reason detail = %q, want it to name the surviving site", rn.Detail)
	}
	if strings.Contains(rn.Detail, "pkg/b.go:12") {
		t.Errorf("reason detail = %q, want it to name only the surviving site", rn.Detail)
	}
	wantLayerStatus(t, r, "pass", "pass", "pass", "blocked")
	if r.Layers.Layer4.Survived != 1 || r.Layers.Layer4.Caught != 0 {
		t.Errorf("Layer4 summary = %+v, want 1 survived row", r.Layers.Layer4)
	}
}

func TestComputeMutationUnappliableIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Mutations = []MutationInput{{RowID: "SC-02", Outcome: "unappliable", Reason: "no site matched"}}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	if got := rowOf(t, r, "SC-02").Layer4.Status; got != "MUTATION-UNAPPLIABLE" {
		t.Errorf("SC-02 Layer4 = %q, want MUTATION-UNAPPLIABLE", got)
	}
	wantReason(t, r, "4", "SC-02", "MUTATION-UNAPPLIABLE")
	if r.Layers.Layer4.Unappliable != 1 {
		t.Errorf("Layer4 summary = %+v, want 1 unappliable", r.Layers.Layer4)
	}
}

func TestComputeMutationEnvironmentalIsPartial(t *testing.T) {
	in := greenInputs()
	in.Mutations = []MutationInput{{
		RowID:               "SC-02",
		Outcome:             "environmental",
		Reason:              "docker unavailable",
		AttemptEvidencePath: "run/mutation-attempt-SC-02.json",
	}}

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	if got := rowOf(t, r, "SC-02").Layer4.Status; got != "MUTATION-N/A (environmental)" {
		t.Errorf("SC-02 Layer4 = %q, want MUTATION-N/A (environmental)", got)
	}
	rn := wantReason(t, r, "4", "SC-02", "mutation-environmental")
	if !strings.Contains(rn.Detail, "docker unavailable") {
		t.Errorf("reason detail = %q, want it to carry the entry reason", rn.Detail)
	}
	wantLayerStatus(t, r, "pass", "pass", "pass", "partial")
}

func TestComputeMutationSnapshotChangedIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Mutations[0].Evidence.SnapshotEqual = false

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	wantReason(t, r, "4", "SC-02", "snapshot-changed")
	wantLayerStatus(t, r, "pass", "pass", "pass", "blocked")
}

func TestComputeBaselineMissingIsPartialUnacknowledged(t *testing.T) {
	in := greenInputs()
	in.Trace.BaselineStatus = "missing"

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	wantReason(t, r, "1", "", "baseline-missing")
	if r.Acknowledged {
		t.Errorf("Acknowledged = true with no acknowledgement line")
	}
	if r.Acknowledgement != "" {
		t.Errorf("Acknowledgement = %q, want empty", r.Acknowledgement)
	}
	wantLayerStatus(t, r, "partial", "pass", "pass", "pass")
}

func TestComputeBaselineMissingIsPartialAcknowledged(t *testing.T) {
	const line = "PARTIAL accepted by igi — no baseline on this branch"
	in := greenInputs()
	in.Trace.BaselineStatus = "missing"
	in.Acknowledgement = line

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	if !r.Acknowledged {
		t.Errorf("Acknowledged = false, want true on an acknowledged PARTIAL")
	}
	if r.Acknowledgement != line {
		t.Errorf("Acknowledgement = %q, want the input line verbatim", r.Acknowledgement)
	}
}

func TestComputeAcknowledgementDoesNotAcknowledgeNonPartial(t *testing.T) {
	in := greenInputs()
	in.Acknowledgement = "PARTIAL accepted by igi — reason"

	r := mustCompute(t, in)

	wantVerdict(t, r, "PASS")
	if r.Acknowledged {
		t.Errorf("Acknowledged = true on a PASS verdict")
	}
}

func TestComputeLiveProfileUnavailableIsPartial(t *testing.T) {
	in := greenInputs()
	in.Live = nil
	in.LiveAttempt = &LiveAttempt{RunID: "run-1", Profile: "profiles/live.json", Refusal: "missing-profile"}

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	rn := wantReason(t, r, "3", "", "live-profile-unavailable")
	if !strings.Contains(rn.Detail, "missing-profile") {
		t.Errorf("reason detail = %q, want it to carry the attempt refusal", rn.Detail)
	}
	if got := rowOf(t, r, "SC-02").Layer3; got != "N/A (profile unavailable)" {
		t.Errorf("SC-02 Layer3 = %q, want N/A (profile unavailable)", got)
	}
	if got := rowOf(t, r, "SC-03").Layer3; got != "N/A (profile unavailable)" {
		t.Errorf("SC-03 Layer3 = %q, want N/A (profile unavailable)", got)
	}
	if got := rowOf(t, r, "SC-01").Layer3; got != "N/A" {
		t.Errorf("SC-01 Layer3 = %q, want plain N/A for a non-applicable row", got)
	}
	wantLayerStatus(t, r, "pass", "pass", "partial", "pass")
	if r.Layers.Layer3 != (Layer3Summary{Status: "partial", Applicable: 2, NA: 2}) {
		t.Errorf("Layer3 summary = %+v, want 2 applicable / 2 n/a", r.Layers.Layer3)
	}
}

func TestComputeLiveCleanupFailedIsPartial(t *testing.T) {
	in := greenInputs()
	in.Live.CleanupFailed = true

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	wantReason(t, r, "3", "", "live-cleanup-failed")
	wantNoReasonCode(t, r, "live-cleanup-missing")
	wantLayerStatus(t, r, "pass", "pass", "partial", "pass")
}

func TestComputeLiveCleanupMissingIsPartial(t *testing.T) {
	in := greenInputs()
	in.Live.CleanupOK = false

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	wantReason(t, r, "3", "", "live-cleanup-missing")
	wantNoReasonCode(t, r, "live-cleanup-failed")
}

func TestComputeLiveExecutionErrorsArePartial(t *testing.T) {
	in := greenInputs()
	in.Live.ExecutionErrors = []string{"step 3 timed out"}

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	rn := wantReason(t, r, "3", "", "live-execution-errors")
	if !strings.Contains(rn.Detail, "step 3 timed out") {
		t.Errorf("reason detail = %q, want it to carry the execution error", rn.Detail)
	}
}

func TestComputeRedactionBarrierIsPartial(t *testing.T) {
	in := greenInputs()
	in.Live.RowVerdicts[1].RedactionBarrier = true

	r := mustCompute(t, in)

	wantVerdict(t, r, "PARTIAL")
	wantReason(t, r, "3", "SC-03", "redaction-barrier")
	wantLayerStatus(t, r, "pass", "pass", "partial", "pass")
}

func TestComputeLiveFailIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Live.RowVerdicts[0].Verdict = "LIVE-FAIL"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	wantReason(t, r, "3", "SC-02", "LIVE-FAIL")
	wantLayerStatus(t, r, "pass", "pass", "blocked", "pass")
	if r.Layers.Layer3.Fail != 1 || r.Layers.Layer3.Pass != 1 {
		t.Errorf("Layer3 summary = %+v, want 1 fail and 1 pass", r.Layers.Layer3)
	}
}

func TestComputeLiveInconclusiveIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Live.RowVerdicts[0].Verdict = "LIVE-INCONCLUSIVE"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	wantReason(t, r, "3", "SC-02", "LIVE-INCONCLUSIVE")
	if r.Layers.Layer3.Inconclusive != 1 {
		t.Errorf("Layer3 summary = %+v, want 1 inconclusive", r.Layers.Layer3)
	}
}

func TestComputeLiveSnapshotChangedIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Live.SnapshotEqual = false

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	wantReason(t, r, "3", "", "snapshot-changed")
}

func TestComputeLayer3NotApplicableIsNA(t *testing.T) {
	in := greenInputs()
	in.Source.Rows = []coverage.Row{{ID: "SC-01", Level: "unit", Risk: "low"}}
	in.Source.Retired = nil
	in.Trace.RowResults = []TraceRowResult{{RowID: "SC-01", Scope: "pkg/a", Name: "TestA", Status: "passed"}}
	in.Trace.Counts = SuiteCounts{Passed: 1}
	in.Trace.RowAccounting = RowAccounting{ActiveRows: 1, WithResult: 1}
	in.Batches = []Batch{{
		BatchID:    "cb1",
		DispatchID: "d1",
		RunID:      "run-1",
		Rows:       []string{"SC-01"},
		Canary:     &Canary{RowID: "SC-01", Kind: "synthesized", Weakening: "dropped the assertion", Caught: true},
	}, {
		BatchID:      "b1",
		DispatchID:   "d2",
		RunID:        "run-1",
		Rows:         []string{"SC-01"},
		InheritsFrom: "cb1",
		Verdicts:     []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
	}}
	in.Live = nil
	in.Mutations = nil

	r := mustCompute(t, in)

	wantVerdict(t, r, "PASS")
	wantLayerStatus(t, r, "pass", "pass", "n/a", "n/a")
	if got := rowOf(t, r, "SC-01").Layer3; got != "N/A" {
		t.Errorf("SC-01 Layer3 = %q, want N/A", got)
	}
}

func TestComputeWeakCriticVerdictIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Batches[1].Verdicts[1] = CriticVerdict{RowID: "SC-03", Verdict: "WEAK", Reason: "asserts only the status code"}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "2", "SC-03", "WEAK")
	if !strings.Contains(rn.Detail, "asserts only the status code") {
		t.Errorf("reason detail = %q, want the critic reason", rn.Detail)
	}
	wantLayerStatus(t, r, "pass", "blocked", "pass", "pass")
	if r.Layers.Layer2.Weak != 1 || r.Layers.Layer2.Holds != 2 {
		t.Errorf("Layer2 summary = %+v, want 1 weak and 2 holds", r.Layers.Layer2)
	}
}

func TestComputeVacuousCriticVerdictIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Batches[1].Verdicts[1].Verdict = "VACUOUS"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	wantReason(t, r, "2", "SC-03", "VACUOUS")
	if r.Layers.Layer2.Vacuous != 1 {
		t.Errorf("Layer2 summary = %+v, want 1 vacuous", r.Layers.Layer2)
	}
}

func TestComputeNewFailuresAreBlocked(t *testing.T) {
	in := greenInputs()
	in.Trace.NewFailures = []TraceNewFailure{{Scope: "pkg/d", Name: "TestD"}}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "1", "", "new-failures")
	if !strings.Contains(rn.Detail, "TestD") {
		t.Errorf("reason detail = %q, want it to name the new failure", rn.Detail)
	}
	if r.Layers.Layer1.NewFailures != 1 {
		t.Errorf("Layer1 summary = %+v, want 1 new failure", r.Layers.Layer1)
	}
	wantLayerStatus(t, r, "blocked", "pass", "pass", "pass")
}

func TestComputeStaleFreshnessIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Freshness = Freshness{Result: "stale", Stale: []StaleFinding{{Hunk: "spec.md:12", Behavior: "retry on 429"}}}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "freshness", "", "stale")
	if !strings.Contains(rn.Detail, "retry on 429") {
		t.Errorf("reason detail = %q, want it to name the stale behavior", rn.Detail)
	}
	wantLayerStatus(t, r, "pass", "pass", "pass", "pass")
}

func TestComputeRedRunnerIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Trace.ExitStatus = 2
	in.Trace.RowResults = nil
	in.Trace.Counts = SuiteCounts{}
	in.Trace.RowAccounting = RowAccounting{ActiveRows: 3, Unaccounted: []string{"SC-01", "SC-02", "SC-03"}}
	in.Batches[0].Verdicts[0].Verdict = "MISSING"
	in.Batches[1].Verdicts[0].Verdict = "MISSING"
	in.Batches[1].Verdicts[1].Verdict = "MISSING"
	in.Live.RowVerdicts[0].Verdict = "LIVE-N/A (layer 1 gap)"
	in.Live.RowVerdicts[1].Verdict = "LIVE-N/A (layer 1 gap)"
	in.Mutations = nil

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "1", "", "red-runner")
	if !strings.Contains(rn.Detail, "2") {
		t.Errorf("reason detail = %q, want it to carry the exit status", rn.Detail)
	}
	if got := rowOf(t, r, "SC-01").Layer1.Status; got != "gap" {
		t.Errorf("SC-01 Layer1 = %q, want gap for an unaccounted row", got)
	}
}

func TestComputeNonZeroExitWithResultsIsNotRedRunner(t *testing.T) {
	in := greenInputs()
	in.Trace.ExitStatus = 1

	r := mustCompute(t, in)

	wantVerdict(t, r, "PASS")
	wantNoReasonCode(t, r, "red-runner")
}

func TestComputeLayerLevelFindingBecomesLayerReason(t *testing.T) {
	in := greenInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "drift", Detail: "retired ID SC-04 at pkg/x_test.go:5"}}
	in.Trace.FindingCounts = map[string]int{"drift": 1}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "1", "", "drift")
	if !strings.Contains(rn.Detail, "SC-04") {
		t.Errorf("reason detail = %q, want the trace detail", rn.Detail)
	}
	if got := rowOf(t, r, "SC-01").Layer1.Status; got != "bound" {
		t.Errorf("SC-01 Layer1 = %q, want bound; a layer-level finding binds to no row", got)
	}
}

func TestComputeFailedRowResultWithoutFindingIsRed(t *testing.T) {
	in := greenInputs()
	in.Trace.RowResults[0].Status = "failed"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	if got := rowOf(t, r, "SC-01").Layer1.Status; got != "red" {
		t.Errorf("SC-01 Layer1 = %q, want red for a failed row result with no finding", got)
	}
	wantReason(t, r, "1", "SC-01", "red")
}

func TestComputeSkippedRowResultWithoutFindingIsGap(t *testing.T) {
	in := greenInputs()
	in.Trace.RowResults[0].Status = "skipped"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	if got := rowOf(t, r, "SC-01").Layer1.Status; got != "gap" {
		t.Errorf("SC-01 Layer1 = %q, want gap for a skipped row result with no finding", got)
	}
}

func TestComputeRefusesUncaughtCanaryNotVoided(t *testing.T) {
	in := greenInputs()
	in.Batches[0].Canary.Caught = false
	wantComputeRefusal(t, in, CodeUncaughtCanaryNotVoided)
}

func TestComputeRefusesVoidedVerdict(t *testing.T) {
	in := greenInputs()
	in.Batches[1].Voided = true
	wantComputeRefusal(t, in, CodeVoidedVerdict)
}

func TestComputeRefusesStaleBatchRunID(t *testing.T) {
	in := greenInputs()
	in.Batches[1].RunID = "run-0"
	wantComputeRefusal(t, in, CodeStaleEvidence)
}

func multiCauseInputs() *Inputs {
	in := greenInputs()
	in.Freshness = Freshness{Result: "stale", Stale: []StaleFinding{{Hunk: "spec.md:12", Behavior: "retry on 429"}}}
	in.Trace.BaselineStatus = "missing"
	in.Trace.NewFailures = []TraceNewFailure{{Scope: "pkg/d", Name: "TestD"}}
	in.Trace.RowResults = []TraceRowResult{
		{RowID: "SC-02", Scope: "pkg/b", Name: "TestB", Status: "passed"},
		{RowID: "SC-03", Scope: "pkg/c", Name: "TestC", Status: "passed"},
	}
	in.Trace.Findings = []TraceFinding{
		{Kind: "drift", Detail: "retired ID SC-04 at pkg/x_test.go:5"},
		{Kind: "gap", RowID: "SC-01", Detail: "no test cites SC-01"},
	}
	in.Trace.FindingCounts = map[string]int{"drift": 1, "gap": 1}
	in.Trace.RowAccounting = RowAccounting{ActiveRows: 3, WithResult: 2}
	in.Batches[0].Verdicts[0].Verdict = "MISSING"
	in.Batches[1].Verdicts[1] = CriticVerdict{RowID: "SC-03", Verdict: "WEAK", Reason: "shallow"}
	in.Live.RowVerdicts[0].Verdict = "LIVE-FAIL"
	in.Live.RowVerdicts[1].RedactionBarrier = true
	in.Live.CleanupFailed = true
	in.Live.ExecutionErrors = []string{"step 3 timed out"}
	in.Mutations[0].Evidence.Sites[1].Outcome = "survived"
	return in
}

func TestComputeReasonOrderFollowsLayerThenRowOrder(t *testing.T) {
	r := mustCompute(t, multiCauseInputs())

	wantVerdict(t, r, "BLOCKED")
	want := []reasonKey{
		{"freshness", "", "stale"},
		{"1", "", "drift"},
		{"1", "", "new-failures"},
		{"1", "", "baseline-missing"},
		{"1", "SC-01", "gap"},
		{"2", "SC-01", "MISSING"},
		{"2", "SC-03", "WEAK"},
		{"3", "", "live-cleanup-failed"},
		{"3", "", "live-execution-errors"},
		{"3", "SC-02", "LIVE-FAIL"},
		{"3", "SC-03", "redaction-barrier"},
		{"4", "SC-02", "MUTATION-SURVIVED"},
	}
	if got := reasonKeys(r); !reflect.DeepEqual(got, want) {
		t.Errorf("reason order =\n%+v\nwant\n%+v", got, want)
	}
}

func TestComputeIsDeterministic(t *testing.T) {
	first := mustCompute(t, multiCauseInputs())
	second := mustCompute(t, multiCauseInputs())

	if !reflect.DeepEqual(first, second) {
		t.Errorf("two Compute calls over equal inputs differ:\n%+v\n%+v", first, second)
	}
}
