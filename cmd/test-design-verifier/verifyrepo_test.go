package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureCoverageSource = "docs/superpowers/coverage/2026-08-30-fixture-coverage.md"

func isolateEnv(t *testing.T) {
	t.Helper()
	tempBase := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("TMPDIR", tempBase)
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

func rewriteFixtureDoc(t *testing.T, repo string, replace func(string) string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(fixtureCoverageSource))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage doc: %v", err)
	}
	if err := os.WriteFile(path, []byte(replace(string(data))), 0o644); err != nil {
		t.Fatalf("write coverage doc: %v", err)
	}
}

// fixtureRepo copies fixtures/go-project into a temp git repository, pins the
// coverage doc's BASELINE_COMMIT placeholder to the first commit, and returns
// the repository root at a clean HEAD.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	isolateEnv(t)
	repo := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "fixtures", "go-project"), repo)

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "fixture")
	baselineCommit := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "BASELINE_COMMIT", baselineCommit)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline commit")
	return repo
}

func verifyRequestJSON(t *testing.T, repo, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func runVerifyOp(t *testing.T, op, request string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{op, "--request", "-"}, strings.NewReader(request), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func gitStatusLines(t *testing.T, repo string) []string {
	t.Helper()
	out := mustGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all")
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
