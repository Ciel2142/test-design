package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const gradleCoverageSource = "docs/superpowers/coverage/2026-08-30-gradle-fixture-coverage.md"

const ghostXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.GhostTest" tests="1">
  <testcase classname="com.example.GhostTest" name="staleGhost"/>
</testsuite>`

func gradleFixtureRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("gradle"); err != nil {
		sdkman := filepath.Join(os.Getenv("HOME"), ".sdkman", "candidates", "gradle", "current", "bin")
		if _, statErr := os.Stat(filepath.Join(sdkman, "gradle")); statErr != nil {
			t.Skip("gradle not on PATH and no sdkman install")
		}
		t.Setenv("PATH", sdkman+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	isolateEnv(t)
	repo := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "fixtures", "gradle-kotlin-project"), repo)

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "fixture")
	baselineCommit := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	rewriteDoc(t, repo, gradleCoverageSource, "BASELINE_COMMIT", baselineCommit)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline commit")
	return repo
}

func rewriteDoc(t *testing.T, repo, docPath, old, new string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(docPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage doc: %v", err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("coverage doc does not contain %q", old)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), old, new)), 0o644); err != nil {
		t.Fatalf("write coverage doc: %v", err)
	}
}

func requestFor(t *testing.T, repo, source, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": source,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func runBaselineAndPinSHA(t *testing.T, repo, source string) {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "baseline", requestFor(t, repo, source, "jvm-baseline"))
	if code != exitOK {
		t.Fatalf("baseline exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out baselineOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("baseline stdout did not decode: %v", err)
	}
	rewriteDoc(t, repo, source, "BASELINE_SHA256", out.EvidenceSHA256)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline sha256")
}

func TestTraceJVM_MavenPipeline(t *testing.T) {
	repo := mavenFixtureRepo(t)
	runBaselineAndPinSHA(t, repo, mavenCoverageSource)

	stale := filepath.Join(repo, "target", "surefire-reports", "TEST-com.example.GhostTest.xml")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte(ghostXML), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "seed stale report")

	code, stdout, stderr := runVerifyOp(t, "trace", requestFor(t, repo, mavenCoverageSource, "jvm-trace"))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	ev := readTraceEvidence(t, out)

	if ev.BaselineStatus != "ok" {
		t.Fatalf("baselineStatus = %q, want ok", ev.BaselineStatus)
	}
	if ev.ModulePath != "" {
		t.Errorf("modulePath = %q, want empty for a jvm runner", ev.ModulePath)
	}

	wantRowResult(t, ev, "SC-01", "passed")
	wantRowResult(t, ev, "SC-02", "failed")
	wantFinding(t, ev, "red", "SC-02", "failsDeliberately")
	wantRowResult(t, ev, "SC-03", "skipped")
	wantFinding(t, ev, "gap", "SC-03", "skipped")
	wantRowResult(t, ev, "SC-04", "passed")
	wantRowResult(t, ev, "SC-05", "passed")
	wantFinding(t, ev, "gap", "SC-06", "no test cites SC-06")

	if len(ev.NewFailures) != 0 {
		t.Errorf("newFailures = %v, want none: the failure already failed at baseline", ev.NewFailures)
	}

	normalized, err := os.ReadFile(filepath.Join(out.RunDir, normalizedFile))
	if err != nil {
		t.Fatalf("normalized report missing: %v", err)
	}
	if strings.Contains(string(normalized), "GhostTest") {
		t.Error("normalized report contains GhostTest from the pre-seeded stale XML; fresh files only")
	}

	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want trace to have written nothing", lines)
	}
}

func TestTraceJVM_NoReportsOnExitZeroRefused(t *testing.T) {
	repo := mavenFixtureRepo(t)
	rewriteDoc(t, repo, mavenCoverageSource, `["mvn", "-B", "test"]`, `["mvn", "-B", "-q", "help:help"]`)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "command that produces no reports")

	code, _, stderr := runVerifyOp(t, "trace", requestFor(t, repo, mavenCoverageSource, "noxml"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "normalization-failed") || !strings.Contains(stderr, "no fresh report") {
		t.Errorf("stderr = %q, want normalization-failed on zero fresh reports despite exit 0", stderr)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want nothing written", lines)
	}
}

func TestTraceJVM_GradlePipeline(t *testing.T) {
	repo := gradleFixtureRepo(t)
	runBaselineAndPinSHA(t, repo, gradleCoverageSource)

	code, stdout, stderr := runVerifyOp(t, "trace", requestFor(t, repo, gradleCoverageSource, "gradle-trace"))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	ev := readTraceEvidence(t, out)

	if ev.BaselineStatus != "ok" {
		t.Fatalf("baselineStatus = %q, want ok", ev.BaselineStatus)
	}

	wantRowResult(t, ev, "SC-01", "passed")
	got, ok := rowResultFor(ev, "SC-01")
	if !ok || got.Scope != "com.example.OrderKtTest" || got.Name != "passes with backticks and spaces" {
		t.Errorf("SC-01 = %+v, want the backtick method bound verbatim", got)
	}
	wantRowResult(t, ev, "SC-02", "failed")
	wantFinding(t, ev, "red", "SC-02", "failsDeliberately")
	wantFinding(t, ev, "gap", "SC-03", "no test executed")
	wantFinding(t, ev, "ambiguity", "", "pretty display name")

	if len(ev.NewFailures) != 0 {
		t.Errorf("newFailures = %v, want none", ev.NewFailures)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want trace to have written nothing", lines)
	}
}
