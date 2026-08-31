package report

import (
	"fmt"
	"sort"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

const (
	VerdictPass    = "PASS"
	VerdictBlocked = "BLOCKED"
	VerdictPartial = "PARTIAL"
)

const (
	layerFreshness = "freshness"
	layer1         = "1"
	layer2         = "2"
	layer3         = "3"
	layer4         = "4"
)

const (
	statusPass    = "pass"
	statusBlocked = "blocked"
	statusPartial = "partial"
	statusNA      = "n/a"
)

const (
	valueNA                  = "N/A"
	valueLiveProfileNA       = "N/A (profile unavailable)"
	valueMutationCaught      = "MUTATION-CAUGHT"
	valueMutationSurvived    = "MUTATION-SURVIVED"
	valueMutationUnappliable = "MUTATION-UNAPPLIABLE"
	valueMutationGap         = "MUTATION-N/A (layer 1 gap)"
	valueMutationEnvironment = "MUTATION-N/A (environmental)"
)

type severity int

const (
	sevNone severity = iota
	sevPartial
	sevBlocked
)

type reasonSet struct {
	list  []Reason
	worst map[string]severity
	max   severity
}

func newReasonSet() *reasonSet {
	return &reasonSet{list: []Reason{}, worst: map[string]severity{}}
}

func (rs *reasonSet) add(sev severity, layer, rowID, code, format string, args ...any) {
	rs.list = append(rs.list, Reason{Layer: layer, RowID: rowID, Code: code, Detail: fmt.Sprintf(format, args...)})
	if sev > rs.worst[layer] {
		rs.worst[layer] = sev
	}
	if sev > rs.max {
		rs.max = sev
	}
}

func (rs *reasonSet) status(layer string) string {
	switch rs.worst[layer] {
	case sevBlocked:
		return statusBlocked
	case sevPartial:
		return statusPartial
	default:
		return statusPass
	}
}

// Compute validates in and derives the canonical verification report: one
// verdict, ordered reasons, per-layer summaries and one row per active row.
// It leaves the fields the op owns — approval, transcripts, tool versions and
// finishedAt — untouched.
func Compute(in *Inputs) (*Report, error) {
	if err := Validate(in); err != nil {
		return nil, err
	}

	active := ActiveRows(in.Source)
	gaps := Layer1Gaps(in.Trace)
	rs := newReasonSet()

	addFreshnessReason(rs, in.Freshness)
	l1 := deriveLayer1(rs, in.Trace, active)
	l2 := deriveLayer2(rs, in, active)
	l3, l3sum := deriveLayer3(rs, in, active)
	l4, l4sum := deriveLayer4(rs, in, active, gaps)

	rows := make([]RowReport, 0, len(active))
	for _, row := range active {
		rows = append(rows, RowReport{
			RowID:  row.ID,
			Level:  row.Level,
			Risk:   row.Risk,
			Layer1: l1[row.ID],
			Layer2: l2[row.ID],
			Layer3: l3[row.ID],
			Layer4: l4[row.ID],
		})
	}

	verdict := VerdictPass
	switch rs.max {
	case sevBlocked:
		verdict = VerdictBlocked
	case sevPartial:
		verdict = VerdictPartial
	}

	return &Report{
		Envelope:        evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:           in.RunID,
		Source:          in.Source.Path,
		SourceSHA256:    in.Source.SHA256,
		HeadCommit:      in.HeadCommit,
		BaselineCommit:  in.Trace.BaselineCommit,
		BaselineStatus:  in.Trace.BaselineStatus,
		Verdict:         verdict,
		Acknowledged:    verdict == VerdictPartial && in.Acknowledgement != "",
		Acknowledgement: in.Acknowledgement,
		Reasons:         rs.list,
		Layers: LayerSummary{
			Layer1: layer1Summary(rs, in.Trace),
			Layer2: layer2Summary(rs, l2, active),
			Layer3: l3sum,
			Layer4: l4sum,
		},
		Rows:    rows,
		Batches: deriveBatches(in.Batches),
	}, nil
}

func addFreshnessReason(rs *reasonSet, f Freshness) {
	if f.Result != "stale" {
		return
	}
	descs := make([]string, 0, len(f.Stale))
	for _, s := range f.Stale {
		descs = append(descs, fmt.Sprintf("%s: %s", s.Hunk, s.Behavior))
	}
	rs.add(sevBlocked, layerFreshness, "", "stale", "the coverage source is stale against the spec: %s", strings.Join(descs, "; "))
}

func deriveLayer1(rs *reasonSet, tr *TraceEvidence, active []coverage.Row) map[string]LayerVerdict {
	if tr.ExitStatus != 0 && len(tr.RowResults) == 0 && tr.Counts == (SuiteCounts{}) {
		rs.add(sevBlocked, layer1, "", "red-runner", "the runner exited %d and produced no normalized report", tr.ExitStatus)
	}

	findings := append([]TraceFinding(nil), tr.Findings...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].RowID != findings[j].RowID {
			return findings[i].RowID < findings[j].RowID
		}
		return findings[i].Detail < findings[j].Detail
	})

	byRow := make(map[string]TraceFinding, len(findings))
	for _, f := range findings {
		if f.RowID == "" {
			rs.add(sevBlocked, layer1, "", f.Kind, "%s", f.Detail)
			continue
		}
		if _, seen := byRow[f.RowID]; !seen {
			byRow[f.RowID] = f
		}
	}

	if n := len(tr.NewFailures); n > 0 {
		names := make([]string, 0, n)
		for _, nf := range tr.NewFailures {
			names = append(names, fmt.Sprintf("%s %s", nf.Scope, nf.Name))
		}
		rs.add(sevBlocked, layer1, "", "new-failures", "%d test(s) fail here that pass on the baseline: %s", n, strings.Join(names, "; "))
	}

	if tr.BaselineStatus != "ok" {
		rs.add(sevPartial, layer1, "", "baseline-"+tr.BaselineStatus, "the baseline comparison is unavailable (baselineStatus %q)", tr.BaselineStatus)
	}

	results := make(map[string]TraceRowResult, len(tr.RowResults))
	for _, rr := range tr.RowResults {
		if _, seen := results[rr.RowID]; !seen {
			results[rr.RowID] = rr
		}
	}
	unaccounted := make(map[string]bool, len(tr.RowAccounting.Unaccounted))
	for _, id := range tr.RowAccounting.Unaccounted {
		unaccounted[id] = true
	}

	out := make(map[string]LayerVerdict, len(active))
	for _, row := range active {
		rr, hasResult := results[row.ID]
		finding, hasFinding := byRow[row.ID]

		verdict := LayerVerdict{Scope: rr.Scope, Name: rr.Name}
		detail := finding.Detail
		switch {
		case unaccounted[row.ID]:
			verdict.Status = "gap"
			if detail == "" {
				detail = "the row is unaccounted for in the trace row accounting"
			}
		case hasResult && rr.Status == "passed":
			verdict.Status = "bound"
		case hasFinding:
			verdict.Status = finding.Kind
		case hasResult && rr.Status == "failed":
			verdict.Status = "red"
			detail = fmt.Sprintf("%s %s failed with no trace finding", rr.Scope, rr.Name)
		case hasResult:
			verdict.Status = "gap"
			detail = fmt.Sprintf("%s %s did not run (%s)", rr.Scope, rr.Name, rr.Status)
		default:
			verdict.Status = "gap"
			detail = "the row has no trace result and no trace finding"
		}
		out[row.ID] = verdict

		if verdict.Status != "bound" {
			rs.add(sevBlocked, layer1, row.ID, verdict.Status, "%s", detail)
		}
	}
	return out
}

