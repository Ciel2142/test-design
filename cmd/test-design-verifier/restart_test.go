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

	"test-design/internal/mutation"
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
			liveFinalizeRequestJSON(t, subject, runID))
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
			liveFinalizeRequestJSON(t, subject, runID))
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

// The mutation restart proof below interrupts a real evaluation the same way:
// the workspace it leaves is a worktree carrying a mutant that was never
// measured and the residue of the run that was measuring it, and what the row
// owes afterwards is read from the run's own state and ledger.

const (
	// slowFixtureTest names the fixture test the SC-09 row's targeted runs
	// select. It is what pgrep looks for while the interrupted run drains; no
	// proof in this package carries it in its own arguments, so a match is
	// always the fixture's run and never the test that is waiting on it.
	slowFixtureTest = "TestSameName"

	// mutationInterruptDeadline bounds the wait for the interrupted site to
	// reach that test. It is generous because it covers the fixture module's
	// first compilation as well as the run itself, and because it only bounds
	// a failure.
	mutationInterruptDeadline = 90 * time.Second

	// mutationDrainDeadline is how long the targeted run the kill never
	// reached may take to finish. It has a process group of its own, so it
	// sleeps out the fixture's stall and exits by itself.
	mutationDrainDeadline = 15 * time.Second
)

// slowFixtureImports replaces the fixture orders test's single import, so the
// slowed target test can write a file and stall.
const slowFixtureImports = `import (
	"os"
	"testing"
	"time"
)`

// slowFixtureTargetHead is the opening of TestSameName exactly as the fixture
// commits it, and slowFixtureSlowHead is that opening with a residue file and
// a stall in front of the assertion it already made. The test's name, the
// coverage tag above it and every other test in the file are outside both, so
// the trace's bindings and the row's identity are what they always were.
const slowFixtureTargetHead = `func TestSameName(t *testing.T) {
	if err := ValidateQuantity(1); err != nil {`

const slowFixtureSlowHead = `func TestSameName(t *testing.T) {
	if err := os.WriteFile("residue.txt", []byte("residue\n"), 0o644); err != nil {
		t.Fatalf("write residue.txt: %v", err)
	}
	time.Sleep(3 * time.Second)
	if err := ValidateQuantity(1); err != nil {`

// slowFixtureEdit returns the fixtureRepoWith edit that makes the SC-09 row's
// target interruptible. The residue is the proof that a run reached the test —
// a file in the package directory, so the workspace carries it where a diff
// would see it — and the stall holds the run open long enough for a kill to
// land inside it. It is committed with the fixture, so the repository the ops
// see is clean.
func slowFixtureEdit(t *testing.T) func(repo string) {
	t.Helper()
	return func(repo string) {
		path := filepath.Join(repo, "orders", "orders_test.go")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the fixture's orders test: %v", err)
		}
		src := replaceExactlyOnce(t, string(data), `import "testing"`, slowFixtureImports)
		src = replaceExactlyOnce(t, src, slowFixtureTargetHead, slowFixtureSlowHead)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write the fixture's orders test: %v", err)
		}
	}
}

// replaceExactlyOnce is strings.Replace that refuses to guess: a fixture edit
// that matched nothing, or matched more than the one place it named, would
// leave a subject nobody described.
func replaceExactlyOnce(t *testing.T, src, old, replacement string) string {
	t.Helper()
	if got := strings.Count(src, old); got != 1 {
		t.Fatalf("the fixture source contains %d occurrences of %q, want exactly 1", got, old)
	}
	return strings.Replace(src, old, replacement, 1)
}

// originalRepoState is the original repository's own standing: the commit it
// sits on, how many times its refs have moved, and what its working tree
// holds. A mutation workspace is a worktree of this repository, so a
// restoration or a recreation that reached past the worktree shows up in one
// of the three.
type originalRepoState struct {
	head   string
	reflog int
	status []string
}

func captureOriginalRepo(t *testing.T, repo string) originalRepoState {
	t.Helper()
	return originalRepoState{
		head:   strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD")),
		reflog: reflogLength(t, repo),
		status: gitStatusLines(t, repo),
	}
}

