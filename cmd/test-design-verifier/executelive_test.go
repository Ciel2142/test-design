package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	liveFixtureCoverageSource = "docs/superpowers/coverage/2026-08-30-live-fixture-coverage.md"
	liveFixtureProfilePath    = ".test-design/live-profiles/fixture-local.json"
	liveFixturePlanPath       = ".test-design/live-plans/fixture-walkthrough.json"
	liveCanaryName            = "LIVE_FIXTURE_CANARY"
	liveCanaryValue           = "canary-tetragrammaton-9f3a"
)

// liveFixtureRepo copies fixtures/live-project into a temp git repository and
// exports the canary secret the redaction assertions look for; the canary is
// deliberately absent from the profile's envAllowlist.
func liveFixtureRepo(t *testing.T) string {
	t.Helper()
	isolateEnv(t)
	t.Setenv(liveCanaryName, liveCanaryValue)

	repo := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "fixtures", "live-project"), repo)

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "live fixture")
	baselineCommit := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	path := filepath.Join(repo, filepath.FromSlash(liveFixtureCoverageSource))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read live coverage doc: %v", err)
	}
	pinned := strings.ReplaceAll(string(data), "BASELINE_COMMIT", baselineCommit)
	if err := os.WriteFile(path, []byte(pinned), 0o644); err != nil {
		t.Fatalf("write live coverage doc: %v", err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline commit")
	return repo
}

// commitLiveProfileOverride rewrites the fixture profile with overrides applied
// and commits it, so prepare-live still sees a clean source.
func commitLiveProfileOverride(t *testing.T, repo string, overrides map[string]any) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(liveFixtureProfilePath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read live profile fixture: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("live profile fixture did not decode: %v", err)
	}
	for k, v := range overrides {
		doc[k] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal live profile fixture: %v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write live profile fixture: %v", err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "override live profile fixture")
}

func liveFixturePrepareRequestJSON(t *testing.T, repo, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": liveFixtureCoverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func preparedLive(t *testing.T, repo, runID string) prepareLiveOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "prepare-live", liveFixturePrepareRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("prepare-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out prepareLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("prepare-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// writeGapTraceEvidence writes the layer-1 evidence execute-live reads, marking
// every id in gapRows as a gap finding.
func writeGapTraceEvidence(t *testing.T, gapRows ...string) string {
	t.Helper()
	findings := make([]map[string]string, 0, len(gapRows))
	for _, id := range gapRows {
		findings = append(findings, map[string]string{
			"kind":   "gap",
			"rowId":  id,
			"detail": "tagged test is skipped",
		})
	}
	doc := map[string]any{
		"schemaVersion":   "1",
		"verifierVersion": "1",
		"runId":           "trace-run",
		"findings":        findings,
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal trace evidence: %v", err)
	}
	path := filepath.Join(t.TempDir(), traceEvidenceFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write trace evidence: %v", err)
	}
	return path
}

func executeLiveRequestJSON(t *testing.T, repo, runDir, planPath, tracePath string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": liveFixtureCoverageSource,
		"runDir":         runDir,
		"plan":           planPath,
		"traceEvidence":  tracePath,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

// writePlanFile writes a plan document outside the repository, the way the
// live-runner hands one to the op.
func writePlanFile(t *testing.T, doc map[string]any) string {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	path := filepath.Join(t.TempDir(), "live-plan.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

func readLiveEvidence(t *testing.T, path string) liveEvidence {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read live evidence %s: %v", path, err)
	}
	var ev liveEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("live evidence did not decode: %v\ndata=%s", err, data)
	}
	return ev
}

func verdictsByRow(verdicts []liveRowVerdict) map[string]string {
	out := make(map[string]string, len(verdicts))
	for _, v := range verdicts {
		out[v.RowID] = v.Verdict
	}
	return out
}

// assertNoCanaryUnder fails when the canary value appears in any file under dir:
// the sanitized environment must keep it out of every artifact the run leaves.
func assertNoCanaryUnder(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), liveCanaryValue) {
			t.Errorf("%s contains the non-allowlisted canary value", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}

// TestExecuteLive_FixtureWalkthroughVerdicts is the wave's live acceptance: a
// local profile whose start creates a run-ID-named scratch resource, one passing
// and one failing walkthrough expectation, a gap row, and cleanup evidence
// proving the resource is gone.
func TestExecuteLive_FixtureWalkthroughVerdicts(t *testing.T) {
	repo := liveFixtureRepo(t)
	headBefore := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	prep := preparedLive(t, repo, "live-fixture")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	got := verdictsByRow(out.RowVerdicts)
	want := map[string]string{
		"SC-11": "LIVE-PASS",
		"SC-12": "LIVE-FAIL",
		"SC-13": "LIVE-N/A (layer 1 gap)",
	}
	for id, verdict := range want {
		if got[id] != verdict {
			t.Errorf("verdict for %s = %q, want %q", id, got[id], verdict)
		}
	}
	if _, present := got["SC-14"]; present {
		t.Errorf("row verdicts = %+v, want the unit-level SC-14 excluded", out.RowVerdicts)
	}
	if len(out.RowVerdicts) != len(want) {
		t.Errorf("row verdicts = %+v, want exactly one entry per applicable row", out.RowVerdicts)
	}
	if !out.CleanupOK || out.CleanupFailed {
		t.Errorf("cleanupOK = %v, cleanupFailed = %v, want true/false", out.CleanupOK, out.CleanupFailed)
	}

	ev := readLiveEvidence(t, out.EvidencePath)
	if ev.SchemaVersion == "" {
		t.Error("evidence schemaVersion is empty")
	}
	if ev.RunID != "live-fixture" {
		t.Errorf("evidence runId = %q, want live-fixture", ev.RunID)
	}
	if ev.ProfilePath != liveFixtureProfilePath {
		t.Errorf("evidence profilePath = %q, want %q", ev.ProfilePath, liveFixtureProfilePath)
	}
	if len(ev.StartSteps) != 1 || ev.StartSteps[0].ExitStatus != 0 {
		t.Fatalf("start steps = %+v, want one step exiting 0", ev.StartSteps)
	}
	// The substituted resource name only exists in the prepared descriptor:
	// running it proves execute-live never re-reads the repository profile.
	if want := "scratch-live-fixture.txt"; ev.StartSteps[0].Command[len(ev.StartSteps[0].Command)-1] != want {
		t.Errorf("start command = %v, want it to end in the substituted %q", ev.StartSteps[0].Command, want)
	}
	if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].ExitStatus != 0 {
		t.Errorf("health steps = %+v, want the scratch resource to exist at health time", ev.HealthSteps)
	}
	if len(ev.Steps) != 3 {
		t.Fatalf("walkthrough steps = %+v, want every plan step executed", ev.Steps)
	}
	if ev.Steps[0].Result != "pass" || ev.Steps[1].Result != "fail" {
		t.Errorf("step results = %q/%q, want pass/fail (a failing step must not short-circuit the walkthrough)",
			ev.Steps[0].Result, ev.Steps[1].Result)
	}
	// Unsanitized inheritance would let printenv find the canary and exit 0.
	if ev.Steps[2].Result != "pass" || ev.Steps[2].ExitStatus != 1 {
		t.Errorf("environment step = %+v, want printenv to fail to find the non-allowlisted canary", ev.Steps[2])
	}
	// plan cleanup step + two profile cleanup commands.
	if len(ev.CleanupSteps) != 3 {
		t.Errorf("cleanup steps = %+v, want the plan cleanup step then both profile cleanup commands", ev.CleanupSteps)
	}
	if !ev.CleanupOK {
		t.Errorf("evidence cleanupOK = false, want the scratch resource proven gone; cleanup=%+v", ev.CleanupSteps)
	}
	if len(ev.RawArtifacts) == 0 {
		t.Error("raw artifacts = none, want a hash per captured stream")
	}
	for _, a := range ev.RawArtifacts {
		if a.SHA256 == "" {
			t.Errorf("raw artifact %s has no sha256", a.Path)
		}
	}
	if len(ev.ExecutionErrors) != 0 {
		t.Errorf("executionErrors = %v, want none on the happy path", ev.ExecutionErrors)
	}
	if !ev.SnapshotEqual {
		t.Error("evidence snapshotEqual = false, want the completed run stamped after the compare")
	}

	assertNoCanaryUnder(t, prep.RunDir)
	if _, err := os.Stat(prep.WorktreePath); !os.IsNotExist(err) {
		t.Errorf("worktree at %s survived, want it removed after the run", prep.WorktreePath)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want none after execute-live", left)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
	if head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD")); head != headBefore {
		t.Errorf("HEAD = %q, want unchanged %q", head, headBefore)
	}
}

func TestExecuteLive_InexpressiblePredicateRejectedBeforeAnyProcess(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-inexpressible")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := writePlanFile(t, map[string]any{
		"schemaVersion": "1",
		"steps": []map[string]any{
			{
				"expectation": "the happy walkthrough reaches the ready marker",
				"command":     []string{"echo", "live-fixture-ready"},
				"predicates":  []map[string]string{{"form": "eventually-consistent", "value": "ready"}},
				"scBindings":  []string{"SC-11"},
			},
			{
				"expectation": "the non-happy walkthrough is refused",
				"command":     []string{"echo", "live-fixture-accepted"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{"SC-12"},
			},
		},
	})

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "inexpressible-predicate" {
		t.Errorf("error code = %q, want inexpressible-predicate", got.Code)
	}
	if !strings.Contains(got.Message, "eventually-consistent") {
		t.Errorf("message = %q, want it to name the rejected form", got.Message)
	}

	scratch := filepath.Join(prep.WorktreePath, "scratch-live-inexpressible.txt")
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Errorf("scratch resource %s exists, want no process started before plan validation", scratch)
	}
	if _, err := os.Stat(filepath.Join(prep.RunDir, liveEvidenceFile)); !os.IsNotExist(err) {
		t.Error("live-evidence.json exists, want no evidence from a run that never started")
	}
}

func TestExecuteLive_MissingStateRefused(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-no-state")
	if err := os.Remove(filepath.Join(prep.RunDir, "live-state.json")); err != nil {
		t.Fatalf("Remove state: %v", err)
	}
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if got := decodeCLIError(t, stderr); got.Code != "state-load-failed" {
		t.Errorf("error code = %q, want state-load-failed", got.Code)
	}
}

func TestExecuteLive_StartFailureMarksBoundRowsInconclusive(t *testing.T) {
	repo := liveFixtureRepo(t)
	commitLiveProfileOverride(t, repo, map[string]any{
		"start": [][]string{{"/nonexistent/live-fixture-start"}},
	})
	prep := preparedLive(t, repo, "live-start-fail")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	got := verdictsByRow(out.RowVerdicts)
	for _, id := range []string{"SC-11", "SC-12"} {
		if got[id] != "LIVE-INCONCLUSIVE" {
			t.Errorf("verdict for %s = %q, want LIVE-INCONCLUSIVE after a start failure", id, got[id])
		}
	}
	if got["SC-13"] != "LIVE-N/A (layer 1 gap)" {
		t.Errorf("verdict for SC-13 = %q, want the layer-1 gap verdict", got["SC-13"])
	}

	ev := readLiveEvidence(t, out.EvidencePath)
	if len(ev.Steps) != 0 {
		t.Errorf("walkthrough steps = %+v, want none after a start failure", ev.Steps)
	}
	if len(ev.HealthSteps) != 0 {
		t.Errorf("health steps = %+v, want health skipped after a start failure", ev.HealthSteps)
	}
	if len(ev.ExecutionErrors) == 0 {
		t.Error("executionErrors = none, want the start failure recorded")
	}
	if len(ev.CleanupSteps) == 0 {
		t.Error("cleanup steps = none, want cleanup to run on every path after start began")
	}
}

func TestExecuteLive_HealthFailureSkipsWalkthrough(t *testing.T) {
	repo := liveFixtureRepo(t)
	commitLiveProfileOverride(t, repo, map[string]any{
		"start": [][]string{{"echo", "started-without-the-scratch-resource"}},
	})
	prep := preparedLive(t, repo, "live-health-fail")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	got := verdictsByRow(out.RowVerdicts)
	for _, id := range []string{"SC-11", "SC-12"} {
		if got[id] != "LIVE-INCONCLUSIVE" {
			t.Errorf("verdict for %s = %q, want LIVE-INCONCLUSIVE after a health failure", id, got[id])
		}
	}

	ev := readLiveEvidence(t, out.EvidencePath)
	if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].ExitStatus == 0 {
		t.Errorf("health steps = %+v, want one failing health command recorded", ev.HealthSteps)
	}
	if len(ev.Steps) != 0 {
		t.Errorf("walkthrough steps = %+v, want the walkthrough skipped", ev.Steps)
	}
	if !ev.CleanupOK {
		t.Errorf("cleanupOK = false, want cleanup to still run and prove the resource absent; cleanup=%+v", ev.CleanupSteps)
	}
}

