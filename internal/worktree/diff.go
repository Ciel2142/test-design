package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Diff returns the unified diff of w's tracked changes relative to its
// checked-out HEAD, with untracked files appended as new-file hunks so a
// mutation hidden in an untracked source file isn't invisible to a later
// diff check. Empty when w is clean and has no untracked files. Build
// outputs are a caller concern: Diff reports what git sees, it does not
// filter.
func (c *Controller) Diff(w *Worktree) (string, error) {
	tracked, err := runGit(w.Path, "diff", "HEAD")
	if err != nil {
		return "", fmt.Errorf("diff worktree %s: %w", w.Path, err)
	}

	status, err := runGit(w.Path, "--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", fmt.Errorf("diff worktree %s: %w", w.Path, err)
	}

	var sb strings.Builder
	sb.WriteString(tracked)
	for _, path := range untrackedPaths(status) {
		hunk, err := untrackedFileDiff(w.Path, path)
		if err != nil {
			return "", fmt.Errorf("diff worktree %s: untracked %s: %w", w.Path, path, err)
		}
		sb.WriteString(hunk)
	}
	return sb.String(), nil
}

// untrackedFileDiff renders path (untracked, so absent from the index) as a
// new-file addition via `git diff --no-index`, which uses classic diff exit
// codes — 1 means "the two sides differ", not an error — unlike the rest of
// this package's git invocations.
func untrackedFileDiff(dir, path string) (string, error) {
	cmd := exec.Command("git", "diff", "--no-index", "--", os.DevNull, path)
	cmd.Dir = dir
	cmd.Env = filteredGitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if err == nil || (errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		return stdout.String(), nil
	}
	return "", fmt.Errorf("git diff --no-index %s (in %s): %w: %s", path, dir, err, strings.TrimSpace(stderr.String()))
}

// RestoreTracked resets w's index and working tree to its checked-out HEAD.
// Untracked files are left alone: `reset --hard` only touches paths that
// differ between the index/worktree and HEAD. It never touches the original
// repository, since it acts entirely inside w.
func (c *Controller) RestoreTracked(w *Worktree) error {
	if _, err := runGit(w.Path, "reset", "--hard", "HEAD"); err != nil {
		return fmt.Errorf("restore tracked %s: %w", w.Path, err)
	}
	return nil
}
