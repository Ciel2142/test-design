package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffCleanWorktreeIsEmpty(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	diff, err := ctrl.Diff(w)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff on a clean worktree = %q, want empty", diff)
	}
}

func TestDiffTrackedChangeShowsInOutput(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	diff, err := ctrl.Diff(w)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "tracked.txt") || !strings.Contains(diff, "mutated") {
		t.Errorf("Diff = %q, want it to show the tracked.txt change", diff)
	}
}

func TestDiffUntrackedFileShowsAsAddition(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.WriteFile(filepath.Join(w.Path, "new-source.txt"), []byte("mutation payload\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	diff, err := ctrl.Diff(w)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "new-source.txt") {
		t.Errorf("Diff = %q, want it to name new-source.txt", diff)
	}
	if !strings.Contains(diff, "mutation payload") {
		t.Errorf("Diff = %q, want it to show new-source.txt's content", diff)
	}
}

func TestDiffLeavesWorktreeAndOriginalRepoUntouched(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	beforeRepo, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}
	beforeStatus := mustGit(t, w.Path, "status", "--porcelain")

	if _, err := ctrl.Diff(w); err != nil {
		t.Fatalf("Diff: %v", err)
	}

	afterRepo, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}
	if equal, differing := Compare(beforeRepo, afterRepo); !equal {
		t.Errorf("Compare() equal = false after Diff, want true; differing = %v", differing)
	}

	afterStatus := mustGit(t, w.Path, "status", "--porcelain")
	if beforeStatus != afterStatus {
		t.Errorf("worktree status changed after Diff:\nbefore: %q\nafter:  %q", beforeStatus, afterStatus)
	}
}

func TestRestoreTrackedRevertsTrackedChangesKeepsUntracked(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := ctrl.RestoreTracked(w); err != nil {
		t.Fatalf("RestoreTracked: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(w.Path, "tracked.txt"))
	if err != nil {
		t.Fatalf("ReadFile tracked.txt: %v", err)
	}
	if string(got) != "original\n" {
		t.Errorf("tracked.txt = %q after RestoreTracked, want %q", got, "original\n")
	}

	if _, err := os.Stat(filepath.Join(w.Path, "scratch.txt")); err != nil {
		t.Errorf("scratch.txt missing after RestoreTracked: %v", err)
	}

	diff, err := ctrl.Diff(w)
	if err != nil {
		t.Fatalf("Diff after RestoreTracked: %v", err)
	}
	if strings.Contains(diff, "tracked.txt") {
		t.Errorf("Diff after RestoreTracked still mentions tracked.txt: %q", diff)
	}
}

func TestRestoreTrackedDoesNotTouchOriginalRepo(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := ctrl.RestoreTracked(w); err != nil {
		t.Fatalf("RestoreTracked: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}
	if equal, differing := Compare(before, after); !equal {
		t.Errorf("Compare() equal = false after RestoreTracked, want true; differing = %v", differing)
	}
}

func TestRestoreTrackedGitFailureWrapsPath(t *testing.T) {
	notAWorktree := &Worktree{Path: t.TempDir()}
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	err = ctrl.RestoreTracked(notAWorktree)
	if err == nil {
		t.Fatalf("RestoreTracked(%q) = nil, want error", notAWorktree.Path)
	}
	if !strings.Contains(err.Error(), notAWorktree.Path) {
		t.Errorf("RestoreTracked error = %v, want it to name %q", err, notAWorktree.Path)
	}
}
