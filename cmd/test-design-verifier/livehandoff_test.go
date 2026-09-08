package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/live"
	"test-design/internal/report"
)

// The three proofs in this file drive the real ops — baseline, trace,
// prepare-live, execute-live, finalize — over a purpose-built subject
// repository, and read only the files those ops emit. No evidence document is
// written by hand.
const (
	handoffCoverageSource = "docs/superpowers/coverage/2026-09-07-live-handoff-coverage.md"
	handoffProfilePath    = ".test-design/live-profiles/handoff-local.json"
	handoffBaselineRunID  = "handoff-baseline"

	// SC-01 is the subject row its own tagged test cites; SC-02 is the row no
	// test cites, so layer 1 reports it as a gap while a journey still names it.
	handoffBoundRow = "SC-01"
	handoffGapRow   = "SC-02"

	handoffReadyMarker = "handoff-ready"
)

const handoffGoMod = `module livehandoff

go 1.26
`

const handoffSource = `package handoff

// Ready is the marker the subject prints once it is running.
func Ready() string { return "handoff-ready" }
`

// handoffTestSource is the subject's own tagged test. Its coverage tag cites
// the subject document's SC-01, never a row of this repository: the text lives
// inside a string literal here and only becomes a comment in the temp
// repository the builder writes.
const handoffTestSource = `package handoff

import "testing"

// covers: SC-01
func TestReadyMarker(t *testing.T) {
	if got := Ready(); got != "handoff-ready" {
		t.Fatalf("Ready() = %q, want handoff-ready", got)
	}
}
`

// handoffCoverageDoc carries a go runner block, a required live decision whose
// single journey names both rows, and two rows below high risk so layer 4 has
// no obligation. BASELINE_COMMIT and BASELINE_SHA256 are pinned by the builder.
const handoffCoverageDoc = `# Coverage: live handoff subject

Spec: docs/superpowers/specs/2026-09-07-live-handoff-subject.md
Baseline: BASELINE_COMMIT · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-live-handoff-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/handoff-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-live-handoff-coverage.md"]

## Live decision

- decision: required
- reason: both rows are only observable against the running subject

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01, SC-02 | Against the running subject the walkthrough prints handoff-ready and the scratch resource the profile created can be read |

## Scenarios

### Behavior: live handoff walkthrough

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the walkthrough reaches the ready marker | walkthrough step prints handoff-ready | smoke | ready marker never printed | med |
| SC-02 | the started resource is readable while the subject runs | the scratch resource can be read | smoke | resource never created | low |
`

// handoffProfile is the committed local profile: start creates the run-named
// scratch resource, health proves it exists, and cleanup removes it and then
// proves its absence with a removal predicate over the search that follows.
const handoffProfile = `{
  "schemaVersion": "1",
  "classification": "local",
  "start": [["touch", "scratch-{run-id}.txt"]],
  "health": [["ls", "scratch-{run-id}.txt"]],
  "walkthrough": [["echo", "handoff-ready"], ["cat", "scratch-{run-id}.txt"]],
  "cleanup": [
    ["rm", "-f", "scratch-{run-id}.txt"],
    ["find", ".", "-maxdepth", "1", "-name", "scratch-{run-id}.txt"]
  ],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "not-matches", "stream": "stdout", "value": "scratch-"}
  ],
  "envAllowlist": ["PATH"],
  "resourcePrefixes": ["scratch-{run-id}"],
  "endpoints": []
}
`

// handoffSubject is the temporary repository the proofs run against, at the
// clean commit every op in the pipeline must agree on.
type handoffSubject struct {
	repo string
	head string
}

func writeSubjectFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func rewriteHandoffDoc(t *testing.T, repo string, replace func(string) string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(handoffCoverageSource))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read subject coverage doc: %v", err)
	}
	if err := os.WriteFile(path, []byte(replace(string(data))), 0o644); err != nil {
		t.Fatalf("write subject coverage doc: %v", err)
	}
}

