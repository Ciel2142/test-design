package worktree

import (
	"errors"
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

// commitFile writes path under repo, stages and commits it, and returns the
// new HEAD.
func commitFile(t *testing.T, repo, name, content, message string) string {
	t.Helper()
	full := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, repo, "add", name)
	mustGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", message)
	return strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
}

func TestRestoreAllClearsTrackedUntrackedAndIgnoredResidue(t *testing.T) {
	repo := newTestRepo(t)
	head := commitFile(t, repo, ".gitignore", "build/\n", "ignore build output")
	reflogBefore := mustGit(t, repo, "reflog")

	ctrl, err := NewController(repo)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create(head)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tracked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "stray.txt"), []byte("stray\n"), 0o644); err != nil {
		t.Fatalf("WriteFile untracked: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(w.Path, "build"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "build", "out.bin"), []byte("residue\n"), 0o644); err != nil {
		t.Fatalf("WriteFile ignored: %v", err)
	}

	if err := ctrl.RestoreAll(w, head); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}

	if status := strings.TrimSpace(mustGit(t, w.Path, "status", "--porcelain=v1", "--ignored", "--untracked-files=all")); status != "" {
		t.Errorf("worktree status = %q, want empty including ignored paths", status)
	}
	if _, err := os.Stat(filepath.Join(w.Path, "build", "out.bin")); !os.IsNotExist(err) {
		t.Errorf("ignored residue survived RestoreAll: %v", err)
	}
	if got := strings.TrimSpace(mustGit(t, w.Path, "rev-parse", "HEAD")); got != head {
		t.Errorf("worktree HEAD = %q, want %q", got, head)
	}

	if got := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD")); got != head {
		t.Errorf("original repository HEAD = %q, want unchanged %q", got, head)
	}
	if got := mustGit(t, repo, "reflog"); got != reflogBefore {
		t.Errorf("original repository reflog changed:\n%s\nwant:\n%s", got, reflogBefore)
	}
	if status := strings.TrimSpace(mustGit(t, repo, "status", "--porcelain=v1", "--ignored", "--untracked-files=all")); status != "" {
		t.Errorf("original repository status = %q, want it untouched", status)
	}
}

func TestRestoreAllRefusesAWorktreeCheckedOutElsewhere(t *testing.T) {
	repo := newTestRepo(t)
	first := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	head := commitFile(t, repo, "second.txt", "second\n", "second commit")

	ctrl, err := NewController(repo)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create(head)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	mustGit(t, w.Path, "checkout", "--detach", first)

	err = ctrl.RestoreAll(w, head)
	if !errors.Is(err, ErrWrongRevision) {
		t.Fatalf("RestoreAll = %v, want %v", err, ErrWrongRevision)
	}
	if !strings.Contains(err.Error(), head) {
		t.Errorf("error = %v, want it to name the expected revision %q", err, head)
	}
}
