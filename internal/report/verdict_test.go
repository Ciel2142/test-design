package report

import (
	"encoding/json"
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

// greenBatches models the all-green layer-2 shape: two canary-free batches,
// each supplying HOLDS verdicts for the rows it dispatched.
func greenBatches() []Batch {
	return []Batch{
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
}

func greenLive() *LiveEvidence {
	return &LiveEvidence{
		RunID:        "run-1",
		Source:       greenSourcePath,
		SourceSHA256: "sha-source-1",
		RepoCommit:   "head-1",
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
			RunID:        "run-1",
			RepoCommit:   "head-1",
			Source:       greenSourcePath,
			SourceSHA256: "sha-source-1",
			RowID:        "SC-02",
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
		Source:          greenSource(),
		RunID:           "run-1",
		HeadCommit:      "head-1",
		Trace:           greenTrace(),
		Freshness:       Freshness{Result: "fresh"},
		ProductionPaths: []string{},
		Batches:         greenBatches(),
		Live:            greenLive(),
		Mutations:       greenMutations(),
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
	if sc01.Layer2 != (CriticRef{Verdict: "HOLDS", BatchID: "batch-1"}) {
		t.Errorf("SC-01 Layer2 = %+v, want HOLDS from batch-1", sc01.Layer2)
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
	if sc02.Layer2 != (CriticRef{Verdict: "HOLDS", BatchID: "batch-2"}) {
		t.Errorf("SC-02 Layer2 = %+v, want HOLDS from batch-2", sc02.Layer2)
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

	if len(r.Batches) != 2 || r.Batches[0].BatchID != "batch-1" || r.Batches[1].BatchID != "batch-2" {
		t.Errorf("Batches = %+v, want the two canary-free dispatches batch-1 and batch-2", r.Batches)
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

func TestComputeGapRowKeepsItsLiveVerdict(t *testing.T) {
	in := gapInputs()
	in.Live.RowVerdicts[0].Verdict = "LIVE-PASS"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	if got := rowOf(t, r, "SC-02").Layer3; got != "LIVE-PASS" {
		t.Errorf("SC-02 Layer3 = %q, want the live run's own verdict LIVE-PASS", got)
	}
	wantReason(t, r, "1", "SC-02", "gap")
	for _, rn := range r.Reasons {
		if rn.Layer == "3" {
			t.Errorf("layer-3 reason %+v, want none; the gap blocks at layer 1 alone", rn)
		}
	}
	if r.Layers.Layer3 != (Layer3Summary{Status: "pass", Applicable: 2, Pass: 2}) {
		t.Errorf("Layer3 summary = %+v, want 2 applicable / 2 pass", r.Layers.Layer3)
	}
	wantLayerStatus(t, r, "blocked", "blocked", "pass", "n/a")
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
		Attempt:             &Attempt{RunID: "run-1", RowID: "SC-02", RepoCommit: "head-1", Source: greenSourcePath, SourceSHA256: "sha-source-1", Refusal: "docker unavailable"},
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
	in.LiveAttempt = &LiveAttempt{RunID: "run-1", RepoCommit: "head-1", Source: greenSourcePath, SourceSHA256: "sha-source-1", Profile: "profiles/live.json", Refusal: "missing-profile"}

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

func TestComputeLiveUnboundAssertionFailureIsBlocked(t *testing.T) {
	in := greenInputs()
	in.Live.UnboundFailures = []LiveStepFailure{
		{Phase: "step", Index: 2, Expectation: "exit-code 0"},
	}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "3", "", "live-unbound-assertion-failed")
	if !strings.Contains(rn.Detail, "step[2]") || !strings.Contains(rn.Detail, "exit-code 0") {
		t.Errorf("reason detail = %q, want it to name step[2] and its expectation", rn.Detail)
	}
	if got := rowOf(t, r, "SC-02").Layer3; got != "LIVE-PASS" {
		t.Errorf("SC-02 Layer3 = %q, want LIVE-PASS to survive an unrelated unbound failure", got)
	}
}

func TestComputeLiveEvidenceWithoutUnboundFailuresHasNoUnboundReason(t *testing.T) {
	in := greenInputs()
	// in.Live.UnboundFailures stays nil, as historical evidence decodes it.

	r := mustCompute(t, in)

	wantVerdict(t, r, "PASS")
	wantNoReasonCode(t, r, "live-unbound-assertion-failed")
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
		BatchID:  "batch-1",
		RunID:    "run-1",
		Rows:     []string{"SC-01"},
		Verdicts: []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
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

// TestComputeLayer3JourneyedIntegrationRowCarriesItsLiveVerdict locks the
// rendering half of the layer-3 contract: the rows deriveLayer3 renders come
// from the coverage source's live decision, so a journeyed integration row
// carries the live run's own verdict instead of the plain N/A a level filter
// would leave behind.
func TestComputeLayer3JourneyedIntegrationRowCarriesItsLiveVerdict(t *testing.T) {
	in := greenInputs()
	in.Source.Rows[0].Level = "integration"
	// The decision has to be one ValidateLiveDecision accepts, or finalize
	// refuses it before rendering: it names its live-profile target and its
	// journey walks the active smoke and e2e rows as well as SC-01.
	in.Source.LiveProfile = ".test-design/live-profiles/local.json"
	in.Source.LiveDecision = &coverage.LiveDecision{
		Decision: coverage.LiveRequired,
		Reason:   "the walkthrough is only observable against the running system",
		Journeys: []coverage.Journey{{
			ID:        "J-1",
			Scenarios: []string{"SC-01", "SC-02", "SC-03"},
			Expected:  "the walkthrough reaches the ready marker",
		}},
	}
	in.Live.RowVerdicts = append(in.Live.RowVerdicts, LiveRowVerdict{RowID: "SC-01", Verdict: "LIVE-PASS"})

	r := mustCompute(t, in)

	wantVerdict(t, r, "PASS")
	if got := rowOf(t, r, "SC-01").Layer3; got != "LIVE-PASS" {
		t.Errorf("SC-01 Layer3 = %q, want the journeyed row's live verdict LIVE-PASS", got)
	}
	if r.Layers.Layer3 != (Layer3Summary{Status: "pass", Applicable: 3, Pass: 3}) {
		t.Errorf("Layer3 summary = %+v, want 3 applicable / 3 pass", r.Layers.Layer3)
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
	in.ProductionPaths = []string{"spec.md"}
	in.Freshness = Freshness{
		Result: "stale",
		Paths:  []FreshnessPath{{Path: "spec.md", Basis: "adds the 429 retry rule"}},
		Stale:  []StaleFinding{{Hunk: "spec.md:12", Behavior: "retry on 429"}},
	}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	rn := wantReason(t, r, "freshness", "", "stale")
	if !strings.Contains(rn.Detail, "retry on 429") {
		t.Errorf("reason detail = %q, want it to name the stale behavior", rn.Detail)
	}
	wantLayerStatus(t, r, "pass", "pass", "pass", "pass")
}

// TestReportRecordsFreshnessBasis holds the published report's JSON to the
// classification Compute derived: base and measured name the two commits the
// diff spans, and paths carries the classified entry with its basis.
func TestReportRecordsFreshnessBasis(t *testing.T) {
	in := greenInputs()
	in.ProductionPaths = []string{"pkg/a.go"}
	in.Freshness = Freshness{
		Result: "fresh",
		Paths:  []FreshnessPath{{Path: "pkg/a.go", Basis: "adds a comment; no behavior change"}},
	}
	r := mustCompute(t, in)

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded struct {
		Freshness struct {
			Base     string          `json:"base"`
			Measured string          `json:"measured"`
			Paths    []FreshnessPath `json:"paths"`
			Result   string          `json:"result"`
			Stale    []StaleFinding  `json:"stale"`
		} `json:"freshness"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.Freshness.Base != "base-1" {
		t.Errorf("freshness.base = %q, want %q", decoded.Freshness.Base, "base-1")
	}
	if decoded.Freshness.Measured != "head-1" {
		t.Errorf("freshness.measured = %q, want %q", decoded.Freshness.Measured, "head-1")
	}
	if len(decoded.Freshness.Paths) != 1 || decoded.Freshness.Paths[0].Path != "pkg/a.go" || decoded.Freshness.Paths[0].Basis == "" {
		t.Errorf("freshness.paths = %+v, want one classified entry for pkg/a.go with a basis", decoded.Freshness.Paths)
	}
	if decoded.Freshness.Result != "fresh" {
		t.Errorf("freshness.result = %q, want fresh", decoded.Freshness.Result)
	}
	if decoded.Freshness.Stale == nil {
		t.Error("freshness.stale = nil, want an empty array, never null")
	}
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

func TestComputeRefusesStaleBatchRunID(t *testing.T) {
	in := greenInputs()
	in.Batches[0].RunID = "run-0"
	wantComputeRefusal(t, in, CodeStaleEvidence)
}

func multiCauseInputs() *Inputs {
	in := greenInputs()
	in.ProductionPaths = []string{"spec.md"}
	in.Freshness = Freshness{
		Result: "stale",
		Paths:  []FreshnessPath{{Path: "spec.md", Basis: "adds the 429 retry rule"}},
		Stale:  []StaleFinding{{Hunk: "spec.md:12", Behavior: "retry on 429"}},
	}
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

func TestDeriveLayer2InvalidVerdictBlocks(t *testing.T) {
	in := greenInputs()
	in.Batches[0].Verdicts[0].Verdict = "PROBABLY-HOLDS"
	rs := newReasonSet()

	refs := deriveLayer2(rs, in, ActiveRows(in.Source))

	if refs["SC-01"].Verdict != "PROBABLY-HOLDS" {
		t.Errorf("SC-01 ref = %+v, want the raw token preserved", refs["SC-01"])
	}
	if rs.max != sevBlocked {
		t.Errorf("severity = %v, want blocked", rs.max)
	}
	if got := (reasonKey{rs.list[0].Layer, rs.list[0].RowID, rs.list[0].Code}); got != (reasonKey{"2", "SC-01", "invalid-verdict"}) {
		t.Errorf("reason = %+v, want a blocking layer-2 invalid-verdict on SC-01", rs.list)
	}
}

func TestDeriveLayer3InvalidVerdictBlocks(t *testing.T) {
	in := greenInputs()
	in.Live.RowVerdicts[0].Verdict = "LIVE-MAYBE"
	rs := newReasonSet()

	_, sum := deriveLayer3(rs, in, ActiveRows(in.Source))

	if sum.Status != statusBlocked || sum.NA != 1 || sum.Pass != 1 {
		t.Errorf("layer 3 summary = %+v, want blocked with the unknown token counted as n/a", sum)
	}
	if rs.max != sevBlocked {
		t.Errorf("severity = %v, want blocked", rs.max)
	}
	if got := (reasonKey{rs.list[0].Layer, rs.list[0].RowID, rs.list[0].Code}); got != (reasonKey{"3", "SC-02", "invalid-verdict"}) {
		t.Errorf("reason = %+v, want a blocking layer-3 invalid-verdict on SC-02", rs.list)
	}
}

func TestDeriveLayer4UnloadedEvidenceBlocks(t *testing.T) {
	in := greenInputs()
	in.Mutations[0].Evidence = nil
	rs := newReasonSet()

	refs, sum := deriveLayer4(rs, in, ActiveRows(in.Source), Layer1Gaps(in.Trace))

	if refs["SC-02"].Status != "N/A" {
		t.Errorf("SC-02 ref = %+v, want N/A alongside the blocking reason", refs["SC-02"])
	}
	if sum.Status != statusBlocked {
		t.Errorf("layer 4 status = %q, want blocked; an unloaded entry must never pass", sum.Status)
	}
	if got := (reasonKey{rs.list[0].Layer, rs.list[0].RowID, rs.list[0].Code}); got != (reasonKey{"4", "SC-02", "invalid-verdict"}) {
		t.Errorf("reason = %+v, want a blocking layer-4 invalid-verdict on SC-02", rs.list)
	}
}

func TestComputeRowBoundFindingBeatsPassingResult(t *testing.T) {
	in := greenInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "drift", RowID: "SC-01", Detail: "TestA cites SC-01 but asserts the retired rule"}}
	in.Trace.FindingCounts = map[string]int{"drift": 1}

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	if got := rowOf(t, r, "SC-01").Layer1.Status; got != "drift" {
		t.Errorf("SC-01 Layer1 = %q, want drift; a row-bound finding wins over a passing result", got)
	}
	rn := wantReason(t, r, "1", "SC-01", "drift")
	if !strings.Contains(rn.Detail, "retired rule") {
		t.Errorf("reason detail = %q, want the trace finding detail", rn.Detail)
	}
}

func TestComputeAmbiguityFindingOnPassingRowIsGapGated(t *testing.T) {
	in := greenInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "ambiguity", RowID: "SC-01", Detail: "two tests cite SC-01"}}
	in.Trace.FindingCounts = map[string]int{"ambiguity": 1}
	in.Batches[0].Verdicts[0].Verdict = "MISSING"

	r := mustCompute(t, in)

	wantVerdict(t, r, "BLOCKED")
	sc01 := rowOf(t, r, "SC-01")
	if sc01.Layer1.Status != "ambiguity" {
		t.Errorf("SC-01 Layer1 = %q, want ambiguity; a gap-gated row is never bound", sc01.Layer1.Status)
	}
	if sc01.Layer2.Verdict != "MISSING" {
		t.Errorf("SC-01 Layer2 = %+v, want the MISSING its ambiguity requires", sc01.Layer2)
	}
	wantReason(t, r, "1", "SC-01", "ambiguity")
}

func TestComputeAmbiguityFindingOnPassingRowRefusesHoldingCritic(t *testing.T) {
	in := greenInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "ambiguity", RowID: "SC-01", Detail: "two tests cite SC-01"}}
	in.Trace.FindingCounts = map[string]int{"ambiguity": 1}

	wantComputeRefusal(t, in, CodeMissingVerdict)
}

// covers: SC-05
func TestDeriveLayer2BlocksWeakVacuousMissing(t *testing.T) {
	for _, verdict := range []string{"WEAK", "VACUOUS", "MISSING"} {
		t.Run(verdict, func(t *testing.T) {
			in := greenInputs()
			in.Batches[1].Verdicts[1] = CriticVerdict{RowID: "SC-03", Verdict: verdict, Reason: "critic found " + verdict}

			r := mustCompute(t, in)

			wantVerdict(t, r, "BLOCKED")
			rn := wantReason(t, r, "2", "SC-03", verdict)
			if !strings.Contains(rn.Detail, "critic found "+verdict) {
				t.Errorf("reason detail = %q, want the critic reason", rn.Detail)
			}
			sc03 := rowOf(t, r, "SC-03")
			if sc03.Layer2.Verdict != verdict {
				t.Errorf("SC-03 Layer2.Verdict = %q, want %q", sc03.Layer2.Verdict, verdict)
			}
			if sc03.Layer2.Verdict == "HOLDS" {
				t.Errorf("SC-03 Layer2 = %+v, want the row not covered", sc03.Layer2)
			}
		})
	}
}

func TestDeriveLayer1PrefersRedOverGapAcrossContributors(t *testing.T) {
	tr := &TraceEvidence{
		BaselineStatus: "ok",
		RowResults: []TraceRowResult{
			{RowID: "SC-01", Scope: "pkg/a", Name: "TestAlpha", Status: "failed"},
		},
		Findings: []TraceFinding{
			{Kind: "gap", RowID: "SC-01", Detail: "SC-01: Test at pkg/a/skip_test.go:9 skipped"},
			{Kind: "red", RowID: "SC-01", Detail: "SC-01: Test at pkg/a/fail_test.go:14 failed"},
		},
	}
	active := []coverage.Row{{ID: "SC-01", Level: "unit", Risk: "low"}}
	rs := newReasonSet()

	out := deriveLayer1(rs, tr, active)

	if out["SC-01"].Status != "red" {
		t.Fatalf("SC-01 layer1 status = %q, want red (a failed contributor must outrank a skipped one)", out["SC-01"].Status)
	}
	if got := (reasonKey{rs.list[0].Layer, rs.list[0].RowID, rs.list[0].Code}); got != (reasonKey{layer1, "SC-01", "red"}) {
		t.Fatalf("reason = %+v, want a blocking layer-1 red on SC-01", rs.list)
	}
	if !strings.Contains(rs.list[0].Detail, "fail_test.go:14") {
		t.Errorf("reason detail = %q, want it to name the failing contributor", rs.list[0].Detail)
	}
}

func TestComputeCopiesReuseIntoBatchReportAndMutationRef(t *testing.T) {
	in := greenInputs()
	batchRef := &evidence.ReuseReference{
		PriorRunID:        "run-0",
		PriorRepoCommit:   "head-0",
		PriorSourceSHA256: "sha-source-0",
		PriorSeq:          2,
		Outcome:           "HOLDS",
		Artifact:          evidence.FileRef{Path: "runs/run-0/review-g1-2.json", SHA256: "digest-review"},
		Basis:             evidence.ReuseBasis{PriorCommit: "head-0", ClaimRows: []string{"SC-02", "SC-03"}, ContextFiles: 3, Policy: "matched", Toolchain: "matched"},
	}
	in.Batches[1] = Batch{
		BatchID: "reuse:g1",
		RunID:   "run-0",
		Rows:    []string{"SC-02", "SC-03"},
		Verdicts: []CriticVerdict{
			{RowID: "SC-02", Verdict: "HOLDS"},
			{RowID: "SC-03", Verdict: "HOLDS"},
		},
		Reuse: batchRef,
	}
	mutationRef := &evidence.ReuseReference{
		PriorRunID:        "run-0",
		PriorRepoCommit:   "head-0",
		PriorSourceSHA256: "sha-source-0",
		PriorSeq:          5,
		Outcome:           "caught",
		Artifact:          evidence.FileRef{Path: "runs/run-0/mutation-SC-02.json", SHA256: "digest-mutation"},
		Basis:             evidence.ReuseBasis{PriorCommit: "head-0", ClaimRows: []string{"SC-02"}, ContextFiles: 1, Policy: "not-applicable", Toolchain: "matched"},
	}
	in.Mutations[0].Reuse = mutationRef
	in.Mutations[0].Evidence.RunID = "run-0"
	in.Mutations[0].Evidence.RepoCommit = "head-0"
	in.Mutations[0].Evidence.SourceSHA256 = "sha-source-0"

	rep := mustCompute(t, in)

	var adopted *BatchReport
	for i := range rep.Batches {
		if rep.Batches[i].BatchID == "reuse:g1" {
			adopted = &rep.Batches[i]
		}
	}
	if adopted == nil {
		t.Fatalf("Batches = %+v, want the adopted batch reuse:g1", rep.Batches)
	}
	if !reflect.DeepEqual(adopted.Reuse, batchRef) {
		t.Errorf("adopted batch Reuse = %+v, want %+v", adopted.Reuse, batchRef)
	}
	for _, b := range rep.Batches {
		if b.BatchID != "reuse:g1" && b.Reuse != nil {
			t.Errorf("batch %s Reuse = %+v, want nil", b.BatchID, b.Reuse)
		}
	}

	if got := rowOf(t, rep, "SC-02").Layer4; !reflect.DeepEqual(got.Reuse, mutationRef) {
		t.Errorf("SC-02 Layer4 Reuse = %+v, want %+v", got.Reuse, mutationRef)
	}
	if got := rowOf(t, rep, "SC-03").Layer4; got.Reuse != nil {
		t.Errorf("SC-03 Layer4 Reuse = %+v, want nil", got.Reuse)
	}
}

// TestCalibration_PassAgrees uses the all-green fixture, where SC-02 is the
// only high-risk row: HOLDS (layer 2) paired with MUTATION-CAUGHT (layer 4) is
// agreement, so every comparable row agrees.
func TestCalibration_PassAgrees(t *testing.T) {
	r := mustCompute(t, greenInputs())

	if r.Calibration.Comparable != 1 {
		t.Fatalf("Calibration.Comparable = %d, want 1 (SC-02 HOLDS+CAUGHT)", r.Calibration.Comparable)
	}
	if r.Calibration.Agree != r.Calibration.Comparable {
		t.Errorf("Calibration = %+v, want Agree == Comparable", r.Calibration)
	}
}

// TestCalibration_HoldsSurvived makes SC-02's mutation survive instead of
// being caught: the critic held HOLDS on a row whose mutant then survived, so
// calibration must flag it without adding a reason of its own — the layer-4
// MUTATION-SURVIVED reason deriveLayer4 already adds is the only one present.
func TestCalibration_HoldsSurvived(t *testing.T) {
	in := greenInputs()
	in.Mutations[0].Evidence.Sites[1].Outcome = "survived"

	r := mustCompute(t, in)

	if !reflect.DeepEqual(r.Calibration.HoldsSurvived, []string{"SC-02"}) {
		t.Errorf("Calibration.HoldsSurvived = %v, want [SC-02]", r.Calibration.HoldsSurvived)
	}
	if len(r.Reasons) != 1 {
		t.Errorf("Reasons = %+v, want only the layer-4 MUTATION-SURVIVED reason; calibration must never add one", r.Reasons)
	}
	wantReason(t, r, "4", "SC-02", "MUTATION-SURVIVED")
}

// TestCalibration_SuspectSharesGroup reuses the same survived-SC-02 scenario:
// SC-02's batch (batch-2) also judged SC-03 HOLDS. SC-03 is not high-risk, so
// it never earns its own layer-4 comparison, but sharing a batch with an
// overclaiming HOLDS+SURVIVED row makes its HOLDS suspect too. SC-01, in the
// unrelated batch-1, must not be swept in.
func TestCalibration_SuspectSharesGroup(t *testing.T) {
	in := greenInputs()
	in.Mutations[0].Evidence.Sites[0].Outcome = "survived"

	r := mustCompute(t, in)

	if !reflect.DeepEqual(r.Calibration.SuspectRows, []string{"SC-03"}) {
		t.Errorf("Calibration.SuspectRows = %v, want [SC-03] (batch-2, shares the group with HOLDS+SURVIVED SC-02)", r.Calibration.SuspectRows)
	}
}

// calibrationExclusionSource carries four high-risk unit-level rows so layer 3
// never applies: one clean CAUGHT comparison, and one row apiece for the three
// layer-4 outcomes calibration must not count as comparable.
func calibrationExclusionSource() *coverage.Source {
	return &coverage.Source{
		Path:           "docs/superpowers/coverage/calibration-coverage.md",
		SHA256:         "sha-calibration-1",
		BaselineCommit: "base-cal-1",
		Rows: []coverage.Row{
			{ID: "X-01", Level: "unit", Risk: "high"},
			{ID: "X-02", Level: "unit", Risk: "high"},
			{ID: "X-03", Level: "unit", Risk: "high"},
			{ID: "X-04", Level: "unit", Risk: "high"},
		},
	}
}

func calibrationExclusionTrace() *TraceEvidence {
	return &TraceEvidence{
		RunID:          "run-cal-1",
		Source:         "docs/superpowers/coverage/calibration-coverage.md",
		SourceSHA256:   "sha-calibration-1",
		RepoCommit:     "head-cal-1",
		BaselineCommit: "base-cal-1",
		BaselineStatus: "ok",
		ExitStatus:     0,
		Counts:         SuiteCounts{Passed: 3},
		RowAccounting:  RowAccounting{ActiveRows: 4, WithResult: 3},
		FindingCounts:  map[string]int{"gap": 1},
		RowResults: []TraceRowResult{
			{RowID: "X-01", Scope: "pkg/x", Name: "TestX01", Status: "passed"},
			{RowID: "X-03", Scope: "pkg/x", Name: "TestX03", Status: "passed"},
			{RowID: "X-04", Scope: "pkg/x", Name: "TestX04", Status: "passed"},
		},
		Findings: []TraceFinding{{Kind: "gap", RowID: "X-02", Detail: "no test cites X-02"}},
	}
}

func calibrationExclusionBatches() []Batch {
	return []Batch{{
		BatchID: "batch-cal",
		RunID:   "run-cal-1",
		Rows:    []string{"X-01", "X-02", "X-03", "X-04"},
		Verdicts: []CriticVerdict{
			{RowID: "X-01", Verdict: "HOLDS"},
			{RowID: "X-02", Verdict: "MISSING"},
			{RowID: "X-03", Verdict: "HOLDS"},
			{RowID: "X-04", Verdict: "HOLDS"},
		},
	}}
}

func calibrationExclusionMutations() []MutationInput {
	return []MutationInput{
		{
			RowID:        "X-01",
			EvidencePath: "run/mutation-X-01.json",
			Evidence: &MutationEvidence{
				RunID:         "run-cal-1",
				RepoCommit:    "head-cal-1",
				Source:        "docs/superpowers/coverage/calibration-coverage.md",
				SourceSHA256:  "sha-calibration-1",
				RowID:         "X-01",
				Sites:         []MutationSiteOutcome{{Site: "pkg/x.go:1", Outcome: "caught"}},
				SnapshotEqual: true,
			},
		},
		{RowID: "X-03", Outcome: "unappliable", Reason: "no site matched"},
		{
			RowID:               "X-04",
			Outcome:             "environmental",
			Reason:              "docker unavailable",
			AttemptEvidencePath: "run/mutation-attempt-X-04.json",
			Attempt: &Attempt{
				RunID:        "run-cal-1",
				RowID:        "X-04",
				RepoCommit:   "head-cal-1",
				Source:       "docs/superpowers/coverage/calibration-coverage.md",
				SourceSHA256: "sha-calibration-1",
				Refusal:      "docker unavailable",
			},
		},
	}
}

func calibrationExclusionInputs() *Inputs {
	return &Inputs{
		Source:          calibrationExclusionSource(),
		RunID:           "run-cal-1",
		HeadCommit:      "head-cal-1",
		Trace:           calibrationExclusionTrace(),
		Freshness:       Freshness{Result: "fresh"},
		ProductionPaths: []string{},
		Batches:         calibrationExclusionBatches(),
		Mutations:       calibrationExclusionMutations(),
	}
}

// TestCalibration_ExcludesGapEnvUnappliable checks that only the row with a
// real CAUGHT/SURVIVED outcome counts: the layer-1 gap, the environmental
// mutation run, and the unappliable mutation must not inflate Comparable, and
// none of them can appear in either disagreement list.
func TestCalibration_ExcludesGapEnvUnappliable(t *testing.T) {
	r := mustCompute(t, calibrationExclusionInputs())

	if r.Calibration.Comparable != 1 || r.Calibration.Agree != 1 {
		t.Errorf("Calibration = %+v, want only X-01 (HOLDS+CAUGHT) comparable; the gap, environmental and unappliable rows must not count", r.Calibration)
	}
	if len(r.Calibration.HoldsSurvived) != 0 || len(r.Calibration.FlaggedCaught) != 0 {
		t.Errorf("Calibration = %+v, want no disagreements", r.Calibration)
	}
}

// TestCalibration_SuspectIncludesUncomparableHighRiskRow: SuspectRows widens
// past non-high rows to any row with no Comparable pairing of its own. X-01
// flips to HOLDS+SURVIVED, tainting batch-cal; X-03 (HOLDS, unappliable) and
// X-04 (HOLDS, environmental) are both high-risk but never became Comparable,
// so both must surface as suspect too. X-02 (MISSING) still must not.
func TestCalibration_SuspectIncludesUncomparableHighRiskRow(t *testing.T) {
	in := calibrationExclusionInputs()
	in.Mutations[0].Evidence.Sites[0].Outcome = "survived"

	r := mustCompute(t, in)

	if !reflect.DeepEqual(r.Calibration.HoldsSurvived, []string{"X-01"}) {
		t.Fatalf("Calibration.HoldsSurvived = %v, want [X-01]", r.Calibration.HoldsSurvived)
	}
	if !reflect.DeepEqual(r.Calibration.SuspectRows, []string{"X-03", "X-04"}) {
		t.Errorf("Calibration.SuspectRows = %v, want [X-03 X-04]: both are high-risk, HOLDS, and never became Comparable themselves", r.Calibration.SuspectRows)
	}
}

// TestCalibration_MissingVerdictNotComparable: MISSING means the row carries
// no layer-2 verdict at all, so a high-risk row layer 2 marked MISSING
// is never comparable even though its layer 4 caught the mutation — SC-02 is
// not a layer-1 gap (validateLayer2 only requires MISSING on a gap row, it
// never forbids MISSING on a bound one), so this state is reachable.
func TestCalibration_MissingVerdictNotComparable(t *testing.T) {
	in := greenInputs()
	in.Batches[1].Verdicts[0].Verdict = "MISSING"

	r := mustCompute(t, in)

	if r.Calibration.Comparable != 0 || r.Calibration.Agree != 0 {
		t.Errorf("Calibration = %+v, want Comparable == Agree == 0; MISSING is not a verdict to compare", r.Calibration)
	}
	if len(r.Calibration.HoldsSurvived) != 0 || len(r.Calibration.FlaggedCaught) != 0 || len(r.Calibration.SuspectRows) != 0 {
		t.Errorf("Calibration = %+v, want no list to name SC-02", r.Calibration)
	}
}
