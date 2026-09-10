package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// newHandoffSubject names the file set this file's proofs run against and hands
// it to the shared builder, which commits it, runs the real baseline op and
// freezes HEAD.
func newHandoffSubject(t *testing.T) *liveSubject {
	t.Helper()
	return newLiveSubject(t, handoffCoverageSource, handoffProfilePath, []map[string]string{{
		"go.mod":                  handoffGoMod,
		"handoff/handoff.go":      handoffSource,
		"handoff/handoff_test.go": handoffTestSource,
		handoffCoverageSource:     handoffCoverageDoc,
		handoffProfilePath:        handoffProfile,
	}}, nil)
}

// handoffPlan is the plan the live-runner would hand to execute-live: one step
// per journey row, each command drawn from the profile's declared set and each
// row carrying a predicate that settles it.
func handoffPlan(runID string) map[string]any {
	return map[string]any{
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
	}
}

// covers: SC-08
func TestLiveHandoff_GapRowExecutesAndStaysTraceGap(t *testing.T) {
	const runID = "handoff-gap"
	subject := newHandoffSubject(t)
	_, _, exec := runLivePipeline(t, subject, runID, handoffPlan(runID), nil)

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
		liveFinalizeRequestJSON(t, subject, runID))
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
	_, prep, exec := runLivePipeline(t, subject, runID, handoffPlan(runID), nil)

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
	_, _, exec := runLivePipeline(t, subject, runID, handoffPlan(runID), nil)

	// Hashed before finalize reads it: the report's transcript reference has to
	// describe these exact bytes, not a re-serialization of them.
	want, err := evidence.SHA256File(exec.EvidencePath)
	if err != nil {
		t.Fatalf("hash live evidence: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "finalize",
		liveFinalizeRequestJSON(t, subject, runID))
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
