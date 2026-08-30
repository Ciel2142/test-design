package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrace_UncommittedChangeRefused(t *testing.T) {
	repo := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "orders", "orders.go"), []byte("package orders\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "dirty"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if !strings.Contains(stderr, "source not reproducible: uncommitted changes in") {
		t.Errorf("stderr = %q, want the source-not-reproducible refusal", stderr)
	}
	if !strings.Contains(stderr, "orders/orders.go") {
		t.Errorf("stderr = %q, want the uncommitted path named", stderr)
	}
}

func TestTrace_ConcurrentEditDuringRunDetected(t *testing.T) {
	repo := fixtureRepo(t)
	req := &verifyRequest{
		SchemaVersion:  "1",
		RepoRoot:       repo,
		CoverageSource: fixtureCoverageSource,
		RunID:          "concurrent",
	}

	cliErr := traceOp(req, io.Discard, func() {
		if err := os.WriteFile(filepath.Join(repo, "orders", "sneaked.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	})

	if cliErr == nil {
		t.Fatal("traceOp returned no error, want the concurrent-change refusal")
	}
	want := "source repository changed during verification: orders/sneaked.txt; commit or discard the concurrent change, then rerun"
	if !strings.Contains(cliErr.Message, want) {
		t.Errorf("error = %q, want it to contain %q", cliErr.Message, want)
	}
}

func TestTrace_NonGoRunnerRefused(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "- runner-kind: go", "- runner-kind: jvm")
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "switch runner kind")

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "jvm"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "unsupported runner kind jvm") {
		t.Errorf("stderr = %q, want %q", stderr, "unsupported runner kind jvm")
	}
}

func TestTrace_MissingCountOneRefused(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, `["go", "test", "-count=1", "-json", "./..."]`, `["go", "test", "-json", "./..."]`)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "drop count")

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "nocount"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "go suite command must include -count=1") {
		t.Errorf("stderr = %q, want the -count=1 refusal", stderr)
	}
}

func TestTrace_MissingGoModRefused(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "- runner-root: .", "- runner-root: docs")
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "point runner root at a dir with no go.mod")

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "nogomod"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "go.mod") {
		t.Errorf("stderr = %q, want a missing go.mod refusal", stderr)
	}
}

func TestTrace_UnexplainedRunnerFailureFailsClosed(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, `["go", "test", "-count=1", "-json", "./..."]`, `["go", "help", "-count=1"]`)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "runner that fails without reporting anything")

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "unexplained"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "exited 2") || !strings.Contains(stderr, "no failed test and no runner failure") {
		t.Errorf("stderr = %q, want an unexplained-failure refusal", stderr)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want trace to have written nothing into the repository", lines)
	}
}

func TestTrace_MissingBaselineIsRecordedNotFatal(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "nobaseline"))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	if out.BaselineStatus != "missing" {
		t.Errorf("baselineStatus = %q, want %q", out.BaselineStatus, "missing")
	}
	ev := readTraceEvidence(t, out)
	if ev.BaselineStatus != "missing" {
		t.Errorf("evidence baselineStatus = %q, want %q", ev.BaselineStatus, "missing")
	}
	if len(ev.NewFailures) != 0 {
		t.Errorf("newFailures = %v, want none without a baseline to compare against", ev.NewFailures)
	}
}

func TestTrace_InvalidBaselineIsRecordedNotFatal(t *testing.T) {
	repo := fixtureRepo(t)
	baselinePath := filepath.Join(repo, "docs", "superpowers", "coverage", "2026-08-30-fixture-baseline.json")
	if err := os.WriteFile(baselinePath, []byte(`{"schemaVersion":"1"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "BASELINE_SHA256", strings.Repeat("0", 64))
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "baseline with a mismatched hash")

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "badbaseline"))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	if !strings.HasPrefix(out.BaselineStatus, "invalid: ") {
		t.Errorf("baselineStatus = %q, want an %q condition", out.BaselineStatus, "invalid: <why>")
	}
	if !strings.Contains(out.BaselineStatus, "hash mismatch") {
		t.Errorf("baselineStatus = %q, want the mismatch named", out.BaselineStatus)
	}
}
