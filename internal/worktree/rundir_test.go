package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRunDirIsOutsideRepoAndContainsRunID(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	const runID = "run-123"
	dir, err := ctrl.NewRunDir(runID)
	if err != nil {
		t.Fatalf("NewRunDir(%q): %v", runID, err)
	}

	if isWithin(ctrl.repoRoot, dir) {
		t.Errorf("run dir %q is under the repository %q, want outside", dir, ctrl.repoRoot)
	}
	if !isWithin(ctrl.tempRoot, dir) {
		t.Errorf("run dir %q not under verifier root %q", dir, ctrl.tempRoot)
	}
	if filepath.Base(dir) != runID {
		t.Errorf("run dir base = %q, want %q", filepath.Base(dir), runID)
	}
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		t.Errorf("run dir %q does not exist as a directory: stat err = %v", dir, statErr)
	}
}

func TestNewRunDirDuplicateRunIDErrors(t *testing.T) {
	ctrl, err := NewController(newTestRepo(t))
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	const runID = "run-dup"
	first, err := ctrl.NewRunDir(runID)
	if err != nil {
		t.Fatalf("NewRunDir(%q) first call: %v", runID, err)
	}

	second, err := ctrl.NewRunDir(runID)
	if err == nil {
		t.Fatalf("NewRunDir(%q) second call = %q, nil, want an error for a duplicate run ID", runID, second)
	}

	entries, rerr := os.ReadDir(filepath.Dir(first))
	if rerr != nil {
		t.Fatalf("ReadDir(%q): %v", filepath.Dir(first), rerr)
	}
	if len(entries) != 1 {
		t.Errorf("runs dir entries = %v after duplicate NewRunDir, want exactly the first call's directory", entries)
	}
}
