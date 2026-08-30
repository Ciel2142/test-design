package main

import (
	"strings"
	"testing"

	"test-design/internal/gotestjson"
)

func TestRunnerRootOutsideWorktreeRefused(t *testing.T) {
	roots := map[string]string{
		"absolute":  "/etc",
		"traversal": "../..",
	}
	for _, op := range []string{"baseline", "trace"} {
		for name, root := range roots {
			t.Run(op+"/"+name, func(t *testing.T) {
				repo := fixtureRepo(t)
				rewriteFixtureDoc(t, repo, func(s string) string {
					return strings.ReplaceAll(s, "- runner-root: .", "- runner-root: "+root)
				})
				mustGit(t, repo, "add", "-A")
				mustGit(t, repo, "commit", "-q", "-m", "runner root outside the worktree")

				code, stdout, stderr := runVerifyOp(t, op, verifyRequestJSON(t, repo, "badroot"))

				if code != exitError {
					t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
				}
				if !strings.Contains(stderr, "runner-root") || !strings.Contains(stderr, "escapes its root") {
					t.Errorf("stderr = %q, want a runner-root confinement refusal", stderr)
				}
				if left := verifierWorktrees(t); len(left) != 0 {
					t.Errorf("worktrees = %v, want the refusal to land before any worktree work", left)
				}
			})
		}
	}
}

// A duration quoted inside a panic or build message is part of the failure the
// developer must read; only go test's own trailing result line is volatile.
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