func TestExecuteLive_CleanupFailureRecorded(t *testing.T) {
	repo := liveFixtureRepo(t)
	commitLiveProfileOverride(t, repo, map[string]any{
		"cleanup": [][]string{
			{"rm", "-f", "scratch-{run-id}.txt"},
			{"ls", "never-created-{run-id}.txt"},
		},
	})
	prep := preparedLive(t, repo, "live-cleanup-fail")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.CleanupOK || !out.CleanupFailed {
		t.Errorf("cleanupOK = %v, cleanupFailed = %v, want false/true", out.CleanupOK, out.CleanupFailed)
	}
	if got := verdictsByRow(out.RowVerdicts); got["SC-11"] != "LIVE-PASS" {
		t.Errorf("verdict for SC-11 = %q, want the walkthrough verdict kept alongside a failed cleanup", got["SC-11"])
	}

	ev := readLiveEvidence(t, out.EvidencePath)
	if !ev.CleanupFailed {
		t.Error("evidence cleanupFailed = false, want a failed cleanup recorded, never silently dropped")
	}
}

func TestExecuteLive_RedactionBarrierWithholdsStepButKeepsVerdict(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-barrier")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := writePlanFile(t, map[string]any{
		"schemaVersion": "1",
		"steps": []map[string]any{
			{
				"expectation": "the happy walkthrough reaches the ready marker",
				"command":     []string{"echo", liveCanaryValue},
				"predicates": []map[string]string{
					{"form": "matches", "stream": "stdout", "value": liveCanaryValue},
				},
				"scBindings": []string{"SC-11"},
			},
			{
				"expectation": "the non-happy walkthrough is refused",
				"command":     []string{"echo", "live-fixture-accepted"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{"SC-12"},
			},
		},
	})

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	var sc11 liveRowVerdict
	for _, v := range out.RowVerdicts {
		if v.RowID == "SC-11" {
			sc11 = v
		}
	}
	if sc11.Verdict != "LIVE-PASS" {
		t.Errorf("verdict for SC-11 = %q, want the verdict kept behind the redaction barrier", sc11.Verdict)
	}
	if !sc11.RedactionBarrier {
		t.Error("SC-11 redactionBarrier = false, want the barrier recorded for the row")
	}

	ev := readLiveEvidence(t, out.EvidencePath)
	if len(ev.Steps) != 1 {
		t.Errorf("walkthrough steps = %+v, want the barriered step omitted and the other kept", ev.Steps)
	}
	data, err := os.ReadFile(out.EvidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if strings.Contains(string(data), liveCanaryValue) {
		t.Error("live-evidence.json contains the canary value, want the barrier to withhold it")
	}
}

