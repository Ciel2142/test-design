package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTakeSnapshotIgnoresInheritedGitDirEnv(t *testing.T) {
	dir := newTestRepo(t)
	other := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(other, "tracked.txt"), []byte("a different repo entirely\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, other, "add", "tracked.txt")
	mustGit(t, other, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "diverge")

	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	otherHead := strings.TrimSpace(mustGit(t, other, "rev-parse", "HEAD"))
	dirHead := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
	if otherHead == dirHead {
		t.Fatalf("test fixture invalid: both repos share HEAD %q", dirHead)
	}

	t.Setenv("GIT_DIR", other+"/.git")
	t.Setenv("GIT_WORK_TREE", other)

	snap, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot with inherited GIT_DIR set: %v", err)
	}
	if snap.headCommit != dirHead {
		t.Errorf("headCommit = %q, want %q (the controller's own repo, not the GIT_DIR-targeted repo %q)", snap.headCommit, dirHead, otherHead)
	}
}
