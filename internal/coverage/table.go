package coverage

import "strings"

// headerSignature is a Markdown table header's cells, matched
// case-insensitively with surrounding whitespace trimmed. R3: a second
// signature (Task 5's old-header form) is added as another headerSignature
// value checked against the same scanTables output — never a second
// scanner.
type headerSignature []string

func (s headerSignature) matches(cells []string) bool {
	if len(cells) != len(s) {
		return false
	}
	for i, want := range s {
		if !strings.EqualFold(strings.TrimSpace(cells[i]), want) {
			return false
		}
	}
	return true
}

var scenarioHeaderSignature = headerSignature{
	"ID", "Behavior", "Expected", "Level", "Verification mutation", "Risk",
}

var oldScenarioHeaderSignature = headerSignature{
	"ID", "Behavior", "Bug it catches", "Level", "Expected", "Risk",
}

type tableRow struct {
	cells []string
	line  int
}

type tableBlock struct {
	header []string
	rows   []tableRow
}

func splitTableRow(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return nil, false
	}
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells, true
}

func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if c == "" {
			return false
		}
		for i, r := range c {
			if r == ':' && (i == 0 || i == len(c)-1) {
				continue
			}
			if r != '-' {
				return false
			}
		}
	}
	return true
}

// scanTables walks lines and returns every Markdown table found: a header
// row immediately followed by a valid separator row, then data rows until
// the next non-table line. It applies no header-signature filtering — that
// is every caller's own concern, so a second signature reuses this same
// scan instead of forking another one.
func scanTables(lines []string) []tableBlock {
	var blocks []tableBlock
	for i := 0; i < len(lines); i++ {
		headerCells, ok := splitTableRow(lines[i])
		if !ok || i+1 >= len(lines) {
			continue
		}
		sepCells, ok := splitTableRow(lines[i+1])
		if !ok || len(sepCells) != len(headerCells) || !isSeparatorRow(sepCells) {
			continue
		}

		block := tableBlock{header: headerCells}
		j := i + 2
		for j < len(lines) {
			rowCells, ok := splitTableRow(lines[j])
			if !ok {
				break
			}
			block.rows = append(block.rows, tableRow{cells: rowCells, line: j + 1})
			j++
		}
		blocks = append(blocks, block)
		i = j - 1
	}
	return blocks
}
