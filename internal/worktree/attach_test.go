package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAttachRegisteredWorktreeReturnsItsPath(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	created, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	reattached, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController (reattach): %v", err)
	}

	w, err := reattached.Attach(created.Path)
	if err != nil {
		t.Fatalf("Attach(%q): %v", created.Path, err)
	}
	if w.Path != created.Path {
		t.Errorf("Attach Path = %q, want %q", w.Path, created.Path)
	}
}

func TestAttachResolvesSymlinkedPath(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	created, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	link := filepath.Join(t.TempDir(), "link-to-worktree")
	if err := os.Symlink(created.Path, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	w, err := ctrl.Attach(link)
	if err != nil {
		t.Fatalf("Attach(%q): %v", link, err)
	}
	if w.Path != created.Path {
		t.Errorf("Attach via symlink Path = %q, want resolved %q", w.Path, created.Path)
	}
}

func TestAttachNonexistentPathErrors(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	w, err := ctrl.Attach(missing)
	if err == nil {
		t.Fatalf("Attach(%q) = %+v, nil, want error", missing, w)
	}
	if !errors.Is(err, ErrWorktreePathNotFound) {
		t.Errorf("Attach error = %v, want it to wrap ErrWorktreePathNotFound", err)
	}
}

func TestAttachPathIsFileNotDirectoryErrors(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("not a dir\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := ctrl.Attach(file)
	if err == nil {
		t.Fatalf("Attach(%q) = %+v, nil, want error", file, w)
	}
	if !errors.Is(err, ErrWorktreePathNotFound) {
		t.Errorf("Attach error = %v, want it to wrap ErrWorktreePathNotFound", err)
	}
}

func TestAttachUnregisteredDirectoryErrors(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	unregistered := t.TempDir()
	w, err := ctrl.Attach(unregistered)
	if err == nil {
		t.Fatalf("Attach(%q) = %+v, nil, want error", unregistered, w)
	}
	if !errors.Is(err, ErrWorktreeNotRegistered) {
		t.Errorf("Attach error = %v, want it to wrap ErrWorktreeNotRegistered", err)
	}
}

func TestAttachRepoRootItselfErrorsInsideRepo(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	w, err := ctrl.Attach(dir)
	if err == nil {
		t.Fatalf("Attach(%q) = %+v, nil, want error", dir, w)
	}
	if !errors.Is(err, ErrWorktreeInsideRepo) {
		t.Errorf("Attach error = %v, want it to wrap ErrWorktreeInsideRepo", err)
	}
}

func TestAttachNeverModifiesRepoOrWorktrees(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	created, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if _, err := ctrl.Attach(created.Path); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	_, _ = ctrl.Attach(filepath.Join(t.TempDir(), "nope"))
	_, _ = ctrl.Attach(dir)

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}
	if equal, differing := Compare(before, after); !equal {
		t.Errorf("Compare() equal = false after Attach calls (incl. failures), want true; differing = %v", differing)
	}

	entries, rerr := os.ReadDir(ctrl.worktreesDir())
	if rerr != nil {
		t.Fatalf("ReadDir(%q): %v", ctrl.worktreesDir(), rerr)
	}
	if len(entries) != 1 {
		t.Errorf("worktrees dir has %d entries after Attach calls, want 1 (only the pre-existing Create)", len(entries))
	}
}
