package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"test-design/internal/report"
)

const e2eFinalizeRunID = "e2e-finalize"

var finishedAtField = regexp.MustCompile(`(?m)^(\s*"finishedAt": ).*$`)

// TestE2E_FinalizePipeline runs baseline, trace and finalize over
// fixtures/go-project and checks the fixture's seeded findings reach the
// published verification report as a BLOCKED verdict.
func TestE2E_FinalizePipeline(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "baseline", verifyRequestJSON(t, repo, "e2e-finalize-baseline"))
	if code != exitOK {
		t.Fatalf("baseline exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var baseOut baselineOutput
	if err := json.Unmarshal([]byte(stdout), &baseOut); err != nil {
		t.Fatalf("baseline stdout did not decode: %v", err)
	}
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "BASELINE_SHA256", baseOut.EvidenceSHA256)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline sha256")

	code, stdout, stderr = runVerifyOp(t, "trace", verifyRequestJSON(t, repo, e2eFinalizeRunID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	traceOut := decodeTraceOutput(t, stdout)

	// The layers reach finalize the way the workflow leaves them: reviewed
	// through record-review, and — for SC-07 and SC-10, the fixture's high-risk
	// rows, both bound by layer 1 under structural credit — an environmental
	// attempt through record-attempt, so this test never invokes a mutator.
	recordAllRowsReviewed(t, repo, fixtureCoverageSource, e2eFinalizeRunID, traceOut.DispatchPath)
	recordEnvironmentalMutations(t, repo, fixtureCoverageSource, e2eFinalizeRunID, traceOut.DispatchPath)
	attemptRows := requiredMutationRows(t, traceOut.DispatchPath)

	requestBody := finalizeRequestJSONFor(t, repo, fixtureCoverageSource, e2eFinalizeRunID, freshFinalizeBody())

	code, stdout, stderr = runVerifyOp(t, "finalize", requestBody)
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)

	if out.Verdict != report.VerdictBlocked {
		t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictBlocked, out.Reasons)
	}
	codes := map[string]bool{}
	for _, r := range out.Reasons {
		codes[r.Code] = true
	}
	// overclaim is gone from the vocabulary: citing several active IDs at one
	// site is credit. drift still comes from SC-90 and SC-77, and ambiguity
	// from the scanner-ambiguous site at orders/orders_test.go:45.
	for _, kind := range []string{"gap", "red", "drift", "ambiguity"} {
		if !codes[kind] {
			t.Errorf("reasons carry no %q code; reasons=%+v", kind, out.Reasons)
		}
	}
	if !codes["MISSING"] {
		t.Errorf("reasons carry no layer-2 MISSING code; reasons=%+v", out.Reasons)
	}

	transcripts := "docs/superpowers/coverage/transcripts/" + e2eFinalizeRunID
	published := []string{
		"docs/superpowers/coverage/2026-08-30-fixture-verification.json",
		"docs/superpowers/coverage/2026-08-30-fixture-verification.md",
		transcripts + "/trace-evidence.json",
	}
	published = append(published, transcripts+"/review-all.json")
	for _, rowID := range attemptRows {
		published = append(published, transcripts+"/mutation-attempt-"+rowID+".json")
	}
	for _, rel := range published {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); err != nil {
			t.Errorf("stat published %s: %v", rel, err)
		}
	}
	if out.TranscriptsDir != transcripts {
		t.Errorf("transcriptsDir = %q, want %q", out.TranscriptsDir, transcripts)
	}

	publishedJSON := filepath.Join(repo, filepath.FromSlash(out.JSONPath))
	first, err := os.ReadFile(publishedJSON)
	if err != nil {
		t.Fatalf("read published report: %v", err)
	}

	mustGit(t, repo, "clean", "-qfd")
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Fatalf("git status = %v, want a clean tree before the rerun", lines)
	}

	code, _, stderr = runVerifyOp(t, "finalize", requestBody)
	if code != exitOK {
		t.Fatalf("second finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	second, err := os.ReadFile(publishedJSON)
	if err != nil {
		t.Fatalf("read republished report: %v", err)
	}
	maskFinishedAt := func(b []byte) string {
		return finishedAtField.ReplaceAllString(string(b), `${1}"MASKED",`)
	}
	if maskFinishedAt(first) != maskFinishedAt(second) {
		t.Errorf("two finalize runs over identical inputs differ beyond finishedAt:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// requiredMutationRows names the rows the dispatch inputs oblige to layer 4:
// the high-risk rows layer 1 bound, which is the set recordEnvironmentalMutations
// answers for.
func requiredMutationRows(t *testing.T, dispatchPath string) []string {
	t.Helper()
	var rows []string
	for _, row := range readDispatchInputs(t, dispatchPath).Rows {
		if row.Risk == "high" && row.Status != "gap" && row.Status != "ambiguity" {
			rows = append(rows, row.RowID)
		}
	}
	if len(rows) == 0 {
		t.Fatal("the fixture has no bound high-risk row; the layer-4 bookkeeping would prove nothing")
	}
	return rows
}
