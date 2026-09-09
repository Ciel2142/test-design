package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// requestWith is verifyRequestJSON with extra top-level fields, so a test can
// add repeat, a group or a row to the request shape every op shares.
func requestWith(t *testing.T, repo, runID string, extra map[string]any) string {
	t.Helper()
	body := map[string]any{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          runID,
	}
	for name, value := range extra {
		body[name] = value
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(data)
}

// fixtureVerifyRequest is the decoded request shape the ops carry, for a test
// that reaches the ledger plumbing directly rather than through an op.
func fixtureVerifyRequest(repo, runID string) *verifyRequest {
	return &verifyRequest{
		SchemaVersion:  evidence.SchemaVersion,
		RepoRoot:       repo,
		CoverageSource: fixtureCoverageSource,
		RunID:          runID,
	}
}

// startLedgerRecord opens the run's real ledger the way the ops do and opens a
// started record for (kind, key). It stands in for the op that starts the
// obligation in production — prepare-mutation for a mutation row — so a test
// can drive the op that finishes one.
func startLedgerRecord(t *testing.T, repo, runID string, kind runstate.Kind, key string, inputs any) {
	t.Helper()
	req := fixtureVerifyRequest(repo, runID)
	store, cliErr := openLedger("test", req)
	if cliErr != nil {
		t.Fatalf("openLedger: %v", cliErr.Message)
	}
	src, cliErr := loadVerifiedSource("test", repo, fixtureCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr.Message)
	}
	commit, cliErr := headCommit("test", repo)
	if cliErr != nil {
		t.Fatalf("headCommit: %v", cliErr.Message)
	}
	if _, err := store.Start(ledgerIdentity(req, repo, commit, src), kind, key, inputs, "", ""); err != nil {
		t.Fatalf("start %s/%s: %v", kind, key, err)
	}
}

// ledgerRecord returns the resume record for (kind, id) that is not superseded.
func ledgerRecord(t *testing.T, out resumeOutput, kind runstate.Kind, id string) runstate.Record {
	t.Helper()
	for _, rec := range out.Records {
		if rec.Kind == kind && rec.ID == id && rec.Status != runstate.StatusSuperseded {
			return rec
		}
	}
	t.Fatalf("no current %s/%s record in %+v", kind, id, out.Records)
	return runstate.Record{}
}

// TestLedgerResume_RepeatWithoutAReasonIsRefused locks the one gate on the
// additive request field: repetition is allowed, but only against a reason
// that is retained, and every op that decodes a verify request enforces it.
func TestLedgerResume_RepeatWithoutAReasonIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	for _, op := range []string{"resume", "trace", "record-attempt", "baseline"} {
		t.Run(op, func(t *testing.T) {
			request := requestWith(t, repo, "repeat-no-reason", map[string]any{
				"repeat": map[string]any{"reason": ""},
			})
			code, stdout, stderr := runVerifyOp(t, op, request)
			if code != exitError || stdout != "" {
				t.Fatalf("reasonless repeat was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if got := decodeCLIError(t, stderr); got.Code != "missing-repeat-reason" {
				t.Errorf("error code = %q, want missing-repeat-reason", got.Code)
			}
		})
	}
}

// TestLedgerResume_StateDirInsideTheRepositoryIsRefused keeps the ledger out of
// the repository under verification: a state dir that resolves inside it is
// refused before anything is created there, so a run can never dirty the
// working tree it is about to snapshot.
func TestLedgerResume_StateDirInsideTheRepositoryIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := filepath.Join(repo, ".test-design-state")
	t.Setenv("TEST_DESIGN_STATE_DIR", stateDir)

	code, stdout, stderr := runVerifyOp(t, "resume", verifyRequestJSON(t, repo, "state-inside-repo"))
	if code != exitError || stdout != "" {
		t.Fatalf("a state dir inside the repository was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "ledger-open-failed" {
		t.Errorf("error code = %q, want ledger-open-failed", got.Code)
	}
	if !strings.Contains(got.Message, repo) {
		t.Errorf("message = %q, want it to name the repository the state dir resolves inside", got.Message)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) err = %v, want the refusal to land before the directory is created", stateDir, err)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the repository untouched", lines)
	}
}