// newHandoffSubject builds the subject repository and runs the real baseline
// op against it, so the trace that follows compares against a verified
// baseline and reports baselineStatus ok rather than a PARTIAL condition.
func newHandoffSubject(t *testing.T) *handoffSubject {
	t.Helper()
	isolateEnv(t)
	repo := t.TempDir()

	writeSubjectFile(t, repo, "go.mod", handoffGoMod)
	writeSubjectFile(t, repo, "handoff/handoff.go", handoffSource)
	writeSubjectFile(t, repo, "handoff/handoff_test.go", handoffTestSource)
	writeSubjectFile(t, repo, handoffCoverageSource, handoffCoverageDoc)
	writeSubjectFile(t, repo, handoffProfilePath, handoffProfile)

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "live handoff subject")
	baselineCommit := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	rewriteHandoffDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "BASELINE_COMMIT", baselineCommit)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline commit")

	code, stdout, stderr := runVerifyOp(t, "baseline", handoffRequestJSON(t, repo, handoffBaselineRunID))
	if code != exitOK {
		t.Fatalf("baseline exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var baseOut baselineOutput
	if err := json.Unmarshal([]byte(stdout), &baseOut); err != nil {
		t.Fatalf("baseline stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	// The baseline op leaves its published evidence untracked; committing it
	// together with the pinned digest returns the subject to a clean HEAD.
	rewriteHandoffDoc(t, repo, func(s string) string {
		return strings.ReplaceAll(s, "BASELINE_SHA256", baseOut.EvidenceSHA256)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline evidence")

	return &handoffSubject{repo: repo, head: strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))}
}

func handoffRequestJSON(t *testing.T, repo, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       repo,
		"coverageSource": handoffCoverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func handoffExecuteRequestJSON(t *testing.T, repo, runDir, planPath, tracePath string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       repo,
		"coverageSource": handoffCoverageSource,
		"runDir":         runDir,
		"plan":           planPath,
		"traceEvidence":  tracePath,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

// handoffPlanPath writes the plan the live-runner would hand to execute-live:
// one step per journey row, each command drawn from the profile's declared set
// and each row carrying a predicate that settles it.
func handoffPlanPath(t *testing.T, runID string) string {
	t.Helper()
	return writePlanFile(t, map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{
			{
				"expectation": "the running subject prints its ready marker",
				"command":     []string{"echo", handoffReadyMarker},
				"predicates": []map[string]string{
					{"form": "exit-code", "value": "0"},
					{"form": "matches", "stream": "stdout", "value": handoffReadyMarker},
				},
				"scBindings": []string{handoffBoundRow},
			},
			{
				"expectation": "the scratch resource the profile started is readable while the subject runs",
				"command":     []string{"cat", "scratch-" + runID + ".txt"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{handoffGapRow},
			},
		},
	})
}

// runHandoffLive drives trace, prepare-live and execute-live over the subject
// under one run ID, the way the workflow does, and fails on any refusal so a
// regression surfaces with the op's own error code.
func runHandoffLive(t *testing.T, s *handoffSubject, runID string) (traceOutput, prepareLiveOutput, executeLiveOutput) {
	t.Helper()

	code, stdout, stderr := runVerifyOp(t, "trace", handoffRequestJSON(t, s.repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)
	if tr.BaselineStatus != "ok" {
		t.Fatalf("trace baselineStatus = %q, want ok", tr.BaselineStatus)
	}

	code, stdout, stderr = runVerifyOp(t, "prepare-live", handoffRequestJSON(t, s.repo, runID))
	if code != exitOK {
		t.Fatalf("prepare-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var prep prepareLiveOutput
	if err := json.Unmarshal([]byte(stdout), &prep); err != nil {
		t.Fatalf("prepare-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	code, stdout, stderr = runVerifyOp(t, "execute-live",
		handoffExecuteRequestJSON(t, s.repo, prep.RunDir, handoffPlanPath(t, runID), tr.EvidencePath))
	if code != exitOK {
		t.Fatalf("execute-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var exec executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &exec); err != nil {
		t.Fatalf("execute-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return tr, prep, exec
}

// layer2ForSubject reuses the bookkeeping shape the e2e finalize proof builds —
// one caught synthesized canary, its row re-dispatched alone against the real
// source, the rest inheriting the catch, gap rows MISSING — and stamps this
// run's own id, which staleness validation ties every batch to.
func layer2ForSubject(runID string, active []coverage.Row, gaps map[string]bool) []report.Batch {
	batches := layer2FromTrace(active, gaps)
	for i := range batches {
		batches[i].RunID = runID
	}
	return batches
}

// handoffFinalizeRequestJSON assembles the finalize request around the evidence
// the pipeline actually produced: the trace file, the layer-2 verdicts it
// implies, and the live evidence path execute-live wrote.
func handoffFinalizeRequestJSON(t *testing.T, s *handoffSubject, runID, tracePath, livePath string) string {
	t.Helper()
	src, cliErr := loadVerifiedSource("finalize", s.repo, handoffCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	tr, err := report.LoadTrace(tracePath)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	active := report.ActiveRows(src)
	gaps := report.Layer1Gaps(tr)
	if !gaps[handoffGapRow] {
		t.Fatalf("trace gaps = %v, want the untested row %s reported as a layer-1 gap", gaps, handoffGapRow)
	}

	body, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:  evidence.SchemaVersion,
		RepoRoot:       s.repo,
		CoverageSource: handoffCoverageSource,
		RunID:          runID,
		finalizeRequestBody: finalizeRequestBody{
			Approval:      finalizeApproval,
			TraceEvidence: tracePath,
			Freshness:     report.Freshness{Result: "fresh"},
			Layer2:        finalizeLayer2{Batches: layer2ForSubject(runID, active, gaps)},
			Layer3:        &finalizeLayer3{EvidencePath: livePath},
		},
	})
	if err != nil {
		t.Fatalf("marshal finalize request: %v", err)
	}
	return string(body)
}

func readPublishedReport(t *testing.T, repo, jsonPath string) *report.Report {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(jsonPath)))
	if err != nil {
		t.Fatalf("read published report: %v", err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("decode published report: %v", err)
	}
	return &rep
}

func publishedRow(t *testing.T, rep *report.Report, rowID string) report.RowReport {
	t.Helper()
	for _, row := range rep.Rows {
		if row.RowID == rowID {
			return row
		}
	}
	t.Fatalf("report rows = %+v, want an entry for %s", rep.Rows, rowID)
	return report.RowReport{}
}

func publishedTranscript(t *testing.T, rep *report.Report, path string) report.TranscriptRef {
	t.Helper()
	for _, ref := range rep.Transcripts {
		if ref.Path == path {
			return ref
		}
	}
	t.Fatalf("report transcripts = %+v, want an entry for %s", rep.Transcripts, path)
	return report.TranscriptRef{}
}

func requireReason(t *testing.T, reasons []report.Reason, layer, rowID, code string) {
	t.Helper()
	for _, r := range reasons {
		if r.Layer == layer && r.RowID == rowID && r.Code == code {
			return
		}
	}
	t.Errorf("reasons = %+v, want a layer-%s %q reason for %s", reasons, layer, code, rowID)
}

// covers: SC-08
func TestLiveHandoff_GapRowExecutesAndStaysTraceGap(t *testing.T) {
	const runID = "handoff-gap"
	subject := newHandoffSubject(t)
	tr, _, exec := runHandoffLive(t, subject, runID)

	// The plan binds the row layer 1 reported as a gap, and execute-live runs
	// it rather than refusing the binding or standing a placeholder in for it.
	ev := readLiveEvidence(t, exec.EvidencePath)
	gap := rowVerdict(t, ev.RowVerdicts, handoffGapRow)
	if gap.Verdict != "LIVE-PASS" {
		t.Errorf("live verdict for %s = %q, want the bound step's real verdict LIVE-PASS", handoffGapRow, gap.Verdict)
	}
	if !gap.Layer1Gap {
		t.Errorf("%s verdict = %+v, want layer1Gap true beside its real verdict", handoffGapRow, gap)
	}
	if bound := rowVerdict(t, ev.RowVerdicts, handoffBoundRow); bound.Layer1Gap {
		t.Errorf("%s verdict = %+v, want layer1Gap false for a row layer 1 bound", handoffBoundRow, bound)
	}
	for _, v := range ev.RowVerdicts {
		if strings.HasPrefix(v.Verdict, "LIVE-N/A") {
			t.Errorf("live verdict for %s = %q, want a real verdict, never a layer-1-gap placeholder", v.RowID, v.Verdict)
		}
	}

	code, stdout, stderr := runVerifyOp(t, "finalize",
		handoffFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, exec.EvidencePath))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictBlocked {
		t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictBlocked, out.Reasons)
	}
	// The layer-1 gap is what blocks the branch; the live pass never retires it.
	requireReason(t, out.Reasons, "1", handoffGapRow, "gap")

	rep := readPublishedReport(t, subject.repo, out.JSONPath)
	row := publishedRow(t, rep, handoffGapRow)
	if row.Layer1.Status != "gap" {
		t.Errorf("published layer1 for %s = %+v, want status gap", handoffGapRow, row.Layer1)
	}
	if row.Layer3 != "LIVE-PASS" {
		t.Errorf("published layer3 for %s = %q, want LIVE-PASS", handoffGapRow, row.Layer3)
	}
	if got := rep.Layers.Layer3; got.Applicable != 2 || got.Pass != 2 {
		t.Errorf("layer3 summary = %+v, want both journeyed rows applicable and passing", got)
	}
}

// covers: SC-09
func TestLiveHandoff_ResolvedProfileAcceptedAndResourceRemoved(t *testing.T) {
	const runID = "handoff-resource"
	subject := newHandoffSubject(t)
	// execute-live validates the stored descriptor with ValidateResolved before
	// it starts anything: a rejected run-ID-bearing prefix would surface here as
	// a descriptor-invalid refusal rather than the exit-0 run below.
	_, prep, exec := runHandoffLive(t, subject, runID)

	wantPrefix := "scratch-" + runID
	if got := descriptorPrefixes(t, prep.Descriptor); len(got) != 1 || got[0] != wantPrefix {
		t.Errorf("prepared resourcePrefixes = %v, want [%s]", got, wantPrefix)
	}
	st, err := live.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := descriptorPrefixes(t, st.Descriptor); len(got) != 1 || got[0] != wantPrefix {
		t.Errorf("state resourcePrefixes after the run = %v, want the resolved [%s] kept", got, wantPrefix)
	}

	ev := readLiveEvidence(t, exec.EvidencePath)
	if len(ev.StartSteps) != 1 || ev.StartSteps[0].ExitStatus != 0 {
		t.Fatalf("start steps = %+v, want the one start command to create the resource and exit 0", ev.StartSteps)
	}
	if last := ev.StartSteps[0].Command; last[len(last)-1] != wantPrefix+".txt" {
		t.Errorf("start command = %v, want it to end in the substituted %s.txt", last, wantPrefix)
	}
	if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].ExitStatus != 0 {
		t.Errorf("health steps = %+v, want the resource to exist while the subject ran", ev.HealthSteps)
	}

	if !exec.CleanupOK || exec.CleanupFailed {
		t.Errorf("cleanupOK = %v, cleanupFailed = %v, want true/false", exec.CleanupOK, exec.CleanupFailed)
	}
	if !ev.CleanupOK {
		t.Errorf("evidence cleanupOK = false, want the removal predicates satisfied; cleanup=%+v", ev.CleanupSteps)
	}
	if len(ev.CleanupSteps) != 2 {
		t.Fatalf("cleanup steps = %+v, want both profile cleanup commands recorded", ev.CleanupSteps)
	}
	if last := ev.CleanupSteps[1]; last.Result != "pass" {
		t.Errorf("removal predicate step = %+v, want it to pass", last)
	}
	// The removal predicate is evaluated over a real search of the run's
	// working directory: an empty stdout is the resource proven gone on disk.
	raw, err := os.ReadFile(filepath.Join(prep.RunDir, "cleanup-2", "stdout.raw"))
	if err != nil {
		t.Fatalf("read the removal search transcript: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("removal search stdout = %q, want no surviving resource", raw)
	}

	if _, err := os.Stat(filepath.Join(subject.repo, wantPrefix+".txt")); !os.IsNotExist(err) {
		t.Errorf("the subject repository holds %s.txt, want the run to leave nothing behind", wantPrefix)
	}
	if lines := gitStatusLines(t, subject.repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the subject repository untouched", lines)
	}
}

// descriptorPrefixes reads the resourcePrefixes of a prepared descriptor.
func descriptorPrefixes(t *testing.T, descriptor json.RawMessage) []string {
	t.Helper()
	var doc struct {
		ResourcePrefixes []string `json:"resourcePrefixes"`
	}
	if err := json.Unmarshal(descriptor, &doc); err != nil {
		t.Fatalf("descriptor did not decode: %v", err)
	}
	return doc.ResourcePrefixes
}

// covers: SC-19
func TestLiveHandoff_FinalizeAcceptsRealLiveEvidence(t *testing.T) {
	const runID = "handoff-evidence"
	subject := newHandoffSubject(t)
	tr, _, exec := runHandoffLive(t, subject, runID)

	// Hashed before finalize reads it: the report's transcript reference has to
	// describe these exact bytes, not a re-serialization of them.
	want, err := evidence.SHA256File(exec.EvidencePath)
	if err != nil {
		t.Fatalf("hash live evidence: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "finalize",
		handoffFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, exec.EvidencePath))
	if code != exitOK {
		t.Fatalf("finalize refused real live evidence: exit code = %d, stderr=%s", code, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)

	rep := readPublishedReport(t, subject.repo, out.JSONPath)
	transcript := out.TranscriptsDir + "/" + liveEvidenceFile
	if got := publishedTranscript(t, rep, transcript).SHA256; got != want {
		t.Errorf("transcript sha256 for %s = %q, want the hash of the file finalize read %q", transcript, got, want)
	}
	// The recorded hash must describe the copy the report ships, too.
	got, err := evidence.SHA256File(filepath.Join(subject.repo, filepath.FromSlash(transcript)))
	if err != nil {
		t.Fatalf("hash the published transcript: %v", err)
	}
	if got != want {
		t.Errorf("published transcript sha256 = %q, want %q", got, want)
	}

	if rep.HeadCommit != subject.head {
		t.Errorf("report headCommit = %q, want the subject's HEAD %q", rep.HeadCommit, subject.head)
	}
	if ev := readLiveEvidence(t, exec.EvidencePath); ev.RepoCommit != subject.head {
		t.Errorf("live evidence repoCommit = %q, want the subject's HEAD %q", ev.RepoCommit, subject.head)
	}
}
