// Package worktree captures the state of the original repository around a
// verification run so later stages can confirm the run left it untouched.
package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	// ErrNotGitWorktree is wrapped into NewController's error when repoRoot
	// is not the root of a Git work tree.
	ErrNotGitWorktree = errors.New("not a git work-tree root")
	// ErrGitNotFound is wrapped into NewController's error when the git
	// binary is absent from PATH.
	ErrGitNotFound = errors.New("git binary not found")
)

// Controller operates on one original repository, plus a verifier-owned
// temporary root that later tasks use for worktree lifecycle work.
type Controller struct {
	repoRoot string
	tempRoot string
}

// NewController resolves repoRoot to a Git work-tree root and establishes a
// verifier-owned temporary root outside of it. It fails before any side
// effect when repoRoot is not a work-tree root or the git binary is absent.
func NewController(repoRoot string) (*Controller, error) {
	resolvedRoot, err := resolveWorkTreeRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	tempRoot, err := newVerifierTempRoot(resolvedRoot)
	if err != nil {
		return nil, err
	}
	return &Controller{repoRoot: resolvedRoot, tempRoot: tempRoot}, nil
}

func resolveWorkTreeRoot(repoRoot string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("new controller %s: %w", repoRoot, ErrGitNotFound)
	}

	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("new controller %s: %w", repoRoot, ErrNotGitWorktree)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("new controller %s: %w", repoRoot, ErrNotGitWorktree)
	}

	out, err := runGit(resolved, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("new controller %s: %w", repoRoot, ErrNotGitWorktree)
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(out))
	if err != nil {
		return "", fmt.Errorf("new controller %s: resolve work-tree root %s: %w", repoRoot, strings.TrimSpace(out), err)
	}
	if top != resolved {
		return "", fmt.Errorf("new controller %s: not the work-tree root (root is %s): %w", repoRoot, top, ErrNotGitWorktree)
	}
	return resolved, nil
}

func newVerifierTempRoot(repoRoot string) (string, error) {
	dir, err := os.MkdirTemp("", "test-design-verifier-")
	if err != nil {
		return "", fmt.Errorf("new controller: create temp root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("new controller: resolve temp root %s: %w", dir, err)
	}
	if isWithin(repoRoot, resolved) {
		os.RemoveAll(dir)
		return "", fmt.Errorf("new controller: temp root %s resolves inside repository %s", resolved, repoRoot)
	}
	return resolved, nil
}

func isWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s (in %s): %v: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
