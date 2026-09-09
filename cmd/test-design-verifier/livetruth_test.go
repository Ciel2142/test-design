package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/live"
	"test-design/internal/report"
)

// The proofs in this file drive the real pipeline — baseline, trace,
// prepare-live, execute-live, finalize — over a subject that carries the
// approved live harness verbatim: the recipe, the service and the root profile
// are copied byte for byte out of this repository, so what they lock is the
// harness the setup proof approved rather than a convenient imitation of it.
//
// The subject holds two revisions of the service. The prior commit prices a
// unit at 100 cents; the frozen HEAD carries the fixture's 125, which is the
// only revision that answers a three-item quote with 375. Distinguishing them
// is what makes "the deployment under test is the measured revision" an
// observation rather than an assumption.
const (
	redesignCoverageSource = "docs/superpowers/coverage/2026-09-07-workflow-redesign-subject-coverage.md"
	redesignProfilePath    = ".test-design/live-profiles/workflow-redesign-local.json"
	redesignRecipePath     = "fixtures/workflow-redesign-live/recipe.py"
	redesignServicePath    = "fixtures/workflow-redesign-live/service.go"

	// SC-01 is the subject's live-only smoke row: no unit test can observe
	// which revision a running deployment was built from. SC-02 is the
	// integration row the subject's own unit test binds.
	redesignSmokeRow       = "SC-01"
	redesignIntegrationRow = "SC-02"

	// redesignHeadPriceLine is the fixture's own constant; the prior revision
	// is that file with this one line lowered.
	redesignHeadPriceLine  = "const unitPriceCents = 125"
	redesignPriorPriceLine = "const unitPriceCents = 100"
)

const redesignGoMod = `module redesignlive

go 1.26
`

const redesignQuoteSource = `package quote

// TotalCents prices quantity at the same unit price the running service
// charges, so the integration row has something a unit test can bind.
func TotalCents(quantity int) int { return quantity * 125 }
`

// redesignQuoteTestSource is the subject's own tagged test. Its tag cites the
// subject document's SC-02, never a row of this repository: the text lives
// inside a string literal here and only becomes a comment in the temporary
// repository the builder writes.
const redesignQuoteTestSource = `package quote

import "testing"

// covers: SC-02
func TestTotalCentsPricesThreeUnits(t *testing.T) {
	if got := TotalCents(3); got != 375 {
		t.Fatalf("TotalCents(3) = %d, want 375", got)
	}
}
`

// redesignCoverageDoc names the approved root profile, declares the live
// decision required, and journeys both rows. BASELINE_COMMIT and
// BASELINE_SHA256 are pinned by the shared subject builder.
const redesignCoverageDoc = `# Coverage: workflow redesign live subject

Spec: docs/superpowers/specs/2026-09-07-workflow-redesign-live-subject.md
Baseline: BASELINE_COMMIT · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-workflow-redesign-subject-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/workflow-redesign-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-workflow-redesign-subject-coverage.md"]

## Live decision

- decision: required
- reason: only the running deployment of the measured revision can price a real quote

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01, SC-02 | The deployment under test answers a three-item quote with 375 cents and stamps the revision it was built from |

## Scenarios

### Behavior: revision-stamped quoting

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the deployment under test is built from the measured revision | the business check reports the prepared HEAD as its repoCommit | smoke | the start phase is skipped so an older deployment answers | med |
| SC-02 | a three-item quote costs 375 cents | the quote reports totalCents 375 | integration | three items priced at the prior unit price | low |
`

// readRepoFile returns the bytes of one of this repository's own files for a
// subject to copy verbatim. The harness files are approved artifacts: a subject
// that edited them would prove something about its copy, not about the harness.
func readRepoFile(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}

// redesignPriorService derives the 100-cent revision from the fixture's own
// bytes, and refuses to hand back an unchanged copy: a silently missed
// replacement would leave both commits identical and every staleness
// observation below vacuous.
func redesignPriorService(t *testing.T, head string) string {
	t.Helper()
	prior := strings.Replace(head, redesignHeadPriceLine, redesignPriorPriceLine, 1)
	if prior == head {
		t.Fatalf("%s carries no %q line to lower for the prior revision", redesignServicePath, redesignHeadPriceLine)
	}
	return prior
}

// newRedesignSubject builds the two-revision harness subject and runs the real
// baseline op over it. The prior commit is priorCommits[0]; the frozen HEAD
// carries the fixture's own service bytes.
func newRedesignSubject(t *testing.T) *liveSubject {
	t.Helper()
	service := readRepoFile(t, redesignServicePath)
	return newLiveSubject(t, redesignCoverageSource, redesignProfilePath, []map[string]string{
		{
			"go.mod":               redesignGoMod,
			"quote/quote.go":       redesignQuoteSource,
			"quote/quote_test.go":  redesignQuoteTestSource,
			redesignServicePath:    redesignPriorService(t, service),
			redesignRecipePath:     readRepoFile(t, redesignRecipePath),
			redesignProfilePath:    readRepoFile(t, redesignProfilePath),
			redesignCoverageSource: redesignCoverageDoc,
		},
		{redesignServicePath: service},
	}, nil)
}

// redesignPlan is the plan the live-runner would hand to execute-live: the one
// walkthrough command the profile declares, binding both journeyed rows, with
// the predicates that settle them. Pinning the prepared HEAD into a predicate
// is what makes a stale deployment a failure rather than a pass.
func redesignPlan(head, runID string) map[string]any {
	return map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{{
			"expectation": "the running deployment prices a three-item quote at the measured revision",
			"command":     []string{"python3", redesignRecipePath, "check", runID},
			"predicates": []map[string]string{
				{"form": "exit-code", "value": "0"},
				{"form": "matches", "stream": "stdout", "value": `"result": "business-pass"`},
				{"form": "matches", "stream": "stdout", "value": `"repoCommit": "` + head + `"`},
			},
			"scBindings": []string{redesignSmokeRow, redesignIntegrationRow},
		}},
	}
}

// redesignRunRoot is the run-owned directory the recipe derives from its run-ID
// argument. It is spelled here only to observe it; no test-owned path ever
// reaches the profile, the plan or the evidence.
func redesignRunRoot(runID string) string { return "/tmp/test-design-workflow-" + runID }

// liveQuote is the quote the running service answered with.
type liveQuote struct {
	Quantity   int    `json:"quantity"`
	TotalCents int    `json:"totalCents"`
	Revision   string `json:"revision"`
}

// recipeLine is the single JSON line the recipe prints for deploy and for
// check. Both carry the revision and artifact identity of the deployment that
// answered, which is how the evidence connects what was built to what was
// measured.
type recipeLine struct {
	Result         string    `json:"result"`
	RepoCommit     string    `json:"repoCommit"`
	ArtifactSHA256 string    `json:"artifactSha256"`
	Target         string    `json:"target"`
	Observed       liveQuote `json:"observed"`
}

// decodeRecipeLine reads the recipe's result out of a reduced step's decisive
// output. Decisive output is the bounded tail of the command's combined
// output, so the recipe's line is its last line.
func decodeRecipeLine(t *testing.T, what, decisive string) recipeLine {
	t.Helper()
	line := strings.TrimRight(decisive, "\n")
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	var doc recipeLine
	if err := json.Unmarshal([]byte(line), &doc); err != nil {
		t.Fatalf("%s: decisive output does not end in the recipe's JSON result: %v\ndecisive=%q", what, err, decisive)
	}
	return doc
}

// requirePython3 skips only for the one thing this machine can lack: the
// interpreter the approved profile invokes.
func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 is not on PATH, so the approved live harness cannot run: %v", err)
	}
}

// recipeEnv is the environment the profile grants a live command. A test that
// invokes the recipe itself hands it the same three names, so a run the test
// deploys and a run the verifier deploys are built the same way.
func recipeEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	if cache := os.Getenv("GOCACHE"); cache != "" {
		env = append(env, "GOCACHE="+cache)
	}
	return env
}

// runRecipe invokes a subject's own helper script from dir the way the profile
// does, by path under python3; script is the path the subject committed it at.
// A script that could not be started at all reports exit code -1 with the
// reason on the stderr result, so a caller inside t.Cleanup can log it rather
// than fail from a cleanup function.
func runRecipe(t *testing.T, dir, script, action, runID string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("python3", filepath.FromSlash(script), action, runID)
	cmd.Dir = dir
	cmd.Env = recipeEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		return -1, stdout.String(), err.Error()
	}
	return cmd.ProcessState.ExitCode(), stdout.String(), stderr.String()
}

// registerRunCleanup gives the subject's own cleanup script the last word on a
// run whatever the assertions did. A run root left under /tmp keeps a service
// listening and makes every later deploy under the same id exit 1, so this has
// to run after a failed assertion as much as after a passing one. A root that
// is already gone is a cleanup pass, so the exit code is only worth logging.
func registerRunCleanup(t *testing.T, s *liveSubject, script, runID string) {
	t.Cleanup(func() {
		if code, stdout, stderr := runRecipe(t, s.repo, script, "cleanup", runID); code != 0 {
			t.Logf("cleanup of run %s exited %d; stdout=%s stderr=%s", runID, code, stdout, stderr)
		}
	})
}

// detachedCheckout adds a second detached worktree of the subject at sha,
// outside the subject repository so prepare-live still sees a clean source. A
// recipe run from it reports sha as its own HEAD, which is how a revision
// other than the prepared one gets deployed.
func detachedCheckout(t *testing.T, s *liveSubject, sha string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "checkout")
	mustGit(t, s.repo, "worktree", "add", "--detach", dir, sha)
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", "--force", dir)
		cmd.Dir = s.repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("git worktree remove %s: %v: %s", dir, err, out)
		}
	})
	return dir
}

// requireAbsent fails when a run root survived the cleanup that owns it.
func requireAbsent(t *testing.T, what, root string) {
	t.Helper()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("%s: %s still exists (stat error %v), want the recipe's cleanup to have removed it", what, root, err)
	}
}

