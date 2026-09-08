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