// TestTrace_RepeatSupersedesTheCompletedPhaseRecord locks the phase obligation:
// one completed trace per run ID, a second refused by name, and a deliberate
// repetition that supersedes the first record while keeping it, with the
// caller's reason retained as the superseded record's detail.
func TestTrace_RepeatSupersedesTheCompletedPhaseRecord(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "trace-repeat"

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	first := decodeTraceOutput(t, stdout)

	code, stdout, stderr = runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitError || stdout != "" {
		t.Fatalf("a second trace for one run ID ran again: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "phase-completed" {
		t.Errorf("error code = %q, want phase-completed", got.Code)
	}

	const reason = "re-run after fixing the runner command"
	code, stdout, stderr = runVerifyOp(t, "trace", requestWith(t, repo, runID, map[string]any{
		"repeat": map[string]any{"reason": reason},
	}))
	if code != exitOK {
		t.Fatalf("repeat trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	second := decodeTraceOutput(t, stdout)
	if second.EvidencePath == first.EvidencePath {
		t.Fatalf("repeat trace reused the first run directory %s", first.EvidencePath)
	}

	out := runResume(t, repo, runID)
	if len(out.Records) != 2 {
		t.Fatalf("records = %+v, want the superseded first phase and the second", out.Records)
	}
	superseded := out.Records[0]
	if superseded.Status != runstate.StatusSuperseded {
		t.Errorf("first phase record status = %q, want %q", superseded.Status, runstate.StatusSuperseded)
	}
	if superseded.Outcome != "ok" {
		t.Errorf("superseded record outcome = %q, want the completed outcome it kept", superseded.Outcome)
	}
	if superseded.Detail != reason {
		t.Errorf("superseded record detail = %q, want the retained repeat reason %q", superseded.Detail, reason)
	}
	if superseded.SupersededBy != 2 {
		t.Errorf("supersededBy = %d, want 2", superseded.SupersededBy)
	}
	if out.Trace.Status != "completed" || out.Trace.EvidencePath != second.EvidencePath {
		t.Errorf("trace = %+v, want the second attempt completed at %s", out.Trace, second.EvidencePath)
	}
	if out.Counts.Superseded != 1 || out.Counts.Completed != 1 {
		t.Errorf("counts = %+v, want one superseded and one completed record", out.Counts)
	}
}

// TestRecordAttempt_Layer4CompletesTheMutationRow proves the row obligation a
// refused mutation discharges: the attempt document becomes the row's evidence,
// resume reports the row completed environmental with that artifact verified,
// and the row is not offered again.
func TestRecordAttempt_Layer4CompletesTheMutationRow(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "attempt-completes-row"

	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	const refusal = "mutation runner unavailable on this machine"
	attempt := requestWith(t, repo, runID, map[string]any{
		"layer": 4, "rowID": "SC-10", "refusal": refusal,
	})
	code, stdout, stderr := runVerifyOp(t, "record-attempt", attempt)
	if code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var recorded attemptOutput
	if err := json.Unmarshal([]byte(stdout), &recorded); err != nil {
		t.Fatalf("record-attempt stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	out := runResume(t, repo, runID)
	row := resumeRow(t, out, "SC-10")
	if row.Status != "completed" || row.Outcome != "environmental" {
		t.Fatalf("SC-10 = %+v, want completed environmental", row)
	}
	rec := ledgerRecord(t, out, runstate.KindMutationRow, "SC-10")
	if got := rec.Artifacts["attempt"].Path; got != recorded.EvidencePath {
		t.Errorf("attempt artifact path = %q, want %q", got, recorded.EvidencePath)
	}
	if reasons := runstate.Verify(&rec); len(reasons) != 0 {
		t.Errorf("Verify(record) = %v, want the retained attempt document to verify", reasons)
	}
	if out.Counts.Failed != 1 {
		t.Errorf("counts.failed = %d, want 1: an environmental outcome is a failed outcome", out.Counts.Failed)
	}
	for _, pending := range out.Pending {
		if pending.Kind == runstate.KindMutationRow && pending.ID == "SC-10" {
			t.Errorf("pending = %+v, want a completed failure never offered again", out.Pending)
		}
	}

	code, stdout, stderr = runVerifyOp(t, "record-attempt", attempt)
	if code != exitError || stdout != "" {
		t.Fatalf("a second attempt overwrote the row's outcome: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "mutation-row-completed" {
		t.Errorf("error code = %q, want mutation-row-completed", got.Code)
	}
}

// TestTrace_ALedgerFromAnotherCommitIsRefused holds a run to the inputs it was
// opened against. A ledger names the commit and the coverage document its
// records were made under; presenting a different pair is a different run, and
// the refusal lands before the worktree the suite would have run in.
func TestTrace_ALedgerFromAnotherCommitIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "identity-moved"
	startLedgerRecord(t, repo, runID, runstate.KindPhase, tracePhaseID, phaseInputs{RunDir: t.TempDir()})

	if err := os.WriteFile(filepath.Join(repo, "MOVED.md"), []byte("moved on\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "move the repository on")

	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitError || stdout != "" {
		t.Fatalf("a ledger from another commit was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "ledger-identity-mismatch" {
		t.Errorf("error code = %q, want ledger-identity-mismatch", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees = %v, want the refusal to land before any worktree work", left)
	}
}

// TestResume_ACorruptCheckpointIsRefused keeps a damaged ledger from reading as
// an empty one: a run whose checkpoint cannot be understood must be reported as
// such, never as a run that owes everything from the start.
func TestResume_ACorruptCheckpointIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "corrupt-checkpoint"
	startLedgerRecord(t, repo, runID, runstate.KindPhase, tracePhaseID, phaseInputs{RunDir: t.TempDir()})

	ledgerPath := runResume(t, repo, runID).LedgerPath
	if err := os.WriteFile(ledgerPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write %s: %v", ledgerPath, err)
	}

	code, stdout, stderr := runVerifyOp(t, "resume", verifyRequestJSON(t, repo, runID))
	if code != exitError || stdout != "" {
		t.Fatalf("a corrupt checkpoint was read: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "ledger-invalid" {
		t.Errorf("error code = %q, want ledger-invalid", got.Code)
	}
}

// TestRecordAttempt_Layer3CompletesTheLiveAttempt proves the whole-layer
// obligation a refused live run discharges: the live attempt is completed
// unavailable, resume reports the layer answered, and the layer is not attested
// to twice for one run.
func TestRecordAttempt_Layer3CompletesTheLiveAttempt(t *testing.T) {
	subject := newRecordAttemptLiveSubject(t)
	runID := liveRunID("live-attempt-ledger")
	request, err := json.Marshal(map[string]any{
		"schemaVersion": evidence.SchemaVersion, "repoRoot": subject.repo,
		"coverageSource": subject.coverageSource, "runID": runID,
		"layer": 3, "refusal": "no live environment is configured for this run",
	})
	if err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runVerifyOp(t, "record-attempt", string(request))
	if code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var recorded attemptOutput
	if err := json.Unmarshal([]byte(stdout), &recorded); err != nil {
		t.Fatalf("record-attempt stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	code, stdout, stderr = runVerifyOp(t, "resume", liveRequestJSON(t, subject.repo, subject.coverageSource, runID))
	if code != exitOK {
		t.Fatalf("resume exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out resumeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("resume stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if !out.Live.Required {
		t.Fatalf("live.required = false, want true for a document whose live decision is required")
	}
	if out.Live.Status != "completed" || out.Live.Outcome != "unavailable" {
		t.Errorf("live = %+v, want completed unavailable", out.Live)
	}
	if out.Live.EvidencePath != recorded.EvidencePath {
		t.Errorf("live.evidencePath = %q, want the attempt document %q", out.Live.EvidencePath, recorded.EvidencePath)
	}
	if out.Live.RecoveryRequired {
		t.Error("live.recoveryRequired = true, want false: the attempt left nothing running")
	}
	rec := ledgerRecord(t, out, runstate.KindLiveAttempt, liveAttemptID)
	if reasons := runstate.Verify(&rec); len(reasons) != 0 {
		t.Errorf("Verify(record) = %v, want the retained attempt document to verify", reasons)
	}

	code, stdout, stderr = runVerifyOp(t, "record-attempt", string(request))
	if code != exitError || stdout != "" {
		t.Fatalf("a second attempt overwrote the live layer: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := decodeCLIError(t, stderr); got.Code != "live-attempt-completed" {
		t.Errorf("error code = %q, want live-attempt-completed", got.Code)
	}
}