// covers: SC-10
func TestLiveTruth_DeployedRevisionJourney(t *testing.T) {
	requirePython3(t)

	t.Run("the journey measures the revision the run prepared", func(t *testing.T) {
		subject := newRedesignSubject(t)
		runID := liveRunID("livetruth-journey")
		registerRunCleanup(t, subject, redesignRecipePath, runID)
		root := redesignRunRoot(runID)

		tr, _, exec := runLivePipeline(t, subject, runID, redesignPlan(subject.head, runID), nil)

		// The run root lives outside the disposable worktree, so its absence is
		// observable here rather than only through the cleanup predicate.
		requireAbsent(t, "after execute-live returned", root)
		if !exec.CleanupOK || exec.CleanupFailed {
			t.Errorf("cleanupOK = %v, cleanupFailed = %v, want true/false", exec.CleanupOK, exec.CleanupFailed)
		}

		code, stdout, stderr := runVerifyOp(t, "finalize",
			liveFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, exec.EvidencePath))
		if code != exitOK {
			t.Fatalf("finalize refused the live evidence: exit code = %d, stderr=%s", code, stderr)
		}
		out := decodeFinalizeOutput(t, stdout)
		for _, reason := range out.Reasons {
			if reason.Layer == "3" {
				t.Errorf("finalize reason %+v, want layer 3 to raise nothing against a clean live run", reason)
			}
		}

		// Everything below is read from the transcript the report published,
		// never from a stream this test captured itself: the record a reviewer
		// receives is the thing under test.
		rep := readPublishedReport(t, subject.repo, out.JSONPath)
		transcript := out.TranscriptsDir + "/" + liveEvidenceFile
		publishedTranscript(t, rep, transcript)
		ev := readLiveEvidence(t, filepath.Join(subject.repo, filepath.FromSlash(transcript)))

		if ev.RepoCommit != subject.head {
			t.Errorf("published live transcript repoCommit = %q, want the prepared HEAD %q", ev.RepoCommit, subject.head)
		}
		if !ev.CleanupOK || ev.CleanupFailed {
			t.Errorf("published cleanupOK = %v, cleanupFailed = %v, want true/false", ev.CleanupOK, ev.CleanupFailed)
		}
		for _, id := range []string{redesignSmokeRow, redesignIntegrationRow} {
			if got := rowVerdict(t, ev.RowVerdicts, id).Verdict; got != "LIVE-PASS" {
				t.Errorf("live verdict for %s = %q, want LIVE-PASS", id, got)
			}
			if got := publishedRow(t, rep, id).Layer3; got != "LIVE-PASS" {
				t.Errorf("published layer3 for %s = %q, want LIVE-PASS", id, got)
			}
		}

		if len(ev.StartSteps) != 1 || ev.StartSteps[0].Result != "pass" {
			t.Fatalf("start steps = %+v, want the one deploy command to pass", ev.StartSteps)
		}
		if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].Result != "pass" {
			t.Fatalf("health steps = %+v, want the one readiness command to pass", ev.HealthSteps)
		}
		if got := ev.HealthSteps[0].Decisive; !strings.Contains(got, `"result": "ready"`) {
			t.Errorf("health decisive output = %q, want the recipe's readiness line", got)
		}
		if len(ev.Steps) != 1 || ev.Steps[0].Result != "pass" {
			t.Fatalf("walkthrough steps = %+v, want the one business check to pass", ev.Steps)
		}

		deploy := decodeRecipeLine(t, "the deploy step", ev.StartSteps[0].Decisive)
		business := decodeRecipeLine(t, "the business check step", ev.Steps[0].Decisive)
		if deploy.Result != "deployed" || business.Result != "business-pass" {
			t.Errorf("recipe results = %q and %q, want deployed and business-pass", deploy.Result, business.Result)
		}
		// One deployment, one artifact, one revision: what was built is what
		// answered, and it is the revision the run prepared.
		if deploy.RepoCommit != subject.head || business.RepoCommit != subject.head {
			t.Errorf("deploy repoCommit = %q and business repoCommit = %q, want the prepared HEAD %q",
				deploy.RepoCommit, business.RepoCommit, subject.head)
		}
		if deploy.ArtifactSHA256 == "" || deploy.ArtifactSHA256 != business.ArtifactSHA256 {
			t.Errorf("deploy artifactSha256 = %q, business artifactSha256 = %q, want one non-empty digest for both",
				deploy.ArtifactSHA256, business.ArtifactSHA256)
		}
		wantTarget := root + "/service.sock"
		if deploy.Target != wantTarget || business.Target != wantTarget {
			t.Errorf("deploy target = %q, business target = %q, want %q", deploy.Target, business.Target, wantTarget)
		}
		if want := (liveQuote{Quantity: 3, TotalCents: 375, Revision: subject.head}); business.Observed != want {
			t.Errorf("observed quote = %+v, want %+v", business.Observed, want)
		}
	})

	t.Run("a deployment of another revision under the run root refuses the start phase", func(t *testing.T) {
		subject := newRedesignSubject(t)
		runID := liveRunID("livetruth-stale")
		registerRunCleanup(t, subject, redesignRecipePath, runID)
		root := redesignRunRoot(runID)

		// The prior revision prices three items at 300 cents. Deployed under
		// the run's own root from its own checkout, it is exactly the healthy
		// older deployment the start phase must refuse to run beside.
		checkout := detachedCheckout(t, subject, subject.priorCommits[0])
		if code, stdout, stderr := runRecipe(t, checkout, redesignRecipePath, "deploy", runID); code != 0 {
			t.Fatalf("deploying the prior revision exited %d, want 0; stdout=%s stderr=%s", code, stdout, stderr)
		}

		_, _, exec := runLivePipeline(t, subject, runID, redesignPlan(subject.head, runID), nil)
		ev := readLiveEvidence(t, exec.EvidencePath)

		if len(ev.StartSteps) != 1 || ev.StartSteps[0].Result != "fail" {
			t.Fatalf("start steps = %+v, want the deploy command recorded as a completed failure", ev.StartSteps)
		}
		// The refusal has to be the existing run root, not some other way for a
		// deploy to exit 1: a green subtest whose start failed for an unrelated
		// reason would prove nothing about an older deployment.
		if got := ev.StartSteps[0].Decisive; !strings.Contains(got, root) {
			t.Errorf("start decisive output = %q, want the refusal to name the run root %s it would have had to overwrite", got, root)
		}
		want := []liveStepFailure{{Phase: "start", Index: 1, Expectation: "profile start command 1"}}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
		// Readiness never runs, so nothing can mistake the older deployment's
		// health for this run's, and no business assertion is ever reached.
		if len(ev.HealthSteps) != 0 {
			t.Errorf("health steps = %+v, want none after the start phase failed", ev.HealthSteps)
		}
		if len(ev.Steps) != 0 {
			t.Errorf("walkthrough steps = %+v, want none after the start phase failed", ev.Steps)
		}
		if got := rowVerdict(t, ev.RowVerdicts, redesignSmokeRow).Verdict; got != "LIVE-INCONCLUSIVE" {
			t.Errorf("live verdict for %s = %q, want LIVE-INCONCLUSIVE", redesignSmokeRow, got)
		}
		requireAbsent(t, "after execute-live returned", root)
	})
}

// covers: SC-13
func TestLiveTruth_CleanupConfinedToRun(t *testing.T) {
	requirePython3(t)
	subject := newRedesignSubject(t)

	runA := liveRunID("livetruth-confined-a")
	runB := liveRunID("livetruth-confined-b")
	registerRunCleanup(t, subject, redesignRecipePath, runA)
	registerRunCleanup(t, subject, redesignRecipePath, runB)

	// The sentinel shares run A's resource prefix without being run A's root.
	// A cleanup that selected by prefix rather than by the run's own directory
	// would take it, and an unrelated neighbour with it.
	sentinel := redesignRunRoot(runA) + "-sentinel"
	keep := filepath.Join(sentinel, "keep")
	if err := os.MkdirAll(keep, 0o700); err != nil {
		t.Fatalf("create the sentinel directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(sentinel); err != nil {
			t.Logf("remove sentinel %s: %v", sentinel, err)
		}
	})

	// Run B is a neighbouring run of the same harness at the same revision,
	// deployed from its own checkout so it belongs to no verifier run.
	checkoutB := detachedCheckout(t, subject, subject.head)
	if code, stdout, stderr := runRecipe(t, checkoutB, redesignRecipePath, "deploy", runB); code != 0 {
		t.Fatalf("deploying the neighbouring run exited %d, want 0; stdout=%s stderr=%s", code, stdout, stderr)
	}

	_, _, exec := runLivePipeline(t, subject, runA, redesignPlan(subject.head, runA), nil)
	if !exec.CleanupOK || exec.CleanupFailed {
		t.Errorf("cleanupOK = %v, cleanupFailed = %v, want true/false", exec.CleanupOK, exec.CleanupFailed)
	}

	requireAbsent(t, "run A after its own cleanup", redesignRunRoot(runA))
	for _, name := range []string{"state.json", "service.sock"} {
		if _, err := os.Stat(filepath.Join(redesignRunRoot(runB), name)); err != nil {
			t.Errorf("the neighbouring run %s lost %s to run %s's cleanup: %v", runB, name, runA, err)
		}
	}
	if code, stdout, stderr := runRecipe(t, subject.repo, redesignRecipePath, "ready", runB); code != 0 {
		t.Errorf("ready %s exited %d after run %s cleaned up, want the neighbour still serving; stdout=%s stderr=%s",
			runB, code, runA, stdout, stderr)
	}
	requireSentinelIntact(t, keep)

	if code, stdout, stderr := runRecipe(t, subject.repo, redesignRecipePath, "cleanup", runB); code != 0 {
		t.Fatalf("cleanup of the neighbouring run exited %d, want 0; stdout=%s stderr=%s", code, stdout, stderr)
	}
	requireAbsent(t, "run A after both cleanups", redesignRunRoot(runA))
	requireAbsent(t, "run B after its own cleanup", redesignRunRoot(runB))
	requireSentinelIntact(t, keep)
}