// reflogLength is `git reflog | wc -l` for repo.
func reflogLength(t *testing.T, repo string) int {
	t.Helper()
	n := 0
	for _, line := range strings.Split(mustGit(t, repo, "reflog"), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// requireOriginalRepoUntouched holds the invariant every step of a mutation
// restart owes: whatever happened to the workspace, the repository the run is
// verifying is where it was when the run began. step names what just ran, so a
// failure says which one reached too far.
func requireOriginalRepoUntouched(t *testing.T, repo string, before originalRepoState, step string) {
	t.Helper()
	got := captureOriginalRepo(t, repo)
	if got.head != before.head {
		t.Errorf("after %s: original repository HEAD = %q, want %q", step, got.head, before.head)
	}
	if got.reflog != before.reflog {
		t.Errorf("after %s: original repository reflog holds %d entries, want %d", step, got.reflog, before.reflog)
	}
	if !reflect.DeepEqual(got.status, before.status) {
		t.Errorf("after %s: original repository status = %v, want %v", step, got.status, before.status)
	}
}

// interruptMutationAtSiteB takes the SC-09 row through its first site in
// process, applies the caught mutant for a second one, and kills a real
// verifier while that site's mutated run is under way. It returns once the
// targeted run has finished on its own, so what the caller inspects is a
// workspace nothing is still writing to.
func interruptMutationAtSiteB(t *testing.T, repo, runID string) prepareMutationOutput {
	t.Helper()

	// The trace is what gives resume the dispatch inputs it reads a row's
	// standing against; without it the mutation layer reports only that it is
	// waiting on the phase.
	code, _, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	prep := preparedMutation(t, repo, runID, "SC-09")

	// site-a is the equivalent mutant the other multi-site proofs use, driven
	// in process: nothing about a site that completes needs interrupting.
	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", false))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	// site-a's own residue goes now, and the test is the one to remove it: the
	// slowed target writes residue.txt on every run and RestoreTracked leaves
	// untracked files where they are, so site-a's copy would still be sitting
	// in the workspace when site-b's diff is policed — and a second entry in
	// that diff is refused as multi-site before the interruption could ever
	// happen. The residue this proof is about is the one site-b's killed
	// mutated run leaves behind.
	residue := filepath.Join(prep.WorktreePath, "orders", "residue.txt")
	if err := os.Remove(residue); err != nil {
		t.Fatalf("remove the completed site's own residue: %v", err)
	}

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	verifier := startVerifierProcess(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-b", true))

	// Both conditions together are the kill window: the state names the site
	// the verifier is inside, and the residue proves its mutated run really
	// reached the target test rather than merely being about to.
	if !pollUntil(mutationInterruptDeadline, func() bool {
		st, err := mutation.LoadState(prep.RunDir)
		if err != nil || st.Started == nil || st.Started.Site != "site-b" || st.Started.Phase != "mutated" {
			return false
		}
		_, err = os.Stat(residue)
		return err == nil
	}) {
		t.Fatalf("site-b's mutated run did not reach the fixture's target test within %s", mutationInterruptDeadline)
	}
	verifier.killAndWait(t)
	waitForNoProcessMatching(t, slowFixtureTest, mutationDrainDeadline)

	return prep
}

// requireInterruptedWorkspace holds what the kill left in the workspace: the
// mutant that was never measured, and the residue of the run that was
// measuring it.
func requireInterruptedWorkspace(t *testing.T, worktreePath string) {
	t.Helper()
	if diff := mustGit(t, worktreePath, "diff", "HEAD"); strings.TrimSpace(diff) == "" {
		t.Error("worktree diff against HEAD is empty, want the mutant the killed run was measuring")
	}
	residue := filepath.Join(worktreePath, "orders", "residue.txt")
	if _, err := os.Stat(residue); err != nil {
		t.Errorf("stat %s: %v, want the interrupted run's residue still in the workspace", residue, err)
	}
}

// requireInterruptedRow holds what the run owes after the kill, read from the
// ledger and the row's own state: the row is open, one site is answered, and
// the other is named as the one the workspace is uncertain about.
func requireInterruptedRow(t *testing.T, repo, runID string) resumeOutput {
	t.Helper()
	res := runResume(t, repo, runID)
	row := resumeRow(t, res, "SC-09")
	if got, want := row.Status, string(runstate.StatusStarted); got != want {
		t.Errorf("row status = %q, want %q after the verifier was killed mid-site (reason %q)", got, want, row.Reason)
	}
	if want := []string{"site-a"}; !reflect.DeepEqual(row.CompletedSites, want) {
		t.Errorf("completedSites = %v, want %v", row.CompletedSites, want)
	}
	if got := row.InterruptedSite; got != "site-b" {
		t.Errorf("interruptedSite = %q, want site-b", got)
	}
	return res
}

// requireSiteInterruptedRefusal drives the real evaluate-mutation over the
// uncertain workspace and holds the refusal by name: nothing is measured until
// a recovery has actually succeeded.
func requireSiteInterruptedRefusal(t *testing.T, repo, runID, runDir string) {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, runDir, "site-b", true))
	if code != exitError {
		t.Fatalf("evaluate-mutation exit code = %d, want %d over an interrupted workspace; stdout=%s", code, exitError, stdout)
	}
	if got := decodeCLIError(t, stderr).Code; got != "site-interrupted" {
		t.Errorf("error code = %q, want site-interrupted", got)
	}
}

// requireRestoredWorkspace holds what a recovered workspace owes the site about
// to be measured in it: nothing of the interrupted run survives, not even an
// ignored path, and it stands at the commit the row was prepared at.
func requireRestoredWorkspace(t *testing.T, worktreePath, commit string) {
	t.Helper()
	status := strings.TrimSpace(mustGit(t, worktreePath, "status", "--porcelain=v1", "--ignored", "--untracked-files=all"))
	if status != "" {
		t.Errorf("worktree status = %q, want empty including ignored paths", status)
	}
	residue := filepath.Join(worktreePath, "orders", "residue.txt")
	if _, err := os.Stat(residue); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) err = %v, want the interrupted run's residue gone", residue, err)
	}
	if head := strings.TrimSpace(mustGit(t, worktreePath, "rev-parse", "HEAD")); head != commit {
		t.Errorf("worktree HEAD = %q, want the prepared commit %q", head, commit)
	}
}

