package report

import (
	"errors"
	"strings"
	"testing"

	"test-design/internal/coverage"
)

func baseSource() *coverage.Source {
	return &coverage.Source{
		Path:   greenSourcePath,
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
		RepoCommit:   "head-1",
		Source:       greenSourcePath,
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
		RepoCommit:   "head-1",
		Source:       greenSourcePath,
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
				RunID:        "run-1",
				RepoCommit:   "head-1",
				Source:       greenSourcePath,
				SourceSHA256: "sha-source-1",
				RowID:        "SC-02",
				Sites:        []MutationSiteOutcome{{Site: "x", Outcome: "caught"}},
			},
		},
	}
}

func baseInputs() *Inputs {
	return &Inputs{
		Source:     baseSource(),
		RunID:      "run-1",
		HeadCommit: "head-1",
		Trace:      baseTrace(),
		Freshness:  Freshness{Result: "fresh"},
		Batches:    baseBatches(),
		Live:       baseLive(),
		Mutations:  baseMutations(),
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
	in.Live = &LiveEvidence{RunID: "run-1", RepoCommit: "head-1", Source: greenSourcePath, SourceSHA256: "sha-source-1", CleanupOK: true}
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
	in.LiveAttempt = &LiveAttempt{RunID: "run-1", RepoCommit: "head-1", Source: greenSourcePath, SourceSHA256: "sha-source-1", Profile: "p", Refusal: "missing-profile"}
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

// journeyedSourceInputs makes SC-01 an integration row and hands the source a
// required live decision whose single journey names it, so layer 3 owes a
// verdict for a row no level filter would ever select.
func journeyedSourceInputs() *Inputs {
	in := baseInputs()
	in.Source.Rows[0].Level = "integration"
	in.Source.LiveDecision = &coverage.LiveDecision{
		Decision: coverage.LiveRequired,
		Reason:   "the walkthrough is only observable against the running system",
		Journeys: []coverage.Journey{{
			ID:        "J-1",
			Scenarios: []string{"SC-01", "SC-02", "SC-03"},
			Expected:  "the walkthrough reaches the ready marker",
		}},
	}
	return in
}

func TestValidateLayer3JourneyedIntegrationRowIsApplicable(t *testing.T) {
	in := journeyedSourceInputs()
	in.Live.RowVerdicts = append(in.Live.RowVerdicts, LiveRowVerdict{RowID: "SC-01", Verdict: "LIVE-PASS"})
	if err := Validate(in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateMissingVerdictLayer3JourneyedRowAbsent(t *testing.T) {
	in := journeyedSourceInputs()
	re := wantRefusal(t, Validate(in), CodeMissingVerdict)
	if !strings.Contains(re.Detail, "SC-01") {
		t.Errorf("Detail = %q, want it to name the journeyed row SC-01", re.Detail)
	}
}

func TestValidateUnexpectedLiveEvidenceNotApplicableDecision(t *testing.T) {
	in := baseInputs()
	// A not-applicable decision is only entitled where no active row is smoke
	// or e2e, so the levels move with the decision: layer 3 is then empty by
	// the document's own choice and the supplied live evidence is unexpected.
	in.Source.Rows[1].Level = "integration"
	in.Source.Rows[2].Level = "integration"
	in.Source.LiveDecision = &coverage.LiveDecision{
		Decision: coverage.LiveNotApplicable,
		Reason:   "the fixture has no deployable surface",
	}
	re := wantRefusal(t, Validate(in), CodeUnexpectedLiveEvidence)
	if !strings.Contains(re.Detail, "no applicable live rows") {
		t.Errorf("Detail = %q, want it to say no applicable live rows exist", re.Detail)
	}
}

// TestValidateLiveDecisionContradictionRefused locks the gate finalize needs
// against a decision no design-time validator ever accepted: not-applicable
// while active smoke or e2e rows exist. Without the gate the applicable set is
// empty, layer 3 goes n/a, and a document talks its own live layer away.
func TestValidateLiveDecisionContradictionRefused(t *testing.T) {
	in := baseInputs()
	in.Source.LiveDecision = &coverage.LiveDecision{
		Decision: coverage.LiveNotApplicable,
		Reason:   "the fixture has no deployable surface",
	}
	in.Live = nil
	re := wantRefusal(t, Validate(in), CodeLiveDecisionIncomplete)
	if !strings.Contains(re.Detail, "contradiction") {
		t.Errorf("Detail = %q, want it to carry the live decision's own code", re.Detail)
	}
	if !strings.Contains(re.Detail, "SC-02") {
		t.Errorf("Detail = %q, want it to name the contradicting smoke row SC-02", re.Detail)
	}
}

// TestValidateLegacyNoLiveDecisionStillValidates keeps the deliberate fallback
// for a document written before the section existed: no section at all is not
// an invalid decision, and the level rule still selects its live rows.
func TestValidateLegacyNoLiveDecisionStillValidates(t *testing.T) {
	in := baseInputs()
	in.Source.LiveDecision = nil
	if err := Validate(in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateGapRowLayer3KeepsItsLiveVerdict(t *testing.T) {
	in := baseInputs()
	in.Batches = greenBatches()
	in.Trace.Findings = []TraceFinding{{Kind: "gap", RowID: "SC-02", Detail: "no tagged test"}}
	in.Batches[2].Verdicts[0].Verdict = "MISSING"
	in.Mutations = nil
	if err := Validate(in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateMutationOnGapRow(t *testing.T) {
	in := baseInputs()
	in.Batches = greenBatches()
	in.Trace.Findings = []TraceFinding{{Kind: "gap", RowID: "SC-02", Detail: "no tagged test"}}
	in.Batches[2].Verdicts[0].Verdict = "MISSING"
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

func TestValidateInvalidVerdictLayer2Token(t *testing.T) {
	in := baseInputs()
	in.Batches[1].Verdicts[0].Verdict = "PROBABLY-HOLDS"
	wantRefusal(t, Validate(in), CodeInvalidVerdict)
}

func TestValidateInvalidVerdictLayer3Token(t *testing.T) {
	in := baseInputs()
	in.Live.RowVerdicts[0].Verdict = "LIVE-MAYBE"
	wantRefusal(t, Validate(in), CodeInvalidVerdict)
}

func TestValidateLayer3HistoricalGapTokenIsAccepted(t *testing.T) {
	in := baseInputs()
	in.Live.RowVerdicts[0].Verdict = "LIVE-N/A (layer 1 gap)"
	if err := Validate(in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateInvalidVerdictLayer4UnloadedEvidence(t *testing.T) {
	in := baseInputs()
	in.Mutations[0].Evidence = nil
	re := wantRefusal(t, Validate(in), CodeInvalidVerdict)
	if !strings.Contains(re.Detail, "testdata/mutation-valid.json") {
		t.Errorf("Detail = %q, want it to name the unloaded evidence path", re.Detail)
	}
}

func TestValidateCanaryBookkeepingCanaryRowNotInBatchRows(t *testing.T) {
	in := baseInputs()
	in.Batches[0].Rows = []string{"SC-01"}
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateCanaryBookkeepingNaturalCaughtHoldsInAnotherBatch(t *testing.T) {
	in := baseInputs()
	in.Batches[0].Verdicts = nil
	in.Batches[2].Rows = []string{"SC-02", "SC-03"}
	in.Batches[2].Verdicts = []CriticVerdict{
		{RowID: "SC-02", Verdict: "HOLDS"},
		{RowID: "SC-03", Verdict: "HOLDS"},
	}
	wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
}

func TestValidateStaleEvidenceLiveAttemptRunIDMismatch(t *testing.T) {
	in := baseInputs()
	in.Live = nil
	in.LiveAttempt = &LiveAttempt{RunID: "other-run", Profile: "p", Refusal: "missing-profile"}
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateStaleEvidenceLiveAttemptRunIDEmpty(t *testing.T) {
	in := baseInputs()
	in.Live = nil
	in.LiveAttempt = &LiveAttempt{Profile: "p", Refusal: "missing-profile"}
	wantRefusal(t, Validate(in), CodeStaleEvidence)
}

func TestValidateMutationEntryOnNonHighRiskRowIsChecked(t *testing.T) {
	in := baseInputs()
	in.Mutations = append(in.Mutations, MutationInput{
		RowID:        "SC-03",
		EvidencePath: "testdata/mutation-foreign.json",
		Evidence:     &MutationEvidence{RunID: "run-1", RepoCommit: "head-1", Source: greenSourcePath, SourceSHA256: "sha-source-1", RowID: "SC-02"},
	})
	re := wantRefusal(t, Validate(in), CodeMutationEvidenceEmpty)
	if !strings.Contains(re.Detail, "SC-03") {
		t.Errorf("Detail = %q, want it to name row SC-03, whose med-risk entry carries empty evidence", re.Detail)
	}
}

func TestValidateRejectsEvidenceFromPreviousCommitWithSameRunID(t *testing.T) {
	for _, layer := range []string{"live", "mutation", "live-attempt", "mutation-attempt"} {
		t.Run(layer, func(t *testing.T) {
			in := greenInputs()
			in.HeadCommit = "head-2"
			in.Trace.RepoCommit = "head-2"
			in.Live.RepoCommit = "head-2"
			in.Mutations[0].Evidence.RepoCommit = "head-2"
			switch layer {
			case "live":
				in.Live.RepoCommit = "head-1"
			case "mutation":
				in.Mutations[0].Evidence.RepoCommit = "head-1"
			case "live-attempt":
				in.Live = nil
				in.LiveAttempt = &LiveAttempt{RunID: in.RunID, RepoCommit: "head-1", Source: in.Source.Path, SourceSHA256: in.Source.SHA256, Refusal: "runner unavailable"}
			case "mutation-attempt":
				in.Mutations[0].AttemptEvidencePath = "attempt.json"
				in.Mutations[0].Attempt = &Attempt{RunID: in.RunID, RowID: "SC-02", RepoCommit: "head-1", Source: in.Source.Path, SourceSHA256: in.Source.SHA256, Refusal: "runner unavailable"}
			}
			wantRefusal(t, Validate(in), CodeStaleEvidence)
		})
	}
}

func TestValidateCanaryKindsAndSourceSeparation(t *testing.T) {
	t.Run("natural verdict belongs to same batch", func(t *testing.T) {
		in := baseInputs()
		in.Batches[0].Verdicts = nil
		in.Batches[1].Verdicts = append(in.Batches[1].Verdicts, CriticVerdict{RowID: "SC-02", Verdict: "WEAK"})
		wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
	})
	t.Run("unknown kind", func(t *testing.T) {
		in := baseInputs()
		in.Batches[0].Canary.Kind = "unknown"
		wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
	})
	t.Run("synthetic verdict is not real evidence", func(t *testing.T) {
		in := greenInputs()
		in.Batches[0].Verdicts = in.Batches[1].Verdicts
		in.Batches[1].Verdicts = nil
		wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
	})
}

func TestValidateRejectsMissingOrMixedProvenance(t *testing.T) {
	for _, field := range []string{"commit", "source", "sha", "wrong-source", "wrong-sha"} {
		t.Run(field, func(t *testing.T) {
			in := greenInputs()
			switch field {
			case "commit":
				in.Live.RepoCommit = ""
			case "source":
				in.Mutations[0].Evidence.Source = ""
			case "sha":
				in.Mutations[0].Evidence.SourceSHA256 = ""
			case "wrong-source":
				in.Live.Source = "other-spec.md"
			case "wrong-sha":
				in.Mutations[0].Evidence.SourceSHA256 = "other-sha"
			}
			wantRefusal(t, Validate(in), CodeStaleEvidence)
		})
	}
}

func TestValidateCanaryUnavailableCannotInheritOrCarryCanary(t *testing.T) {
	for _, mode := range []string{"canary", "inheritance", "whitespace"} {
		t.Run(mode, func(t *testing.T) {
			in := greenInputs()
			switch mode {
			case "canary":
				in.Batches[0].CanaryUnavailableReason = "audited exception"
			case "inheritance":
				in.Batches[1].CanaryUnavailableReason = "audited exception"
			case "whitespace":
				in.Batches[1].InheritsFrom = ""
				in.Batches[1].CanaryUnavailableReason = " \t"
			}
			wantRefusal(t, Validate(in), CodeCanaryBookkeeping)
		})
	}
}
