package worktree

import (
	"sort"
	"strconv"
	"strings"
)

// Compare reports whether before and after capture the same repository
// state. When they differ, differing names each differing path — or the
// pseudo-entries "HEAD" (HEAD moved) and "INDEX" (staged tree changed) —
// sorted deterministically with no duplicates. Compare is pure: it never
// runs git and never touches the filesystem.
func Compare(before, after *Snapshot) (equal bool, differing []string) {
	diffs := make(map[string]struct{})

	if before.headCommit != after.headCommit {
		diffs["HEAD"] = struct{}{}
	}
	if before.indexTree != after.indexTree {
		diffs["INDEX"] = struct{}{}
	}
	addDiffingPaths(diffs, trackedPathStatuses(before.porcelainStatus), trackedPathStatuses(after.porcelainStatus))
	addDiffingPaths(diffs, before.untrackedHashes, after.untrackedHashes)

	if len(diffs) == 0 {
		return true, nil
	}

	differing = make([]string, 0, len(diffs))
	for path := range diffs {
		differing = append(differing, path)
	}
	sort.Strings(differing)
	return false, differing
}

func addDiffingPaths(diffs map[string]struct{}, before, after map[string]string) {
	for path, value := range before {
		if after[path] != value {
			diffs[path] = struct{}{}
		}
	}
	for path, value := range after {
		if before[path] != value {
			diffs[path] = struct{}{}
		}
	}
}

// "?? " entries skipped: untrackedHashes covers those.
// Codes are relative to HEAD, not the other snapshot — dirty vs dirty with
// different content but same code is invisible here; callers require
// Clean() so this doesn't bite. Staged half caught via indexTree in Compare.
func trackedPathStatuses(porcelain string) map[string]string {
	statuses := make(map[string]string)
	for line := range strings.SplitSeq(porcelain, "\n") {
		if len(line) < 3 || strings.HasPrefix(line, "?? ") {
			continue
		}
		status, path := line[:2], line[3:]
		statuses[unquotePorcelainPath(path)] = status
	}
	return statuses
}

func unquotePorcelainPath(path string) string {
	if !strings.HasPrefix(path, `"`) {
		return path
	}
	if unquoted, err := strconv.Unquote(path); err == nil {
		return unquoted
	}
	return path
}
