package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
)

// doc-supplied paths get joined onto roots the ops own; none may steer outside
func TestDocSuppliedPathsAreConfined(t *testing.T) {
	fields := []struct {
		name     string
		line     string
		absolute string
		escape   string
	}{
		{"runner-root", "- runner-root: .", "- runner-root: /etc", "- runner-root: ../.."},
		{
			"baseline-evidence",
			"- baseline-evidence: docs/superpowers/coverage/2026-08-30-fixture-baseline.json",
			"- baseline-evidence: /tmp/planted-baseline.json",
			"- baseline-evidence: ../../planted-baseline.json",
		},
	}
	for _, op := range []string{"baseline", "trace"} {
		for _, field := range fields {
			for shape, replacement := range map[string]string{"absolute": field.absolute, "traversal": field.escape} {
				t.Run(op+"/"+field.name+"/"+shape, func(t *testing.T) {
					repo := fixtureRepo(t)
					rewriteFixtureDoc(t, repo, func(s string) string {
						return strings.ReplaceAll(s, field.line, replacement)
					})
					mustGit(t, repo, "add", "-A")
					mustGit(t, repo, "commit", "-q", "-m", "doc path outside its root")

					code, stdout, stderr := runVerifyOp(t, op, verifyRequestJSON(t, repo, "badpath"))

					if code != exitError {
						t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
					}
					if !strings.Contains(stderr, field.name) || !strings.Contains(stderr, "escapes its root") {
						t.Errorf("stderr = %q, want a %s confinement refusal", stderr, field.name)
					}
					if left := verifierWorktrees(t); len(left) != 0 {
						t.Errorf("worktrees = %v, want the refusal to land before any worktree work", left)
					}
				})
			}
		}
	}
}

// a ../-escape reaches a real file; unconfined it would verify as the baseline
func TestTrace_BaselineOutsideRepoIsNeverRead(t *testing.T) {
	repo := fixtureRepo(t)
	body := []byte(`{"schemaVersion":"1","verifierVersion":"0.6.0"}`)
	planted := filepath.Join(filepath.Dir(repo), "planted-baseline.json")
	if err := os.WriteFile(planted, body, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	plantedSHA, err := evidence.SHA256File(planted)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	rewriteFixtureDoc(t, repo, func(s string) string {
		s = strings.ReplaceAll(s,
			"- baseline-evidence: docs/superpowers/coverage/2026-08-30-fixture-baseline.json",
			"- baseline-evidence: ../planted-baseline.json")
		return strings.ReplaceAll(s, "BASELINE_SHA256", plantedSHA)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "baseline evidence planted outside the repo")

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, "planted"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if strings.Contains(stdout, `"baselineStatus":"ok"`) {
		t.Errorf("stdout = %s, want no verified status from a baseline outside the repository", stdout)
	}
	if !strings.Contains(stderr, "baseline-evidence") || !strings.Contains(stderr, "escapes its root") {
		t.Errorf("stderr = %q, want a baseline-evidence confinement refusal", stderr)
	}
}

// only go test's own trailing result line is volatile; a panic's duration is evidence
func TestCanonicalizeReport_ScrubsOnlyTheTrailingResultLine(t *testing.T) {
	rep := &gotestjson.Report{
		Failures: []gotestjson.RunnerFailure{{
			Package: "example/pkg",
			Detail:  "panic: timeout after 1.500s waiting for lock\nFAIL\texample/pkg\t0.003s\n",
		}},
	}

	canonicalizeReport(rep)

	got := rep.Failures[0].Detail
	if !strings.Contains(got, "panic: timeout after 1.500s waiting for lock") {
		t.Errorf("detail = %q, want the panic's own duration untouched", got)
	}
	if !strings.Contains(got, "FAIL\texample/pkg\t<elapsed>") {
		t.Errorf("detail = %q, want the trailing result line's elapsed time scrubbed", got)
	}
}
