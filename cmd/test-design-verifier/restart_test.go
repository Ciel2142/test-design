package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"test-design/internal/report"
	"test-design/internal/runstate"
)

// The proofs in this file are the ones an in-process call cannot make: they
// interrupt a verifier mid-op with a real kill and then ask what the run owes.
// Everything they observe afterwards is read from the subject and from the
// run's own ledger — the phase log the deployment writes, the records resume
// reports — never from the output of the process that was killed, which by
// definition never wrote one.

const (
	// phaseRestartTimeoutSeconds gives every phase command more room than the
	// 20 s the interrupted business check spends stalling: the check has to
	// still be running when the kill lands, so the profile's own deadline must
	// not end it first.
	phaseRestartTimeoutSeconds = 30

	// restartWaitDeadline bounds every wait for a path or a phase the
	// interrupted attempt is expected to reach. It is generous because it only
	// bounds a failure: a healthy run reaches the check in seconds.
	restartWaitDeadline = 60 * time.Second

	// restartDrainDeadline is how long the interrupted live command may take to
	// finish on its own. The kill never reached it — subprocess.Run gave it its
	// own process group — so it sleeps out its 20 s stall and exits.
	restartDrainDeadline = 25 * time.Second
)

// interruptedLiveAttempt is what a killed attempt leaves for the attempt that
// follows it: the same run directory, the same plan and the same trace
// evidence an operator would hand the repeated call.
type interruptedLiveAttempt struct {
	tracePath string
	runDir    string
	planPath  string
}

