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

// voidedBatches resolves which batches are voided, following inheritsFrom
// transitively: a batch inheriting from a voided batch is itself voided.
func voidedBatches(batches []Batch) map[string]bool {
	byID := make(map[string]Batch, len(batches))
	for _, b := range batches {
		byID[b.BatchID] = b
	}
	voided := make(map[string]bool, len(batches))
	var resolve func(id string, visiting map[string]bool) bool
	resolve = func(id string, visiting map[string]bool) bool {
		if v, ok := voided[id]; ok {
			return v
		}
		b, ok := byID[id]
		if !ok || visiting[id] {
			return false
		}
		visiting[id] = true
		v := b.Voided
		if !v && b.InheritsFrom != "" {
			v = resolve(b.InheritsFrom, visiting)
		}
		voided[id] = v
		return v
	}
	for _, b := range batches {
		resolve(b.BatchID, map[string]bool{})
	}
	return voided
}
