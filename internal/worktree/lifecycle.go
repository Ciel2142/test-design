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

	if _, err := c.runGit("worktree", "add", "--detach", path, commit); err != nil {
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
	if _, err := c.runGit("worktree", "remove", "--force", w.Path); err != nil {
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
	_, _ = c.runGit("worktree", "prune")
	return dest, nil
}

// Dispose retires w: it tries Remove first; on Remove failure it falls back
// to Quarantine so the worktree survives inspection rather than vanishing.
// It returns nil only when Remove succeeds. On Remove failure followed by a
// successful Quarantine, it returns an error naming the surviving
// quarantine path. When both Remove and Quarantine fail, it returns an
// error naming w's in-place path — the worktree is never silently lost.
func (c *Controller) Dispose(w *Worktree) error {
	removeErr := c.Remove(w)
	if removeErr == nil {
		return nil
	}

	quarantined, quarantineErr := c.Quarantine(w)
	if quarantineErr == nil {
		return fmt.Errorf("dispose worktree %s: remove failed (%w); quarantined at %s", w.Path, removeErr, quarantined)
	}
	return fmt.Errorf("dispose worktree %s: remove failed (%w); quarantine also failed (%w); worktree remains at %s", w.Path, removeErr, quarantineErr, quarantined)
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

// Forget drops the registry entry of a worktree whose directory is gone — the
// admin files under .git/worktrees that a directory swept away out from under
// git leaves behind — through git worktree remove --force on that path alone.
// It never prunes: the registry is the repository's, shared with every other
// worktree of it, and another whose directory happens to be absent at the same
// moment — one on an unmounted volume, say — keeps its entry. A path git does
// not list reports ErrWorktreeNotRegistered; a path that still exists is
// refused, since removing a present worktree is Remove's job. The original
// repository's HEAD, index and working tree are untouched.
func (c *Controller) Forget(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("forget worktree %s: %w", path, err)
	}
	if _, err := os.Lstat(abs); err == nil {
		return fmt.Errorf("forget worktree %s: the path still exists; remove it instead", path)
	}

	porcelain, err := c.runGit("worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("forget worktree %s: %w", path, err)
	}
	if !isListedWorktree(abs, porcelain) {
		return fmt.Errorf("forget worktree %s: %w", path, ErrWorktreeNotRegistered)
	}

	if _, err := c.runGit("worktree", "remove", "--force", abs); err != nil {
		return fmt.Errorf("forget worktree %s: %w", path, err)
	}
	return nil
}

// isListedWorktree reports whether git lists abs among the repository's
// worktrees. Unlike isRegisteredWorktree it does not need the directory to
// exist: git records a worktree by its real path, so both sides are compared
// after resolving the longest prefix of them that still does — the same
// forgiving match git applies to the path handed to worktree remove.
func isListedWorktree(abs, porcelain string) bool {
	want := resolveExistingPrefix(abs)
	for _, entry := range worktreeListPaths(porcelain) {
		if resolveExistingPrefix(entry) == want {
			return true
		}
	}
	return false
}

// resolveExistingPrefix resolves path's symlinks as far as the filesystem
// allows: the longest existing ancestor is resolved and the missing tail
// rejoined unchanged.
func resolveExistingPrefix(path string) string {
	current, tail := filepath.Clean(path), ""
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, tail)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Join(current, tail)
		}
		tail = filepath.Join(filepath.Base(current), tail)
		current = parent
	}
}
