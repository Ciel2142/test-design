package worktree

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"test-design/internal/evidence"
)

func TestTakeSnapshotCapturesHeadCommit(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	want := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))

	snap, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if snap.headCommit != want {
		t.Errorf("headCommit = %q, want %q", snap.headCommit, want)
	}
}

func TestTakeSnapshotCleanRepoIsClean(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	snap, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if !snap.Clean() {
		t.Errorf("Clean() = false, want true for a freshly committed repo")
	}
}

func TestTakeSnapshotStagedOnlyChangeIsNotClean(t *testing.T) {
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

	if after.Clean() {
		t.Errorf("Clean() = true after staging a change, want false")
	}
	if before.indexTree == after.indexTree {
		t.Errorf("indexTree unchanged after staging tracked.txt, want a different tree hash")
	}
}

func TestTakeSnapshotUntrackedFileRecordsPathAndHash(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	content := []byte("scratch data\n")
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	snap, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if snap.Clean() {
		t.Errorf("Clean() = true with an untracked file present, want false")
	}

	want, err := evidence.SHA256File(filepath.Join(dir, "scratch.txt"))
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	got, ok := snap.untrackedHashes["scratch.txt"]
	if !ok {
		t.Fatalf("untrackedHashes missing %q, has %v", "scratch.txt", snap.untrackedHashes)
	}
	if got != want {
		t.Errorf("untrackedHashes[%q] = %q, want %q", "scratch.txt", got, want)
	}
}

func TestTwoSnapshotsOfUntouchedRepoAreEqual(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	first, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot first: %v", err)
	}
	second, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot second: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Errorf("two snapshots of an untouched repo differ:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestTakeSnapshotDoesNotMutateRepo(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before := mustGit(t, dir, "status", "--porcelain")
	if _, err := ctrl.TakeSnapshot(); err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	after := mustGit(t, dir, "status", "--porcelain")

	if before != after {
		t.Errorf("git status --porcelain changed after TakeSnapshot:\nbefore: %q\nafter:  %q", before, after)
	}
}