func TestExecuteLive_SnapshotMismatchDetected(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-mismatch")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	req := &executeLiveRequest{
		verifyRequest: verifyRequest{
			SchemaVersion:  "1",
			RepoRoot:       repo,
			CoverageSource: liveFixtureCoverageSource,
		},
		RunDir:        prep.RunDir,
		PlanPath:      planPath,
		TraceEvidence: tracePath,
	}
	cliErr := executeLiveOp(req, discard{}, func() {
		if err := os.WriteFile(filepath.Join(repo, "sneaked.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	})
	if cliErr == nil {
		t.Fatal("executeLiveOp returned no error, want the snapshot-mismatch refusal")
	}
	if cliErr.Code != "snapshot-mismatch" {
		t.Errorf("error code = %q, want snapshot-mismatch", cliErr.Code)
	}
	want := "sneaked.txt; commit or discard the concurrent change, then rerun"
	if !strings.Contains(cliErr.Message, want) {
		t.Errorf("message = %q, want it to contain %q", cliErr.Message, want)
	}

	// The blocked run still leaves its evidence behind; the unstamped
	// snapshotEqual is what keeps finalize from reading it as a good run.
	ev := readLiveEvidence(t, filepath.Join(prep.RunDir, liveEvidenceFile))
	if ev.SnapshotEqual {
		t.Error("evidence snapshotEqual = true after a snapshot mismatch, want it left unstamped")
	}
	if len(ev.RowVerdicts) == 0 {
		t.Error("blocked evidence has no row verdicts, want the run it did perform still recorded")
	}
}

// TestExecuteLive_WorstVerdictWinsAcrossSteps locks the merge order for a row
// bound to several steps: FAIL beats INCONCLUSIVE beats PASS, and a later
// passing step never upgrades an earlier verdict.
func TestExecuteLive_WorstVerdictWinsAcrossSteps(t *testing.T) {
	repo := liveFixtureRepo(t)
	commitLiveProfileOverride(t, repo, map[string]any{
		"walkthrough": [][]string{{"echo", "live-fixture-ready"}, {"/nonexistent/live-fixture-step"}},
	})
	prep := preparedLive(t, repo, "live-worst-wins")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := writePlanFile(t, map[string]any{
		"schemaVersion": "1",
		"steps": []map[string]any{
			{
				"expectation": "both rows start from a passing step",
				"command":     []string{"echo", "live-fixture-ready"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{"SC-11", "SC-12"},
			},
			{
				"expectation": "SC-11 also has an unmet expectation",
				"command":     []string{"echo", "live-fixture-accepted"},
				"predicates": []map[string]string{
					{"form": "matches", "stream": "stdout", "value": "live-fixture-refused"},
				},
				"scBindings": []string{"SC-11"},
			},
			{
				"expectation": "SC-12 also has a step that cannot run at all",
				"command":     []string{"/nonexistent/live-fixture-step"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{"SC-12"},
			},
			{
				"expectation": "a later passing step must not upgrade either row",
				"command":     []string{"echo", "live-fixture-ready"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{"SC-11", "SC-12"},
			},
		},
	})

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	got := verdictsByRow(out.RowVerdicts)
	if got["SC-11"] != "LIVE-FAIL" {
		t.Errorf("verdict for SC-11 = %q, want LIVE-FAIL to beat the two passing steps", got["SC-11"])
	}
	if got["SC-12"] != "LIVE-INCONCLUSIVE" {
		t.Errorf("verdict for SC-12 = %q, want LIVE-INCONCLUSIVE to beat the two passing steps", got["SC-12"])
	}

	ev := readLiveEvidence(t, out.EvidencePath)
	if len(ev.Steps) != 3 {
		t.Errorf("walkthrough steps = %+v, want the step that never started absent from the transcripts", ev.Steps)
	}
	if len(ev.ExecutionErrors) != 1 {
		t.Errorf("executionErrors = %v, want exactly the failed process start recorded", ev.ExecutionErrors)
	}
}

func TestExecuteLive_MissingPlanRefused(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-no-plan")
	tracePath := writeGapTraceEvidence(t, "SC-13")

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, filepath.Join(repo, "no-such-plan.json"), tracePath))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if got := decodeCLIError(t, stderr); got.Code != "plan-unreadable" {
		t.Errorf("error code = %q, want plan-unreadable", got.Code)
	}
}

func TestExecuteLive_MissingTraceEvidenceRefused(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-no-trace")
	planPath := filepath.Join(repo, filepath.FromSlash(liveFixturePlanPath))

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, filepath.Join(t.TempDir(), "missing.json")))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if got := decodeCLIError(t, stderr); got.Code != "trace-evidence-unreadable" {
		t.Errorf("error code = %q, want trace-evidence-unreadable", got.Code)
	}
}

// TestExecuteLive_GapRowMustNotBeBound locks the plan-validation refusal at the
// op boundary: a plan that claims a layer-1 gap row is rejected, not run.
func TestExecuteLive_GapRowMustNotBeBound(t *testing.T) {
	repo := liveFixtureRepo(t)
	prep := preparedLive(t, repo, "live-gap-bound")
	tracePath := writeGapTraceEvidence(t, "SC-13")
	planPath := writePlanFile(t, map[string]any{
		"schemaVersion": "1",
		"steps": []map[string]any{
			{
				"expectation": "the happy walkthrough reaches the ready marker",
				"command":     []string{"echo", "live-fixture-ready"},
				"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
				"scBindings":  []string{"SC-11", "SC-12", "SC-13"},
			},
		},
	})

	code, stdout, stderr := runVerifyOp(t, "execute-live",
		executeLiveRequestJSON(t, repo, prep.RunDir, planPath, tracePath))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "gap-row-bound" {
		t.Errorf("error code = %q, want gap-row-bound", got.Code)
	}
	if !strings.Contains(got.Message, "SC-13") {
		t.Errorf("message = %q, want it to name SC-13", got.Message)
	}
}
