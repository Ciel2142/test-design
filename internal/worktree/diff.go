package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"test-design/internal/subprocess"
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
	err := subprocess.Run(cmd, subprocess.UtilityTimeout)

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

var (
	// ErrResidue is wrapped into RestoreAll's error when the ignored-inclusive
	// status is still non-empty after the restoration ran.
	ErrResidue = errors.New("worktree residue remains")
	// ErrWrongRevision is wrapped into RestoreAll's error when the restored
	// worktree's HEAD is not the commit it was prepared at.
	ErrWrongRevision = errors.New("worktree is not at the expected revision")
)

// RestoreAll returns w to the pristine checkout of commit: it resets the index
// and working tree to HEAD and removes every untracked path, ignored ones
// included. It then proves the result rather than trusting it. `git clean`'s
// own exit status is not that proof — it is nonzero for a path it merely warned
// about as much as for one it truly left behind — so what survived is read from
// the ignored-inclusive status afterwards, which names every remaining path
// (ErrResidue); the revision is read from rev-parse (ErrWrongRevision). Every
// command runs inside w, so the original repository's HEAD, index, working tree
// and reflog are untouched.
func (c *Controller) RestoreAll(w *Worktree, commit string) error {
	if _, err := runGit(w.Path, "reset", "--hard", "HEAD"); err != nil {
		return fmt.Errorf("restore all %s: %w", w.Path, err)
	}
	_, _ = runGit(w.Path, "clean", "-fdx")

	status, err := runGit(w.Path, "--no-optional-locks", "status", "--porcelain=v1", "--ignored", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("restore all %s: %w", w.Path, err)
	}
	if residue := statusPaths(status); len(residue) > 0 {
		return fmt.Errorf("restore all %s: %w: %s", w.Path, ErrResidue, strings.Join(residue, ", "))
	}

	head, err := runGit(w.Path, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("restore all %s: %w", w.Path, err)
	}
	if got := strings.TrimSpace(head); got != commit {
		return fmt.Errorf("restore all %s: %w: HEAD is %s, want %s", w.Path, ErrWrongRevision, got, commit)
	}
	return nil
}

// statusPaths names every path a porcelain=v1 status reports, whatever its
// status letters: after a restoration any line at all is residue.
func statusPaths(porcelain string) []string {
	var paths []string
	for line := range strings.SplitSeq(porcelain, "\n") {
		if len(line) < 4 {
			continue
		}
		paths = append(paths, unquotePorcelainPath(line[3:]))
	}
	return paths
}
