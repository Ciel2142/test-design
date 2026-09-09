package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/report"
)

// TestRecordAttemptLayer3RefusesASourceWithoutALiveDecision locks the layer-3
// gate: an attempt cannot stand in for a live layer the document never decided
// it has.
func TestRecordAttemptLayer3RefusesASourceWithoutALiveDecision(t *testing.T) {
	repo := fixtureRepo(t)
	body, err := json.Marshal(map[string]any{
		"schemaVersion": "1", "repoRoot": repo, "coverageSource": fixtureCoverageSource,
		"runID": "undecided-live-attempt", "layer": 3, "refusal": "live profile unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(body))
	if code != exitError || stdout != "" {
		t.Fatalf("undecided live layer acquired evidence: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want live-decision-incomplete", got.Code)
	}
	if !strings.Contains(got.Message, "no ## Live decision section") {
		t.Errorf("message = %q, want the missing code's own detail, not any refusal carrying \"missing\"", got.Message)
	}
}

// TestRecordAttemptLayer3NotApplicableSaysSo names the fault the operator has
// to repair: under a not-applicable decision there is no live layer to attest
// to, which is a different repair from scoping an attempt to one row.
func TestRecordAttemptLayer3NotApplicableSaysSo(t *testing.T) {
	repo := fixtureRepo(t)
	addNotApplicableLiveDecision(t, repo)
	body, err := json.Marshal(map[string]any{
		"schemaVersion": "1", "repoRoot": repo, "coverageSource": fixtureCoverageSource,
		"runID": "not-applicable-live-attempt", "layer": 3, "refusal": "live profile unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(body))
	if code != exitError || stdout != "" {
		t.Fatalf("a document with no live layer acquired evidence: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "no-applicable-rows" {
		t.Errorf("error code = %q, want no-applicable-rows", got.Code)
	}
	if !strings.Contains(got.Message, "not-applicable") {
		t.Errorf("message = %q, want it to name the not-applicable live decision", got.Message)
	}
	if strings.Contains(got.Message, "individual row") {
		t.Errorf("message = %q, want it not to blame a row-scoped attempt that was never made", got.Message)
	}
}

func TestRecordAttemptCannotAttestToAnUnrelatedRow(t *testing.T) {
	repo := fixtureRepo(t)
	body, err := json.Marshal(map[string]any{
		"schemaVersion": "1", "repoRoot": repo, "coverageSource": fixtureCoverageSource,
		"runID": "foreign-attempt", "layer": 4, "rowID": "SC-999", "refusal": "runner unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(body))
	if code != exitError || stdout != "" || !strings.Contains(stderr, "unknown-row") {
		t.Fatalf("unrelated row acquired evidence: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

// The rest of this file proves the layer-3 record-attempt green path over a
// real liveSubject: a required live decision, a refused live run, and the
// attempt document that refusal leaves behind, which finalize accepts as a
// PARTIAL live-profile-unavailable reason rather than blocking outright. No
// prepare-live or execute-live runs here; a refused live run never reaches
// them, so the attempt is recorded straight off the real trace.

const (
	recordAttemptSubjectCoverageSource = "docs/superpowers/coverage/2026-09-07-record-attempt-live-coverage.md"
	recordAttemptSubjectProfilePath    = ".test-design/live-profiles/record-attempt-local.json"
	recordAttemptSubjectRow            = "SC-01"
)

const recordAttemptSubjectGoMod = `module recordattemptsubject

go 1.26
`

const recordAttemptSubjectSource = `package subject

// Ready reports the subject is alive.
func Ready() string { return "record-attempt-ready" }
`

// recordAttemptSubjectTestSource is the subject's own tagged test. Its
// coverage tag cites the subject document's SC-01, never a row of this
// repository: the text lives inside a string literal here and only becomes a
// comment in the temp repository the builder writes.
const recordAttemptSubjectTestSource = `package subject

import "testing"

// covers: SC-01
func TestReady(t *testing.T) {
	if got := Ready(); got != "record-attempt-ready" {
		t.Fatalf("Ready() = %q, want record-attempt-ready", got)
	}
}
`

// recordAttemptSubjectCoverageDoc carries a go runner block, a required live
// decision whose single journey names the one row, and that row below high
// risk so layer 4 has no obligation. BASELINE_COMMIT and BASELINE_SHA256 are
// pinned by newLiveSubject.
const recordAttemptSubjectCoverageDoc = `# Coverage: record-attempt live subject

Spec: docs/superpowers/specs/2026-09-07-record-attempt-live-subject.md
Baseline: BASELINE_COMMIT · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-record-attempt-live-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/record-attempt-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-record-attempt-live-coverage.md"]

## Live decision

- decision: required
- reason: the row is only observable against the running subject

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | Against the running subject the walkthrough prints record-attempt-ready |

## Scenarios

### Behavior: record-attempt live subject walkthrough

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the walkthrough reaches the ready marker | walkthrough step prints record-attempt-ready | smoke | ready marker never printed | low |
`

// recordAttemptSubjectProfile is never read by this file's test: a refused
// live run never reaches prepare-live or execute-live, so nothing parses this
// file's content. It is still committed so the coverage document's
// live-profile key names a real path, the way a genuine subject would.
const recordAttemptSubjectProfile = `{
  "schemaVersion": "1",
  "classification": "local",
  "start": [],
  "health": [],
  "walkthrough": [["echo", "record-attempt-ready"]],
  "cleanup": [],
  "cleanupEvidence": [],
  "envAllowlist": ["PATH"],
  "resourcePrefixes": [],
  "endpoints": []
}
`

// newRecordAttemptLiveSubject names the file set this file's green-path proof
// runs against and hands it to the shared builder, which commits it, runs the
// real baseline op and freezes HEAD.
func newRecordAttemptLiveSubject(t *testing.T) *liveSubject {
	t.Helper()
	return newLiveSubject(t, recordAttemptSubjectCoverageSource, recordAttemptSubjectProfilePath, []map[string]string{{
		"go.mod":                           recordAttemptSubjectGoMod,
		"subject/subject.go":               recordAttemptSubjectSource,
		"subject/subject_test.go":          recordAttemptSubjectTestSource,
		recordAttemptSubjectCoverageSource: recordAttemptSubjectCoverageDoc,
		recordAttemptSubjectProfilePath:    recordAttemptSubjectProfile,
	}}, nil)
}

// liveAttemptFinalizeRequestJSON assembles a finalize request around the
// subject's real trace evidence and layer-2 bookkeeping, naming a layer-3
// attempt document instead of live evidence — the shape finalize sees when a
// live run was refused before it ran.
func liveAttemptFinalizeRequestJSON(t *testing.T, s *liveSubject, runID, tracePath, attemptPath string) string {
	t.Helper()
	src, cliErr := loadVerifiedSource("finalize", s.repo, s.coverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	tr, err := report.LoadTrace(tracePath)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	active := report.ActiveRows(src)
	gaps := report.Layer1Gaps(tr)

	body, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:  evidence.SchemaVersion,
		RepoRoot:       s.repo,
		CoverageSource: s.coverageSource,
		RunID:          runID,
		finalizeRequestBody: finalizeRequestBody{
			Approval:      finalizeApproval,
			TraceEvidence: tracePath,
			Freshness:     report.Freshness{Result: "fresh"},
			Layer2:        finalizeLayer2{Batches: layer2ForSubject(runID, active, gaps)},
			Layer3:        &finalizeLayer3{AttemptEvidencePath: attemptPath},
		},
	})
	if err != nil {
		t.Fatalf("marshal finalize request: %v", err)
	}
	return string(body)
}

// TestRecordAttemptLayer3GreenPathAcceptedByFinalize drives the real trace and
// record-attempt ops over a subject whose live decision is required, then
// hands the attempt document record-attempt wrote to a real finalize run. The
// attempt document is expected to carry the subject's own run, commit and
// source identity, and finalize is expected to fold the refusal into a
// PARTIAL live-profile-unavailable reason rather than blocking the run.
func TestRecordAttemptLayer3GreenPathAcceptedByFinalize(t *testing.T) {
	runID := liveRunID("record-attempt-green")
	subject := newRecordAttemptLiveSubject(t)

	code, stdout, stderr := runVerifyOp(t, "trace", liveRequestJSON(t, subject.repo, subject.coverageSource, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)
	if tr.BaselineStatus != "ok" {
		t.Fatalf("trace baselineStatus = %q, want ok", tr.BaselineStatus)
	}

	const refusal = "live profile unavailable: no environment configured for this run"
	attemptRequest, err := json.Marshal(map[string]any{
		"schemaVersion": evidence.SchemaVersion, "repoRoot": subject.repo, "coverageSource": subject.coverageSource,
		"runID": runID, "layer": 3, "refusal": refusal,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runVerifyOp(t, "record-attempt", string(attemptRequest))
	if code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var attemptOut attemptOutput
	if err := json.Unmarshal([]byte(stdout), &attemptOut); err != nil {
		t.Fatalf("record-attempt stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	data, err := os.ReadFile(attemptOut.EvidencePath)
	if err != nil {
		t.Fatalf("read attempt evidence %s: %v", attemptOut.EvidencePath, err)
	}
	var attemptDoc attemptEvidence
	if err := json.Unmarshal(data, &attemptDoc); err != nil {
		t.Fatalf("attempt evidence did not decode: %v\ndata=%s", err, data)
	}
	if attemptDoc.RunID != runID {
		t.Errorf("attempt evidence runID = %q, want %q", attemptDoc.RunID, runID)
	}
	if attemptDoc.RepoCommit != subject.head {
		t.Errorf("attempt evidence repoCommit = %q, want the subject's HEAD %q", attemptDoc.RepoCommit, subject.head)
	}
	if attemptDoc.Source != subject.coverageSource {
		t.Errorf("attempt evidence source = %q, want %q", attemptDoc.Source, subject.coverageSource)
	}
	if attemptDoc.SourceSHA256 == "" {
		t.Error("attempt evidence sourceSha256 is empty, want the coverage document's hash")
	}
	if attemptDoc.Refusal != refusal {
		t.Errorf("attempt evidence refusal = %q, want %q", attemptDoc.Refusal, refusal)
	}

	code, stdout, stderr = runVerifyOp(t, "finalize",
		liveAttemptFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, attemptOut.EvidencePath))
	if code != exitOK {
		t.Fatalf("finalize refused the recorded attempt: exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictPartial {
		t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictPartial, out.Reasons)
	}
	requireReason(t, out.Reasons, "3", "", "live-profile-unavailable")

	rep := readPublishedReport(t, subject.repo, out.JSONPath)
	if rep.Verdict != report.VerdictPartial {
		t.Errorf("published verdict = %q, want %q", rep.Verdict, report.VerdictPartial)
	}
	requireReason(t, rep.Reasons, "3", "", "live-profile-unavailable")
	row := publishedRow(t, rep, recordAttemptSubjectRow)
	if row.Layer3 != "N/A (profile unavailable)" {
		t.Errorf("published layer3 for %s = %q, want N/A (profile unavailable)", recordAttemptSubjectRow, row.Layer3)
	}
}