// requireSentinelIntact fails when the unrelated directory under the shared
// prefix did not survive.
func requireSentinelIntact(t *testing.T, keep string) {
	t.Helper()
	info, err := os.Stat(keep)
	if err != nil {
		t.Errorf("sentinel %s: %v, want the unrelated directory untouched", keep, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("sentinel %s is not a directory, want the unrelated directory untouched", keep)
	}
}

// The proofs below run against a second subject: a phase-logging deployment
// whose helper script records which phases the run reached. Where the harness
// subject proves what a healthy journey observes, this one is built for the
// paths that fail — a refused start, a readiness command that exits nonzero,
// stalls or terminates the verifier, and the cleanup every one of them still
// owes. The log is a file on disk rather than an evidence field, so "the
// business check never ran" is read from the run itself.
const (
	phaseCoverageSource = "docs/superpowers/coverage/2026-09-07-phase-logging-subject-coverage.md"
	phaseProfilePath    = ".test-design/live-profiles/phase-logging-local.json"
	phaseScriptPath     = "scripts/phase.py"

	// The subject journeys one row, so exactly one row is applicable and the
	// plan's single business step is what settles it.
	phaseRow = "SC-01"

	// A patient deadline lets every ordinary phase finish; the impatient one
	// is the deadline a stalled readiness command is held past.
	phasePatientTimeoutSeconds   = 20
	phaseImpatientTimeoutSeconds = 1
)

// phaseScript is the subject's committed helper script, invoked by path under
// python3 exactly as the profile declares it. It derives both of the run's
// roots from the run identifier it is handed, so no path this test owns ever
// reaches the profile, the plan, the evidence or the script's own output.
const phaseScript = `#!/usr/bin/env python3
"""The phase-logging live subject.

Every path this script touches is derived from the run identifier it is given,
under /tmp: the run root a deployment owns, and the sibling log root that
records the phases the run reached. A test asks a phase to behave differently
by leaving a directive file for it under the log root, so the profile's command
arrays stay the committed ones whatever the run is meant to prove.
"""

import json
import os
import shutil
import signal
import sys
import time

# A readiness poll is short, so a deployment that becomes ready on its second
# poll costs a fraction of any deadline. A stall or a signal outlives one.
POLL_SECONDS = 0.05
MAX_POLLS = 40
STALL_SECONDS = 20


def roots(run_id):
    """Return the run root and the log root this run owns."""
    base = "/tmp/test-design-livetruth-" + run_id
    return base, base + "-log"


def record(log_root, phase):
    """Append one phase line to the log this run owns."""
    os.makedirs(log_root, mode=0o700, exist_ok=True)
    with open(os.path.join(log_root, "phases.log"), "a") as handle:
        handle.write(phase + "\n")


def directive(log_root, phase):
    """Return what the test asked of this phase, or pass when it asked nothing."""
    try:
        with open(os.path.join(log_root, phase + ".directive")) as handle:
            return handle.read().strip()
    except FileNotFoundError:
        return "pass"


def obey(log_root, phase):
    """Carry out a directive that ends this invocation; return any other one.

    fail    refuses the phase the way a real one does, with a nonzero exit
    stall   stays busy past any deadline the profile sets
    signal  terminates the verifier and then stays alive, so the signal is
            observed while the verifier is still waiting for this process
            rather than after it has already exited
    """
    asked = directive(log_root, phase)
    if asked == "fail":
        print(json.dumps({"result": phase + "-refused"}))
        sys.exit(1)
    if asked == "stall":
        time.sleep(STALL_SECONDS)
        sys.exit(0)
    if asked == "signal":
        os.kill(os.getppid(), signal.SIGTERM)
        time.sleep(STALL_SECONDS)
        sys.exit(0)
    return asked


def polls_required(asked):
    """polls-<n> holds the deployment back until its nth readiness poll."""
    if asked.startswith("polls-"):
        return int(asked.split("-", 1)[1])
    return 1


def ready(run_root, log_root, asked):
    """Poll until the deployment is ready, recording every poll it took.

    The caller already recorded this invocation's first poll; each further one
    records itself, so the log carries one ready line per poll and the count in
    the result is the poll the deployment became ready on.
    """
    required = polls_required(asked)
    attempts = 1
    while attempts <= MAX_POLLS:
        if attempts >= required and os.path.isdir(run_root):
            print(json.dumps({"result": "ready", "polls": attempts}))
            return 0
        time.sleep(POLL_SECONDS)
        attempts += 1
        record(log_root, "ready")
    print(json.dumps({"result": "never-ready", "polls": attempts}))
    return 1


def main(argv):
    if len(argv) != 3:
        print("usage: phase.py <phase> <run-id>", file=sys.stderr)
        return 2
    phase, run_id = argv[1], argv[2]
    run_root, log_root = roots(run_id)
    record(log_root, phase)
    asked = obey(log_root, phase)

    if phase == "deploy":
        os.makedirs(run_root, mode=0o700, exist_ok=True)
        print(json.dumps({"result": "deployed"}))
        return 0
    if phase == "seed":
        if not os.path.isdir(run_root):
            print(json.dumps({"result": "seed-refused"}))
            return 1
        with open(os.path.join(run_root, "seeded"), "w") as handle:
            handle.write(run_id + "\n")
        print(json.dumps({"result": "seeded"}))
        return 0
    if phase == "ready":
        return ready(run_root, log_root, asked)
    if phase == "check":
        if not os.path.isfile(os.path.join(run_root, "seeded")):
            print(json.dumps({"result": "business-refused"}))
            return 1
        print(json.dumps({"result": "business-pass"}))
        return 0
    if phase == "teardown":
        print(json.dumps({"result": "teardown-pass"}))
        return 0
    if phase == "cleanup":
        removed = os.path.isdir(run_root)
        shutil.rmtree(run_root, ignore_errors=True)
        print(json.dumps({"result": "cleanup-pass", "removed": removed, "absent": not os.path.exists(run_root)}))
        return 0
    print("unknown phase " + phase, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
`

const phaseGoMod = `module phaselive

go 1.26
`

const phaseGateSource = `package gate

// Ready reports whether a deployment that has answered polls readiness polls
// may be treated as ready. The running subject applies the same rule, which is
// why one row can be bound by a unit test and settled live.
func Ready(polls int) bool { return polls >= 2 }
`

// phaseGateTestSource is the subject's own tagged test. Its tag cites the
// subject document's SC-01, never a row of this repository: the text lives
// inside a string literal here and only becomes a comment in the temporary
// repository the builder writes.
const phaseGateTestSource = `package gate

import "testing"

// covers: SC-01
func TestReadyOnlyAfterTheSecondPoll(t *testing.T) {
	if Ready(1) {
		t.Fatalf("Ready(1) = true, want the first poll to be too early")
	}
	if !Ready(2) {
		t.Fatalf("Ready(2) = false, want the second poll to report ready")
	}
}
`

// phaseCoverageDoc declares the live decision required and journeys the one
// row the walkthrough settles. BASELINE_COMMIT and BASELINE_SHA256 are pinned
// by the shared subject builder.
const phaseCoverageDoc = `# Coverage: phase-logging live subject

Spec: docs/superpowers/specs/2026-09-07-phase-logging-live-subject.md
Baseline: BASELINE_COMMIT · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-phase-logging-subject-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/phase-logging-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-phase-logging-subject-coverage.md"]

## Live decision

- decision: required
- reason: only a running deployment can show that the business check waited for readiness

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | The business check runs only after the deployment reported ready, and the run's own resources are released whatever the outcome |

## Scenarios

### Behavior: readiness before business

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the business check runs against a deployment that reported ready | the check reports business-pass after the readiness command passed | integration | the health phase result is ignored so the check runs against a deployment that never reported ready | med |
`

// phaseProfileJSON is the subject's committed profile. Its command arrays never
// vary; a variant is a run whose directives the test leaves under the run's own
// log root. envAllowlist is PATH alone, so the script must live on what a live
// command is granted and nothing else. timeoutSeconds is the one thing a
// caller chooses: a readiness command held past a deadline needs a short one.
func phaseProfileJSON(timeoutSeconds int) string {
	return fmt.Sprintf(`{
  "schemaVersion": %q,
  "classification": "local",
  "start": [
    ["python3", "scripts/phase.py", "deploy", "{run-id}"],
    ["python3", "scripts/phase.py", "seed", "{run-id}"]
  ],
  "health": [["python3", "scripts/phase.py", "ready", "{run-id}"]],
  "walkthrough": [["python3", "scripts/phase.py", "check", "{run-id}"]],
  "cleanup": [["python3", "scripts/phase.py", "cleanup", "{run-id}"]],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "matches", "stream": "stdout", "value": "\"result\": \"cleanup-pass\""},
    {"form": "matches", "stream": "stdout", "value": "\"absent\": true"}
  ],
  "envAllowlist": ["PATH"],
  "resourcePrefixes": ["/tmp/test-design-livetruth-{run-id}"],
  "endpoints": [],
  "timeoutSeconds": %d,
  "cleanupTimeoutSeconds": 15,
  "maxOutputBytes": 1048576
}
`, evidence.SchemaVersion, timeoutSeconds)
}

// newPhaseSubject builds the phase-logging subject and runs the real baseline
// op over it. timeoutSeconds is the profile's deadline for every command
// outside cleanup.
func newPhaseSubject(t *testing.T, timeoutSeconds int) *liveSubject {
	t.Helper()
	return newLiveSubject(t, phaseCoverageSource, phaseProfilePath, []map[string]string{{
		"go.mod":            phaseGoMod,
		"gate/gate.go":      phaseGateSource,
		"gate/gate_test.go": phaseGateTestSource,
		phaseScriptPath:     phaseScript,
		phaseProfilePath:    phaseProfileJSON(timeoutSeconds),
		phaseCoverageSource: phaseCoverageDoc,
	}}, nil)
}

// phaseRunRoot and phaseLogRoot are the two roots the script derives from a run
// identifier. They are spelled here only to observe and remove them; no path
// this test owns ever reaches the profile or the plan.
func phaseRunRoot(runID string) string { return "/tmp/test-design-livetruth-" + runID }
func phaseLogRoot(runID string) string { return phaseRunRoot(runID) + "-log" }

// newPhaseRun mints a run identifier, creates the log root the script derives
// from it, leaves the directives this variant needs there, and registers the
// removal of both roots. directives maps a phase to what it must do; a phase
// with no directive behaves.
func newPhaseRun(t *testing.T, s *liveSubject, prefix string, directives map[string]string) string {
	t.Helper()
	runID := liveRunID(prefix)
	logRoot := phaseLogRoot(runID)

	// Registered before the script's own cleanup so it runs after it: the
	// script recreates the log root whenever it records a phase, and nothing
	// of this run may outlive the test under /tmp.
	t.Cleanup(func() {
		for _, root := range []string{phaseRunRoot(runID), logRoot} {
			if err := os.RemoveAll(root); err != nil {
				t.Logf("remove %s: %v", root, err)
			}
		}
	})
	// The shared registry hands a script the harness recipe's three names
	// while this profile grants PATH alone. This script reads no environment
	// variable at all, so the run it cleans up is the run the verifier made.
	registerRunCleanup(t, s, phaseScriptPath, runID)

	if err := os.MkdirAll(logRoot, 0o700); err != nil {
		t.Fatalf("create the log root for run %s: %v", runID, err)
	}
	for phase, asked := range directives {
		path := filepath.Join(logRoot, phase+".directive")
		if err := os.WriteFile(path, []byte(asked), 0o644); err != nil {
			t.Fatalf("write the %s directive: %v", phase, err)
		}
	}
	return runID
}

// phasePlanCleanupExpectation is the plan-cleanup step's own text, which is
// also how its failure is named in unboundFailures.
const phasePlanCleanupExpectation = "the walkthrough releases what it took"

// phasePlan is the plan the live-runner would hand to execute-live for this
// subject: one bound business check and, when the variant needs one, a bound
// plan-cleanup step of the walkthrough's own.
func phasePlan(runID string, withPlanCleanup bool) map[string]any {
	plan := map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{{
			"expectation": "the deployment that reported ready answers the business check",
			"command":     []string{"python3", phaseScriptPath, "check", runID},
			"predicates": []map[string]string{
				{"form": "exit-code", "value": "0"},
				{"form": "matches", "stream": "stdout", "value": `"result": "business-pass"`},
			},
			"scBindings": []string{phaseRow},
		}},
	}
	if withPlanCleanup {
		plan["cleanup"] = []map[string]any{{
			"expectation": phasePlanCleanupExpectation,
			"command":     []string{"python3", phaseScriptPath, "teardown", runID},
			"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
			"scBindings":  []string{phaseRow},
		}}
	}
	return plan
}

// readPhaseLog returns the phases the run recorded, in the order it reached
// them. Reading the log off disk rather than out of the evidence is what makes
// "the business check never ran" an observation of the subject.
func readPhaseLog(t *testing.T, runID string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(phaseLogRoot(runID), "phases.log"))
	if err != nil {
		t.Fatalf("read the phase log of run %s: %v", runID, err)
	}
	var phases []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			phases = append(phases, line)
		}
	}
	return phases
}