func deriveLayer2(rs *reasonSet, in *Inputs, active []coverage.Row) map[string]CriticRef {
	voided := voidedBatches(in.Batches)
	refs := make(map[string]CriticRef, len(active))
	reasons := make(map[string]string, len(active))
	for _, b := range in.Batches {
		if voided[b.BatchID] {
			continue
		}
		for _, v := range b.Verdicts {
			refs[v.RowID] = CriticRef{Verdict: v.Verdict, BatchID: b.BatchID, DispatchID: b.DispatchID}
			reasons[v.RowID] = v.Reason
		}
	}

	for _, row := range active {
		ref := refs[row.ID]
		switch ref.Verdict {
		case "WEAK", "VACUOUS", "MISSING":
			detail := reasons[row.ID]
			if detail == "" {
				detail = fmt.Sprintf("batch %s returned %s", ref.BatchID, ref.Verdict)
			}
			rs.add(sevBlocked, layer2, row.ID, ref.Verdict, "%s", detail)
		}
	}
	return refs
}

func deriveLayer3(rs *reasonSet, in *Inputs, active []coverage.Row) (map[string]string, Layer3Summary) {
	values := make(map[string]string, len(active))
	var applicable []coverage.Row
	for _, row := range active {
		values[row.ID] = valueNA
		if row.Level == "smoke" || row.Level == "e2e" {
			applicable = append(applicable, row)
		}
	}

	sum := Layer3Summary{Applicable: len(applicable)}
	if len(applicable) == 0 {
		sum.Status = statusNA
		return values, sum
	}

	if in.Live == nil {
		rs.add(sevPartial, layer3, "", "live-profile-unavailable",
			"the live profile %s was refused (%s); no live run happened", in.LiveAttempt.Profile, in.LiveAttempt.Refusal)
		for _, row := range applicable {
			values[row.ID] = valueLiveProfileNA
			sum.NA++
		}
		sum.Status = statusPartial
		return values, sum
	}

	if !in.Live.SnapshotEqual {
		rs.add(sevBlocked, layer3, "", "snapshot-changed", "the repository changed during the live run")
	}
	switch {
	case in.Live.CleanupFailed:
		rs.add(sevPartial, layer3, "", "live-cleanup-failed", "the live profile cleanup step failed")
	case !in.Live.CleanupOK:
		rs.add(sevPartial, layer3, "", "live-cleanup-missing", "the live run recorded no successful cleanup")
	}
	if len(in.Live.ExecutionErrors) > 0 {
		rs.add(sevPartial, layer3, "", "live-execution-errors", "%s", strings.Join(in.Live.ExecutionErrors, "; "))
	}

	byRow := make(map[string]LiveRowVerdict, len(in.Live.RowVerdicts))
	for _, v := range in.Live.RowVerdicts {
		byRow[v.RowID] = v
	}
	for _, row := range applicable {
		v := byRow[row.ID]
		values[row.ID] = v.Verdict
		switch v.Verdict {
		case "LIVE-PASS":
			sum.Pass++
		case "LIVE-FAIL":
			sum.Fail++
			rs.add(sevBlocked, layer3, row.ID, "LIVE-FAIL", "the live walkthrough failed")
		case "LIVE-INCONCLUSIVE":
			sum.Inconclusive++
			rs.add(sevBlocked, layer3, row.ID, "LIVE-INCONCLUSIVE", "the live walkthrough was inconclusive")
		default:
			sum.NA++
		}
		if v.RedactionBarrier {
			rs.add(sevPartial, layer3, row.ID, "redaction-barrier", "the live transcript could not be reduced without redacting evidence")
		}
	}

	sum.Status = layerStatus(rs, layer3, len(applicable), sum.NA)
	return values, sum
}

