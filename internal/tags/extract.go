package tags

import (
	"regexp"
	"strings"
)

var markerPattern = regexp.MustCompile(`\bcovers:`)

var tokenSplitter = regexp.MustCompile(`[,\s]+`)

// Extract returns the complete tokens following the covers: marker, one
// line at a time. It never substring-searches for "covers: SC-" — that
// would silently drop area-prefixed forms like orders/SC-01. The marker
// itself is word-boundary anchored, so "recovers:" or "discovers:" is not
// mistaken for a tag. A line with no marker contributes nothing; a line
// that has one contributes only itself, so a multi-line comment block
// never bleeds tokens across lines. Tokens are not validated — a malformed
// one is returned as written.
func Extract(comment string) []string {
	var tokens []string
	for _, line := range strings.Split(comment, "\n") {
		loc := markerPattern.FindStringIndex(line)
		if loc == nil {
			continue
		}
		rest := line[loc[1]:]
		for _, tok := range tokenSplitter.Split(rest, -1) {
			if tok == "" {
				continue
			}
			tokens = append(tokens, tok)
		}
	}
	return tokens
}
