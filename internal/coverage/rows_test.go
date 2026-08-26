package coverage

import (
	"strings"
	"testing"
)

func wantErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

func validScenarioBlock(rows ...tableRow) tableBlock {
	return tableBlock{
		header: []string{"ID", "Behavior", "Expected", "Level", "Verification mutation", "Risk"},
		rows:   rows,
	}
}

func TestParseRowAcceptsWellFormedRow(t *testing.T) {
	row, err := parseRow("path.md", tableRow{
		cells: []string{"orders/SC-07", "does a thing", "does the thing", "unit", "flips the guard", "high"},
		line:  9,
	})
	if err != nil {
		t.Fatalf("parseRow: %v", err)
	}
	want := Row{ID: "orders/SC-07", Behavior: "does a thing", Expected: "does the thing", Level: "unit", Mutation: "flips the guard", Risk: "high"}
	if row != want {
		t.Errorf("parseRow = %+v, want %+v", row, want)
	}
}

func TestParseRowRejectsWrongColumnCount(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-01", "b", "e", "unit", "m"},
		line:  4,
	})
	wantErrContains(t, err, "expected 6 columns, got 5")
}

func TestParseRowRejectsMalformedID(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-abc", "b", "e", "unit", "m", "low"},
		line:  9,
	})
	wantErrContains(t, err, "SC-abc")
	wantErrContains(t, err, "9")
	wantErrContains(t, err, "ID")
}

func TestParseRowRejectsUnknownLevel(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-01", "b", "e", "unittest", "m", "low"},
		line:  4,
	})
	wantErrContains(t, err, "unittest")
	wantErrContains(t, err, "Level")
}

func TestParseRowRejectsUnknownRisk(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-01", "b", "e", "unit", "m", "critical"},
		line:  4,
	})
	wantErrContains(t, err, "critical")
	wantErrContains(t, err, "Risk")
}

func TestParseRowRejectsEmptyMutation(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-01", "b", "e", "unit", "   ", "low"},
		line:  4,
	})
	wantErrContains(t, err, "Verification mutation")
}

func TestParseRowRejectsEmptyBehavior(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-01", "  ", "e", "unit", "m", "low"},
		line:  4,
	})
	wantErrContains(t, err, "Behavior")
}

func TestParseRowRejectsEmptyExpected(t *testing.T) {
	_, err := parseRow("path.md", tableRow{
		cells: []string{"SC-01", "b", "   ", "unit", "m", "low"},
		line:  4,
	})
	wantErrContains(t, err, "Expected")
}

func TestExtractRowsIgnoresNonMatchingTable(t *testing.T) {
	blocks := []tableBlock{
		{
			header: []string{"Test", "Status"},
			rows: []tableRow{
				{cells: []string{"not-an-id", "irrelevant"}, line: 2},
			},
		},
		validScenarioBlock(tableRow{
			cells: []string{"SC-01", "b", "e", "unit", "m", "low"},
			line:  6,
		}),
	}

	rows, err := extractRows("path.md", blocks)
	if err != nil {
		t.Fatalf("extractRows: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "SC-01" {
		t.Fatalf("extractRows = %+v, want single SC-01 row", rows)
	}
}

func TestExtractRowsAccumulatesFromEveryMatchingTable(t *testing.T) {
	blocks := []tableBlock{
		validScenarioBlock(tableRow{
			cells: []string{"SC-01", "b1", "e1", "unit", "m1", "low"},
			line:  3,
		}),
		{
			header: []string{"Notes"},
			rows:   []tableRow{{cells: []string{"irrelevant"}, line: 8}},
		},
		validScenarioBlock(tableRow{
			cells: []string{"SC-02", "b2", "e2", "component", "m2", "high"},
			line:  12,
		}),
	}

	rows, err := extractRows("path.md", blocks)
	if err != nil {
		t.Fatalf("extractRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("extractRows returned %d rows, want 2", len(rows))
	}
	if rows[0].ID != "SC-01" || rows[1].ID != "SC-02" {
		t.Errorf("extractRows = %+v, want SC-01 then SC-02 in table order", rows)
	}
}

func TestExtractRowsRejectsDuplicateIDAcrossTables(t *testing.T) {
	blocks := []tableBlock{
		validScenarioBlock(tableRow{
			cells: []string{"SC-01", "b1", "e1", "unit", "m1", "low"},
			line:  3,
		}),
		validScenarioBlock(tableRow{
			cells: []string{"SC-01", "b2", "e2", "unit", "m2", "low"},
			line:  9,
		}),
	}

	rows, err := extractRows("doc.md", blocks)
	if err == nil {
		t.Fatalf("extractRows returned nil error with %+v, want error", rows)
	}
	if rows != nil {
		t.Errorf("extractRows returned %+v alongside error, want nil rows", rows)
	}
	wantErrContains(t, err, "doc.md")
	wantErrContains(t, err, "SC-01")
}