func deriveLayer4(rs *reasonSet, in *Inputs, active []coverage.Row, gaps map[string]bool) (map[string]MutationRef, Layer4Summary) {
	entries := make(map[string]MutationInput, len(in.Mutations))
	for _, m := range in.Mutations {
		entries[m.RowID] = m
	}

	refs := make(map[string]MutationRef, len(active))
	var applicable []coverage.Row
	for _, row := range active {
		refs[row.ID] = MutationRef{Status: valueNA}
		if _, supplied := entries[row.ID]; row.Risk == "high" || supplied {
			applicable = append(applicable, row)
		}
	}

	sum := Layer4Summary{Applicable: len(applicable)}
	if len(applicable) == 0 {
		sum.Status = statusNA
		return refs, sum
	}

	for _, row := range applicable {
		m := entries[row.ID]
		switch {
		case gaps[row.ID]:
			refs[row.ID] = MutationRef{Status: valueMutationGap}
			sum.NA++
		case m.Outcome == "unappliable":
			refs[row.ID] = MutationRef{Status: valueMutationUnappliable}
			sum.Unappliable++
			rs.add(sevBlocked, layer4, row.ID, "MUTATION-UNAPPLIABLE", "%s", detailOr(m.Reason, "the named mutation could not be applied"))
		case m.Outcome == "environmental":
			refs[row.ID] = MutationRef{Status: valueMutationEnvironment}
			sum.NA++
			rs.add(sevPartial, layer4, row.ID, "mutation-environmental", "%s", detailOr(m.Reason, "the mutation run failed for environmental reasons"))
		case m.Evidence != nil:
			var survivors []string
			for _, site := range m.Evidence.Sites {
				if site.Outcome == "survived" {
					survivors = append(survivors, site.Site)
				}
			}
			if len(survivors) > 0 {
				refs[row.ID] = MutationRef{Status: valueMutationSurvived, Sites: survivors}
				sum.Survived++
				rs.add(sevBlocked, layer4, row.ID, "MUTATION-SURVIVED", "the tagged test did not fail at %s", strings.Join(survivors, "; "))
			} else {
				refs[row.ID] = MutationRef{Status: valueMutationCaught}
				sum.Caught++
			}
			if !m.Evidence.SnapshotEqual {
				rs.add(sevBlocked, layer4, row.ID, "snapshot-changed", "the repository changed during the mutation run")
			}
		default:
			sum.NA++
		}
	}

	sum.Status = layerStatus(rs, layer4, len(applicable), sum.NA)
	return refs, sum
}

