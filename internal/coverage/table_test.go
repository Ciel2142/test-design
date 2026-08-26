package coverage

import "testing"

func TestHeaderSignatureMatchesCaseAndWhitespace(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		want   bool
	}{
		{
			name:   "exact match",
			header: []string{"ID", "Behavior", "Expected", "Level", "Verification mutation", "Risk"},
			want:   true,
		},
		{
			name:   "case-insensitive and padded",
			header: []string{" id ", "BEHAVIOR", "expected", " Level", "verification MUTATION", "risk "},
			want:   true,
		},
		{
			name:   "wrong column count",
			header: []string{"ID", "Behavior", "Expected", "Level", "Risk"},
			want:   false,
		},
		{
			name:   "wrong column order",
			header: []string{"ID", "Behavior", "Level", "Expected", "Verification mutation", "Risk"},
			want:   false,
		},
		{
			name:   "old header does not match new signature",
			header: []string{"ID", "Behavior", "Bug it catches", "Level", "Expected", "Risk"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scenarioHeaderSignature.matches(tt.header)
			if got != tt.want {
				t.Errorf("scenarioHeaderSignature.matches(%v) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestOldHeaderSignatureMatchesOldColumnOrder(t *testing.T) {
	header := []string{"ID", "Behavior", "Bug it catches", "Level", "Expected", "Risk"}
	if !oldScenarioHeaderSignature.matches(header) {
		t.Errorf("oldScenarioHeaderSignature.matches(%v) = false, want true", header)
	}
	if oldScenarioHeaderSignature.matches(scenarioHeaderSignature) {
		t.Errorf("oldScenarioHeaderSignature.matches(new header) = true, want false")
	}
}

func TestScanTablesFindsSingleTable(t *testing.T) {
	lines := []string{
		"# Title",
		"",
		"| ID | Behavior | Expected | Level | Verification mutation | Risk |",
		"|----|----------|----------|-------|------------------------|------|",
		"| SC-01 | b | e | unit | m | low |",
		"| SC-02 | b2 | e2 | unit | m2 | low |",
		"",
		"trailer prose",
	}

	blocks := scanTables(lines)
	if len(blocks) != 1 {
		t.Fatalf("scanTables found %d blocks, want 1", len(blocks))
	}
	if len(blocks[0].rows) != 2 {
		t.Fatalf("scanTables block has %d rows, want 2", len(blocks[0].rows))
	}
	if blocks[0].rows[0].cells[0] != "SC-01" {
		t.Errorf("first row ID = %q, want SC-01", blocks[0].rows[0].cells[0])
	}
	if blocks[0].rows[1].cells[0] != "SC-02" {
		t.Errorf("second row ID = %q, want SC-02", blocks[0].rows[1].cells[0])
	}
}

func TestScanTablesFindsMultipleTablesInOrder(t *testing.T) {
	lines := []string{
		"| ID | Behavior | Expected | Level | Verification mutation | Risk |",
		"|----|----------|----------|-------|------------------------|------|",
		"| SC-01 | b | e | unit | m | low |",
		"",
		"| Test | Status |",
		"|------|--------|",
		"| not-an-id | ignored |",
		"",
		"| ID | Behavior | Expected | Level | Verification mutation | Risk |",
		"|----|----------|----------|-------|------------------------|------|",
		"| SC-02 | b | e | unit | m | low |",
	}

	blocks := scanTables(lines)
	if len(blocks) != 3 {
		t.Fatalf("scanTables found %d blocks, want 3", len(blocks))
	}
	if !scenarioHeaderSignature.matches(blocks[0].header) {
		t.Errorf("block 0 header %v does not match scenario signature", blocks[0].header)
	}
	if scenarioHeaderSignature.matches(blocks[1].header) {
		t.Errorf("block 1 header %v unexpectedly matches scenario signature", blocks[1].header)
	}
	if !scenarioHeaderSignature.matches(blocks[2].header) {
		t.Errorf("block 2 header %v does not match scenario signature", blocks[2].header)
	}
}

func TestScanTablesIgnoresRowWithoutValidSeparator(t *testing.T) {
	lines := []string{
		"| ID | Behavior | Expected | Level | Verification mutation | Risk |",
		"this is not a separator row",
		"| SC-01 | b | e | unit | m | low |",
	}

	blocks := scanTables(lines)
	if len(blocks) != 0 {
		t.Fatalf("scanTables found %d blocks, want 0 (no valid separator row)", len(blocks))
	}
}

func TestScanTablesRecordsLineNumbers(t *testing.T) {
	lines := []string{
		"prose",
		"| ID | Behavior | Expected | Level | Verification mutation | Risk |",
		"|----|----------|----------|-------|------------------------|------|",
		"| SC-01 | b | e | unit | m | low |",
	}

	blocks := scanTables(lines)
	if len(blocks) != 1 {
		t.Fatalf("scanTables found %d blocks, want 1", len(blocks))
	}
	if blocks[0].rows[0].line != 4 {
		t.Errorf("row line = %d, want 4 (1-indexed)", blocks[0].rows[0].line)
	}
}