// requirePhaseOrder fails unless the run reached after only once every before
// line had been recorded.
func requirePhaseOrder(t *testing.T, phases []string, before, after string) {
	t.Helper()
	lastBefore, firstAfter := -1, -1
	for i, phase := range phases {
		if phase == before {
			lastBefore = i
		}
		if phase == after && firstAfter < 0 {
			firstAfter = i
		}
	}
	if lastBefore < 0 || firstAfter < 0 || firstAfter < lastBefore {
		t.Errorf("phase log = %v, want every %s line before the first %s line", phases, before, after)
	}
}

// countPhase reports how many times the run recorded one phase.
func countPhase(phases []string, phase string) int {
	n := 0
	for _, recorded := range phases {
		if recorded == phase {
			n++
		}
	}
	return n
}

// requireExecutionError fails unless one execution error names the command
// position and carries the cause the row calls for.
func requireExecutionError(t *testing.T, errs []string, position, cause string) {
	t.Helper()
	for _, entry := range errs {
		if strings.HasPrefix(entry, position) && strings.Contains(entry, cause) {
			return
		}
	}
	t.Errorf("executionErrors = %v, want an entry naming %s and the cause %q", errs, position, cause)
}

// stepByExpectation returns the reduced step carrying this expectation.
// Locating a cleanup command by the text the profile gave it, rather than by
// its position among the plan's own cleanup steps, is what keeps the assertion
// about the command the profile declared.
func stepByExpectation(t *testing.T, steps []live.ReducedStep, expectation string) live.ReducedStep {
	t.Helper()
	for _, step := range steps {
		if step.Expectation == expectation {
			return step
		}
	}
	t.Fatalf("steps = %+v, want one carrying the expectation %q", steps, expectation)
	return live.ReducedStep{}
}

// requireBusinessNeverRan holds what every gated variant shares: the log has no
// check line, no walkthrough step was reduced, and the row the walkthrough
// would have settled stays inconclusive rather than inheriting a phase result.
func requireBusinessNeverRan(t *testing.T, ev liveEvidence, phases []string) {
	t.Helper()
	if slices.Contains(phases, "check") {
		t.Errorf("phase log = %v, want no check line: the business assertion must not run", phases)
	}
	if len(ev.Steps) != 0 {
		t.Errorf("walkthrough steps = %+v, want none after the readiness gate held", ev.Steps)
	}
	if got := rowVerdict(t, ev.RowVerdicts, phaseRow).Verdict; got != "LIVE-INCONCLUSIVE" {
		t.Errorf("live verdict for %s = %q, want LIVE-INCONCLUSIVE", phaseRow, got)
	}
}

// requireRunRootCreated reads the start phase as the proof that the run really
// made the resource its cleanup then has to remove: the seed command refuses
// unless the deploy before it created the run root.
func requireRunRootCreated(t *testing.T, ev liveEvidence) {
	t.Helper()
	if len(ev.StartSteps) != 2 || ev.StartSteps[0].Result != "pass" || ev.StartSteps[1].Result != "pass" {
		t.Fatalf("start steps = %+v, want the deploy to create the run root and the seed to find it", ev.StartSteps)
	}
}

// requireOwnedCleanup holds what a run owes after any failed phase: the run
// root is gone from disk by the time execute-live returns, the profile's own
// cleanup command ran and passed, and the removal predicate the profile author
// wrote is what says the resource is absent.
func requireOwnedCleanup(t *testing.T, out executeLiveOutput, ev liveEvidence, runID string) {
	t.Helper()
	requireAbsent(t, "after execute-live returned", phaseRunRoot(runID))
	if !out.CleanupOK || out.CleanupFailed {
		t.Errorf("cleanupOK = %v, cleanupFailed = %v in the op output, want true/false", out.CleanupOK, out.CleanupFailed)
	}
	if !ev.CleanupOK || ev.CleanupFailed {
		t.Errorf("cleanupOK = %v, cleanupFailed = %v in the evidence, want true/false", ev.CleanupOK, ev.CleanupFailed)
	}
	step := stepByExpectation(t, ev.CleanupSteps, "profile cleanup command 1")
	if step.Result != "pass" {
		t.Errorf("profile cleanup command 1 = %+v, want a passing step", step)
	}
	if got := step.Decisive; !strings.Contains(got, `"absent": true`) {
		t.Errorf("profile cleanup decisive output = %q, want the removal predicate's own proof of absence", got)
	}
	// An absence that was never a presence would prove nothing: the cleanup
	// command reports that the run root really was there for it to take.
	if got := step.Decisive; !strings.Contains(got, `"removed": true`) {
		t.Errorf("profile cleanup decisive output = %q, want the run root the run created to have been there to remove", got)
	}
	if phases := readPhaseLog(t, runID); !slices.Contains(phases, "cleanup") {
		t.Errorf("phase log = %v, want a cleanup line: the profile's cleanup command must run after a failed phase", phases)
	}
}

