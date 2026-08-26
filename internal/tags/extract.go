package tags

import (
	"regexp"
	"strings"
)

const marker = "covers:"

var tokenSplitter = regexp.MustCompile(`[,\s]+`)

// Extract returns the complete tokens following the covers: marker, one
// line at a time. It never substring-searches for "covers: SC-" — that
// would silently drop area-prefixed forms like orders/SC-01. A line with no
// marker contributes nothing; a line that has one contributes only itself,
// so a multi-line comment block never bleeds tokens across lines. Tokens
// are not validated — a malformed one is returned as written.
func Extract(comment string) []string {
	var tokens []string
	for _, line := range strings.Split(comment, "\n") {
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(marker):]
		for _, tok := range tokenSplitter.Split(rest, -1) {
			if tok == "" {
				continue
			}
			tokens = append(tokens, tok)
		}
	}
	return tokens
}
