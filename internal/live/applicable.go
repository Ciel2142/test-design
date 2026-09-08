package live

import "test-design/internal/coverage"

// ApplicableRows returns the layer-3 obligations of src in source order and
// without duplicates.
//
// With decision required the obligation is every active row a journey names
// plus every active smoke or e2e row. A journeyed unit, component or
// integration row is as much an obligation as a smoke row, and taking the
// union rather than the journeys alone keeps a smoke row inside the set even
// if no journey happens to name it.
//
// With decision not-applicable there is no obligation at all. A source that
// carries no decision is legacy: the live ops refuse it before reaching here,
// so the old level rule stands in as the answer rather than a silent empty
// set.
//
// A journey reference to a retired row or to a row this document does not own
// is skipped, never credited, and every row is returned with the level it was
// written with — applicability never relabels a row.
func ApplicableRows(src *coverage.Source) []coverage.Row {
	if src == nil {
		return nil
	}
	if src.LiveDecision != nil && src.LiveDecision.Decision == coverage.LiveNotApplicable {
		return nil
	}

	retired := make(map[string]bool, len(src.Retired))
	for _, id := range src.Retired {
		retired[id] = true
	}

	journeyed := make(map[string]bool)
	if src.LiveDecision != nil && src.LiveDecision.Decision == coverage.LiveRequired {
		for _, journey := range src.LiveDecision.Journeys {
			for _, id := range journey.Scenarios {
				journeyed[id] = true
			}
		}
	}

	// Walking src.Rows once, in source order, is what makes the result
	// duplicate-free: a row named by two journeys, named twice by one journey,
	// or both journeyed and selected by its level is still visited once here.
	rows := make([]coverage.Row, 0, len(src.Rows))
	for _, row := range src.Rows {
		if retired[row.ID] {
			continue
		}
		if journeyed[row.ID] || row.Level == "smoke" || row.Level == "e2e" {
			rows = append(rows, row)
		}
	}
	return rows
}
