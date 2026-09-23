package worktree

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"test-design/internal/evidence"
)

func TestTakeSnapshotUnbornHeadReturnsSentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	mustGit(t, dir, "init", "-q")

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	snap, err := ctrl.TakeSnapshot()
	if err == nil {
		t.Fatalf("TakeSnapshot on a fresh, commit-less repo = %+v, nil, want an error", snap)
	}
	if !errors.Is(err, ErrUnbornHead) {
		t.Errorf("TakeSnapshot error = %v, want it to wrap ErrUnbornHead", err)
	}
}

func TestTakeSnapshotUnmergedIndexReturnsSentinel(t *testing.T) {
	dir := newTestRepo(t)
	mustGit(t, dir, "checkout", "-q", "-b", "other")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("branch content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "tracked.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "branch change")

	mustGit(t, dir, "checkout", "-q", "master")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("master content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "tracked.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "master change")

	mergeCmd := exec.Command("git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "merge", "-q", "--no-edit", "other")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run() // conflict expected; exit status intentionally ignored

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	snap, err := ctrl.TakeSnapshot()
	if err == nil {
		t.Fatalf("TakeSnapshot on a conflicted merge = %+v, nil, want an error", snap)
	}
	if !errors.Is(err, ErrUnmergedIndex) {
		t.Errorf("TakeSnapshot error = %v, want it to wrap ErrUnmergedIndex", err)
	}
}

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

func TestTakeSnapshotDanglingUntrackedSymlinkDoesNotError(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	snap, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot with a dangling untracked symlink present: %v", err)
	}
	if _, ok := snap.untrackedHashes["dangling"]; !ok {
		t.Errorf("untrackedHashes missing %q, has %v", "dangling", snap.untrackedHashes)
	}
}

func TestCompareChangedUntrackedSymlinkTargetIsUnequal(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "target-a"), link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot before: %v", err)
	}

	if err := os.Remove(link); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "target-b"), link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot after: %v", err)
	}

	equal, differing := Compare(before, after)
	if equal {
		t.Fatalf("Compare() equal = true, want false after changing the untracked symlink's target")
	}
	if !reflect.DeepEqual(differing, []string{"dangling"}) {
		t.Errorf("Compare() differing = %v, want [dangling]", differing)
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

func TestTakeSnapshotLeavesStatusUnchanged(t *testing.T) {
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

func TestSnapshotJSONRoundTripMatchesCompareBehavior(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	original, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded Snapshot
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fresh, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot fresh: %v", err)
	}

	wantEqual, wantDiffering := Compare(original, fresh)
	gotEqual, gotDiffering := Compare(&loaded, fresh)

	if gotEqual != wantEqual {
		t.Errorf("Compare(loaded, fresh) equal = %v, want %v", gotEqual, wantEqual)
	}
	if !reflect.DeepEqual(gotDiffering, wantDiffering) {
		t.Errorf("Compare(loaded, fresh) differing = %v, want %v", gotDiffering, wantDiffering)
	}
}

func TestSnapshotUnmarshalMissingFieldFailsClosed(t *testing.T) {
	full := map[string]any{
		"headCommit":      "abc123",
		"indexTree":       "def456",
		"porcelainStatus": "",
		"untrackedHashes": map[string]string{},
	}

	for _, missing := range []string{"headCommit", "indexTree", "porcelainStatus", "untrackedHashes"} {
		t.Run(missing, func(t *testing.T) {
			doc := make(map[string]any, len(full)-1)
			for k, v := range full {
				if k != missing {
					doc[k] = v
				}
			}
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("Marshal fixture: %v", err)
			}

			var s Snapshot
			err = json.Unmarshal(data, &s)
			if err == nil {
				t.Fatalf("Unmarshal missing %q = nil, want error", missing)
			}
			if !errors.Is(err, ErrSnapshotFieldMissing) {
				t.Errorf("Unmarshal error = %v, want it to wrap ErrSnapshotFieldMissing", err)
			}
		})
	}
}

func TestTakeSnapshot_ConcurrentCallsDoNotCollide(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	const n = 8
	snaps := make([]*Snapshot, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			snaps[i], errs[i] = ctrl.TakeSnapshot()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("TakeSnapshot[%d]: %v", i, err)
		}
	}
	if t.Failed() {
		return
	}

	want := snaps[0].indexTree
	for i, snap := range snaps {
		if snap.indexTree != want {
			t.Errorf("TakeSnapshot[%d].indexTree = %q, want %q (snapshot 0)", i, snap.indexTree, want)
		}
	}
}
