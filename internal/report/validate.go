package report

import (
	"fmt"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/live"
)

const (
	CodeStaleEvidence          = "stale-evidence"
	CodeMissingFreshness       = "missing-freshness"
	CodeInvalidFreshness       = "invalid-freshness"
	CodeDuplicateVerdict       = "duplicate-verdict"
	CodeMissingVerdict         = "missing-verdict"
	CodeUnknownRow             = "unknown-row"
	CodeUnexpectedLiveEvidence = "unexpected-live-evidence"
	CodeMutationOnGapRow       = "mutation-on-gap-row"
	CodeMutationEvidenceEmpty  = "mutation-evidence-empty"
	CodeMutationRowMismatch    = "mutation-row-mismatch"
	CodeMissingAttemptEvidence = "missing-attempt-evidence"
	CodeInvalidMutationOutcome = "invalid-mutation-outcome"
	CodeInvalidVerdict         = "invalid-verdict"
	CodeLiveDecisionIncomplete = "live-decision-incomplete"
	// CodeReuseNotAdmitted refuses a parent-pasted batch or layer-4 row that
	// carries a reuse reference: only reuse itself may adopt a prior record.
	CodeReuseNotAdmitted = "reuse-not-admitted"
	// CodeReuseIdentityMismatch refuses an adopted record whose own identity
	// differs from the prior identity its reuse reference names.
	CodeReuseIdentityMismatch = "reuse-identity-mismatch"
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
	if err := validateLayer3(in); err != nil {
		return err
	}
	if err := validateLayer4(in, active, gaps); err != nil {
		return err
	}
	return nil
}

func validateStaleness(in *Inputs) error {
	matches := func(kind, runID, commit, source, sha string) error {
		if runID == "" || commit == "" || source == "" || sha == "" ||
			runID != in.RunID || commit != in.Trace.RepoCommit ||
			source != in.Source.Path || sha != in.Source.SHA256 {
			return refuse(CodeStaleEvidence, "%s identity does not match trace commit/source/run; regenerate evidence", kind)
		}
		return nil
	}
	if err := matches("trace", in.Trace.RunID, in.Trace.RepoCommit, in.Trace.Source, in.Trace.SourceSHA256); err != nil {
		return err
	}
	if in.HeadCommit == "" || in.Trace.RepoCommit != in.HeadCommit {
		return refuse(CodeStaleEvidence, "trace commit does not match measured HEAD")
	}
	if ev := in.Live; ev != nil {
		if err := matches("live", ev.RunID, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
			return err
		}
	}
	if ev := in.LiveAttempt; ev != nil {
		if err := matches("live attempt", ev.RunID, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
			return err
		}
	}
	for _, b := range in.Batches {
		if b.Reuse != nil {
			// An adopted batch keeps the prior run's identity; it is measured
			// against its reuse reference, never against this run.
			if b.RunID != b.Reuse.PriorRunID {
				return refuse(CodeReuseIdentityMismatch, "batch %s runId %q does not match its reuse reference prior run %q", b.BatchID, b.RunID, b.Reuse.PriorRunID)
			}
			continue
		}
		if b.RunID != in.RunID {
			return refuse(CodeStaleEvidence, "batch %s runId %q does not match request runId %q", b.BatchID, b.RunID, in.RunID)
		}
	}
	for _, m := range in.Mutations {
		if m.Reuse != nil {
			if err := matchesReuse(m, m.Reuse); err != nil {
				return err
			}
			if ev := m.Attempt; ev != nil && ev.RowID != m.RowID {
				return refuse(CodeMutationRowMismatch, "row %s mutation attempt names row %s", m.RowID, ev.RowID)
			}
			continue
		}
		if ev := m.Evidence; ev != nil {
			if err := matches("mutation", ev.RunID, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
				return err
			}
		}
		if ev := m.Attempt; ev != nil {
			if err := matches("mutation attempt", ev.RunID, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
				return err
			}
			if ev.RowID != m.RowID {
				return refuse(CodeMutationRowMismatch, "row %s mutation attempt names row %s", m.RowID, ev.RowID)
			}
		}
	}
	return nil
}

