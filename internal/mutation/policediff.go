// Package mutation polices a captured worktree diff before any mutation
// test runs, rejecting anything that is not exactly one site edit in
// production source.
package mutation

import (
	"errors"
	"fmt"
	"strings"
)

type DiffSummary struct {
	File  string
	Hunks int
}

var (
	ErrEmptyDiff   = errors.New("diff is empty: no mutation applied")
	ErrMultiSite   = errors.New("diff touches more than one mutation site")
	ErrForeignDiff = errors.New("diff touches a path that is not production source")
)

// PoliceDiff accepts diff only when it changes exactly one file with
// exactly one hunk, editing existing production source for kind in place.
// It does not shell out; diff is parsed as text.
func PoliceDiff(diff, kind string) (DiffSummary, error) {
	if !validKind(kind) {
		return DiffSummary{}, fmt.Errorf("mutation: unknown kind %q", kind)
	}
	if strings.TrimSpace(diff) == "" {
		return DiffSummary{}, ErrEmptyDiff
	}
	entries, err := parseDiff(diff)
	if err != nil {
		return DiffSummary{}, err
	}
	if len(entries) != 1 {
		return DiffSummary{}, fmt.Errorf("%w: %s", ErrMultiSite, joinPaths(entries))
	}

	entry := entries[0]
	switch entry.kind {
	case changeAdded:
		return DiffSummary{}, fmt.Errorf("%w: %s (new file)", ErrForeignDiff, entry.path)
	case changeDeleted:
		return DiffSummary{}, fmt.Errorf("%w: %s (deleted file)", ErrForeignDiff, entry.path)
	case changeRenamed:
		return DiffSummary{}, fmt.Errorf("%w: %s (renamed)", ErrForeignDiff, entry.path)
	}
	if entry.hunks != 1 {
		return DiffSummary{}, fmt.Errorf("%w: %s has %d hunks", ErrMultiSite, entry.path, entry.hunks)
	}
	if !isProductionSource(entry.path, kind) {
		return DiffSummary{}, fmt.Errorf("%w: %s", ErrForeignDiff, entry.path)
	}
	return DiffSummary{File: entry.path, Hunks: entry.hunks}, nil
}

func joinPaths(entries []fileChange) string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.path
	}
	return strings.Join(paths, ", ")
}
