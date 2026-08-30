package mutation

import (
	"fmt"
	"regexp"
	"strings"
)

type changeKind int

const (
	changeModify changeKind = iota
	changeAdded
	changeDeleted
	changeRenamed
)

type fileChange struct {
	path  string
	kind  changeKind
	hunks int
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -\d+(,\d+)? \+\d+(,\d+)? @@`)

// parseDiff reads unified diff text (as produced by `git diff HEAD`, plus
// untracked additions rendered via `git diff --no-index`) into one
// fileChange per "diff --git" block. It fails closed: any line it cannot
// place structurally is an error naming that line, never a silent skip.
func parseDiff(diff string) ([]fileChange, error) {
	lines := strings.Split(diff, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var entries []fileChange
	i := 0
	for i < len(lines) {
		_, bPath, ok := parseGitHeader(lines[i])
		if !ok {
			return nil, fmt.Errorf("mutation: expected a \"diff --git\" header, got %q", lines[i])
		}
		entry := fileChange{path: bPath, kind: changeModify}
		i++
		for i < len(lines) && !strings.HasPrefix(lines[i], "diff --git ") {
			line := lines[i]
			switch {
			case strings.HasPrefix(line, "new file mode "):
				entry.kind = changeAdded
			case strings.HasPrefix(line, "deleted file mode "):
				entry.kind = changeDeleted
			case strings.HasPrefix(line, "rename from "):
				entry.kind = changeRenamed
			case strings.HasPrefix(line, "rename to "):
				entry.path = strings.TrimPrefix(line, "rename to ")
			case strings.HasPrefix(line, "--- "):
				if src := strings.TrimPrefix(line, "--- "); src != "/dev/null" {
					entry.path = stripABPrefix(src)
				}
			case strings.HasPrefix(line, "+++ "):
				if dst := strings.TrimPrefix(line, "+++ "); dst != "/dev/null" {
					entry.path = stripABPrefix(dst)
				}
			case strings.HasPrefix(line, "@@ "):
				if !hunkHeaderRe.MatchString(line) {
					return nil, fmt.Errorf("mutation: malformed hunk header %q", line)
				}
				entry.hunks++
			}
			i++
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseGitHeader(line string) (aPath, bPath string, ok bool) {
	const prefix = "diff --git a/"
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := line[len(prefix):]
	idx := strings.Index(rest, " b/")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+len(" b/"):], true
}

func stripABPrefix(path string) string {
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
