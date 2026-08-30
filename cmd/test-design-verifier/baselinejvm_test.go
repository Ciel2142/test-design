package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
	"test-design/internal/trace"
)

const mavenCoverageSource = "docs/superpowers/coverage/2026-08-30-maven-fixture-coverage.md"

func mavenFixtureRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not on PATH")
	}
	isolateEnv(t)
	repo := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "fixtures", "maven-java-project"), repo)

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "fixture")
	baselineCommit := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	path := filepath.Join(repo, filepath.FromSlash(mavenCoverageSource))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage doc: %v", err)
	}
	rewritten := strings.ReplaceAll(string(data), "BASELINE_COMMIT", baselineCommit)
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		t.Fatalf("write coverage doc: %v", err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline commit")
	return repo
}

func mavenRequestJSON(t *testing.T, repo, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": mavenCoverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func TestBaselineJVM_MavenE2EPublishesEvidence(t *testing.T) {
	repo := mavenFixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "baseline", mavenRequestJSON(t, repo, "maven-baseline"))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out baselineOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.ExitStatus == 0 {
		t.Errorf("exitStatus = 0, want nonzero from the seeded failing test")
	}

	published := filepath.Join(repo, filepath.FromSlash(out.EvidencePath))
	b, err := trace.LoadBaseline(published, out.EvidenceSHA256)
	if err != nil {
		t.Fatalf("published baseline did not load: %v", err)
	}
	if b.RunnerKind != "maven" {
		t.Errorf("runnerKind = %q, want maven", b.RunnerKind)
	}
	if b.NormalizationError != "" {
		t.Errorf("normalizationError = %q, want empty", b.NormalizationError)
	}
	if b.BaselineCommit == "" || b.BaselineCommit != b.RepoCommit {
		t.Errorf("baselineCommit = %q, repoCommit = %q, want both the pinned commit", b.BaselineCommit, b.RepoCommit)
	}

	wantFailed := trace.BaselineIdentity{Scope: "com.example.OrdersTest", Name: "failsDeliberately"}
	if !slices.Contains(b.Failed, wantFailed) {
		t.Errorf("Failed = %v, want %v", b.Failed, wantFailed)
	}
	wantSkipped := trace.BaselineIdentity{Scope: "com.example.OrdersTest", Name: "skippedDeliberately"}
	if !slices.Contains(b.Skipped, wantSkipped) {
		t.Errorf("Skipped = %v, want %v", b.Skipped, wantSkipped)
	}

	var rep struct {
		Records  []runrecord.Record         `json:"records"`
		Counts   gotestjson.Counts          `json:"counts"`
		Failures []gotestjson.RunnerFailure `json:"failures"`
	}
	data, err := os.ReadFile(filepath.Join(out.RunDir, normalizedFile))
	if err != nil {
		t.Fatalf("normalized report missing: %v", err)
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("normalized report did not decode: %v", err)
	}
	if !slices.Contains(rep.Records, runrecord.Record{Status: runrecord.Passed, Scope: "com.example.OuterTest$InnerTest", Name: "nestedPasses"}) {
		t.Errorf("Records = %+v, want nested Outer$Inner identity passed", rep.Records)
	}
	param := 0
	for _, r := range rep.Records {
		if r.Scope == "com.example.ParamTest" && r.Name == "allPositive" {
			param++
			if r.Status != runrecord.Passed {
				t.Errorf("ParamTest.allPositive = %+v, want passed", r)
			}
		}
	}
	if param != 1 {
		t.Errorf("ParamTest.allPositive families = %d, want exactly 1 collapsed family", param)
	}
	if rep.Counts.Passed < 5 {
		t.Errorf("Counts = %+v, want raw per-invocation tally with at least 5 passed", rep.Counts)
	}

	if _, ok := b.ToolVersions["mvn"]; !ok {
		t.Errorf("ToolVersions = %v, want an mvn entry", b.ToolVersions)
	}
	if _, ok := b.ToolVersions["java"]; !ok {
		t.Errorf("ToolVersions = %v, want a java entry", b.ToolVersions)
	}

	lines := gitStatusLines(t, repo)
	if len(lines) != 1 || !strings.Contains(lines[0], out.EvidencePath) {
		t.Errorf("git status = %v, want exactly the published evidence file", lines)
	}
}
