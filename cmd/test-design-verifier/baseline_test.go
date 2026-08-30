package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/trace"
)

func TestBaseline_DirtyRepositoryRefused(t *testing.T) {
	repo := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "orders", "scratch.txt"), []byte("dirt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "dirty"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "before snapshot is not clean") {
		t.Errorf("stderr = %q, want Publish's dirty vocabulary", stderr)
	}
	if !strings.Contains(stderr, "orders/scratch.txt") {
		t.Errorf("stderr = %q, want the dirty path named", stderr)
	}
}

func TestBaseline_MissingCountOneRefused(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, `["go", "test", "-count=1", "-json", "./..."]`, `["go", "test", "-json", "./..."]`)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "drop count")

	code, stdout, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "nocount"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if !strings.Contains(stderr, "go suite command must include -count=1") {
		t.Errorf("stderr = %q, want the -count=1 refusal", stderr)
	}
}

func TestBaseline_NonGoRunnerRefused(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "- runner-kind: go", "- runner-kind: jvm")
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "switch runner kind")

	code, _, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "jvm"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "unsupported runner kind jvm") {
		t.Errorf("stderr = %q, want %q", stderr, "unsupported runner kind jvm")
	}
}

func TestBaseline_MissingCoverageSourceNamesPath(t *testing.T) {
	repo := fixtureRepo(t)
	request := `{"schemaVersion":"1","repoRoot":"` + repo + `","coverageSource":"docs/superpowers/coverage/absent-coverage.md","runID":"absent"}`

	code, _, stderr := runVerifyOp(t, "baseline", request)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "docs/superpowers/coverage/absent-coverage.md") {
		t.Errorf("stderr = %q, want the missing source path named", stderr)
	}
}

func TestBaseline_CoverageSourceOutsideRepoRefused(t *testing.T) {
	repo := fixtureRepo(t)
	request := `{"schemaVersion":"1","repoRoot":"` + repo + `","coverageSource":"../escape-coverage.md","runID":"escape"}`

	code, _, stderr := runVerifyOp(t, "baseline", request)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "escapes repository root") {
		t.Errorf("stderr = %q, want an escape refusal", stderr)
	}
}

func TestBaseline_InvalidRunIDRefused(t *testing.T) {
	repo := fixtureRepo(t)

	code, _, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "../escape"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "run ID") {
		t.Errorf("stderr = %q, want a run ID refusal", stderr)
	}
}

func TestBaseline_RequestRequired(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"baseline"}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	got := decodeError(t, &stderr)
	if got.Code != "missing-request" {
		t.Errorf("error code = %q, want %q", got.Code, "missing-request")
	}
}

func TestBaseline_E2EPublishesEvidenceBesideCoverageDoc(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "baseline-e2e"))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out baselineOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.EvidencePath != "docs/superpowers/coverage/2026-08-30-fixture-baseline.json" {
		t.Errorf("evidencePath = %q, want the coverage doc's sibling baseline path", out.EvidencePath)
	}
	if out.ExitStatus == 0 {
		t.Errorf("exitStatus = 0, want the fixture suite's nonzero exit recorded as a valid baseline")
	}

	published := filepath.Join(repo, filepath.FromSlash(out.EvidencePath))
	b, err := trace.LoadBaseline(published, out.EvidenceSHA256)
	if err != nil {
		t.Fatalf("published baseline did not load: %v", err)
	}
	if b.RepoCommit == "" || b.RepoCommit != strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD~1")) {
		t.Errorf("repoCommit = %q, want the coverage doc's BASELINE_COMMIT", b.RepoCommit)
	}
	if b.NormalizationError != "" {
		t.Errorf("normalizationError = %q, want empty", b.NormalizationError)
	}
	want := trace.BaselineIdentity{Scope: "fixturemod/orders", Name: "TestValidateQuantityRejectsZero"}
	if !slices.Contains(b.Failed, want) {
		t.Errorf("baseline Failed = %v, want it to contain the seeded deliberate failure %v", b.Failed, want)
	}

	lines := gitStatusLines(t, repo)
	if len(lines) != 1 || !strings.Contains(lines[0], out.EvidencePath) {
		t.Errorf("git status = %v, want exactly the published evidence file and nothing else", lines)
	}
}

func TestBaseline_SecondPublishOnStaleSnapshotRefused(t *testing.T) {
	repo := fixtureRepo(t)

	if code, _, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "first")); code != exitOK {
		t.Fatalf("first baseline exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	code, _, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "second"))

	if code != exitError {
		t.Fatalf("second baseline exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "before snapshot is not clean") {
		t.Errorf("stderr = %q, want the dirty refusal naming the first run's own output", stderr)
	}
}
