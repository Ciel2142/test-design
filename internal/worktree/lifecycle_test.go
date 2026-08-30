package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateDoesNotRunRepoHooks(t *testing.T) {
	dir := newTestRepo(t)
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	evidence := filepath.Join(dir, "hook-ran.txt")
	hookScript := "#!/bin/sh\necho ran > " + evidence + "\n"
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		t.Fatalf("WriteFile hook: %v", err)
	}

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ctrl.Remove(w); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(evidence); !os.IsNotExist(err) {
		t.Errorf("post-checkout hook ran (evidence at %q, stat err = %v), want it never invoked", evidence, err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}
	if equal, differing := Compare(before, after); !equal {
		t.Errorf("Compare() equal = false after Create+Remove with a repo hook present, want true; differing = %v", differing)
	}
}

func TestCreateAtHEADIsDetachedUnderVerifierRoot(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))

	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !isWithin(ctrl.tempRoot, w.Path) {
		t.Errorf("worktree path %q not under verifier root %q", w.Path, ctrl.tempRoot)
	}
	if isWithin(ctrl.repoRoot, w.Path) {
		t.Errorf("worktree path %q is under the repository %q, want outside", w.Path, ctrl.repoRoot)
	}

	symref := exec.Command("git", "symbolic-ref", "-q", "HEAD")
	symref.Dir = w.Path
	if err := symref.Run(); err == nil {
		t.Errorf("git symbolic-ref HEAD succeeded in %q, want failure (detached)", w.Path)
	}

	got := strings.TrimSpace(mustGit(t, w.Path, "rev-parse", "HEAD"))
	if got != head {
		t.Errorf("worktree HEAD = %q, want %q", got, head)
	}
}

func TestCreateGarbageCommitErrors(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	const badCommit = "not-a-real-commit"
	w, err := ctrl.Create(badCommit)
	if err == nil {
		t.Fatalf("Create(%q) = %+v, want error", badCommit, w)
	}
	if !strings.Contains(err.Error(), badCommit) {
		t.Errorf("Create error = %v, want it to name the commit %q", err, badCommit)
	}

	entries, rerr := os.ReadDir(ctrl.worktreesDir())
	if rerr != nil {
		t.Fatalf("ReadDir(%q): %v", ctrl.worktreesDir(), rerr)
	}
	if len(entries) != 0 {
		t.Errorf("Create(%q) left entries %v under the verifier root, want none", badCommit, entries)
	}
}

func TestIsolationWalkthroughRemoveLeavesOriginalRepoUntouched(t *testing.T) {
	dir := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "second.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "add second")

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile in worktree: %v", err)
	}
	if err := os.Remove(filepath.Join(w.Path, "second.txt")); err != nil {
		t.Fatalf("Remove in worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile new in worktree: %v", err)
	}

	if err := ctrl.Remove(w); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if !equal {
		t.Errorf("Compare() equal = false after isolation walkthrough, want true; differing = %v", differing)
	}
}

func TestRemoveDirtyWorktreeSucceeds(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := ctrl.Remove(w); err != nil {
		t.Fatalf("Remove(dirty worktree): %v", err)
	}
	if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir %q still exists after Remove, stat err = %v", w.Path, err)
	}
}

func TestRemoveFailureFallsBackToQuarantine(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod-based permission checks don't apply")
	}
	dir := newTestRepo(t)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "sub/nested.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "add sub")

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Write-protected subdir blocks git's recursive rm but not a rename of
	// the worktree root, so Remove fails and Quarantine's move still works.
	nested := filepath.Join(w.Path, "sub")
	if err := os.Chmod(nested, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	var restore []string
	restore = append(restore, nested)
	t.Cleanup(func() {
		for _, p := range restore {
			_ = os.Chmod(p, 0o700)
		}
	})

	if err := ctrl.Remove(w); err == nil {
		t.Fatalf("Remove(%q) = nil, want a permission error to force the Quarantine fallback", w.Path)
	}

	quarantined, err := ctrl.Quarantine(w)
	if err != nil {
		t.Fatalf("Quarantine after failed Remove: %v", err)
	}
	restore = append(restore, filepath.Join(quarantined, "sub"))
	if _, err := os.Stat(quarantined); err != nil {
		t.Errorf("quarantined path %q does not exist: %v", quarantined, err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}
	equal, differing := Compare(before, after)
	if !equal {
		t.Errorf("Compare() equal = false after Remove-failure + Quarantine, want true; differing = %v", differing)
	}
}

func TestDisposeHappyPathRemovesWorktree(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ctrl.Dispose(w); err != nil {
		t.Fatalf("Dispose: %v", err)
	}
	if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir %q still exists after Dispose, stat err = %v", w.Path, err)
	}
}

func TestDisposeQuarantinesOnRemoveFailureAndNamesSurvivingPath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod-based permission checks don't apply")
	}
	dir := newTestRepo(t)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "sub/nested.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "add sub")

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	nested := filepath.Join(w.Path, "sub")
	if err := os.Chmod(nested, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(nested, 0o700) })

	err = ctrl.Dispose(w)
	if err == nil {
		t.Fatalf("Dispose(%q) = nil, want an error naming the surviving quarantine path", w.Path)
	}

	quarantineBase := ctrl.quarantineDir()
	if !strings.Contains(err.Error(), quarantineBase) {
		t.Errorf("Dispose error = %v, want it to name a path under quarantine dir %q", err, quarantineBase)
	}
	entries, rerr := os.ReadDir(quarantineBase)
	if rerr != nil {
		t.Fatalf("ReadDir(%q): %v", quarantineBase, rerr)
	}
	if len(entries) != 1 {
		t.Fatalf("quarantine dir has %d entries, want 1", len(entries))
	}
	survivingPath := filepath.Join(quarantineBase, entries[0].Name())
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(survivingPath, "sub"), 0o700) })
	if !strings.Contains(err.Error(), survivingPath) {
		t.Errorf("Dispose error = %v, want it to name the surviving path %q", err, survivingPath)
	}
	if _, statErr := os.Stat(survivingPath); statErr != nil {
		t.Errorf("surviving quarantine path %q does not exist: %v", survivingPath, statErr)
	}
}

func TestQuarantineMoveFailureReturnsOriginalPathWithNamedError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod-based permission checks don't apply")
	}
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	w, err := ctrl.Create("HEAD")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// rename(2) needs write on the dir being moved (updates its ".." entry).
	if err := os.Chmod(w.Path, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(w.Path, 0o700) })

	got, err := ctrl.Quarantine(w)
	if err == nil {
		t.Fatalf("Quarantine(%q) = %q, nil, want a move error", w.Path, got)
	}
	if got != w.Path {
		t.Errorf("Quarantine on move failure returned %q, want original path %q", got, w.Path)
	}
	if !strings.Contains(err.Error(), w.Path) {
		t.Errorf("Quarantine error = %v, want it to name %q", err, w.Path)
	}
}
