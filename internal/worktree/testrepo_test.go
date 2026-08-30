package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, dir, "add", "tracked.txt")
	mustGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "init")
	return dir
}