// matchesReuse checks an adopted layer-4 entry's loaded evidence against the
// prior identity its reuse reference names, not against this run's trace.
func matchesReuse(m MutationInput, ref *evidence.ReuseReference) error {
	check := func(kind, runID, commit, sha string) error {
		if runID != ref.PriorRunID || commit != ref.PriorRepoCommit || sha != ref.PriorSourceSHA256 {
			return refuse(CodeReuseIdentityMismatch,
				"row %s reused %s identity (run %q, commit %q, source %q) does not match its reuse reference (run %q, commit %q, source %q)",
				m.RowID, kind, runID, commit, sha, ref.PriorRunID, ref.PriorRepoCommit, ref.PriorSourceSHA256)
		}
		return nil
	}
	if ev := m.Evidence; ev != nil {
		if err := check("mutation", ev.RunID, ev.RepoCommit, ev.SourceSHA256); err != nil {
			return err
		}
	}
	if ev := m.Attempt; ev != nil {
		if err := check("mutation attempt", ev.RunID, ev.RepoCommit, ev.SourceSHA256); err != nil {
			return err
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
	for _, b := range in.Batches {
		for _, v := range b.Verdicts {
			if !validCriticVerdict(v.Verdict) {
				return refuse(CodeInvalidVerdict, "batch %s verdicts row %s with %q, outside HOLDS|WEAK|VACUOUS|MISSING", b.BatchID, v.RowID, v.Verdict)
			}
		}
	}

	for _, row := range active {
		var verdicts []CriticVerdict
		for _, b := range in.Batches {
			for _, v := range b.Verdicts {
				if v.RowID == row.ID {
					verdicts = append(verdicts, v)
				}
			}
		}
		switch {
		case len(verdicts) > 1:
			return refuse(CodeDuplicateVerdict, "row %s has %d layer-2 verdicts", row.ID, len(verdicts))
		case len(verdicts) == 0:
			return refuse(CodeMissingVerdict, "row %s has no layer-2 verdict", row.ID)
		case gaps[row.ID] && verdicts[0].Verdict != "MISSING":
			return refuse(CodeMissingVerdict, "row %s is a layer-1 gap and must carry a MISSING layer-2 verdict, got %q", row.ID, verdicts[0].Verdict)
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

// validateLayer3 checks the live evidence against the layer-3 obligation the
// coverage source itself declares. The applicable set comes from
// live.ApplicableRows and from nothing else: a row is owed a live verdict
// because a required journey names it or because the decision is absent and
// its level is smoke or e2e, never because of the level alone.
//
// A declared decision is checked before it is trusted. Finalize sizes layer 3
// from the decision, so a decision no design-time validator ever accepted must
// not size anything: a not-applicable one would zero the layer outright, and
// an incomplete required one measures the document against journeys it never
// finished declaring. A source carrying no section at all is legacy, falls
// through, and keeps the level rule.
func validateLayer3(in *Inputs) error {
	if in.Source != nil && in.Source.LiveDecision != nil {
		if err := coverage.ValidateLiveDecision(in.Source); err != nil {
			return refuse(CodeLiveDecisionIncomplete, "the coverage source's live decision cannot be trusted to size layer 3: %v", err)
		}
	}

	liveRows := live.ApplicableRows(in.Source)
	liveApplicable := make(map[string]bool, len(liveRows))
	for _, r := range liveRows {
		liveApplicable[r.ID] = true
	}

	if len(liveRows) == 0 {
		if in.Live != nil {
			return refuse(CodeUnexpectedLiveEvidence, "live evidence supplied but no applicable live rows exist")
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
			return refuse(CodeUnexpectedLiveEvidence, "live rowVerdicts names row %s, which is not an applicable live row", v.RowID)
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
	}
	return nil
}

func validateLayer4(in *Inputs, active []coverage.Row, gaps map[string]bool) error {
	mutRows := layer4Applicable(active, in.Mutations)

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
		if m.AttemptEvidencePath != "" && m.Attempt == nil {
			return refuse(CodeMissingAttemptEvidence, "row %s names attempt evidence that was not loaded", m.RowID)
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
