package coverage

import "strings"

// findSection returns the lines strictly between a heading line equal to
// heading and the next line starting with "#", or nil if heading is absent.
func findSection(lines []string, heading string) []string {
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}

	var section []string
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
			break
		}
		section = append(section, lines[i])
	}
	return section
}
