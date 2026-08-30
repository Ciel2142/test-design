package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrWorktreePathNotFound is wrapped into Attach's error when path does
	// not exist or is not a directory.
	ErrWorktreePathNotFound = errors.New("worktree path does not exist or is not a directory")
	// ErrWorktreeNotRegistered is wrapped into Attach's error when path
	// exists but git does not list it as a worktree of this controller's
	// repository.
	ErrWorktreeNotRegistered = errors.New("path is not a registered worktree of this repository")
	// ErrWorktreeInsideRepo is wrapped into Attach's error when path
	// resolves inside the original repository root.
	ErrWorktreeInsideRepo = errors.New("worktree path resolves inside the original repository root")
)

// Attach returns the Worktree for an existing directory that git already
// lists as a worktree of this controller's repository: the cross-process
// counterpart to Create, letting a later process reattach to a worktree by
// path alone. It never creates, prunes, or modifies anything.
func (c *Controller) Attach(path string) (*Worktree, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("attach worktree %s: %w: %w", path, ErrWorktreePathNotFound, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("attach worktree %s: %w: %w", path, ErrWorktreePathNotFound, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("attach worktree %s: not a directory: %w", path, ErrWorktreePathNotFound)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("attach worktree %s: %w: %w", path, ErrWorktreePathNotFound, err)
	}

	porcelain, err := c.runGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("attach worktree %s: %w", path, err)
	}
	if !isRegisteredWorktree(resolved, porcelain) {
		return nil, fmt.Errorf("attach worktree %s: %w", path, ErrWorktreeNotRegistered)
	}

	if isWithin(c.repoRoot, resolved) {
		return nil, fmt.Errorf("attach worktree %s: %w", path, ErrWorktreeInsideRepo)
	}

	return &Worktree{Path: resolved}, nil
}

func isRegisteredWorktree(resolved, porcelain string) bool {
	for _, entry := range worktreeListPaths(porcelain) {
		entryResolved, err := filepath.EvalSymlinks(entry)
		if err == nil && entryResolved == resolved {
			return true
		}
	}
	return false
}

func worktreeListPaths(porcelain string) []string {
	var paths []string
	for line := range strings.SplitSeq(porcelain, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, path)
		}
	}
	return paths
}
