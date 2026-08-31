package report

import (
	"errors"
	"strings"
	"testing"

	"test-design/internal/coverage"
)

func baseSource() *coverage.Source {
	return &coverage.Source{
		SHA256: "sha-source-1",
		Rows: []coverage.Row{
			{ID: "SC-01", Level: "unit", Risk: "low"},
			{ID: "SC-02", Level: "smoke", Risk: "high"},
			{ID: "SC-03", Level: "e2e", Risk: "med"},
			{ID: "SC-04", Level: "unit", Risk: "low"},
		},
		Retired: []string{"SC-04"},
	}
}

func baseTrace() *TraceEvidence {
	return &TraceEvidence{
		RunID:        "run-1",
		SourceSHA256: "sha-source-1",
	}
}

func baseBatches() []Batch {
	return []Batch{
		{
			BatchID: "cb1",
			RunID:   "run-1",
			Rows:    []string{"SC-02"},
			Canary:  &Canary{RowID: "SC-02", Kind: "natural", Caught: true},
			Verdicts: []CriticVerdict{
				{RowID: "SC-02", Verdict: "WEAK"},
			},
		},
		{
			BatchID:      "b1",
			RunID:        "run-1",
			Rows:         []string{"SC-01"},
			InheritsFrom: "cb1",
			Verdicts: []CriticVerdict{
				{RowID: "SC-01", Verdict: "HOLDS"},
			},
		},
		{
			BatchID:      "b2",
			RunID:        "run-1",
			Rows:         []string{"SC-03"},
			InheritsFrom: "cb1",
			Verdicts: []CriticVerdict{
				{RowID: "SC-03", Verdict: "HOLDS"},
			},
		},
	}
}

func baseLive() *LiveEvidence {
	return &LiveEvidence{
		RunID:        "run-1",
		SourceSHA256: "sha-source-1",
		RowVerdicts: []LiveRowVerdict{
			{RowID: "SC-02", Verdict: "LIVE-PASS"},
			{RowID: "SC-03", Verdict: "LIVE-PASS"},
		},
		CleanupOK: true,
	}
}

func baseMutations() []MutationInput {
	return []MutationInput{
		{
			RowID:        "SC-02",
			EvidencePath: "testdata/mutation-valid.json",
			Evidence: &MutationEvidence{
				RunID: "run-1",
				RowID: "SC-02",
				Sites: []MutationSiteOutcome{{Site: "x", Outcome: "caught"}},
			},
		},
	}
}

func baseInputs() *Inputs {
	return &Inputs{
		Source:    baseSource(),
		RunID:     "run-1",
		Trace:     baseTrace(),
		Freshness: Freshness{Result: "fresh"},
		Batches:   baseBatches(),
		Live:      baseLive(),
		Mutations: baseMutations(),
	}
}

func wantRefusal(t *testing.T, err error, code string) *RefusalError {
	t.Helper()
	if err == nil {
		t.Fatalf("Validate returned nil error, want refusal %q", code)
	}
	var re *RefusalError
	if !errors.As(err, &re) {
		t.Fatalf("Validate returned %v (%T), want *RefusalError", err, err)
	}
	if re.Code != code {
		t.Errorf("Validate refusal code = %q, want %q (detail: %s)", re.Code, code, re.Detail)
	}
	return re
}