// requireRegisteredWorktree holds that path is one of the worktrees the
// verifier owns. Both sides are resolved before they are compared: the glob
// walks TMPDIR as the environment spells it, which on macOS is /var, while a
// controller resolves its temporary root before it creates anything under it,
// so every path an op reports names the same directory as /private/var.
func requireRegisteredWorktree(t *testing.T, path string) {
	t.Helper()
	registered := verifierWorktrees(t)
	want := resolvedPath(t, path)
	for _, wt := range registered {
		if resolvedPath(t, wt) == want {
			return
		}
	}
	t.Errorf("registered worktrees = %v, want the recreated %q among them", registered, path)
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return resolved
}

// requireRecoveredProgress holds what a successful recovery reports about the
// row it recovered: the site that finished is kept, and the site the kill
// landed in is named.
func requireRecoveredProgress(t *testing.T, out recoverMutationOutput) {
	t.Helper()
	if out.RowID != "SC-09" {
		t.Errorf("rowId = %q, want SC-09", out.RowID)
	}
	if want := []string{"site-a"}; !reflect.DeepEqual(out.CompletedSites, want) {
		t.Errorf("completedSites = %v, want %v", out.CompletedSites, want)
	}
	if out.InterruptedSite != "site-b" {
		t.Errorf("interruptedSite = %q, want site-b", out.InterruptedSite)
	}
}

