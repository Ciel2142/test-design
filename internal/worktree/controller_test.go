package worktree

import (
	"errors"
	"os"
	"testing"
)

func TestNewControllerNonRepoDirectoryErrors(t *testing.T) {
	dir := t.TempDir()

	ctrl, err := NewController(dir)
	if err == nil {
		t.Fatalf("NewController(%q) = %+v, want error", dir, ctrl)
	}
	if !errors.Is(err, ErrNotGitWorktree) {
		t.Errorf("NewController error = %v, want wrapping ErrNotGitWorktree", err)
	}

	entries, rerr := os.ReadDir(dir)
	if rerr != nil {
		t.Fatalf("ReadDir(%q): %v", dir, rerr)
	}
	if len(entries) != 0 {
		t.Errorf("NewController(%q) created entries %v inside non-repo dir, want none", dir, entries)
	}
}

func TestNewControllerGitBinaryAbsentErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "")

	ctrl, err := NewController(dir)
	if err == nil {
		t.Fatalf("NewController(%q) with empty PATH = %+v, want error", dir, ctrl)
	}
	if !errors.Is(err, ErrGitNotFound) {
		t.Errorf("NewController error = %v, want wrapping ErrGitNotFound", err)
	}
}

func TestNewControllerTempRootOutsideRepo(t *testing.T) {
	dir := newTestRepo(t)

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	if isWithin(ctrl.repoRoot, ctrl.tempRoot) {
		t.Errorf("tempRoot %q resolves inside repoRoot %q, want outside", ctrl.tempRoot, ctrl.repoRoot)
	}
}

func TestNewControllerRefusesTempRootInsideRepo(t *testing.T) {
	dir := newTestRepo(t)
	nestedTemp := dir + "/.verifier-tmp"
	if err := os.MkdirAll(nestedTemp, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Setenv("TMPDIR", nestedTemp)

	ctrl, err := NewController(dir)
	if err == nil {
		t.Fatalf("NewController(%q) = %+v, want error when TMPDIR resolves inside repo", dir, ctrl)
	}
}
