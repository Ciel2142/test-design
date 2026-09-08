package live

import (
	"strings"
	"testing"

	"test-design/internal/coverage"
)

// journeySource builds a source whose rows cover every level the applicability
// rule has to distinguish: SC-1 unit, SC-2 smoke, SC-3 integration, SC-4 e2e,
// SC-5 component, plus the retired smoke row SC-90. decision is attached as
// written so each case can vary only the live decision.
func journeySource(decision *coverage.LiveDecision) *coverage.Source {
	return &coverage.Source{
		Path: "docs/coverage.md",
		Rows: []coverage.Row{
			{ID: "SC-1", Level: "unit"},
			{ID: "SC-2", Level: "smoke"},
			{ID: "SC-3", Level: "integration"},
			{ID: "SC-4", Level: "e2e"},
			{ID: "SC-5", Level: "component"},
			{ID: "SC-90", Level: "smoke"},
		},
		Retired:      []string{"SC-90"},
		LiveDecision: decision,
	}
}

// idsAndLevels renders rows as "ID/level" in the order returned, so a failure
// reports both the selection and the level each row kept.
func idsAndLevels(rows []coverage.Row) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, row.ID+"/"+row.Level)
	}
	return strings.Join(parts, ", ")
}

// covers: SC-07
func TestApplicableRowsFollowRequiredJourneys(t *testing.T) {
	tests := []struct {
		name     string
		decision *coverage.LiveDecision
		want     string
	}{
		{
			name: "required journeys widen the set past smoke and e2e",
			decision: &coverage.LiveDecision{
				Decision: coverage.LiveRequired,
				Journeys: []coverage.Journey{
					{ID: "J-1", Scenarios: []string{"SC-3", "SC-1", "SC-3"}, Expected: "the recipe renders"},
					{ID: "J-2", Scenarios: []string{"SC-1", "SC-2"}, Expected: "the service refuses an unknown recipe"},
				},
			},
			want: "SC-1/unit, SC-2/smoke, SC-3/integration, SC-4/e2e",
		},
		{
			name: "retired and unknown journey references are not credited",
			decision: &coverage.LiveDecision{
				Decision: coverage.LiveRequired,
				Journeys: []coverage.Journey{
					{ID: "J-1", Scenarios: []string{"SC-90", "SC-77", "SC-3"}, Expected: "the recipe renders"},
				},
			},
			want: "SC-2/smoke, SC-3/integration, SC-4/e2e",
		},
		{
			name:     "a not-applicable decision leaves no obligation",
			decision: &coverage.LiveDecision{Decision: coverage.LiveNotApplicable, Reason: "no deployable surface"},
			want:     "",
		},
		{
			name:     "a source with no decision falls back to the active smoke and e2e rows",
			decision: nil,
			want:     "SC-2/smoke, SC-4/e2e",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := idsAndLevels(ApplicableRows(journeySource(tc.decision)))
			if got != tc.want {
				t.Errorf("ApplicableRows() = [%s], want [%s]", got, tc.want)
			}
		})
	}
}