// interruptLiveAttemptAtTheBusinessCheck drives the real trace and prepare-live
// in process, runs execute-live as a separate verifier process, and kills that
// process — group and all — while the walkthrough's business check is stalling.
// The subject's own phase log, not the verifier's output, is what says the
// check was really running when the kill landed; the verifier wrote no output
// at all, which is the point.
//
// It returns once the interrupted command has finished on its own, so what the
// caller inspects is a run that is over rather than one still moving.
func interruptLiveAttemptAtTheBusinessCheck(t *testing.T, s *liveSubject, runID string) interruptedLiveAttempt {
	t.Helper()

	code, stdout, stderr := runVerifyOp(t, "trace", liveRequestJSON(t, s.repo, s.coverageSource, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	tr := decodeTraceOutput(t, stdout)
	if tr.BaselineStatus != "ok" {
		t.Fatalf("trace baselineStatus = %q, want ok", tr.BaselineStatus)
	}

	code, stdout, stderr = runVerifyOp(t, "prepare-live", liveRequestJSON(t, s.repo, s.coverageSource, runID))
	if code != exitOK {
		t.Fatalf("prepare-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var prep prepareLiveOutput
	if err := json.Unmarshal([]byte(stdout), &prep); err != nil {
		t.Fatalf("prepare-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	attempt := interruptedLiveAttempt{
		tracePath: tr.EvidencePath,
		runDir:    prep.RunDir,
		planPath:  writePlanFile(t, phasePlan(runID, false)),
	}
	verifier := startVerifierProcess(t, "execute-live",
		liveExecuteRequestJSON(t, s.repo, s.coverageSource, attempt.runDir, attempt.planPath, tr.EvidencePath))

	// The run root proves the deployment was made; the check line proves the
	// walkthrough had begun against it. Killing before either would interrupt
	// an attempt that owned nothing yet.
	waitForPath(t, phaseRunRoot(runID), restartWaitDeadline)
	waitForPhaseLine(t, runID, "check", restartWaitDeadline)
	verifier.killAndWait(t)
	waitForNoProcessMatching(t, "phase.py check "+runID, restartDrainDeadline)

	return attempt
}

// waitForPhaseLine polls the run's own phase log until it records phase.
func waitForPhaseLine(t *testing.T, runID, phase string, deadline time.Duration) {
	t.Helper()
	if !pollUntil(deadline, func() bool {
		return slices.Contains(readPhaseLog(t, runID), phase)
	}) {
		t.Fatalf("run %s did not record the %s phase within %s; log = %v", runID, phase, deadline, readPhaseLog(t, runID))
	}
}

// resumeSubjectRun asks the real resume op what a subject's run still owes.
func resumeSubjectRun(t *testing.T, s *liveSubject, runID string) resumeOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "resume", liveRequestJSON(t, s.repo, s.coverageSource, runID))
	if code != exitOK {
		t.Fatalf("resume exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out resumeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("resume stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// requireInterruptedAttemptOwed holds what a resumed run is told about the
// attempt that was killed: it is still open, and repeating it is not simply
// allowed — the resources it left behind have to be recovered first.
func requireInterruptedAttemptOwed(t *testing.T, s *liveSubject, runID string) {
	t.Helper()
	res := resumeSubjectRun(t, s, runID)
	if got, want := res.Live.Status, string(runstate.StatusStarted); got != want {
		t.Fatalf("resume live status = %q, want %q after the verifier was killed mid-attempt (reason %q, records %+v)",
			got, want, res.Live.Reason, res.Records)
	}
	if !res.Live.RecoveryRequired {
		t.Errorf("resume live recoveryRequired = false, want true: the interrupted attempt owns live resources")
	}
}

// supersededLiveAttempt returns the live-attempt record the repeated attempt
// pushed aside. The interrupted attempt has to still be in the run's history:
// a ledger that forgot it would leave a reviewer unable to tell a restarted
// run from a run that only ever attempted once.
func supersededLiveAttempt(t *testing.T, res resumeOutput) runstate.Record {
	t.Helper()
	for _, rec := range res.Records {
		if rec.Kind == runstate.KindLiveAttempt && rec.ID == liveAttemptID && rec.Status == runstate.StatusSuperseded {
			return rec
		}
	}
	t.Fatalf("resume records = %+v, want the interrupted attempt retained as a superseded live-attempt record", res.Records)
	return runstate.Record{}
}

// runExecuteLive drives the real execute-live op in process over an already
// prepared run and returns its exit code with its two streams.
func runExecuteLive(t *testing.T, s *liveSubject, attempt interruptedLiveAttempt) (int, string, string) {
	t.Helper()
	return runVerifyOp(t, "execute-live",
		liveExecuteRequestJSON(t, s.repo, s.coverageSource, attempt.runDir, attempt.planPath, attempt.tracePath))
}

// covers: SC-23
func TestRestart_RecoversLiveStateBeforeRepeating(t *testing.T) {
	requirePython3(t)

	t.Run("the repeated attempt recovers what the interrupted one left, then runs", func(t *testing.T) {
		subject := newPhaseSubject(t, phaseRestartTimeoutSeconds)
		runID := newPhaseRun(t, subject, "restart-live-recovered", map[string]string{"check": "stall"})

		// The sentinel shares the run's resource prefix without being the run's
		// own root. Recovery runs the profile's cleanup commands, which remove
		// the run's directory by name; a recovery that swept the prefix would
		// take this with it.
		sentinel := phaseRunRoot(runID) + "-sentinel"
		keep := filepath.Join(sentinel, "keep")
		if err := os.MkdirAll(keep, 0o700); err != nil {
			t.Fatalf("create the sentinel directory: %v", err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(sentinel); err != nil {
				t.Logf("remove sentinel %s: %v", sentinel, err)
			}
		})

		attempt := interruptLiveAttemptAtTheBusinessCheck(t, subject, runID)
		requireInterruptedAttemptOwed(t, subject, runID)

		// The interruption is over; the check that stalled through it is asked
		// to behave, so the repeated attempt is a healthy run of the same plan.
		if err := os.Remove(filepath.Join(phaseLogRoot(runID), "check.directive")); err != nil {
			t.Fatalf("remove the check directive: %v", err)
		}

		code, stdout, stderr := runExecuteLive(t, subject, attempt)
		if code != exitOK {
			t.Fatalf("repeated execute-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		var out executeLiveOutput
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("execute-live stdout did not decode: %v\nstdout=%s", err, stdout)
		}
		ev := readLiveEvidence(t, out.EvidencePath)

		if ev.Recovery == nil {
			t.Fatalf("evidence recovery = nil, want the repeated attempt to report the recovery it performed")
		}
		if !ev.Recovery.Performed || !ev.Recovery.OK {
			t.Errorf("recovery = %+v, want performed and ok", ev.Recovery)
		}
		if len(ev.Recovery.Steps) == 0 {
			t.Fatalf("recovery steps = %+v, want one per profile cleanup command", ev.Recovery.Steps)
		}
		if got, want := ev.Recovery.Steps[0].Expectation, "profile recovery command 1"; got != want {
			t.Errorf("recovery expectation = %q, want %q", got, want)
		}
		// The removal is the profile author's own proof, read off the command
		// that made it rather than off the verifier's summary of it.
		if got := ev.Recovery.Steps[0].Decisive; !strings.Contains(got, `"removed": true`) {
			t.Errorf("recovery decisive output = %q, want the cleanup's own proof that it removed the run root", got)
		}

		// The log is the order the subject really ran in: the interrupted
		// attempt through its business check, then one cleanup with nothing
		// started after it, then the whole run again.
		phases := readPhaseLog(t, runID)
		want := []string{"deploy", "seed", "ready", "check", "cleanup", "deploy", "seed", "ready", "check", "cleanup"}
		if !reflect.DeepEqual(phases, want) {
			t.Fatalf("phase log = %v, want %v", phases, want)
		}
		requirePhaseOrder(t, phases[:5], "check", "cleanup")
		firstCleanup := slices.Index(phases, "cleanup")
		secondDeploy := slices.Index(phases[1:], "deploy") + 1
		if firstCleanup > secondDeploy {
			t.Errorf("phase log = %v, want the recovery cleanup at %d before the fresh deploy at %d",
				phases, firstCleanup, secondDeploy)
		}
		requireSentinelIntact(t, keep)

		if got := rowVerdict(t, ev.RowVerdicts, phaseRow).Verdict; got != verdictPass {
			t.Errorf("live verdict for %s = %q, want %q", phaseRow, got, verdictPass)
		}
		if !ev.CleanupOK || ev.CleanupFailed {
			t.Errorf("cleanupOK = %v, cleanupFailed = %v, want true/false from the repeated run's own cleanup",
				ev.CleanupOK, ev.CleanupFailed)
		}
		requireAbsent(t, "after the repeated attempt returned", phaseRunRoot(runID))

		res := resumeSubjectRun(t, subject, runID)
		if res.Live.Status != string(runstate.StatusCompleted) {
			t.Errorf("resume live status = %q, want completed (reason %q)", res.Live.Status, res.Live.Reason)
		}
		if res.Live.Outcome != "pass" {
			t.Errorf("resume live outcome = %q, want pass", res.Live.Outcome)
		}
		if res.Live.RecoveryRequired {
			t.Error("resume live recoveryRequired = true, want false once the attempt completed")
		}
		if got := supersededLiveAttempt(t, res).Detail; got != "restarted" {
			t.Errorf("superseded attempt detail = %q, want %q", got, "restarted")
		}

		code, stdout, stderr = runVerifyOp(t, "finalize",
			liveFinalizeRequestJSON(t, subject, runID, attempt.tracePath, out.EvidencePath))
		if code != exitOK {
			t.Fatalf("finalize refused the recovered run's evidence: exit code = %d, stderr=%s", code, stderr)
		}
		for _, reason := range decodeFinalizeOutput(t, stdout).Reasons {
			if reason.Layer == "3" {
				t.Errorf("finalize reason %+v, want layer 3 to raise nothing against a recovered run that passed", reason)
			}
		}
	})

	t.Run("a recovery that cannot prove removal blocks the run instead of starting it", func(t *testing.T) {
		subject := newPhaseSubject(t, phaseRestartTimeoutSeconds)
		runID := newPhaseRun(t, subject, "restart-live-blocked", map[string]string{"check": "stall"})

		attempt := interruptLiveAttemptAtTheBusinessCheck(t, subject, runID)
		requireInterruptedAttemptOwed(t, subject, runID)

		if err := os.Remove(filepath.Join(phaseLogRoot(runID), "check.directive")); err != nil {
			t.Fatalf("remove the check directive: %v", err)
		}
		// The cleanup this run owns is asked to refuse, which is the one thing
		// recovery cannot proceed past. Removing the directive before the test
		// ends is what lets the subject's own registered cleanup release the
		// run root it leaves standing.
		writeDirectives(t, runID, map[string]string{"cleanup": "fail"})
		t.Cleanup(func() {
			if err := os.Remove(filepath.Join(phaseLogRoot(runID), "cleanup.directive")); err != nil {
				t.Logf("remove the cleanup directive: %v", err)
			}
		})

		code, stdout, stderr := runExecuteLive(t, subject, attempt)
		if code != exitOK {
			t.Fatalf("repeated execute-live exit code = %d, want %d: a failed recovery is a completed attempt, not a refusal; stderr=%s",
				code, exitOK, stderr)
		}
		var out executeLiveOutput
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("execute-live stdout did not decode: %v\nstdout=%s", err, stdout)
		}
		ev := readLiveEvidence(t, out.EvidencePath)

		if ev.Recovery == nil {
			t.Fatalf("evidence recovery = nil, want the attempt to report the recovery it attempted")
		}
		if !ev.Recovery.Performed || ev.Recovery.OK {
			t.Errorf("recovery = %+v, want performed and not ok", ev.Recovery)
		}
		if len(ev.Recovery.Steps) == 0 {
			t.Fatalf("recovery steps = %+v, want the refused cleanup command recorded", ev.Recovery.Steps)
		}
		if got := ev.Recovery.Steps[0].Result; got != "fail" {
			t.Errorf("recovery result = %q, want fail", got)
		}

		// One deploy in the whole log: the interrupted attempt's. Nothing was
		// started over a run whose earlier resources were never proved gone.
		phases := readPhaseLog(t, runID)
		if got := countPhase(phases, "deploy"); got != 1 {
			t.Errorf("phase log = %v, recorded %d deploy lines, want only the interrupted attempt's", phases, got)
		}
		if len(ev.StartSteps) != 0 || len(ev.HealthSteps) != 0 || len(ev.Steps) != 0 {
			t.Errorf("startSteps = %+v, healthSteps = %+v, steps = %+v, want all empty after a failed recovery",
				ev.StartSteps, ev.HealthSteps, ev.Steps)
		}
		if got := rowVerdict(t, ev.RowVerdicts, phaseRow).Verdict; got != verdictInconclusive {
			t.Errorf("live verdict for %s = %q, want %q: nothing ran to judge it", phaseRow, got, verdictInconclusive)
		}
		if ev.CleanupOK || !ev.CleanupFailed {
			t.Errorf("cleanupOK = %v, cleanupFailed = %v, want false/true from the recovery tri-state",
				ev.CleanupOK, ev.CleanupFailed)
		}
		requireExecutionError(t, ev.ExecutionErrors, "recovery:", "did not prove removal")

		// The run root the interrupted attempt made is still there: a recovery
		// that could not remove it must not pretend otherwise, and releasing it
		// is the registered cleanup's business, not this attempt's.
		if _, err := os.Stat(phaseRunRoot(runID)); err != nil {
			t.Errorf("stat %s: %v, want the run root the failed recovery could not remove to still stand",
				phaseRunRoot(runID), err)
		}

		code, stdout, stderr = runVerifyOp(t, "finalize",
			liveFinalizeRequestJSON(t, subject, runID, attempt.tracePath, out.EvidencePath))
		if code != exitOK {
			t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		final := decodeFinalizeOutput(t, stdout)
		if final.Verdict != report.VerdictBlocked {
			t.Errorf("verdict = %q, want %q; reasons=%+v", final.Verdict, report.VerdictBlocked, final.Reasons)
		}
		requireReason(t, final.Reasons, "3", phaseRow, verdictInconclusive)
		requireReason(t, final.Reasons, "3", "", "live-cleanup-failed")

		// The attempt is completed, failed recovery and all, so the run's next
		// execute-live is refused by name rather than quietly recovering again.
		code, stdout, stderr = runExecuteLive(t, subject, attempt)
		if code != exitError {
			t.Fatalf("third execute-live exit code = %d, want %d; stdout=%s", code, exitError, stdout)
		}
		if got := decodeCLIError(t, stderr).Code; got != "live-attempt-completed" {
			t.Errorf("error code = %q, want live-attempt-completed", got)
		}
	})
}