func TestValidateAcceptsHappyPath(t *testing.T) {
	if err := Validate(baseInputs()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateStaleEvidenceTraceRunIDMismatch(t *testing.T) {
	in := baseInputs()
	in.Trace.RunID = "other-run"
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateStaleEvidenceTraceSourceSHAMismatch(t *testing.T) {
	in := baseInputs()
	in.Trace.SourceSHA256 = "different-sha"
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateStaleEvidenceLiveRunIDMismatch(t *testing.T) {
	in := baseInputs()
	in.Live.RunID = "other-run"
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateStaleEvidenceBatchRunIDEmpty(t *testing.T) {
	in := baseInputs()
	in.Batches[0].RunID = ""
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateStaleEvidenceMutationRunIDMismatch(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].Evidence.RunID = "other-run"
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateMissingFreshness(t *testing.T) {
	in := baseInputs()
	in.Freshness = Freshness{}
	wantRefusal(t, Validate(in), CodeMissingFreshness)
}

func TestValidateInvalidFreshnessUnknownValue(t *testing.T) {
	in := baseInputs()
	in.Freshness = Freshness{Result: "weird"}
	wantRefusal(t, Validate(in), CodeInvalidFreshness)
}

func TestValidateInvalidFreshnessStaleWithNoFindings(t *testing.T) {
	in := baseInputs()
	in.Freshness = Freshness{Result: "stale"}
	wantRefusal(t, Validate(in), CodeInvalidFreshness)
}

func TestValidateUnknownRowRetiredInBatch(t *testing.T) {
	in := baseInputs()
	in.Batches[1].Rows = []string{"SC-04"}
	wantRefusal(t, Validate(in), CodeUnknownRow)
}

func TestValidateUnknownRowRetiredInMutations(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].RowID = "SC-04"
	wantRefusal(t, Validate(in), CodeUnknownRow)
}

func TestValidateDuplicateVerdictLayer2(t *testing.T) {
	in := baseInputs()
	in.Batches = append(in.Batches, Batch{
		BatchID:      "b3",
		RunID:        "run-1",
		Rows:         []string{"SC-01"},
		InheritsFrom: "cb1",
		Verdicts:     []CriticVerdict{{RowID: "SC-01", Verdict: "WEAK"}},
	})
	wantRefusal(t, Validate(in), CodeDuplicateVerdict)
}

func TestValidateMissingVerdictLayer2(t *testing.T) {
	in := baseInputs()
	in.Batches = in.Batches[:2]
	wantRefusal(t, Validate(in), CodeMissingVerdict)
}

func TestValidateVoidedVerdictLayer2(t *testing.T) {
	in := baseInputs()
	in.Batches[1].Voided = true
	wantRefusal(t, Validate(in), CodeVoidedVerdict)
}

func TestValidateVoidedVerdictTransitiveOneLevel(t *testing.T) {
	in := baseInputs()
	base := baseBatches()
	in.Batches = []Batch{
		{BatchID: "root", RunID: "run-1", Voided: true},
		{
			BatchID:      "b1",
			RunID:        "run-1",
			Rows:         []string{"SC-01"},
			InheritsFrom: "root",
			Verdicts:     []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
		},
		base[0],
		base[2],
	}
	re := wantRefusal(t, Validate(in), CodeVoidedVerdict)
	if !strings.Contains(re.Detail, "SC-01") {
		t.Errorf("Detail = %q, want it to name row SC-01, whose only batch (b1) is voided purely via inheritsFrom root", re.Detail)
	}
}

func TestValidateVoidedVerdictTransitiveTwoLevelChain(t *testing.T) {
	in := baseInputs()
	base := baseBatches()
	in.Batches = []Batch{
		{BatchID: "root", RunID: "run-1", Voided: true},
		{BatchID: "mid", RunID: "run-1", InheritsFrom: "root"},
		{
			BatchID:      "b1",
			RunID:        "run-1",
			Rows:         []string{"SC-01"},
			InheritsFrom: "mid",
			Verdicts:     []CriticVerdict{{RowID: "SC-01", Verdict: "HOLDS"}},
		},
		base[0],
		base[2],
	}
	re := wantRefusal(t, Validate(in), CodeVoidedVerdict)
	if !strings.Contains(re.Detail, "SC-01") {
		t.Errorf("Detail = %q, want it to name row SC-01, whose only batch (b1) is voided via a two-level inheritsFrom chain (b1 -> mid -> root)", re.Detail)
	}
}

func TestValidateGapRowMustCarryMissingLayer2(t *testing.T) {
	in := baseInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "gap", RowID: "SC-01", Detail: "no tagged test"}}
	wantRefusal(t, Validate(in), CodeMissingVerdict)
}

func TestValidateUncaughtCanaryNotVoided(t *testing.T) {
	in := baseInputs()
	in.Batches[0].Canary.Caught = false
	wantRefusal(t, Validate(in), CodeUncaughtCanaryNotVoided)
}

func TestValidateCanaryBookkeepingCanaryFreeWithoutInheritsFrom(t *testing.T) {
	in := baseInputs()
	in.Batches[1].InheritsFrom = ""
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateCanaryBookkeepingInheritsFromNonCaughtBatch(t *testing.T) {
	in := baseInputs()
	in.Batches[1].InheritsFrom = "b2"
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateCanaryBookkeepingSynthesizedEmptyWeakening(t *testing.T) {
	in := baseInputs()
	in.Batches[0].Canary.Kind = "synthesized"
	in.Batches[0].Canary.Weakening = ""
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateCanaryBookkeepingSynthesizedMissingRealSourceVerdict(t *testing.T) {
	in := baseInputs()
	in.Batches[0].Canary.Kind = "synthesized"
	in.Batches[0].Canary.Weakening = "off-by-one"
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateCanaryBookkeepingNaturalCaughtHolds(t *testing.T) {
	in := baseInputs()
	in.Batches[0].Verdicts[0].Verdict = "HOLDS"
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateUnexpectedLiveEvidence(t *testing.T) {
	in := baseInputs()
	in.Source.Rows = []coverage.Row{{ID: "SC-01", Level: "unit", Risk: "low"}}
	in.Source.Retired = nil
	in.Live = &LiveEvidence{RunID: "run-1", SourceSHA256: "sha-source-1", CleanupOK: true}
	in.Batches = []Batch{{
		BatchID: "b1",
		RunID:   "run-1",
		Rows:    []string{"SC-01"},
		Canary:  &Canary{RowID: "SC-01", Kind: "natural", Caught: true},
		Verdicts: []CriticVerdict{
			{RowID: "SC-01", Verdict: "WEAK"},
		},
	}}
	in.Mutations = nil
	wantRefusal(t, Validate(in), CodeUnexpectedLiveEvidence)
}

func TestValidateMissingVerdictLayer3NoLiveNoAttempt(t *testing.T) {
	in := baseInputs()
	in.Live = nil
	wantRefusal(t, Validate(in), CodeMissingVerdict)
}

func TestValidateLayer3NoLiveWithAttemptIsAccepted(t *testing.T) {
	in := baseInputs()
	in.Live = nil
	in.LiveAttempt = &LiveAttempt{RunID: "run-1", Profile: "p", Refusal: "missing-profile"}
	if err := Validate(in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateDuplicateVerdictLayer3(t *testing.T) {
	in := baseInputs()
	in.Live.RowVerdicts = append(in.Live.RowVerdicts, LiveRowVerdict{RowID: "SC-02", Verdict: "LIVE-FAIL"})
	wantRefusal(t, Validate(in), CodeDuplicateVerdict)
}

func TestValidateMissingVerdictLayer3RowAbsent(t *testing.T) {
	in := baseInputs()
	in.Live.RowVerdicts = []LiveRowVerdict{{RowID: "SC-02", Verdict: "LIVE-PASS"}}
	wantRefusal(t, Validate(in), CodeMissingVerdict)
}

func TestValidateUnexpectedLiveEvidenceInapplicableRow(t *testing.T) {
	in := baseInputs()
	in.Live.RowVerdicts = append(in.Live.RowVerdicts, LiveRowVerdict{RowID: "SC-01", Verdict: "LIVE-PASS"})
	re := wantRefusal(t, Validate(in), CodeUnexpectedLiveEvidence)
	if !strings.Contains(re.Detail, "SC-01") {
		t.Errorf("Detail = %q, want it to name row SC-01", re.Detail)
	}
}

func TestValidateGapRowLayer3MustCarryNA(t *testing.T) {
	in := baseInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "gap", RowID: "SC-02", Detail: "no tagged test"}}
	in.Batches[0].Verdicts[0].Verdict = "MISSING"
	in.Mutations = nil
	wantRefusal(t, Validate(in), CodeMissingVerdict)
}

func TestValidateMutationOnGapRow(t *testing.T) {
	in := baseInputs()
	in.Trace.Findings = []TraceFinding{{Kind: "gap", RowID: "SC-02", Detail: "no tagged test"}}
	in.Batches[0].Verdicts[0].Verdict = "MISSING"
	in.Live.RowVerdicts[0] = LiveRowVerdict{RowID: "SC-02", Verdict: "LIVE-N/A (layer 1 gap)"}
	wantRefusal(t, Validate(in), CodeMutationOnGapRow)
}

func TestValidateMissingVerdictLayer4(t *testing.T) {
	in := baseInputs()
	in.Mutations = nil
	wantRefusal(t, Validate(in), CodeMissingVerdict)
}

func TestValidateDuplicateVerdictLayer4(t *testing.T) {
	in := baseInputs()
	in.Mutations = append(in.Mutations, MutationInput{RowID: "SC-02", Outcome: "unappliable"})
	wantRefusal(t, Validate(in), CodeDuplicateVerdict)
}

func TestValidateMutationEvidenceEmpty(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].Evidence.Sites = nil
	wantRefusal(t, Validate(in), CodeMutationEvidenceEmpty)
}

func TestValidateMutationRowMismatch(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].Evidence.RowID = "SC-03"
	wantRefusal(t, Validate(in), CodeMutationRowMismatch)
}

func TestValidateMissingAttemptEvidence(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].Evidence = nil
	in.Mutations[0].EvidencePath = ""
	in.Mutations[0].Outcome = "environmental"
	wantRefusal(t, Validate(in), CodeMissingAttemptEvidence)
}

func TestValidateInvalidMutationOutcome(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].Evidence = nil
	in.Mutations[0].EvidencePath = ""
	in.Mutations[0].Outcome = "bogus"
	wantRefusal(t, Validate(in), CodeInvalidMutationOutcome)
}
