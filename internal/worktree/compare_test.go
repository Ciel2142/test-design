package worktree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompareUntouchedRepoSnapshotsAreEqual(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}
	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if !equal {
		t.Errorf("Compare() equal = false, want true; differing = %v", differing)
	}
	if differing != nil {
		t.Errorf("Compare() differing = %v, want nil", differing)
	}
}

func TestCompareWorktreeAdminChurnIsInvisible(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	wtPath := filepath.Join(t.TempDir(), "wt")
	mustGit(t, dir, "worktree", "add", "--detach", wtPath, "HEAD")
	mustGit(t, dir, "worktree", "remove", wtPath)

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if !equal {
		t.Errorf("Compare() equal = false, want true; differing = %v", differing)
	}
}

func TestCompareUnstagedEditIsUnequal(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false after unstaged edit")
	}
	if !reflect.DeepEqual(differing, []string{"tracked.txt"}) {
		t.Errorf("Compare() differing = %v, want [tracked.txt]", differing)
	}
}

func TestCompareStagedChangeIsUnequal(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "tracked.txt")

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false after staging a change")
	}
	if !reflect.DeepEqual(differing, []string{"tracked.txt"}) {
		t.Errorf("Compare() differing = %v, want [tracked.txt]", differing)
	}
}

func TestCompareNewUntrackedFileIsUnequal(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false after adding an untracked file")
	}
	if !reflect.DeepEqual(differing, []string{"scratch.txt"}) {
		t.Errorf("Compare() differing = %v, want [scratch.txt]", differing)
	}
}

func TestCompareDeletedUntrackedFileIsUnequal(t *testing.T) {
	dir := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.Remove(filepath.Join(dir, "scratch.txt")); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false after deleting an untracked file")
	}
	if !reflect.DeepEqual(differing, []string{"scratch.txt"}) {
		t.Errorf("Compare() differing = %v, want [scratch.txt]", differing)
	}
}

func TestCompareCommitMovesHead(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "tracked.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "change")

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false after a commit")
	}
	found := false
	for _, path := range differing {
		if path == "HEAD" {
			found = true
		}
	}
	if !found {
		t.Errorf("Compare() differing = %v, want it to contain %q", differing, "HEAD")
	}
}

func TestCompareTwoDifferingPathsBothNamedSortedOnce(t *testing.T) {
	dir := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "zzz.txt"), []byte("z\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "zzz.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "add zzz")

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "zzz.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aaa.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false with two differing paths")
	}
	want := []string{"aaa.txt", "zzz.txt"}
	if !reflect.DeepEqual(differing, want) {
		t.Errorf("Compare() differing = %v, want %v", differing, want)
	}
}
