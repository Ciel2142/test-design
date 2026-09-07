package coverage

import (
	"fmt"
	"os"
	"strings"

	"test-design/internal/evidence"
)

// ParseSource parses the coverage-source Markdown document at path. It
// fails closed: on any error the returned *Source is nil.
func ParseSource(path string) (*Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parse source %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	blocks := scanTables(lines)
	if old, found := findOldHeaderTable(blocks); found {
		return nil, fmt.Errorf("parse source %s: old header %q found in header %v, migrate to the new coverage-source header before parsing", path, "Bug it catches", old.header)
	}

	specPath, baselineCommit, err := parseHeaderLines(path, lines)
	if err != nil {
		return nil, err
	}

	vf, err := parseVerificationSection(path, lines)
	if err != nil {
		return nil, err
	}

	retired, err := parseRetiredSection(path, lines)
	if err != nil {
		return nil, err
	}

	rows, err := extractRows(path, blocks)
	if err != nil {
		return nil, err
	}

	liveDecision, err := parseLiveDecisionSection(path, lines, blocks)
	if err != nil {
		return nil, err
	}

	sha, err := evidence.SHA256File(path)
	if err != nil {
		return nil, fmt.Errorf("parse source %s: %w", path, err)
	}

	return &Source{
		Path:                 path,
		SpecPath:             specPath,
		BaselineCommit:       baselineCommit,
		BaselineEvidencePath: vf.BaselineEvidencePath,
		BaselineSHA256:       vf.BaselineSHA256,
		SHA256:               sha,
		Runner:               vf.Runner,
		LiveProfile:          vf.LiveProfile,
		LiveDecision:         liveDecision,
		Rows:                 rows,
		Retired:              retired,
		FinalCommand:         vf.FinalCommand,
	}, nil
}