// covers: SC-11
func TestLiveTruth_ReadinessGatesBusinessAssertions(t *testing.T) {
	requirePython3(t)
	patient := newPhaseSubject(t, phasePatientTimeoutSeconds)

	t.Run("a start command that exits nonzero stops the run before readiness", func(t *testing.T) {
		runID := newPhaseRun(t, patient, "lt-start-fails", map[string]string{"deploy": "fail"})
		_, _, execOut := runLivePipeline(t, patient, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if len(ev.StartSteps) != 1 || ev.StartSteps[0].Result != "fail" {
			t.Fatalf("start steps = %+v, want the refused deploy recorded as a completed failure", ev.StartSteps)
		}
		want := []liveStepFailure{{Phase: "start", Index: 1, Expectation: "profile start command 1"}}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
		phases := readPhaseLog(t, runID)
		if slices.Contains(phases, "ready") {
			t.Errorf("phase log = %v, want no ready line: readiness must not be polled after the start phase failed", phases)
		}
		if len(ev.HealthSteps) != 0 {
			t.Errorf("health steps = %+v, want none after the start phase failed", ev.HealthSteps)
		}
		requireBusinessNeverRan(t, ev, phases)
	})

	t.Run("a health command that exits nonzero stops the run before the business step", func(t *testing.T) {
		runID := newPhaseRun(t, patient, "lt-ready-fails", map[string]string{"ready": "fail"})
		_, _, execOut := runLivePipeline(t, patient, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if len(ev.StartSteps) != 2 {
			t.Fatalf("start steps = %+v, want both start commands to have run", ev.StartSteps)
		}
		if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].Result != "fail" {
			t.Fatalf("health steps = %+v, want the refused readiness command recorded as a completed failure", ev.HealthSteps)
		}
		want := []liveStepFailure{{Phase: "health", Index: 1, Expectation: "profile health command 1"}}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
		requireBusinessNeverRan(t, ev, readPhaseLog(t, runID))
	})

	t.Run("a health command held past the deadline is recorded as no result at all", func(t *testing.T) {
		impatient := newPhaseSubject(t, phaseImpatientTimeoutSeconds)
		runID := newPhaseRun(t, impatient, "lt-ready-stalls", map[string]string{"ready": "stall"})
		_, _, execOut := runLivePipeline(t, impatient, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		// A command that never completed has no reduced step to read a result
		// off, and no unbound failure either: converting the deadline into a
		// completed failure is exactly what must not happen.
		if len(ev.HealthSteps) != 0 {
			t.Errorf("health steps = %+v, want none: the stalled command never completed", ev.HealthSteps)
		}
		requireExecutionError(t, ev.ExecutionErrors, "health[1]:", "subprocess timeout")
		if len(ev.UnboundFailures) != 0 {
			t.Errorf("unboundFailures = %+v, want none for a command that never completed", ev.UnboundFailures)
		}
		requireBusinessNeverRan(t, ev, readPhaseLog(t, runID))
	})

	t.Run("a health command that terminates the verifier and stays alive is an interruption", func(t *testing.T) {
		runID := newPhaseRun(t, patient, "lt-ready-signals", map[string]string{"ready": "signal"})
		_, _, execOut := runLivePipeline(t, patient, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if len(ev.HealthSteps) != 0 {
			t.Errorf("health steps = %+v, want none: the interrupted command never completed", ev.HealthSteps)
		}
		requireExecutionError(t, ev.ExecutionErrors, "health[1]:", "subprocess interrupted")
		if len(ev.UnboundFailures) != 0 {
			t.Errorf("unboundFailures = %+v, want none for a command that never completed", ev.UnboundFailures)
		}
		requireBusinessNeverRan(t, ev, readPhaseLog(t, runID))
	})

	t.Run("a deployment that becomes ready on its second poll is checked after it did", func(t *testing.T) {
		runID := newPhaseRun(t, patient, "lt-ready-turns", map[string]string{"ready": "polls-2"})
		_, _, execOut := runLivePipeline(t, patient, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].Result != "pass" {
			t.Fatalf("health steps = %+v, want the readiness command to pass once it polled through", ev.HealthSteps)
		}
		if got := ev.HealthSteps[0].Decisive; !strings.Contains(got, `"polls": 2`) {
			t.Errorf("health decisive output = %q, want the poll the deployment became ready on", got)
		}
		phases := readPhaseLog(t, runID)
		if got := countPhase(phases, "ready"); got != 2 {
			t.Errorf("phase log = %v, recorded %d ready lines, want the two polls readiness took", phases, got)
		}
		requirePhaseOrder(t, phases, "ready", "check")
		if len(ev.Steps) != 1 || ev.Steps[0].Result != "pass" {
			t.Fatalf("walkthrough steps = %+v, want the one business check to pass", ev.Steps)
		}
		if got := rowVerdict(t, ev.RowVerdicts, phaseRow).Verdict; got != "LIVE-PASS" {
			t.Errorf("live verdict for %s = %q, want LIVE-PASS", phaseRow, got)
		}
	})
}

// covers: SC-12
func TestLiveTruth_OwnedCleanupAfterFailedPhase(t *testing.T) {
	requirePython3(t)
	subject := newPhaseSubject(t, phasePatientTimeoutSeconds)

	t.Run("a start command fails after an earlier one created the run root", func(t *testing.T) {
		runID := newPhaseRun(t, subject, "lt-owned-start", map[string]string{"seed": "fail", "teardown": "fail"})
		_, _, execOut := runLivePipeline(t, subject, runID, phasePlan(runID, true), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		// The first start command created the run root; only the second one
		// failed, so what cleanup owes here is a resource the run really made.
		if len(ev.StartSteps) != 2 || ev.StartSteps[0].Result != "pass" || ev.StartSteps[1].Result != "fail" {
			t.Fatalf("start steps = %+v, want the deploy to pass and the seed to fail", ev.StartSteps)
		}
		// The plan's own cleanup step ran after the failed phase and failed;
		// its verdict application was suppressed because the walkthrough never
		// ran, which is precisely why it has to stay visible here.
		want := []liveStepFailure{
			{Phase: "start", Index: 2, Expectation: "profile start command 2"},
			{Phase: "plan-cleanup", Index: 1, Expectation: phasePlanCleanupExpectation},
		}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
		requireBusinessNeverRan(t, ev, readPhaseLog(t, runID))
		requireOwnedCleanup(t, execOut, ev, runID)
	})

	t.Run("the health phase fails after the start phase created the run root", func(t *testing.T) {
		runID := newPhaseRun(t, subject, "lt-owned-health", map[string]string{"ready": "fail"})
		_, _, execOut := runLivePipeline(t, subject, runID, phasePlan(runID, true), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		requireRunRootCreated(t, ev)
		if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].Result != "fail" {
			t.Fatalf("health steps = %+v, want the refused readiness command recorded as a completed failure", ev.HealthSteps)
		}
		want := []liveStepFailure{{Phase: "health", Index: 1, Expectation: "profile health command 1"}}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
		requireBusinessNeverRan(t, ev, readPhaseLog(t, runID))
		requireOwnedCleanup(t, execOut, ev, runID)
	})

	t.Run("a bound business step fails against a deployment that started and reported ready", func(t *testing.T) {
		runID := newPhaseRun(t, subject, "lt-owned-check", map[string]string{"check": "fail"})
		_, _, execOut := runLivePipeline(t, subject, runID, phasePlan(runID, true), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		requireRunRootCreated(t, ev)
		if len(ev.Steps) != 1 || ev.Steps[0].Result != "fail" {
			t.Fatalf("walkthrough steps = %+v, want the one business check recorded as a completed failure", ev.Steps)
		}
		if got := rowVerdict(t, ev.RowVerdicts, phaseRow).Verdict; got != "LIVE-FAIL" {
			t.Errorf("live verdict for %s = %q, want LIVE-FAIL", phaseRow, got)
		}
		// The failure reached a row verdict, so it is not an unbound one too.
		if len(ev.UnboundFailures) != 0 {
			t.Errorf("unboundFailures = %+v, want none: the failing step is bound to %s", ev.UnboundFailures, phaseRow)
		}
		requireOwnedCleanup(t, execOut, ev, runID)
	})
}

// The proofs below add two more subjects to the two above, each built for one
// thing a run can get wrong once its business assertions are over. The
// cleanup-accounting subject drives the profile's own cleanup commands into
// every combination the tri-state distinguishes; the bounded-output subject
// runs a walkthrough whose steps are cut short in each of the four ways a
// command can fail to leave a result behind at all.

// newLiveTruthRun mints a run identifier for a subject whose helper script
// derives every path it touches from that identifier, and registers the removal
// of the two roots such a script can own. Nothing of a run may outlive the test
// under /tmp, including a root a variant deliberately leaves behind.
func newLiveTruthRun(t *testing.T, prefix string) string {
	t.Helper()
	runID := liveRunID(prefix)
	t.Cleanup(func() {
		for _, root := range []string{phaseRunRoot(runID), phaseLogRoot(runID)} {
			if err := os.RemoveAll(root); err != nil {
				t.Logf("remove %s: %v", root, err)
			}
		}
	})
	return runID
}

// writeDirectives leaves what this variant asks of each phase under the run's
// own log root, so a subject's committed command arrays stay the committed ones
// whatever the run is meant to prove.
func writeDirectives(t *testing.T, runID string, directives map[string]string) {
	t.Helper()
	logRoot := phaseLogRoot(runID)
	if err := os.MkdirAll(logRoot, 0o700); err != nil {
		t.Fatalf("create the log root for run %s: %v", runID, err)
	}
	for phase, asked := range directives {
		path := filepath.Join(logRoot, phase+".directive")
		if err := os.WriteFile(path, []byte(asked), 0o644); err != nil {
			t.Fatalf("write the %s directive: %v", phase, err)
		}
	}
}

// liveReasonDetail returns the detail of the layer-3 reason carrying code, and
// fails when finalize raised none. The detail is where a run's own failure is
// named, so a reason whose text named nothing would leave a reviewer with a
// code and no way to act on it.
func liveReasonDetail(t *testing.T, reasons []report.Reason, code string) string {
	t.Helper()
	for _, r := range reasons {
		if r.Layer == "3" && r.Code == code {
			return r.Detail
		}
	}
	t.Fatalf("reasons = %+v, want a layer-3 %q reason", reasons, code)
	return ""
}

// unboundProbeExpectation is the plan's own text for the step that is meant to
// fail with no row to carry the failure. It is what the evidence and finalize's
// reason detail must repeat back: a listed failure a reviewer cannot identify
// is no better than an unlisted one.
const unboundProbeExpectation = "the walkthrough's own probe answers before the run is judged"

// phaseUnboundPlan is the phase subject's plan with one extra step the subject
// refuses and no row is bound to: appended to the walkthrough when inCleanup is
// false, and standing alone in the plan's own cleanup section when it is true.
func phaseUnboundPlan(t *testing.T, runID string, inCleanup bool) map[string]any {
	t.Helper()
	probe := map[string]any{
		"expectation": unboundProbeExpectation,
		"command":     []string{"python3", phaseScriptPath, "teardown", runID},
		"predicates":  []map[string]string{{"form": "exit-code", "value": "0"}},
	}
	plan := phasePlan(runID, false)
	if inCleanup {
		plan["cleanup"] = []map[string]any{probe}
		return plan
	}
	steps, ok := plan["steps"].([]map[string]any)
	if !ok {
		t.Fatalf("plan steps = %T, want the walkthrough step list the probe extends", plan["steps"])
	}
	plan["steps"] = append(steps, probe)
	return plan
}

// covers: SC-15
func TestLiveTruth_UnboundAssertionFailureBlocks(t *testing.T) {
	requirePython3(t)

	// Each placement of the failing step publishes a report, which dirties the
	// subject repository, so each one gets a subject of its own.
	for _, tc := range []struct {
		name     string
		prefix   string
		cleanup  bool
		expected liveStepFailure
	}{
		{
			name:     "a walkthrough step no row is bound to fails and blocks the run",
			prefix:   "lt-unbound-step",
			expected: liveStepFailure{Phase: "step", Index: 2, Expectation: unboundProbeExpectation},
		},
		{
			name:     "a plan cleanup step no row is bound to fails and blocks the run",
			prefix:   "lt-unbound-cleanup",
			cleanup:  true,
			expected: liveStepFailure{Phase: "plan-cleanup", Index: 1, Expectation: unboundProbeExpectation},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subject := newPhaseSubject(t, phasePatientTimeoutSeconds)
			runID := newPhaseRun(t, subject, tc.prefix, map[string]string{"teardown": "fail"})

			tr, _, execOut := runLivePipeline(t, subject, runID, phaseUnboundPlan(t, runID, tc.cleanup), nil)
			ev := readLiveEvidence(t, execOut.EvidencePath)

			want := []liveStepFailure{tc.expected}
			if !reflect.DeepEqual(ev.UnboundFailures, want) {
				t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
			}
			// The failure belongs to the step that failed and to nothing else:
			// the row a passing step settled keeps the verdict it earned.
			if got := rowVerdict(t, ev.RowVerdicts, phaseRow).Verdict; got != "LIVE-PASS" {
				t.Errorf("live verdict for %s = %q, want LIVE-PASS", phaseRow, got)
			}

			code, stdout, stderr := runVerifyOp(t, "finalize",
				liveFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, execOut.EvidencePath))
			if code != exitOK {
				t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
			}
			out := decodeFinalizeOutput(t, stdout)
			if out.Verdict != report.VerdictBlocked {
				t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictBlocked, out.Reasons)
			}
			requireReason(t, out.Reasons, "3", "", "live-unbound-assertion-failed")
			wantDetail := fmt.Sprintf("%s[%d] %s", tc.expected.Phase, tc.expected.Index, tc.expected.Expectation)
			if got := liveReasonDetail(t, out.Reasons, "live-unbound-assertion-failed"); !strings.Contains(got, wantDetail) {
				t.Errorf("unbound-assertion detail = %q, want it to name %q", got, wantDetail)
			}

			// The published report is the record a reviewer receives: the same
			// blocking reason, and beside it the row the run really did settle.
			rep := readPublishedReport(t, subject.repo, out.JSONPath)
			requireReason(t, rep.Reasons, "3", "", "live-unbound-assertion-failed")
			if got := liveReasonDetail(t, rep.Reasons, "live-unbound-assertion-failed"); !strings.Contains(got, wantDetail) {
				t.Errorf("published unbound-assertion detail = %q, want it to name %q", got, wantDetail)
			}
			if got := publishedRow(t, rep, phaseRow).Layer3; got != "LIVE-PASS" {
				t.Errorf("published layer3 for %s = %q, want LIVE-PASS", phaseRow, got)
			}
		})
	}

	// Neither readiness phase can ever reach a row, so a command of theirs that
	// completed and failed has unboundFailures as its only destination. Both
	// variants read the reduced step too: the failure is listed because the
	// command failed, not because it went missing.
	gated := newPhaseSubject(t, phasePatientTimeoutSeconds)

	t.Run("a start command that fails is listed with the expectation it broke", func(t *testing.T) {
		runID := newPhaseRun(t, gated, "lt-unbound-start", map[string]string{"deploy": "fail"})
		_, _, execOut := runLivePipeline(t, gated, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if len(ev.StartSteps) != 1 || ev.StartSteps[0].Result != "fail" {
			t.Fatalf("start steps = %+v, want the refused deploy recorded as a completed failure", ev.StartSteps)
		}
		want := []liveStepFailure{{Phase: "start", Index: 1, Expectation: "profile start command 1"}}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
	})

	t.Run("a health command that fails is listed with the expectation it broke", func(t *testing.T) {
		runID := newPhaseRun(t, gated, "lt-unbound-health", map[string]string{"ready": "fail"})
		_, _, execOut := runLivePipeline(t, gated, runID, phasePlan(runID, false), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if len(ev.HealthSteps) != 1 || ev.HealthSteps[0].Result != "fail" {
			t.Fatalf("health steps = %+v, want the refused readiness command recorded as a completed failure", ev.HealthSteps)
		}
		want := []liveStepFailure{{Phase: "health", Index: 1, Expectation: "profile health command 1"}}
		if !reflect.DeepEqual(ev.UnboundFailures, want) {
			t.Errorf("unboundFailures = %+v, want %+v", ev.UnboundFailures, want)
		}
	})
}

// The cleanup-accounting subject exists for the phase no assertion watches: the
// profile's own cleanup commands. Its script can be asked to refuse, to outlive
// the cleanup deadline, or to remove the wrong name and leave the run's root
// standing, and the checker that always runs last reports what survived. What
// the evidence must then say is not "cleanup passed" or "cleanup failed" but
// which of three things happened: a command failed, a command never finished,
// or every command finished and held.
const (
	sweepCoverageSource = "docs/superpowers/coverage/2026-09-07-cleanup-accounting-subject-coverage.md"
	sweepProfilePath    = ".test-design/live-profiles/cleanup-accounting-local.json"
	sweepScriptPath     = "scripts/sweep.py"

	// The subject journeys one row, so the plan's single business step is what
	// settles it and every cleanup outcome below is beside that verdict.
	sweepRow = "SC-01"

	// Cleanup gets an impatient deadline of its own, so a command held past
	// it costs the run little; every other command keeps a patient one.
	sweepTimeoutSeconds        = 20
	sweepCleanupTimeoutSeconds = 2
)

// sweepScript is the subject's committed helper script, invoked by path under
// python3 exactly as the profile declares it. Like the phase-logging subject's
// script it derives both of the run's roots from the run identifier it is
// handed and takes its instructions from directive files under the log root, so
// no path this test owns ever reaches the profile, the plan or the evidence.
const sweepScript = `#!/usr/bin/env python3
"""The cleanup-accounting live subject.

The remove phase releases the run's own root, and the absent phase that follows
it reports whatever is still standing. A test asks a cleanup command to refuse,
to outlive its deadline, or to remove a name that never existed by leaving a
directive file for that phase under the log root.
"""

import os
import shutil
import sys
import time

# Longer than any deadline this profile sets, so a command asked to outlive one
# is still running when the verifier gives up on it.
STALL_SECONDS = 20


def roots(run_id):
    """Return the run root and the log root this run owns."""
    base = "/tmp/test-design-livetruth-" + run_id
    return base, base + "-log"


def directive(log_root, phase):
    """Return what the test asked of this phase, or pass when it asked nothing."""
    try:
        with open(os.path.join(log_root, phase + ".directive")) as handle:
            return handle.read().strip()
    except FileNotFoundError:
        return "pass"


def remove(run_root, asked):
    """Release the run's own root the way this variant asks for it.

    wrong-name takes a neighbouring name that never existed, so the run root
    survives for the checker to find. Every other directive releases the root
    first and only then refuses or stalls, so a cleanup command that fails to
    complete still leaves a clean run behind and the checker reports on the
    command's own failure rather than on residue the failure caused.
    """
    if asked == "wrong-name":
        shutil.rmtree(run_root + "-absent", ignore_errors=True)
        return 0
    shutil.rmtree(run_root, ignore_errors=True)
    if asked == "fail":
        print("sweep-refused")
        return 1
    if asked == "stall":
        time.sleep(STALL_SECONDS)
    return 0


def main(argv):
    if len(argv) != 3:
        print("usage: sweep.py <phase> <run-id>", file=sys.stderr)
        return 2
    phase, run_id = argv[1], argv[2]
    run_root, log_root = roots(run_id)
    asked = directive(log_root, phase)

    if phase == "deploy":
        os.makedirs(run_root, mode=0o700, exist_ok=True)
        print("deployed")
        return 0
    if phase == "ready":
        if not os.path.isdir(run_root):
            print("not-ready")
            return 1
        print("ready")
        return 0
    if phase == "check":
        if not os.path.isdir(run_root):
            print("business-refused")
            return 1
        print("business-pass")
        return 0
    if phase == "remove":
        return remove(run_root, asked)
    if phase == "pause":
        if asked == "stall":
            time.sleep(STALL_SECONDS)
        return 0
    if phase == "absent":
        # Silence is the proof of absence; anything still standing is named.
        if os.path.isdir(run_root):
            print("residue " + run_root)
        if asked == "fail":
            return 1
        return 0
    print("unknown phase " + phase, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
`

const sweepGoMod = `module cleanuplive

go 1.26
`

const sweepReleaseSource = `package release

// Clean reports whether a run released everything it took: no cleanup command
// refused, and the checker that ran last found nothing standing.
func Clean(refusals, residue int) bool { return refusals == 0 && residue == 0 }
`

// sweepReleaseTestSource is the subject's own tagged test. Its tag cites the
// subject document's SC-01, never a row of this repository: the text lives
// inside a string literal here and only becomes a comment in the temporary
// repository the builder writes.
const sweepReleaseTestSource = `package release

import "testing"

// covers: SC-01
func TestCleanNeedsBothNoRefusalAndNoResidue(t *testing.T) {
	if !Clean(0, 0) {
		t.Fatalf("Clean(0, 0) = false, want a run that refused nothing and left nothing to be clean")
	}
	if Clean(1, 0) {
		t.Fatalf("Clean(1, 0) = true, want a refused cleanup command to count against the run")
	}
	if Clean(0, 1) {
		t.Fatalf("Clean(0, 1) = true, want residue to count against the run")
	}
}
`

// sweepCoverageDoc declares the live decision required and journeys the one row
// the walkthrough settles. BASELINE_COMMIT and BASELINE_SHA256 are pinned by
// the shared subject builder.
const sweepCoverageDoc = `# Coverage: cleanup accounting live subject

Spec: docs/superpowers/specs/2026-09-07-cleanup-accounting-live-subject.md
Baseline: BASELINE_COMMIT · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-cleanup-accounting-subject-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/cleanup-accounting-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-cleanup-accounting-subject-coverage.md"]

## Live decision

- decision: required
- reason: only a running deployment can show what its own cleanup commands released

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | The business check runs against a deployment the run made, and the run's own cleanup commands report what they released |

## Scenarios

### Behavior: released after the check

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the business check runs against the deployment the run made | the check reports business-pass | integration | the start phase is skipped so the check runs against nothing | med |
`

// sweepCleanupCommands renders the profile's cleanup array from the phases a
// variant declares, in order. The configured last command is the one that
// carries cleanupEvidence, which is why the absence checker is written last
// wherever a variant wants its proof of removal to be the deciding one.
func sweepCleanupCommands(phases []string) string {
	entries := make([]string, len(phases))
	for i, phase := range phases {
		entries[i] = fmt.Sprintf(`["python3", %q, %q, "{run-id}"]`, sweepScriptPath, phase)
	}
	return "[" + strings.Join(entries, ", ") + "]"
}

// sweepProfileJSON is the subject's committed profile. Only its cleanup array
// varies between variants, because the number and order of cleanup commands is
// exactly what the tri-state is about; every other field, the removal evidence
// included, is the same profile in each of them.
func sweepProfileJSON(cleanupPhases []string) string {
	return fmt.Sprintf(`{
  "schemaVersion": %q,
  "classification": "local",
  "start": [["python3", "scripts/sweep.py", "deploy", "{run-id}"]],
  "health": [["python3", "scripts/sweep.py", "ready", "{run-id}"]],
  "walkthrough": [["python3", "scripts/sweep.py", "check", "{run-id}"]],
  "cleanup": %s,
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "not-matches", "stream": "stdout", "value": "residue"}
  ],
  "envAllowlist": ["PATH"],
  "resourcePrefixes": ["/tmp/test-design-livetruth-{run-id}"],
  "endpoints": [],
  "timeoutSeconds": %d,
  "cleanupTimeoutSeconds": %d,
  "maxOutputBytes": 1048576
}
`, evidence.SchemaVersion, sweepCleanupCommands(cleanupPhases), sweepTimeoutSeconds, sweepCleanupTimeoutSeconds)
}

// newSweepSubject builds the cleanup-accounting subject and runs the real
// baseline op over it. cleanupPhases are the phases the profile declares as its
// cleanup commands, in order.
func newSweepSubject(t *testing.T, cleanupPhases []string) *liveSubject {
	t.Helper()
	return newLiveSubject(t, sweepCoverageSource, sweepProfilePath, []map[string]string{{
		"go.mod":                  sweepGoMod,
		"release/release.go":      sweepReleaseSource,
		"release/release_test.go": sweepReleaseTestSource,
		sweepScriptPath:           sweepScript,
		sweepProfilePath:          sweepProfileJSON(cleanupPhases),
		sweepCoverageSource:       sweepCoverageDoc,
	}}, nil)
}

// sweepPlan is the plan the live-runner would hand to execute-live for this
// subject: one bound business check and no cleanup section of its own, so every
// cleanup command the run makes is one the profile declared.
func sweepPlan(runID string) map[string]any {
	return map[string]any{
		"schemaVersion": evidence.SchemaVersion,
		"steps": []map[string]any{{
			"expectation": "the deployment the run made answers the business check",
			"command":     []string{"python3", sweepScriptPath, "check", runID},
			"predicates": []map[string]string{
				{"form": "exit-code", "value": "0"},
				{"form": "matches", "stream": "stdout", "value": "business-pass"},
			},
			"scBindings": []string{sweepRow},
		}},
	}
}

// profileCleanupStep returns the reduced step of the profile's nth cleanup
// command, located by the expectation the verifier gives it.
func profileCleanupStep(t *testing.T, ev liveEvidence, n int) live.ReducedStep {
	t.Helper()
	return stepByExpectation(t, ev.CleanupSteps, fmt.Sprintf("profile cleanup command %d", n))
}

// requireCleanupState fails unless the two booleans read exactly this way in
// both the op output and the evidence. They are a tri-state, so both are always
// named together: "not failed" is not the same claim as "clean".
func requireCleanupState(t *testing.T, out executeLiveOutput, ev liveEvidence, wantOK, wantFailed bool) {
	t.Helper()
	if out.CleanupOK != wantOK || out.CleanupFailed != wantFailed {
		t.Errorf("op output cleanupOK = %v, cleanupFailed = %v, want %v/%v",
			out.CleanupOK, out.CleanupFailed, wantOK, wantFailed)
	}
	if ev.CleanupOK != wantOK || ev.CleanupFailed != wantFailed {
		t.Errorf("evidence cleanupOK = %v, cleanupFailed = %v, want %v/%v",
			ev.CleanupOK, ev.CleanupFailed, wantOK, wantFailed)
	}
}

// covers: SC-17
func TestLiveTruth_EveryCleanupFailureRetained(t *testing.T) {
	requirePython3(t)

	t.Run("a cleanup command that refused is retained though the checker after it held", func(t *testing.T) {
		subject := newSweepSubject(t, []string{"remove", "absent"})
		runID := newLiveTruthRun(t, "lt-sweep-refused")
		writeDirectives(t, runID, map[string]string{"remove": "fail"})

		tr, _, execOut := runLivePipeline(t, subject, runID, sweepPlan(runID), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if got := profileCleanupStep(t, ev, 1).Result; got != "fail" {
			t.Errorf("profile cleanup command 1 result = %q, want fail", got)
		}
		// The checker really did hold: it exited zero and named nothing left
		// standing, so nothing about the run itself is what withholds cleanupOK.
		checker := profileCleanupStep(t, ev, 2)
		if checker.Result != "pass" || checker.Decisive != "" {
			t.Errorf("profile cleanup command 2 = %+v, want a passing step that named no residue", checker)
		}
		requireAbsent(t, "after execute-live returned", phaseRunRoot(runID))
		requireCleanupState(t, execOut, ev, false, true)
		if got := rowVerdict(t, ev.RowVerdicts, sweepRow).Verdict; got != "LIVE-PASS" {
			t.Errorf("live verdict for %s = %q, want LIVE-PASS: a cleanup failure judges no row", sweepRow, got)
		}

		code, stdout, stderr := runVerifyOp(t, "finalize",
			liveFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, execOut.EvidencePath))
		if code != exitOK {
			t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		out := decodeFinalizeOutput(t, stdout)
		// PARTIAL, not BLOCKED: a failed cleanup is the human's to accept after
		// publication, and no row's verdict is touched by it.
		if out.Verdict != report.VerdictPartial {
			t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictPartial, out.Reasons)
		}
		requireReason(t, out.Reasons, "3", "", "live-cleanup-failed")

		rep := readPublishedReport(t, subject.repo, out.JSONPath)
		requireReason(t, rep.Reasons, "3", "", "live-cleanup-failed")
		if got := publishedRow(t, rep, sweepRow).Layer3; got != "LIVE-PASS" {
			t.Errorf("published layer3 for %s = %q, want LIVE-PASS", sweepRow, got)
		}
	})

	t.Run("a cleanup that left the run root standing fails through the removal predicate", func(t *testing.T) {
		subject := newSweepSubject(t, []string{"remove", "absent"})
		runID := newLiveTruthRun(t, "lt-sweep-residue")
		writeDirectives(t, runID, map[string]string{"remove": "wrong-name"})

		_, _, execOut := runLivePipeline(t, subject, runID, sweepPlan(runID), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		// Every cleanup command exited zero, so an exit status alone would have
		// called this run clean; what refuses it is the profile author's own
		// proof of removal, read off the checker's output.
		if got := profileCleanupStep(t, ev, 1).Result; got != "pass" {
			t.Errorf("profile cleanup command 1 result = %q, want pass: this variant's refusal is the residue, not an exit code", got)
		}
		checker := profileCleanupStep(t, ev, 2)
		if checker.Result != "fail" || checker.ExitStatus != 0 {
			t.Errorf("profile cleanup command 2 = %+v, want a step that exited zero and still failed its removal predicate", checker)
		}
		if got := checker.Decisive; !strings.Contains(got, "residue "+phaseRunRoot(runID)) {
			t.Errorf("profile cleanup command 2 decisive output = %q, want it to name the surviving run root %s", got, phaseRunRoot(runID))
		}
		if _, err := os.Stat(phaseRunRoot(runID)); err != nil {
			t.Errorf("stat %s: %v, want the run root this variant deliberately left standing", phaseRunRoot(runID), err)
		}
		requireCleanupState(t, execOut, ev, false, true)
	})

	t.Run("a cleanup command held past its own deadline leaves removal unproven rather than failed", func(t *testing.T) {
		subject := newSweepSubject(t, []string{"remove", "absent"})
		runID := newLiveTruthRun(t, "lt-sweep-held")
		writeDirectives(t, runID, map[string]string{"remove": "stall"})

		tr, _, execOut := runLivePipeline(t, subject, runID, sweepPlan(runID), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		// The held command has no reduced step to read a result off, and the
		// checker after it held, so no completed command failed. Neither
		// boolean may be set: the removal is unproven, not vindicated.
		requireExecutionError(t, ev.ExecutionErrors, "cleanup[1]:", "subprocess timeout")
		if got := profileCleanupStep(t, ev, 2).Result; got != "pass" {
			t.Errorf("profile cleanup command 2 result = %q, want pass", got)
		}
		requireCleanupState(t, execOut, ev, false, false)

		code, stdout, stderr := runVerifyOp(t, "finalize",
			liveFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, execOut.EvidencePath))
		if code != exitOK {
			t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		out := decodeFinalizeOutput(t, stdout)
		if out.Verdict != report.VerdictPartial {
			t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictPartial, out.Reasons)
		}
		requireReason(t, out.Reasons, "3", "", "live-cleanup-missing")
		requireReason(t, out.Reasons, "3", "", "live-execution-errors")

		rep := readPublishedReport(t, subject.repo, out.JSONPath)
		requireReason(t, rep.Reasons, "3", "", "live-cleanup-missing")
		requireReason(t, rep.Reasons, "3", "", "live-execution-errors")
	})

	// The two precedence orders, run in both directions: a known failure before
	// a non-completion, and a non-completion before a known failure. Neither
	// masks the other, and neither is cleared by a command that succeeded.
	t.Run("a refusal before a held command is retained", func(t *testing.T) {
		subject := newSweepSubject(t, []string{"remove", "pause", "absent"})
		runID := newLiveTruthRun(t, "lt-sweep-first")
		writeDirectives(t, runID, map[string]string{"remove": "fail", "pause": "stall"})

		_, _, execOut := runLivePipeline(t, subject, runID, sweepPlan(runID), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		if got := profileCleanupStep(t, ev, 1).Result; got != "fail" {
			t.Errorf("profile cleanup command 1 result = %q, want fail", got)
		}
		requireExecutionError(t, ev.ExecutionErrors, "cleanup[2]:", "subprocess timeout")
		if got := profileCleanupStep(t, ev, 3).Result; got != "pass" {
			t.Errorf("profile cleanup command 3 result = %q, want pass", got)
		}
		requireCleanupState(t, execOut, ev, false, true)
	})

	t.Run("a refusal after a held command is retained", func(t *testing.T) {
		subject := newSweepSubject(t, []string{"remove", "absent"})
		runID := newLiveTruthRun(t, "lt-sweep-last")
		writeDirectives(t, runID, map[string]string{"remove": "stall", "absent": "fail"})

		_, _, execOut := runLivePipeline(t, subject, runID, sweepPlan(runID), nil)
		ev := readLiveEvidence(t, execOut.EvidencePath)

		requireExecutionError(t, ev.ExecutionErrors, "cleanup[1]:", "subprocess timeout")
		if got := profileCleanupStep(t, ev, 2).Result; got != "fail" {
			t.Errorf("profile cleanup command 2 result = %q, want fail", got)
		}
		requireCleanupState(t, execOut, ev, false, true)
	})
}

// The bounded-output subject runs under limits tight enough to break its own
// walkthrough: a one-second deadline, sixty-four bytes of output shared by both
// of a command's streams, and a cleanup allowance of five seconds that belongs
// to cleanup alone. Two of its steps complete under those limits and settle
// their rows; the other four are cut short in each of the four ways a command
// can fail to leave a result behind, and what they must not do is let the
// output captured before the interruption stand in for a result.
const (
	incompleteCoverageSource = "docs/superpowers/coverage/2026-09-07-bounded-output-subject-coverage.md"
	incompleteProfilePath    = ".test-design/live-profiles/bounded-output-local.json"
	incompleteScriptPath     = "scripts/probe.py"

	// The limits the walkthrough below is written against. The output budget is
	// spent exactly by one step and exceeded by another; the deadline is
	// outlived by a third; cleanup takes longer than the deadline and finishes
	// anyway, which is what an allowance of its own means.
	incompleteTimeoutSeconds        = 1
	incompleteMaxOutputBytes        = 64
	incompleteCleanupTimeoutSeconds = 5
)

// incompleteScript is the subject's committed helper script, invoked by path
// under python3. Its mode is an argument rather than a directive file: a plan
// step names the mode it needs, and the run root the script touches is derived
// from the run identifier it is handed, so no path this test owns reaches the
// profile, the plan or the evidence.
const incompleteScript = `#!/usr/bin/env python3
"""The bounded-output live subject.

Each mode is one way a command can end: refusing with a status of its own,
spending the whole output budget and no more, outliving the deadline, burying a
success-shaped line past the budget, terminating the verifier and staying alive
to be terminated in turn, or saying nothing at all.
"""

import os
import shutil
import signal
import sys
import time

# Longer than any deadline this profile sets, so a command asked to outlive one
# is still running when the verifier gives up on it, and a command that has
# signalled the verifier is still running when the signal is observed.
STALL_SECONDS = 20

# The profile's output budget, shared by both streams. The exact-cap mode spends
# it once and exactly; the flooding mode spends it many times over.
CAP_BYTES = 64

# Longer than an ordinary command's deadline and well inside the cleanup
# allowance, so a cleanup that takes this long can only have run under an
# allowance of its own.
RELEASE_SECONDS = 1.2

# The status the refusing mode reports: a completed command whose nonzero exit
# is the result its predicate expects, not a failure of the run.
REFUSED_STATUS = 3


def run_root(run_id):
    """Return the run root this run owns."""
    return "/tmp/test-design-livetruth-" + run_id


def spend_budget_exactly():
    """Emit the whole budget and not one byte more, half on each stream."""
    half = CAP_BYTES // 2
    sys.stdout.write("ok".ljust(half, "."))
    sys.stdout.flush()
    sys.stderr.write("-" * half)
    sys.stderr.flush()


def flood():
    """Emit a line shaped like success, then bury it past the budget."""
    sys.stdout.write("ok\n")
    sys.stdout.flush()
    for _ in range(CAP_BYTES):
        sys.stdout.write("o" * CAP_BYTES)
        sys.stdout.flush()
        sys.stderr.write("e" * CAP_BYTES)
        sys.stderr.flush()


def main(argv):
    if len(argv) != 3:
        print("usage: probe.py <mode> <run-id>", file=sys.stderr)
        return 2
    mode, run_id = argv[1], argv[2]
    root = run_root(run_id)

    if mode == "prepare":
        os.makedirs(root, mode=0o700, exist_ok=True)
        return 0
    if mode == "refuse":
        return REFUSED_STATUS
    if mode == "cap":
        spend_budget_exactly()
        return 0
    if mode == "linger":
        time.sleep(STALL_SECONDS)
        return 0
    if mode == "flood":
        flood()
        return 0
    if mode == "signal":
        os.kill(os.getppid(), signal.SIGTERM)
        time.sleep(STALL_SECONDS)
        return 0
    if mode == "quiet":
        return 0
    if mode == "release":
        time.sleep(RELEASE_SECONDS)
        shutil.rmtree(root, ignore_errors=True)
        if os.path.exists(root):
            print("residue")
            return 1
        print("absent")
        return 0
    print("unknown mode " + mode, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
`

const incompleteGoMod = `module boundedlive

go 1.26
`

const incompleteLimitSource = `package limit

// Budget is the output allowance a live command is given: one budget its two
// streams spend together.
const Budget = 64

// Refused is the status the subject reports when it declines an operation. It
// is a result, not an interruption: a command that reports it ran to the end.
const Refused = 3

// WithinBudget reports whether the bytes a command emitted on both streams
// together stayed inside the allowance.
func WithinBudget(stdout, stderr int) bool { return stdout+stderr <= Budget }

// Result is what a live command left behind: a status only when it completed,
// and otherwise the reason it was cut short.
type Result struct {
	Completed bool
	Status    int
	Cause     string
}

// Judgeable reports whether an expectation may be judged against this result. A
// command that was cut short left no proof of anything, however much of its
// output was captured before it stopped.
func Judgeable(r Result) bool { return r.Completed }
`

// incompleteLimitTestSource is the subject's own tagged tests, one per row. The
// tags cite the subject document's rows, never a row of this repository: the
// text lives inside a string literal here and only becomes comments in the
// temporary repository the builder writes.
const incompleteLimitTestSource = `package limit

import "testing"

// covers: SC-01
func TestRefusedIsACompletedStatus(t *testing.T) {
	if !Judgeable(Result{Completed: true, Status: Refused}) {
		t.Fatalf("a completed refusal is not judgeable, want a command that ran to the end to be judged")
	}
	if Refused != 3 {
		t.Fatalf("Refused = %d, want 3", Refused)
	}
}

// covers: SC-02
func TestSpendingTheWholeBudgetStaysWithinIt(t *testing.T) {
	if !WithinBudget(32, 32) {
		t.Fatalf("WithinBudget(32, 32) = false, want the exact allowance to be inside it")
	}
}

// covers: SC-03
func TestADeadlineLeavesNothingToJudge(t *testing.T) {
	if Judgeable(Result{Cause: "deadline"}) {
		t.Fatalf("a command cut short by its deadline is judgeable, want no result to judge")
	}
}

// covers: SC-04
func TestOneByteBeyondTheBudgetIsOutsideIt(t *testing.T) {
	if WithinBudget(33, 32) {
		t.Fatalf("WithinBudget(33, 32) = true, want one byte past the allowance to be outside it")
	}
}

// covers: SC-05
func TestATerminationLeavesNothingToJudge(t *testing.T) {
	if Judgeable(Result{Cause: "terminated"}) {
		t.Fatalf("a terminated command is judgeable, want no result to judge")
	}
}

// covers: SC-06
func TestAFailedCaptureLeavesNothingToJudge(t *testing.T) {
	if Judgeable(Result{Cause: "capture"}) {
		t.Fatalf("a command whose output could not be captured is judgeable, want no result to judge")
	}
}
`

// incompleteCoverageDoc journeys all six rows, so every one of them is a
// layer-3 obligation the plan must bind and the run must settle.
// BASELINE_COMMIT and BASELINE_SHA256 are pinned by the shared subject builder.
const incompleteCoverageDoc = `# Coverage: bounded output live subject

Spec: docs/superpowers/specs/2026-09-07-bounded-output-live-subject.md
Baseline: BASELINE_COMMIT · 6 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-bounded-output-subject-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/bounded-output-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-bounded-output-subject-coverage.md"]

## Live decision

- decision: required
- reason: only a running deployment can be interrupted, and only a real interruption shows what its captured output is worth

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01, SC-02, SC-03, SC-04, SC-05, SC-06 | A command that ran to the end is judged by what it reported, and a command that was cut short is judged by nothing at all |

## Scenarios

### Behavior: judging a command by whether it finished

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | a command that refuses with a status of its own ran to the end | the refusal status is the result its expectation is judged against | integration | the refusal status is reported as an interruption | low |
| SC-02 | a command spends the whole output allowance and stops | the command completes and its output is judged | integration | the allowance is treated as exceeded when it is spent exactly | med |
| SC-03 | a command outlives its deadline | the command leaves no result to judge | integration | the deadline is reported as an ordinary completion | med |
| SC-04 | a command emits more than its output allowance | the command leaves no result to judge, whatever it emitted first | integration | the captured prefix is judged as if the command had finished | med |
| SC-05 | a command terminates the verifier and stays alive | the command leaves no result to judge | integration | the termination is reported as an ordinary completion | med |
| SC-06 | a command's output cannot be captured at all | the command leaves no result to judge | integration | the capture failure is reported as an ordinary completion | med |
`

// incompleteProfileJSON is the subject's committed profile. Its limits are the
// point of the subject, so they are spelled from the constants the walkthrough
// is written against rather than left to the defaults.
func incompleteProfileJSON() string {
	return fmt.Sprintf(`{
  "schemaVersion": %q,
  "classification": "local",
  "start": [["python3", "scripts/probe.py", "prepare", "{run-id}"]],
  "health": [],
  "walkthrough": [["python3", "scripts/probe.py", "quiet", "{run-id}"]],
  "cleanup": [["python3", "scripts/probe.py", "release", "{run-id}"]],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "matches", "stream": "stdout", "value": "absent"}
  ],
  "envAllowlist": ["PATH"],
  "resourcePrefixes": ["/tmp/test-design-livetruth-{run-id}"],
  "endpoints": [],
  "timeoutSeconds": %d,
  "cleanupTimeoutSeconds": %d,
  "maxOutputBytes": %d
}
`, evidence.SchemaVersion, incompleteTimeoutSeconds, incompleteCleanupTimeoutSeconds, incompleteMaxOutputBytes)
}

// newIncompleteSubject builds the bounded-output subject and runs the real
// baseline op over it.
func newIncompleteSubject(t *testing.T) *liveSubject {
	t.Helper()
	return newLiveSubject(t, incompleteCoverageSource, incompleteProfilePath, []map[string]string{{
		"go.mod":                 incompleteGoMod,
		"limit/limit.go":         incompleteLimitSource,
		"limit/limit_test.go":    incompleteLimitTestSource,
		incompleteScriptPath:     incompleteScript,
		incompleteProfilePath:    incompleteProfileJSON(),
		incompleteCoverageSource: incompleteCoverageDoc,
	}}, nil)
}

// incompleteStep is one step of the bounded-output walkthrough: the mode the
// helper script runs in, the row it binds, the predicate that would settle that
// row, and the verdict the row must carry once the run is over.
type incompleteStep struct {
	mode        string
	row         string
	expectation string
	predicate   map[string]string
	verdict     string
}

// incompleteWalkthrough is the walkthrough in plan order, so a step's position
// here is its index in the evidence and in the run directory. Step 6 is last
// because its raw capture directory is made unwritable before the run starts,
// and the directory is named after the step's own position.
func incompleteWalkthrough() []incompleteStep {
	return []incompleteStep{
		{
			mode:        "refuse",
			row:         "SC-01",
			expectation: "the command that refuses reports the status its caller expects",
			predicate:   map[string]string{"form": "exit-code", "value": "3"},
			verdict:     "LIVE-PASS",
		},
		{
			mode:        "cap",
			row:         "SC-02",
			expectation: "the command that spends the whole output allowance still finishes",
			predicate:   map[string]string{"form": "matches", "stream": "stdout", "value": "ok"},
			verdict:     "LIVE-PASS",
		},
		{
			// An exit status no completed run could report either, which
			// is the point: even an expectation nothing could satisfy may
			// not be settled against a command that produced no result to
			// compare it with.
			mode:        "linger",
			row:         "SC-03",
			expectation: "the command that outlives its deadline reports a status",
			predicate:   map[string]string{"form": "exit-code", "value": "-1"},
			verdict:     "LIVE-INCONCLUSIVE",
		},
		{
			mode:        "flood",
			row:         "SC-04",
			expectation: "the command that floods its streams still reports success first",
			predicate:   map[string]string{"form": "matches", "stream": "stdout", "value": "ok"},
			verdict:     "LIVE-INCONCLUSIVE",
		},
		{
			mode:        "signal",
			row:         "SC-05",
			expectation: "the command that terminates the verifier reports no error of its own",
			predicate:   map[string]string{"form": "not-matches", "stream": "combined", "value": "ERROR"},
			verdict:     "LIVE-INCONCLUSIVE",
		},
		{
			mode:        "quiet",
			row:         "SC-06",
			expectation: "the command whose output cannot be captured reports success",
			predicate:   map[string]string{"form": "exit-code", "value": "0"},
			verdict:     "LIVE-INCONCLUSIVE",
		},
	}
}

// incompletePlan is the plan the live-runner would hand to execute-live for
// this subject: one step per row, each bound to its own, so no row inherits
// another's verdict and each interruption is answered for on its own.
func incompletePlan(runID string) map[string]any {
	walkthrough := incompleteWalkthrough()
	steps := make([]map[string]any, 0, len(walkthrough))
	for _, step := range walkthrough {
		steps = append(steps, map[string]any{
			"expectation": step.expectation,
			"command":     []string{"python3", incompleteScriptPath, step.mode, runID},
			"predicates":  []map[string]string{step.predicate},
			"scBindings":  []string{step.row},
		})
	}
	return map[string]any{"schemaVersion": evidence.SchemaVersion, "steps": steps}
}

// blockStepCapture makes the raw-capture directory of one step unwritable
// before the run starts, which is the only way to reach the capture-error path
// from outside the verifier: the files are created before any process is, so a
// directory that refuses them stops the step before it begins. The mode is
// restored in cleanup so the temporary root the test framework owns can still
// be removed.
func blockStepCapture(t *testing.T, index int) func(prepareLiveOutput) {
	return func(prep prepareLiveOutput) {
		dir := filepath.Join(prep.RunDir, fmt.Sprintf("step-%d", index))
		if err := os.Mkdir(dir, 0o500); err != nil {
			t.Fatalf("pre-create the unwritable capture directory %s: %v", dir, err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Logf("restore %s: %v", dir, err)
			}
		})
	}
}

// covers: SC-18
func TestLiveTruth_IncompleteOutputIsNotProof(t *testing.T) {
	requirePython3(t)
	subject := newIncompleteSubject(t)
	runID := newLiveTruthRun(t, "lt-incomplete")
	walkthrough := incompleteWalkthrough()

	tr, _, execOut := runLivePipeline(t, subject, runID, incompletePlan(runID), blockStepCapture(t, len(walkthrough)))
	ev := readLiveEvidence(t, execOut.EvidencePath)

	for i, step := range walkthrough {
		if got := rowVerdict(t, ev.RowVerdicts, step.row).Verdict; got != step.verdict {
			t.Errorf("live verdict for %s (step %d, mode %s) = %q, want %q", step.row, i+1, step.mode, got, step.verdict)
		}
	}

	// Only the two commands that ran to the end left a reduced step behind. A
	// third would mean an interrupted command had been reduced to a result.
	if len(ev.Steps) != 2 {
		t.Fatalf("walkthrough steps = %+v, want only the two commands that completed", ev.Steps)
	}
	for _, step := range ev.Steps {
		if step.Result != "pass" {
			t.Errorf("reduced step %+v, want the two completed commands to pass", step)
		}
	}
	if got := ev.Steps[0].ExitStatus; got != 3 {
		t.Errorf("the refusing step exited %d, want its own status 3 to be the result its predicate judged", got)
	}
	if got := ev.Steps[1].Decisive; !strings.Contains(got, "ok") {
		t.Errorf("the exact-allowance step's decisive output = %q, want the match its predicate found", got)
	}

	// Each interruption is named for what it was, at the position it happened.
	requireExecutionError(t, ev.ExecutionErrors, "step[3]:", "subprocess timeout")
	requireExecutionError(t, ev.ExecutionErrors, "step[4]:", "live output limit exceeded")
	requireExecutionError(t, ev.ExecutionErrors, "step[5]:", "subprocess interrupted")
	requireExecutionError(t, ev.ExecutionErrors, "step[6]:", "create raw capture")

	// Cleanup takes longer than an ordinary command's whole deadline and still
	// finishes: the allowance it runs under is its own, not the walkthrough's.
	requireAbsent(t, "after execute-live returned", phaseRunRoot(runID))
	requireCleanupState(t, execOut, ev, true, false)

	code, stdout, stderr := runVerifyOp(t, "finalize",
		liveFinalizeRequestJSON(t, subject, runID, tr.EvidencePath, execOut.EvidencePath))
	if code != exitOK {
		t.Fatalf("finalize exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictBlocked {
		t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictBlocked, out.Reasons)
	}
	rep := readPublishedReport(t, subject.repo, out.JSONPath)
	for _, step := range walkthrough {
		if step.verdict == "LIVE-INCONCLUSIVE" {
			requireReason(t, out.Reasons, "3", step.row, "LIVE-INCONCLUSIVE")
			requireReason(t, rep.Reasons, "3", step.row, "LIVE-INCONCLUSIVE")
		}
		if got := publishedRow(t, rep, step.row).Layer3; got != step.verdict {
			t.Errorf("published layer3 for %s = %q, want %q", step.row, got, step.verdict)
		}
	}
	requireReason(t, out.Reasons, "3", "", "live-execution-errors")
	requireReason(t, rep.Reasons, "3", "", "live-execution-errors")
}
