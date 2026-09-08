package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"test-design/internal/evidence"
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

// runRecipe invokes the harness recipe from dir the way the profile does, by
// path under python3. A recipe that could not be started at all reports exit
// code -1 with the reason on the stderr result, so a caller inside t.Cleanup
// can log it rather than fail from a cleanup function.
func runRecipe(t *testing.T, dir, action, runID string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("python3", filepath.FromSlash(redesignRecipePath), action, runID)
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

// registerRunCleanup gives the recipe the last word on a run whatever the
// assertions did. A run root left under /tmp keeps a service listening and
// makes every later deploy under the same id exit 1, so this has to run after
// a failed assertion as much as after a passing one. A root that is already
// gone is a cleanup pass, so the exit code is only worth logging.
func registerRunCleanup(t *testing.T, s *liveSubject, runID string) {
	t.Cleanup(func() {
		if code, stdout, stderr := runRecipe(t, s.repo, "cleanup", runID); code != 0 {
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
		registerRunCleanup(t, subject, runID)
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
		registerRunCleanup(t, subject, runID)
		root := redesignRunRoot(runID)

		// The prior revision prices three items at 300 cents. Deployed under
		// the run's own root from its own checkout, it is exactly the healthy
		// older deployment the start phase must refuse to run beside.
		checkout := detachedCheckout(t, subject, subject.priorCommits[0])
		if code, stdout, stderr := runRecipe(t, checkout, "deploy", runID); code != 0 {
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
	registerRunCleanup(t, subject, runA)
	registerRunCleanup(t, subject, runB)

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
	if code, stdout, stderr := runRecipe(t, checkoutB, "deploy", runB); code != 0 {
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
	if code, stdout, stderr := runRecipe(t, subject.repo, "ready", runB); code != 0 {
		t.Errorf("ready %s exited %d after run %s cleaned up, want the neighbour still serving; stdout=%s stderr=%s",
			runB, code, runA, stdout, stderr)
	}
	requireSentinelIntact(t, keep)

	if code, stdout, stderr := runRecipe(t, subject.repo, "cleanup", runB); code != 0 {
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
