package main

import (
	"sort"
	"strings"
)

// This file holds the repository queries a reuse decision is made from. Every
// one of them asks git rather than the working tree, so a prior run's commit
// can be examined at a HEAD that has moved past it, and each answers with a
// plain value: a query that cannot be answered is not an event this layer
// reports, it is a fact the caller weighs.

// commitExists reports whether repoRoot holds commit. A prior run naming a
// commit this repository never had — a dropped branch, a rewritten history —
// cannot be compared against, and is not an error to be surfaced.
func commitExists(repoRoot, commit string) bool {
	_, err := gitOutput(repoRoot, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// changedPaths names every tracked path that differs between from and to, as
// repo-relative slash paths in sorted order. Renames are not followed, so a
// moved file appears under both its old and its new path: a reuse decision
// must see that the old path is gone rather than be told the content merely
// travelled.
func changedPaths(repoRoot, from, to string) ([]string, error) {
	// core.quotePath=false keeps a non-ASCII path as the bytes it is rather
	// than an escaped rendering no digest or comparison would match.
	out, err := gitOutput(repoRoot, "-c", "core.quotePath=false", "diff", "--name-only", "--no-renames", from, to)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, line := range strings.Split(out, "\n") {
		if path := strings.TrimSpace(line); path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// trackedAt reports whether commit carries path. A file the working tree holds
// but the commit does not is untracked state: identifiable now, unidentifiable
// to anything reading this run later.
func trackedAt(repoRoot, commit, path string) bool {
	_, err := gitOutput(repoRoot, "cat-file", "-e", commit+":"+path)
	return err == nil
}

// isTestFile reports whether path is a test file of the runner's kind. A
// change to a test is read differently from a change to production source, and
// only the kinds whose layout the verifier knows can be classified: an
// unsupported kind claims nothing rather than guessing.
func isTestFile(runnerKind, path string) bool {
	switch runnerKind {
	case "go":
		return strings.HasSuffix(path, "_test.go")
	case "maven", "gradle":
		segments := strings.Split(path, "/")
		for i := 0; i+1 < len(segments); i++ {
			if segments[i] == "src" && segments[i+1] == "test" {
				return true
			}
		}
	}
	return false
}

// isGoTestdataPath reports whether path carries a "testdata" directory
// segment — the layout Go's own tooling excludes from a package's build, so a
// golden file or fixture living there is not production source even when it
// carries no "_test.go" suffix.
func isGoTestdataPath(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == "testdata" {
			return true
		}
	}
	return false
}

// productionPathsSince names every changed production path between baseline
// and measured: changedPaths(repoRoot, baseline, measured), minus a test
// file, a Go testdata path, and anything the coverage source's own
// publication directory holds. trace and finalize both classify a freshness
// answer against this same set, computed the same way from the same two
// commits.
//
// It returns nil, nil when baseline does not resolve in this repository:
// there is no diff to compute. trace reports that by omitting its output
// field entirely; finalize refuses it as baseline-commit-unknown. A resolved
// baseline always returns a non-nil slice, even when it names no paths.
func productionPathsSince(repoRoot, coverageSource, runnerKind, baseline, measured string) ([]string, error) {
	if !commitExists(repoRoot, baseline) {
		return nil, nil
	}
	changed, err := changedPaths(repoRoot, baseline, measured)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(changed))
	for _, path := range changed {
		switch {
		case isTestFile(runnerKind, path):
			continue
		case runnerKind == "go" && isGoTestdataPath(path):
			continue
		case isPublication(coverageSource, path):
			continue
		}
		out = append(out, path)
	}
	return out, nil
}
