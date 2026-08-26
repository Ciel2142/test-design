package coverage

import (
	"fmt"
	"strings"
)

func parseRow(path string, r tableRow) (Row, error) {
	if len(r.cells) != len(scenarioHeaderSignature) {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: expected %d columns, got %d", path, r.line, len(scenarioHeaderSignature), len(r.cells))
	}
	id, behavior, expected, level, mutation, risk := r.cells[0], r.cells[1], r.cells[2], r.cells[3], r.cells[4], r.cells[5]

	if !idPattern.MatchString(id) {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: column ID: malformed id %q", path, r.line, id)
	}
	if strings.TrimSpace(behavior) == "" {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: column Behavior: empty", path, r.line)
	}
	if strings.TrimSpace(expected) == "" {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: column Expected: empty", path, r.line)
	}
	if !validLevels[level] {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: column Level: unknown level %q", path, r.line, level)
	}
	if strings.TrimSpace(mutation) == "" {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: column Verification mutation: empty", path, r.line)
	}
	if !validRisks[risk] {
		return Row{}, fmt.Errorf("parse source %s: row at line %d: column Risk: unknown risk %q", path, r.line, risk)
	}

	return Row{ID: id, Behavior: behavior, Expected: expected, Level: level, Mutation: mutation, Risk: risk}, nil
}

func findOldHeaderTable(blocks []tableBlock) (tableBlock, bool) {
	for _, b := range blocks {
		if oldScenarioHeaderSignature.matches(b.header) {
			return b, true
		}
	}
	return tableBlock{}, false
}

func extractRows(path string, blocks []tableBlock) ([]Row, error) {
	seen := make(map[string]bool)
	var rows []Row
	for _, b := range blocks {
		if !scenarioHeaderSignature.matches(b.header) {
			continue
		}
		for _, r := range b.rows {
			row, err := parseRow(path, r)
			if err != nil {
				return nil, err
			}
			if seen[row.ID] {
				return nil, fmt.Errorf("parse source %s: duplicate id %s", path, row.ID)
			}
			seen[row.ID] = true
			rows = append(rows, row)
		}
	}
	return rows, nil
}
