package coverage

import (
	"fmt"
	"strings"
)

func parseHeaderLines(path string, lines []string) (specPath, baselineCommit string, err error) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "##") {
			break
		}
		switch {
		case specPath == "" && strings.HasPrefix(trimmed, "Spec:"):
			specPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "Spec:"))
		case baselineCommit == "" && strings.HasPrefix(trimmed, "Baseline:"):
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Baseline:"))
			if fields := strings.Fields(rest); len(fields) > 0 {
				baselineCommit = fields[0]
			}
		}
	}

	if specPath == "" {
		return "", "", fmt.Errorf("parse source %s: missing Spec: header", path)
	}
	if baselineCommit == "" {
		return "", "", fmt.Errorf("parse source %s: missing Baseline: header", path)
	}
	return specPath, baselineCommit, nil
}
