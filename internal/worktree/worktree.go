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
	"slices"
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
	hooksDir string
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
	hooksDir := filepath.Join(tempRoot, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return nil, fmt.Errorf("new controller: create hooks dir: %w", err)
	}
	return &Controller{repoRoot: resolvedRoot, tempRoot: tempRoot, hooksDir: hooksDir}, nil
}

func resolveWorkTreeRoot(repoRoot string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("new controller %s: %w", repoRoot, ErrGitNotFound)
	}

	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("new controller %s: %w: %w", repoRoot, ErrNotGitWorktree, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("new controller %s: %w: %w", repoRoot, ErrNotGitWorktree, err)
	}

	out, err := runGit(resolved, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("new controller %s: %w: %w", repoRoot, ErrNotGitWorktree, err)
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
	base := os.TempDir()
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("new controller: resolve temp base %s: %w", base, err)
	}
	resolvedBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("new controller: resolve temp base %s: %w", base, err)
	}
	if isWithin(repoRoot, resolvedBase) {
		return "", fmt.Errorf("new controller: temp base %s resolves inside repository %s", resolvedBase, repoRoot)
	}

	dir, err := os.MkdirTemp(resolvedBase, "test-design-verifier-")
	if err != nil {
		return "", fmt.Errorf("new controller: create temp root: %w", err)
	}
	return dir, nil
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

// gitEnvOverrideVars retarget git at a repository other than cmd.Dir; a
// caller's inherited value would silently redirect the controller's git
// invocations at a different repository.
var gitEnvOverrideVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_COMMON_DIR",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
	"GIT_CEILING_DIRECTORIES",
}

func filteredGitEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if slices.Contains(gitEnvOverrideVars, name) {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = filteredGitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s (in %s): %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// runGit runs git in the controller's repository with an empty,
// verifier-owned hooks path, so repo-supplied hooks (e.g. post-checkout on
// `worktree add`) never execute.
func (c *Controller) runGit(args ...string) (string, error) {
	full := append([]string{"-c", "core.hooksPath=" + c.hooksDir}, args...)
	return runGit(c.repoRoot, full...)
}
