package report

import "test-design/internal/coverage"

// ActiveRows returns src.Rows minus src.Retired, in source order.
func ActiveRows(src *coverage.Source) []coverage.Row {
	retired := make(map[string]bool, len(src.Retired))
	for _, id := range src.Retired {
		retired[id] = true
	}
	active := make([]coverage.Row, 0, len(src.Rows))
	for _, r := range src.Rows {
		if !retired[r.ID] {
			active = append(active, r)
		}
	}
	return active
}

// Layer1Gaps returns the set of row IDs whose layer-1 status gates layer 2/3/4
// as a gap: a gap or ambiguity finding in tr, or a row listed in
// tr.RowAccounting.Unaccounted.
func Layer1Gaps(tr *TraceEvidence) map[string]bool {
	gaps := make(map[string]bool)
	if tr == nil {
		return gaps
	}
	for _, f := range tr.Findings {
		if f.RowID == "" {
			continue
		}
		if f.Kind == "gap" || f.Kind == "ambiguity" {
			gaps[f.RowID] = true
		}
	}
	for _, id := range tr.RowAccounting.Unaccounted {
		gaps[id] = true
	}
	return gaps
}
