package coverage

import (
	"fmt"
	"strings"
)

func parseRetiredSection(path string, lines []string) ([]string, error) {
	section := findSection(lines, "## Retired")
	if section == nil {
		return nil, nil
	}

	var ids []string
	for _, line := range section {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		bullet := strings.TrimPrefix(trimmed, "- ")
		fields := strings.Fields(bullet)
		if len(fields) == 0 || !idPattern.MatchString(fields[0]) {
			return nil, fmt.Errorf("parse source %s: retired bullet %q: malformed leading id", path, bullet)
		}
		ids = append(ids, fields[0])
	}
	return ids, nil
}