func layerStatus(rs *reasonSet, layer string, applicable, na int) string {
	switch {
	case applicable == 0:
		return statusNA
	case rs.worst[layer] != sevNone:
		return rs.status(layer)
	case na == applicable:
		return statusNA
	default:
		return statusPass
	}
}

func layer1Summary(rs *reasonSet, tr *TraceEvidence) Layer1Summary {
	counts := make(map[string]int, len(tr.FindingCounts))
	for kind, n := range tr.FindingCounts {
		counts[kind] = n
	}
	return Layer1Summary{
		Status:        rs.status(layer1),
		Suite:         tr.Counts,
		FindingCounts: counts,
		NewFailures:   len(tr.NewFailures),
	}
}

func layer2Summary(rs *reasonSet, refs map[string]CriticRef, active []coverage.Row) Layer2Summary {
	sum := Layer2Summary{Applicable: len(active)}
	if len(active) == 0 {
		sum.Status = statusNA
		return sum
	}
	for _, row := range active {
		switch refs[row.ID].Verdict {
		case "HOLDS":
			sum.Holds++
		case "WEAK":
			sum.Weak++
		case "VACUOUS":
			sum.Vacuous++
		case "MISSING":
			sum.Missing++
		}
	}
	sum.Status = rs.status(layer2)
	return sum
}

func deriveBatches(batches []Batch) []BatchReport {
	voided := voidedBatches(batches)
	out := make([]BatchReport, 0, len(batches))
	for _, b := range batches {
		report := BatchReport{
			BatchID:      b.BatchID,
			DispatchID:   b.DispatchID,
			RunID:        b.RunID,
			Rows:         b.Rows,
			InheritsFrom: b.InheritsFrom,
			Voided:       voided[b.BatchID],
			Verdicts:     b.Verdicts,
		}
		if b.Canary != nil {
			canary := *b.Canary
			report.Canary = &canary
		}
		out = append(out, report)
	}
	return out
}

func detailOr(detail, fallback string) string {
	if detail == "" {
		return fallback
	}
	return detail
}