// requireSecondSiteMeasured measures the site the interruption cost the row, in
// the workspace the recovery handed back, and holds the row's answer: the
// caught mutant, evidence naming both sites in the order they ran, and a
// completed row whose outcome is the one site the mutant walked past.
func requireSecondSiteMeasured(t *testing.T, repo, runID, runDir, worktreePath string) {
	t.Helper()
	mutateOrdersGuard(t, worktreePath, `	return fmt.Errorf("always rejected")`)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, runDir, "site-b", true))
	if code != exitOK {
		t.Fatalf("repeated site-b exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("evaluate-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "caught" {
		t.Errorf("site-b outcome = %q, want caught", out.Outcome)
	}
	if len(out.Sites) != 2 {
		t.Fatalf("sites = %+v, want the site that finished before the kill and the one repeated after it", out.Sites)
	}
	if out.Sites[0].Site != "site-a" || out.Sites[0].Outcome != "survived" {
		t.Errorf("sites[0] = %+v, want site-a survived", out.Sites[0])
	}
	if out.Sites[1].Site != "site-b" || out.Sites[1].Outcome != "caught" {
		t.Errorf("sites[1] = %+v, want site-b caught", out.Sites[1])
	}
	// The repeated site reuses the interrupted site's index, so its captures
	// overwrite the ones the kill left rather than accumulating beside them.
	if !strings.HasSuffix(out.Sites[1].MutatedRunDir, "site-2-mutated") {
		t.Errorf("sites[1].MutatedRunDir = %q, want it to end with site-2-mutated", out.Sites[1].MutatedRunDir)
	}
	assertEvidenceFile(t, out.EvidencePath, runID, "SC-09", "go", 2, []string{"survived", "caught"})

	row := resumeRow(t, runResume(t, repo, runID), "SC-09")
	if got, want := row.Status, string(runstate.StatusCompleted); got != want {
		t.Errorf("row status = %q, want %q (reason %q)", got, want, row.Reason)
	}
	if row.Outcome != "survived" {
		t.Errorf("row outcome = %q, want survived: one site the mutant walked past answers the whole row", row.Outcome)
	}
	if row.EvidencePath != out.EvidencePath {
		t.Errorf("row evidencePath = %q, want %q", row.EvidencePath, out.EvidencePath)
	}
}

// covers: SC-24
func TestRestart_RestoresMutationWorkspaceBeforeMeasuring(t *testing.T) {
	t.Run("the recovered workspace is restored in place, then measured", func(t *testing.T) {
		repo := fixtureRepoWith(t, slowFixtureEdit(t))
		const runID = "restart-mutation-restored"
		before := captureOriginalRepo(t, repo)

		prep := interruptMutationAtSiteB(t, repo, runID)
		requireOriginalRepoUntouched(t, repo, before, "the interruption")
		requireInterruptedWorkspace(t, prep.WorktreePath)

		requireInterruptedRow(t, repo, runID)
		requireOriginalRepoUntouched(t, repo, before, "resume")

		requireSiteInterruptedRefusal(t, repo, runID, prep.RunDir)
		requireOriginalRepoUntouched(t, repo, before, "the refused evaluation")

		rec := runRecoverMutation(t, repo, runID, prep.RunDir)
		if rec.Recreated {
			t.Errorf("recreated = true, want the surviving workspace restored in place")
		}
		if rec.WorktreePath != prep.WorktreePath {
			t.Errorf("worktreePath = %q, want the prepared %q", rec.WorktreePath, prep.WorktreePath)
		}
		requireRecoveredProgress(t, rec)
		requireRestoredWorkspace(t, rec.WorktreePath, prep.RepoCommit)
		requireOriginalRepoUntouched(t, repo, before, "the recovery")

		requireSecondSiteMeasured(t, repo, runID, prep.RunDir, rec.WorktreePath)
		requireOriginalRepoUntouched(t, repo, before, "the repeated site")
	})

	t.Run("a workspace that vanished is recreated before the row is measured", func(t *testing.T) {
		repo := fixtureRepoWith(t, slowFixtureEdit(t))
		const runID = "restart-mutation-recreated"
		before := captureOriginalRepo(t, repo)

		prep := interruptMutationAtSiteB(t, repo, runID)
		requireOriginalRepoUntouched(t, repo, before, "the interruption")

		// The workspace itself is gone, which is what a cleanup sweeping the
		// machine's temporary directory after a crash would leave.
		if err := os.RemoveAll(prep.WorktreePath); err != nil {
			t.Fatalf("remove the interrupted workspace: %v", err)
		}

		rec := runRecoverMutation(t, repo, runID, prep.RunDir)
		if !rec.Recreated {
			t.Errorf("recreated = false, want a workspace that no longer exists recreated")
		}
		if rec.WorktreePath == prep.WorktreePath {
			t.Fatalf("worktreePath = %q, want a fresh workspace, not the one that vanished", rec.WorktreePath)
		}
		requireRegisteredWorktree(t, rec.WorktreePath)
		requireRecoveredProgress(t, rec)
		requireRestoredWorkspace(t, rec.WorktreePath, prep.RepoCommit)
		requireOriginalRepoUntouched(t, repo, before, "the recreation")

		// A parent re-dispatching the mutator is sent to the workspace that
		// exists, not to the one the row was opened against.
		if got := resumeRow(t, runResume(t, repo, runID), "SC-09").WorktreePath; got != rec.WorktreePath {
			t.Errorf("row worktreePath = %q, want the recreated %q", got, rec.WorktreePath)
		}

		requireSecondSiteMeasured(t, repo, runID, prep.RunDir, rec.WorktreePath)
		requireOriginalRepoUntouched(t, repo, before, "the repeated site")
	})

	// A workspace the clean could not empty is not a baseline either: the
	// leftover path is read back from the workspace's own status after the
	// restoration ran, so residue git merely warned about refuses the recovery
	// by name instead of passing for a clean revision.
	t.Run("residue that survives the clean is refused before anything is measured", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: an unwritable directory does not stop root from emptying it")
		}
		repo := fixtureRepoWith(t, slowFixtureEdit(t))
		const runID = "restart-mutation-residue"
		before := captureOriginalRepo(t, repo)

		prep := interruptMutationAtSiteB(t, repo, runID)
		requireOriginalRepoUntouched(t, repo, before, "the interruption")
		requireInterruptedWorkspace(t, prep.WorktreePath)

		// An untracked directory nothing can unlink from: `git clean` reports
		// what it could not remove as a warning and still leaves the file
		// behind, which is the case a clean exit status would hide.
		guarded := filepath.Join(prep.WorktreePath, "residue")
		if err := os.Mkdir(guarded, 0o700); err != nil {
			t.Fatalf("create the guarded directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(guarded, "leftover.txt"), []byte("left behind\n"), 0o644); err != nil {
			t.Fatalf("write the guarded residue: %v", err)
		}
		if err := os.Chmod(guarded, 0o500); err != nil {
			t.Fatalf("make the guarded directory unwritable: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(guarded, 0o700) })

		got := refuseRecoverMutation(t, repo, runID, prep.RunDir)
		if got.Code != "residue-remains" {
			t.Errorf("error code = %q, want residue-remains (message %q)", got.Code, got.Message)
		}
		if !strings.Contains(got.Message, "residue/leftover.txt") {
			t.Errorf("message = %q, want it to name the path that survived the clean", got.Message)
		}
		requireOriginalRepoUntouched(t, repo, before, "the refused recovery")

		// The row is measured on a workspace somebody vouched for or not at
		// all: the marker the refused recovery left standing keeps the second
		// site refused, and the row stays open.
		requireSiteInterruptedRefusal(t, repo, runID, prep.RunDir)
		requireInterruptedRow(t, repo, runID)
		requireOriginalRepoUntouched(t, repo, before, "the refused evaluation")
	})

	t.Run("a restoration that fails leaves the row where the interruption left it", func(t *testing.T) {
		repo := fixtureRepoWith(t, slowFixtureEdit(t))
		const runID = "restart-mutation-unrestorable"
		before := captureOriginalRepo(t, repo)

		prep := interruptMutationAtSiteB(t, repo, runID)
		requireOriginalRepoUntouched(t, repo, before, "the interruption")

		// A workspace whose gitdir link is no longer a gitdir link cannot be
		// restored by any git command run inside it.
		if err := os.WriteFile(filepath.Join(prep.WorktreePath, ".git"), []byte("not a gitdir link\n"), 0o644); err != nil {
			t.Fatalf("break the workspace's gitdir link: %v", err)
		}

		if got := refuseRecoverMutation(t, repo, runID, prep.RunDir).Code; got != "restore-failed" {
			t.Errorf("error code = %q, want restore-failed", got)
		}
		requireOriginalRepoUntouched(t, repo, before, "the refused recovery")

		// The marker the failed recovery left standing is what keeps the row
		// from being measured on a workspace nobody can vouch for.
		requireSiteInterruptedRefusal(t, repo, runID, prep.RunDir)
		requireOriginalRepoUntouched(t, repo, before, "the refused evaluation")

		res := requireInterruptedRow(t, repo, runID)
		if res.Mutation.Complete {
			t.Error("mutation.complete = true, want false while the row is still open")
		}
		// The row is still owed, and a parent reading pending top to bottom is
		// told so by name: an interrupted row nothing can measure and nothing
		// has recovered is exactly the obligation that list is for.
		owed := resumePending{Kind: runstate.KindMutationRow, ID: "SC-09", Reason: reasonNotCompleted}
		if !slices.Contains(res.Pending, owed) {
			t.Errorf("pending = %+v, want %+v among them", res.Pending, owed)
		}
		if res.Counts.Started != 1 {
			t.Errorf("counts.started = %d, want 1: the interrupted row is the run's one open obligation", res.Counts.Started)
		}
		requireOriginalRepoUntouched(t, repo, before, "resume")
	})
}
