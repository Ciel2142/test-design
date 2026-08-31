package report

import (
	"fmt"
	"slices"

	"test-design/internal/coverage"
)

const (
	CodeStaleEvidence           = "stale-evidence"
	CodeMissingFreshness        = "missing-freshness"
	CodeInvalidFreshness        = "invalid-freshness"
	CodeDuplicateVerdict        = "duplicate-verdict"
	CodeMissingVerdict          = "missing-verdict"
	CodeVoidedVerdict           = "voided-verdict"
	CodeUncaughtCanaryNotVoided = "uncaught-canary-not-voided"
	CodeUnknownRow              = "unknown-row"
	CodeCanaryBookkeeping       = "canary-bookkeeping"
	CodeUnexpectedLiveEvidence  = "unexpected-live-evidence"
	CodeMutationOnGapRow        = "mutation-on-gap-row"
	CodeMutationEvidenceEmpty   = "mutation-evidence-empty"
	CodeMutationRowMismatch     = "mutation-row-mismatch"
	CodeMissingAttemptEvidence  = "missing-attempt-evidence"
	CodeInvalidMutationOutcome  = "invalid-mutation-outcome"
	CodeInvalidVerdict          = "invalid-verdict"
)

type RefusalError struct {
	Code   string
	Detail string
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

func refuse(code, format string, args ...any) *RefusalError {
	return &RefusalError{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// Validate checks row accounting and layer-2/3/4 bookkeeping over in,
// returning the first violation found in a fixed order: evidence staleness,
// freshness, row universe, layer 2, layer 3, layer 4.
func Validate(in *Inputs) error {
	active := ActiveRows(in.Source)
	activeSet := make(map[string]bool, len(active))
	for _, r := range active {
		activeSet[r.ID] = true
	}
	gaps := Layer1Gaps(in.Trace)

	if err := validateStaleness(in); err != nil {
		return err
	}
	if err := validateFreshness(in.Freshness); err != nil {
		return err
	}
	if err := validateRowUniverse(in, activeSet); err != nil {
		return err
	}
	if err := validateLayer2(in, active, gaps); err != nil {
		return err
	}
	if err := validateLayer3(in, active, gaps); err != nil {
		return err
	}
	if err := validateLayer4(in, active, gaps); err != nil {
		return err
	}
	return nil
}

func validateStaleness(in *Inputs) error {
	if in.Trace.RunID != in.RunID {
		return refuse(CodeStaleEvidence, "trace runId %q does not match request runId %q", in.Trace.RunID, in.RunID)
	}
	if in.Trace.SourceSHA256 != in.Source.SHA256 {
		return refuse(CodeStaleEvidence, "trace sourceSha256 %q does not match source sha256 %q", in.Trace.SourceSHA256, in.Source.SHA256)
	}
	if in.Live != nil {
		if in.Live.RunID != in.RunID {
			return refuse(CodeStaleEvidence, "live runId %q does not match request runId %q", in.Live.RunID, in.RunID)
		}
		if in.Live.SourceSHA256 != in.Source.SHA256 {
			return refuse(CodeStaleEvidence, "live sourceSha256 %q does not match source sha256 %q", in.Live.SourceSHA256, in.Source.SHA256)
		}
	}
	for _, b := range in.Batches {
		if b.RunID != in.RunID {
			return refuse(CodeStaleEvidence, "batch %s runId %q does not match request runId %q", b.BatchID, b.RunID, in.RunID)
		}
	}
	for _, m := range in.Mutations {
		if m.Evidence != nil && m.Evidence.RunID != in.RunID {
			return refuse(CodeStaleEvidence, "mutation evidence for row %s runId %q does not match request runId %q", m.RowID, m.Evidence.RunID, in.RunID)
		}
	}
	return nil
}

func validateFreshness(f Freshness) error {
	switch f.Result {
	case "":
		return refuse(CodeMissingFreshness, "freshness result is empty")
	case "fresh":
		return nil
	case "stale":
		if len(f.Stale) == 0 {
			return refuse(CodeInvalidFreshness, "freshness result is stale with no stale findings")
		}
		return nil
	default:
		return refuse(CodeInvalidFreshness, "freshness result %q outside fresh|stale", f.Result)
	}
}

func validateRowUniverse(in *Inputs, activeSet map[string]bool) error {
	for _, b := range in.Batches {
		for _, rowID := range b.Rows {
			if !activeSet[rowID] {
				return refuse(CodeUnknownRow, "batch %s binds unknown or retired row %s", b.BatchID, rowID)
			}
		}
		if b.Canary != nil && !activeSet[b.Canary.RowID] {
			return refuse(CodeUnknownRow, "batch %s canary names unknown or retired row %s", b.BatchID, b.Canary.RowID)
		}
		for _, v := range b.Verdicts {
			if !activeSet[v.RowID] {
				return refuse(CodeUnknownRow, "batch %s verdicts name unknown or retired row %s", b.BatchID, v.RowID)
			}
		}
	}
	if in.Live != nil {
		for _, v := range in.Live.RowVerdicts {
			if !activeSet[v.RowID] {
				return refuse(CodeUnknownRow, "live evidence names unknown or retired row %s", v.RowID)
			}
		}
	}
	for _, m := range in.Mutations {
		if !activeSet[m.RowID] {
			return refuse(CodeUnknownRow, "layer4 entry names unknown or retired row %s", m.RowID)
		}
	}
	return nil
}

func validateLayer2(in *Inputs, active []coverage.Row, gaps map[string]bool) error {
	byID := make(map[string]Batch, len(in.Batches))
	for _, b := range in.Batches {
		byID[b.BatchID] = b
	}

	voided := voidedBatches(in.Batches)

	for _, b := range in.Batches {
		for _, v := range b.Verdicts {
			if !validCriticVerdict(v.Verdict) {
				return refuse(CodeInvalidVerdict, "batch %s verdicts row %s with %q, outside HOLDS|WEAK|VACUOUS|MISSING", b.BatchID, v.RowID, v.Verdict)
			}
		}
	}

	for _, b := range in.Batches {
		if b.Canary != nil && !b.Canary.Caught && !voided[b.BatchID] {
			return refuse(CodeUncaughtCanaryNotVoided, "batch %s has an uncaught canary and is not voided", b.BatchID)
		}
	}

	for _, row := range active {
		var nonVoided, onlyVoided []CriticVerdict
		for _, b := range in.Batches {
			for _, v := range b.Verdicts {
				if v.RowID != row.ID {
					continue
				}
				if voided[b.BatchID] {
					onlyVoided = append(onlyVoided, v)
				} else {
					nonVoided = append(nonVoided, v)
				}
			}
		}
		switch {
		case len(nonVoided) > 1:
			return refuse(CodeDuplicateVerdict, "row %s has %d non-voided layer-2 verdicts", row.ID, len(nonVoided))
		case len(nonVoided) == 0 && len(onlyVoided) > 0:
			return refuse(CodeVoidedVerdict, "row %s is verdicted only by voided batches", row.ID)
		case len(nonVoided) == 0:
			return refuse(CodeMissingVerdict, "row %s has no layer-2 verdict", row.ID)
		case gaps[row.ID] && nonVoided[0].Verdict != "MISSING":
			return refuse(CodeMissingVerdict, "row %s is a layer-1 gap and must carry a MISSING layer-2 verdict, got %q", row.ID, nonVoided[0].Verdict)
		}
	}

	for _, b := range in.Batches {
		if err := validateCanaryBookkeeping(b, byID, voided); err != nil {
			return err
		}
	}
	return nil
}

func validateCanaryBookkeeping(b Batch, byID map[string]Batch, voided map[string]bool) error {
	if b.Canary == nil {
		if b.InheritsFrom == "" {
			return refuse(CodeCanaryBookkeeping, "canary-free batch %s does not inherit from a caught-canary batch", b.BatchID)
		}
		inherited, ok := byID[b.InheritsFrom]
		if !ok || inherited.Canary == nil || !inherited.Canary.Caught {
			return refuse(CodeCanaryBookkeeping, "batch %s inheritsFrom %s, which is not a caught-canary batch", b.BatchID, b.InheritsFrom)
		}
		return nil
	}

	if !slices.Contains(b.Rows, b.Canary.RowID) {
		return refuse(CodeCanaryBookkeeping, "batch %s carries a canary on row %s, which the batch does not dispatch", b.BatchID, b.Canary.RowID)
	}

	switch b.Canary.Kind {
	case "synthesized":
		if b.Canary.Weakening == "" {
			return refuse(CodeCanaryBookkeeping, "batch %s synthesized canary has empty weakening", b.BatchID)
		}
		if !hasRealSourceVerdict(b, byID, voided) {
			return refuse(CodeCanaryBookkeeping, "batch %s synthesized canary row %s has no separate real-source verdict", b.BatchID, b.Canary.RowID)
		}
	case "natural":
		if b.Canary.Caught {
			for _, other := range byID {
				for _, v := range other.Verdicts {
					if v.RowID == b.Canary.RowID && v.Verdict == "HOLDS" {
						return refuse(CodeCanaryBookkeeping, "batch %s natural canary row %s is caught, yet batch %s verdicts it HOLDS", b.BatchID, b.Canary.RowID, other.BatchID)
					}
				}
			}
		}
	}
	return nil
}

func validCriticVerdict(verdict string) bool {
	switch verdict {
	case "HOLDS", "WEAK", "VACUOUS", "MISSING":
		return true
	}
	return false
}

func validLiveVerdict(verdict string) bool {
	switch verdict {
	case "LIVE-PASS", "LIVE-FAIL", "LIVE-INCONCLUSIVE", valueLiveGap:
		return true
	}
	return false
}

func hasRealSourceVerdict(canaryBatch Batch, byID map[string]Batch, voided map[string]bool) bool {
	for id, b := range byID {
		if id == canaryBatch.BatchID || voided[id] {
			continue
		}
		if len(b.Rows) != 1 || b.Rows[0] != canaryBatch.Canary.RowID {
			continue
		}
		for _, v := range b.Verdicts {
			if v.RowID == canaryBatch.Canary.RowID {
				return true
			}
		}
	}
	return false
}

func validateLayer3(in *Inputs, active []coverage.Row, gaps map[string]bool) error {
	var liveRows []coverage.Row
	liveApplicable := make(map[string]bool)
	for _, r := range active {
		if r.Level == "smoke" || r.Level == "e2e" {
			liveRows = append(liveRows, r)
			liveApplicable[r.ID] = true
		}
	}

	if len(liveRows) == 0 {
		if in.Live != nil {
			return refuse(CodeUnexpectedLiveEvidence, "live evidence supplied but no smoke/e2e active rows exist")
		}
		return nil
	}

	if in.Live == nil {
		if in.LiveAttempt == nil {
			return refuse(CodeMissingVerdict, "layer 3 has applicable rows but no live evidence or attempt was supplied")
		}
		return nil
	}

	counts := make(map[string]int, len(in.Live.RowVerdicts))
	for _, v := range in.Live.RowVerdicts {
		counts[v.RowID]++
	}
	for _, v := range in.Live.RowVerdicts {
		if counts[v.RowID] > 1 {
			return refuse(CodeDuplicateVerdict, "row %s appears more than once in live rowVerdicts", v.RowID)
		}
	}

	for _, v := range in.Live.RowVerdicts {
		if !liveApplicable[v.RowID] {
			return refuse(CodeUnexpectedLiveEvidence, "live rowVerdicts names row %s, which is not a smoke/e2e active row", v.RowID)
		}
	}

	byRow := make(map[string]LiveRowVerdict, len(in.Live.RowVerdicts))
	for _, v := range in.Live.RowVerdicts {
		byRow[v.RowID] = v
	}
	for _, row := range liveRows {
		v, ok := byRow[row.ID]
		if !ok {
			return refuse(CodeMissingVerdict, "row %s is layer-3 applicable but absent from live rowVerdicts", row.ID)
		}
		if !validLiveVerdict(v.Verdict) {
			return refuse(CodeInvalidVerdict, "row %s carries live verdict %q, outside LIVE-PASS|LIVE-FAIL|LIVE-INCONCLUSIVE|%s", row.ID, v.Verdict, valueLiveGap)
		}
		if gaps[row.ID] && v.Verdict != valueLiveGap {
			return refuse(CodeMissingVerdict, "row %s is a layer-1 gap and must carry %s, got %q", row.ID, valueLiveGap, v.Verdict)
		}
		if !gaps[row.ID] && v.Verdict == valueLiveGap {
			return refuse(CodeInvalidVerdict, "row %s is not a layer-1 gap and must not carry %s", row.ID, valueLiveGap)
		}
	}
	return nil
}

func validateLayer4(in *Inputs, active []coverage.Row, gaps map[string]bool) error {
	var mutRows []coverage.Row
	for _, r := range active {
		if r.Risk == "high" {
			mutRows = append(mutRows, r)
		}
	}

	counts := make(map[string]int, len(in.Mutations))
	for _, m := range in.Mutations {
		counts[m.RowID]++
	}
	for _, m := range in.Mutations {
		if counts[m.RowID] > 1 {
			return refuse(CodeDuplicateVerdict, "row %s supplied more than once in layer4.rows", m.RowID)
		}
	}

	for _, m := range in.Mutations {
		if gaps[m.RowID] {
			return refuse(CodeMutationOnGapRow, "row %s is a layer-1 gap and must not carry mutation evidence", m.RowID)
		}
		if m.EvidencePath != "" && m.Evidence == nil {
			return refuse(CodeInvalidVerdict, "row %s names mutation evidence %s that was not loaded", m.RowID, m.EvidencePath)
		}
	}

	byRow := make(map[string]MutationInput, len(in.Mutations))
	for _, m := range in.Mutations {
		byRow[m.RowID] = m
	}

	for _, row := range mutRows {
		if gaps[row.ID] {
			continue
		}
		m, ok := byRow[row.ID]
		if !ok || (m.Outcome == "" && m.EvidencePath == "") {
			return refuse(CodeMissingVerdict, "row %s is layer-4 applicable but carries no mutation entry", row.ID)
		}
	}

	for _, row := range mutRows {
		if gaps[row.ID] {
			continue
		}
		m := byRow[row.ID]

		if m.Evidence != nil {
			if len(m.Evidence.Sites) == 0 {
				return refuse(CodeMutationEvidenceEmpty, "row %s mutation evidence has zero sites", row.ID)
			}
			if m.Evidence.RowID != row.ID {
				return refuse(CodeMutationRowMismatch, "row %s mutation evidence names row %s", row.ID, m.Evidence.RowID)
			}
		}

		switch m.Outcome {
		case "", "unappliable":
		case "environmental":
			if m.AttemptEvidencePath == "" {
				return refuse(CodeMissingAttemptEvidence, "row %s outcome environmental has no attemptEvidencePath", row.ID)
			}
		default:
			return refuse(CodeInvalidMutationOutcome, "row %s has invalid outcome %q", row.ID, m.Outcome)
		}
	}
	return nil
}
