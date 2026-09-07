package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/report"
)

const e2eFinalizeRunID = "e2e-finalize"

// layer2FromTrace builds the bookkeeping a critic dispatch would have produced
// for this trace: gap rows can only be MISSING, the rest HOLDS, all inheriting
// one caught synthesized canary.
func layer2FromTrace(active []coverage.Row, gaps map[string]bool) []report.Batch {
	canaryRow := ""
	for _, row := range active {
		if !gaps[row.ID] {
			canaryRow = row.ID
			break
		}
	}
	var rest []string
	var verdicts []report.CriticVerdict
	for _, row := range active {
		if row.ID == canaryRow {
			continue
		}
		verdict := "HOLDS"
		if gaps[row.ID] {
			verdict = "MISSING"
		}
		rest = append(rest, row.ID)
		verdicts = append(verdicts, report.CriticVerdict{RowID: row.ID, Verdict: verdict, Reason: "critic verdict for " + row.ID})
	}
	return []report.Batch{
		{
			BatchID: "cb1", DispatchID: "d1", RunID: e2eFinalizeRunID, Rows: []string{canaryRow},
			Canary: &report.Canary{RowID: canaryRow, Kind: "synthesized", Weakening: "dropped the boundary assertion", Caught: true},
		},
		{
			BatchID: "b1", DispatchID: "d2", RunID: e2eFinalizeRunID, Rows: []string{canaryRow}, InheritsFrom: "cb1",
			Verdicts: []report.CriticVerdict{{RowID: canaryRow, Verdict: "HOLDS"}},
		},
		{
			BatchID: "b2", DispatchID: "d3", RunID: e2eFinalizeRunID, Rows: rest, InheritsFrom: "cb1",
			Verdicts: verdicts,
		},
	}
}

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

	src, cliErr := loadVerifiedSource("finalize", repo, fixtureCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	tr, err := report.LoadTrace(traceOut.EvidencePath)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	active := report.ActiveRows(src)
	gaps := report.Layer1Gaps(tr)

	// SC-10 is the one high-risk bound row; its mutation run is reported as
	// environmental so this test never runs a mutator.
	attemptPath := filepath.Join(t.TempDir(), "mutation-attempt-SC-10.json")
	if err := evidence.WriteJSON(attemptPath, map[string]string{
		"schemaVersion":   evidence.SchemaVersion,
		"verifierVersion": evidence.VerifierVersion,
		"runID": e2eFinalizeRunID,
		"repoCommit": tr.RepoCommit,
		"source": src.Path,
		"sourceSha256": src.SHA256,
		"rowID":           "SC-10",
		"refusal":         "runner-start-failed",
	}); err != nil {
		t.Fatalf("write attempt evidence: %v", err)
	}

	requestBody, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:  evidence.SchemaVersion,
		RepoRoot:       repo,
		CoverageSource: fixtureCoverageSource,
		RunID:          e2eFinalizeRunID,
		finalizeRequestBody: finalizeRequestBody{
			Approval:      finalizeApproval,
			TraceEvidence: traceOut.EvidencePath,
			Freshness:     report.Freshness{Result: "fresh"},
			Layer2:        finalizeLayer2{Batches: layer2FromTrace(active, gaps)},
			Layer4: finalizeLayer4{Rows: []report.MutationInput{{
				RowID:               "SC-10",
				Outcome:             "environmental",
				Reason:              "the mutation runner could not start in this environment",
				AttemptEvidencePath: attemptPath,
			}}},
		},
	})
	if err != nil {
		t.Fatalf("marshal finalize request: %v", err)
	}

	code, stdout, stderr = runVerifyOp(t, "finalize", string(requestBody))
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
	for _, kind := range []string{"gap", "red", "drift", "overclaim", "ambiguity"} {
		if !codes[kind] {
			t.Errorf("reasons carry no %q code; reasons=%+v", kind, out.Reasons)
		}
	}
	if !codes["MISSING"] {
		t.Errorf("reasons carry no layer-2 MISSING code; reasons=%+v", out.Reasons)
	}

	transcripts := "docs/superpowers/coverage/transcripts/" + e2eFinalizeRunID
	for _, rel := range []string{
		"docs/superpowers/coverage/2026-08-30-fixture-verification.json",
		"docs/superpowers/coverage/2026-08-30-fixture-verification.md",
		transcripts + "/trace-evidence.json",
		transcripts + "/mutation-attempt-SC-10.json",
	} {
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

	code, _, stderr = runVerifyOp(t, "finalize", string(requestBody))
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
