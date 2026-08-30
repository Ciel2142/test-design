package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

// Worktree is a detached git worktree created under the controller's
// verifier-owned temporary root.
type Worktree struct {
	Path string
}

// Create makes a detached worktree at commit under the controller's
// verifier-owned temporary root. No branch is created or moved in the
// original repository. On failure the target directory is removed and the
// error names commit.
func (c *Controller) Create(commit string) (*Worktree, error) {
	dir := c.worktreesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create worktree %s: %w", commit, err)
	}
	path, err := reservePath(dir, "wt-")
	if err != nil {
		return nil, fmt.Errorf("create worktree %s: %w", commit, err)
	}

	if _, err := runGit(c.repoRoot, "worktree", "add", "--detach", path, commit); err != nil {
		_ = os.RemoveAll(path)
		return nil, fmt.Errorf("create worktree %s: %w", commit, err)
	}
	return &Worktree{Path: path}, nil
}

// Remove removes w through git, so the original repository's
// .git/worktrees admin entry is cleaned up too. It succeeds even when w is
// dirty. On any path Remove leaves the original repository's working tree,
// index, and refs untouched.
func (c *Controller) Remove(w *Worktree) error {
	if _, err := runGit(c.repoRoot, "worktree", "remove", "--force", w.Path); err != nil {
		return fmt.Errorf("remove worktree %s: %w", w.Path, err)
	}
	return nil
}

// Quarantine moves w's directory under the verifier-owned root's quarantine
// location and returns the surviving path. When the move fails, w is left in
// place and the returned path names it, paired with an error naming w.Path.
// Either way the original repository is untouched.
func (c *Controller) Quarantine(w *Worktree) (string, error) {
	dir := c.quarantineDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return w.Path, fmt.Errorf("quarantine worktree %s: %w", w.Path, err)
	}

	dest := filepath.Join(dir, filepath.Base(w.Path))
	if err := os.Rename(w.Path, dest); err != nil {
		return w.Path, fmt.Errorf("quarantine worktree %s: %w", w.Path, err)
	}

	// The quarantined directory is the evidence that matters; a stale admin
	// entry left behind by a prune failure is not.
	_, _ = runGit(c.repoRoot, "worktree", "prune")
	return dest, nil
}

func (c *Controller) worktreesDir() string {
	return filepath.Join(c.tempRoot, "worktrees")
}

func (c *Controller) quarantineDir() string {
	return filepath.Join(c.tempRoot, "quarantine")
}

func reservePath(dir, prefix string) (string, error) {
	tmp, err := os.MkdirTemp(dir, prefix)
	if err != nil {
		return "", err
	}
	if err := os.Remove(tmp); err != nil {
		return "", err
	}
	return tmp, nil
}
