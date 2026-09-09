package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/live"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

const liveEvidenceFile = "live-evidence.json"

const (
	verdictPass         = "LIVE-PASS"
	verdictFail         = "LIVE-FAIL"
	verdictInconclusive = "LIVE-INCONCLUSIVE"
)

type executeLiveRequest struct {
	verifyRequest
	RunDir        string
	PlanPath      string
	TraceEvidence string
}

type liveRowVerdict struct {
	RowID            string `json:"rowId"`
	Verdict          string `json:"verdict"`
	RedactionBarrier bool   `json:"redactionBarrier,omitempty"`
	// the trace evidence reported this row as a gap or an ambiguity; the row
	// still carries its own live verdict beside the annotation
	Layer1Gap bool `json:"layer1Gap,omitempty"`
}

// liveStepFailure names one command that completed, failed its predicates and
// whose outcome reached no row verdict: a start or health command, a plan step
// with no scBindings, or a plan-cleanup step with no scBindings or whose
// verdict application was suppressed because the walkthrough never ran. Profile
// cleanup commands are never listed here; cleanupOK and cleanupFailed carry
// them. Expectation passes the same redaction barrier as a transcript.
type liveStepFailure struct {
	Phase       string `json:"phase"`
	Index       int    `json:"index"`
	Expectation string `json:"expectation"`
}

type liveRawArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// liveRecovery is the record of an attempt that found a prior attempt for this
// run: the profile's own cleanup commands, run as recovery before anything is
// started, and whether they proved removal. It is absent from a first attempt,
// which has nothing to recover.
type liveRecovery struct {
	Performed bool               `json:"performed"`
	OK        bool               `json:"ok"`
	Steps     []live.ReducedStep `json:"steps"`
}

type liveEvidence struct {
	evidence.Envelope
	RunID           string             `json:"runId"`
	RepoCommit      string             `json:"repoCommit"`
	Source          string             `json:"source"`
	SourceSHA256    string             `json:"sourceSha256"`
	ProfilePath     string             `json:"profilePath"`
	RowVerdicts     []liveRowVerdict   `json:"rowVerdicts"`
	StartSteps      []live.ReducedStep `json:"startSteps"`
	HealthSteps     []live.ReducedStep `json:"healthSteps"`
	Steps           []live.ReducedStep `json:"steps"`
	CleanupSteps    []live.ReducedStep `json:"cleanupSteps"`
	CleanupOK       bool               `json:"cleanupOK"`
	CleanupFailed   bool               `json:"cleanupFailed"`
	ExecutionErrors []string           `json:"executionErrors"`
	UnboundFailures []liveStepFailure  `json:"unboundFailures"`
	RawArtifacts    []liveRawArtifact  `json:"rawArtifacts"`
	StartedAt       string             `json:"startedAt"`
	FinishedAt      string             `json:"finishedAt"`
	// stamped only after the compare; blocked run leaves evidence behind and
	// finalize must not read it as a good run
	SnapshotEqual bool          `json:"snapshotEqual"`
	Recovery      *liveRecovery `json:"recovery,omitempty"`
}

type executeLiveOutput struct {
	evidence.Envelope
	RunID           string            `json:"runID"`
	EvidencePath    string            `json:"evidencePath"`
	RowVerdicts     []liveRowVerdict  `json:"rowVerdicts"`
	CleanupOK       bool              `json:"cleanupOK"`
	CleanupFailed   bool              `json:"cleanupFailed"`
	UnboundFailures []liveStepFailure `json:"unboundFailures"`
	Recovery        *liveRecovery     `json:"recovery,omitempty"`
}

func decodeExecuteLiveRequest(op string, requestBody []byte) (*executeLiveRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}

	var extra struct {
		RunDir        string `json:"runDir"`
		Plan          string `json:"plan"`
		TraceEvidence string `json:"traceEvidence"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}

	if extra.RunDir == "" {
		return nil, &cliError{Code: "missing-run-dir", Message: fmt.Sprintf("%s: runDir is required", op)}
	}
	if !filepath.IsAbs(extra.RunDir) {
		return nil, &cliError{Code: "run-dir-not-absolute", Message: fmt.Sprintf("%s: runDir %q must be an absolute path", op, extra.RunDir)}
	}
	if info, err := os.Stat(extra.RunDir); err != nil || !info.IsDir() {
		return nil, &cliError{Code: "run-dir-not-found", Message: fmt.Sprintf("%s: runDir %q does not exist", op, extra.RunDir)}
	}
	if extra.Plan == "" {
		return nil, &cliError{Code: "missing-plan", Message: fmt.Sprintf("%s: plan is required", op)}
	}
	if !filepath.IsAbs(extra.Plan) {
		return nil, &cliError{Code: "plan-not-absolute", Message: fmt.Sprintf("%s: plan %q must be an absolute path", op, extra.Plan)}
	}
	if extra.TraceEvidence == "" {
		return nil, &cliError{Code: "missing-trace-evidence", Message: fmt.Sprintf("%s: traceEvidence is required", op)}
	}
	if !filepath.IsAbs(extra.TraceEvidence) {
		return nil, &cliError{
			Code:    "trace-evidence-not-absolute",
			Message: fmt.Sprintf("%s: traceEvidence %q must be an absolute path", op, extra.TraceEvidence),
		}
	}

	return &executeLiveRequest{
		verifyRequest: *base,
		RunDir:        extra.RunDir,
		PlanPath:      extra.Plan,
		TraceEvidence: extra.TraceEvidence,
	}, nil
}

func executeLiveHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeExecuteLiveRequest("execute-live", requestBody)
	if cliErr != nil {
		return cliErr
	}
	return executeLiveOp(req, stdout, nil)
}

// executeLiveOp is the execute-live op body. The between hook runs after the
// worktree is disposed of and before the final Compare; it is nil in
// production and exists so a test can mutate the original repository at
// exactly the moment the op is blind to it.
func executeLiveOp(req *executeLiveRequest, stdout io.Writer, between func()) (opErr *cliError) {
	src, cliErr := loadVerifiedSource("execute-live", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	if cliErr := requireLiveDecision("execute-live", src); cliErr != nil {
		return cliErr
	}

	st, err := live.LoadState(req.RunDir)
	if err != nil {
		return &cliError{Code: "state-load-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	var descriptor live.Profile
	if err := json.Unmarshal(st.Descriptor, &descriptor); err != nil {
		return &cliError{Code: "descriptor-invalid", Message: fmt.Sprintf("execute-live: decode descriptor: %v", err)}
	}
	if err := descriptor.ValidateResolved(st.RunID); err != nil {
		return &cliError{Code: "descriptor-invalid", Message: fmt.Sprintf("execute-live: %v", err)}
	}

	gaps, cliErr := layer1GapRows(req.TraceEvidence)
	if cliErr != nil {
		return cliErr
	}
	applicable := live.ApplicableRows(src)

	planData, err := os.ReadFile(req.PlanPath)
	if err != nil {
		return &cliError{Code: "plan-unreadable", Message: fmt.Sprintf("execute-live: read plan: %v", err)}
	}
	pl, err := live.DecodePlan(planData)
	if err != nil {
		return &cliError{Code: "plan-decode-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	if err := pl.Validate(&descriptor, applicable); err != nil {
		return &cliError{Code: planRefusalCode(err), Message: fmt.Sprintf("execute-live: %v", err)}
	}

	// The attempt is opened once the plan is known to be runnable and before
	// any environment is: a run whose live layer is already answered is refused
	// here, where nothing has been started to recover from. The run identity is
	// the prepared state's, not the request's — the run directory names which
	// run this attempt belongs to.
	const kind = runstate.KindLiveAttempt
	ledgerReq := req.verifyRequest
	ledgerReq.RunID = st.RunID
	repoCommit, cliErr := headCommit("execute-live", req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	store, cliErr := openLedger("execute-live", &ledgerReq)
	if cliErr != nil {
		return cliErr
	}
	ledger, cliErr := loadLedgerFor("execute-live", kind, store)
	if cliErr != nil {
		return cliErr
	}
	prev := runstate.Current(ledger, kind, liveAttemptID)
	if prev != nil && prev.Status == runstate.StatusCompleted && repeatReason(&ledgerReq) == "" {
		return completedRecordRefusal("execute-live", kind, liveAttemptID)
	}
	identity := ledgerIdentity(&ledgerReq, req.RepoRoot, repoCommit, src)
	if _, err := store.Start(identity, kind, liveAttemptID, liveAttemptInputs{
		RunDir:       req.RunDir,
		WorktreePath: st.WorktreePath,
		ProfilePath:  st.ProfilePath,
		PlanSHA256:   sha256Bytes(planData),
	}, "", repeatReason(&ledgerReq)); err != nil {
		return ledgerError("execute-live", kind, err)
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	wt, err := ctrl.Attach(st.WorktreePath)
	if err != nil {
		return &cliError{Code: "worktree-attach-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	disposed := false
	defer func() {
		if !disposed {
			if err := ctrl.Dispose(wt); err != nil {
				if opErr == nil {
					opErr = &cliError{Code: "worktree-dispose-failed", Message: fmt.Sprintf("execute-live: %v", err)}
				} else {
					opErr.Message += fmt.Sprintf("; dispose worktree: %v", err)
				}
			}
		}
	}()

	run := newLiveRun(req.RunDir, wt.Path, &descriptor)
	// Any prior record — started, or completed and now repeated — means this
	// run already owned live resources once.
	run.recover = prev != nil
	run.execute(pl)

	ev := liveEvidence{
		Envelope:        evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:           st.RunID,
		RepoCommit:      st.RepoCommit,
		Source:          src.Path,
		SourceSHA256:    src.SHA256,
		ProfilePath:     st.ProfilePath,
		RowVerdicts:     run.rowVerdicts(applicable, gaps),
		StartSteps:      run.startSteps,
		HealthSteps:     run.healthSteps,
		Steps:           run.steps,
		CleanupSteps:    run.cleanupSteps,
		CleanupOK:       run.cleanupOK,
		CleanupFailed:   run.cleanupFailed,
		ExecutionErrors: run.executionErrors,
		UnboundFailures: run.unboundFailures,
		RawArtifacts:    run.rawArtifacts,
		StartedAt:       run.startedAt,
		FinishedAt:      run.finishedAt,
		Recovery:        run.recovery(),
	}
	evidencePath := filepath.Join(req.RunDir, liveEvidenceFile)
	if err := evidence.WriteJSON(evidencePath, &ev); err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}

	disposed = true
	if err := ctrl.Dispose(wt); err != nil {
		return &cliError{Code: "worktree-dispose-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}

	if between != nil {
		between()
	}

	var before worktree.Snapshot
	if err := json.Unmarshal(st.Snapshot, &before); err != nil {
		return &cliError{Code: "state-load-failed", Message: fmt.Sprintf("execute-live: decode snapshot: %v", err)}
	}
	after, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	if equal, differing := worktree.Compare(&before, after); !equal {
		return &cliError{
			Code: "snapshot-mismatch",
			Message: fmt.Sprintf("execute-live: original repository changed during verification: %s; commit or discard the concurrent change, then rerun",
				strings.Join(differing, ", ")),
		}
	}

	ev.SnapshotEqual = true
	if err := evidence.WriteJSON(evidencePath, &ev); err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("execute-live: stamp snapshot equality: %v", err)}
	}

	// The attempt is closed only once the repository has been proved untouched:
	// an attempt that changed its own subject is not an answer, and leaving the
	// record started is what tells a resumed run to recover before repeating.
	if _, err := store.Complete(identity, kind, liveAttemptID, liveAttemptOutcome(&ev),
		map[string]string{artifactEvidence: evidencePath}, ""); err != nil {
		cliErr := ledgerError("execute-live", kind, err)
		cliErr.Message = fmt.Sprintf("%s; the evidence stands at %s", cliErr.Message, evidencePath)
		return cliErr
	}

	out := executeLiveOutput{
		Envelope:        ev.Envelope,
		RunID:           ev.RunID,
		EvidencePath:    evidencePath,
		RowVerdicts:     ev.RowVerdicts,
		CleanupOK:       ev.CleanupOK,
		CleanupFailed:   ev.CleanupFailed,
		UnboundFailures: ev.UnboundFailures,
		Recovery:        ev.Recovery,
	}
	return writeStdout(stdout, out)
}

// planRefusalCode lifts Plan.Validate's named refusal out of its message, so
// the op's error code is the rule the plan broke.
func planRefusalCode(err error) string {
	rest, ok := strings.CutPrefix(err.Error(), "live plan: ")
	if !ok {
		return "plan-invalid"
	}
	token, _, _ := strings.Cut(rest, " ")
	if token == "" {
		return "plan-invalid"
	}
	return token
}

// liveAttemptOutcome is the one word the run's ledger keeps for this attempt.
// A row that failed outranks everything: the layer has an answer and it is a
// bad one. Short of that, anything the attempt could not prove — an unjudged
// row, an unproven removal, a failure no row carries, a command that never
// completed — leaves the attempt inconclusive rather than passing.
func liveAttemptOutcome(ev *liveEvidence) string {
	inconclusive := false
	for _, v := range ev.RowVerdicts {
		switch v.Verdict {
		case verdictFail:
			return "fail"
		case verdictInconclusive:
			inconclusive = true
		}
	}
	if inconclusive || !ev.CleanupOK || len(ev.UnboundFailures) > 0 || len(ev.ExecutionErrors) > 0 {
		return "inconclusive"
	}
	return "pass"
}

// sha256Bytes digests the plan document as the op received it, which is what
// the attempt's input identity pins: a repeated attempt against a rewritten
// plan is a different attempt, and the record says so.
func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// layer1GapRows reads the trace run's evidence: a row named by a gap or
// ambiguity finding has no honest layer-1 result. Layer 3 still plans, runs and
// judges that row; the finding is recorded beside its verdict as layer1Gap, and
// blocking the branch stays layer 1's own business.
func layer1GapRows(path string) (map[string]bool, *cliError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &cliError{Code: "trace-evidence-unreadable", Message: fmt.Sprintf("execute-live: read trace evidence: %v", err)}
	}
	if _, err := evidence.DecodeEnvelope(data); err != nil {
		return nil, &cliError{Code: "trace-evidence-invalid", Message: fmt.Sprintf("execute-live: %s: %v", path, err)}
	}
	var doc struct {
		Findings []traceFinding `json:"findings"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, &cliError{Code: "trace-evidence-invalid", Message: fmt.Sprintf("execute-live: %s: %v", path, err)}
	}

	gaps := map[string]bool{}
	for _, f := range doc.Findings {
		if f.RowID == "" {
			continue
		}
		if f.Kind == "gap" || f.Kind == "ambiguity" {
			gaps[f.RowID] = true
		}
	}
	return gaps, nil
}

// liveRun executes one walkthrough under a prepared descriptor. No process
// outcome fails the op: a start, health, step or cleanup failure is a recorded
// condition, and finalize is what turns it into a non-PASS verdict.
type liveRun struct {
	runDir     string
	workDir    string
	descriptor *live.Profile
	parentEnv  []string
	env        []string

	startSteps      []live.ReducedStep
	healthSteps     []live.ReducedStep
	steps           []live.ReducedStep
	cleanupSteps    []live.ReducedStep
	rawArtifacts    []liveRawArtifact
	executionErrors []string

	unboundFailures []liveStepFailure

	verdicts map[string]string
	barriers map[string]bool

	startFailed   bool
	healthFailed  bool
	cleanupOK     bool
	cleanupFailed bool
	startedAt     string
	finishedAt    string

	// recover is set when this run's ledger already held an attempt for the
	// run, in any state; recoveryFailed and recoverySteps are what the
	// recovery it then performs left behind.
	recover        bool
	recoveryFailed bool
	recoverySteps  []live.ReducedStep
}

func newLiveRun(runDir, workDir string, descriptor *live.Profile) *liveRun {
	parentEnv := os.Environ()
	return &liveRun{
		runDir:          runDir,
		workDir:         workDir,
		descriptor:      descriptor,
		parentEnv:       parentEnv,
		env:             live.SanitizedEnv(parentEnv, descriptor.EnvAllowlist),
		startSteps:      []live.ReducedStep{},
		healthSteps:     []live.ReducedStep{},
		steps:           []live.ReducedStep{},
		cleanupSteps:    []live.ReducedStep{},
		unboundFailures: []liveStepFailure{},
		rawArtifacts:    []liveRawArtifact{},
		executionErrors: []string{},
		verdicts:        map[string]string{},
		barriers:        map[string]bool{},
	}
}

// recoveryExecutionError is the one entry a failed recovery adds to
// executionErrors. The per-command causes are recorded beside it; this names
// the decision the run made because of them.
const recoveryExecutionError = "recovery: owned cleanup did not prove removal before a fresh start"

func (r *liveRun) execute(pl *live.Plan) {
	r.startedAt = time.Now().UTC().Format(time.RFC3339Nano)
	defer func() { r.finishedAt = time.Now().UTC().Format(time.RFC3339Nano) }()

	// A run that follows an earlier attempt for the same run ID runs the
	// profile's own cleanup as recovery first: the resources the interrupted
	// attempt owns are exactly the ones those commands remove, and a fresh
	// start over a half-live environment would judge rows against something
	// nobody designed. Unless the recovery proves removal the attempt stops
	// here, with nothing started and every row left unjudged.
	if r.recover {
		steps, completedAll, failed := r.runCleanupCommands("recovery")
		r.recoverySteps = steps
		if !r.provedRemoval(completedAll, failed) {
			r.recoveryFailed = true
			r.cleanupFailed = failed
			r.cleanupOK = false
			r.executionErrors = append(r.executionErrors, recoveryExecutionError)
			return
		}
	}

	for i, argv := range r.descriptor.Start {
		step := live.Step{
			Expectation: fmt.Sprintf("profile start command %d", i+1),
			Command:     argv,
			Predicates:  exitZeroPredicates(),
		}
		reduced, ok := r.runPhaseCommand("start", i+1, step)
		r.startSteps = appendReduced(r.startSteps, reduced)
		if !ok {
			r.startFailed = true
			break
		}
	}

	if !r.startFailed {
		for i, argv := range r.descriptor.Health {
			step := live.Step{
				Expectation: fmt.Sprintf("profile health command %d", i+1),
				Command:     argv,
				Predicates:  exitZeroPredicates(),
			}
			reduced, ok := r.runPhaseCommand("health", i+1, step)
			r.healthSteps = appendReduced(r.healthSteps, reduced)
			if !ok {
				r.healthFailed = true
				break
			}
		}
	}

	walkthroughRan := !r.startFailed && !r.healthFailed
	if walkthroughRan {
		for i, step := range pl.Steps {
			r.steps = appendReduced(r.steps, r.runPlanStep("step", i+1, step, true))
		}
	}

	// Cleanup runs on every path once start began. Its own steps only carry
	// verdicts when the walkthrough they belong to actually ran.
	for i, step := range pl.Cleanup {
		r.cleanupSteps = appendReduced(r.cleanupSteps, r.runPlanStep("plan-cleanup", i+1, step, walkthroughRan))
	}
	r.runProfileCleanup()
}

// exitZeroPredicates is the expectation every profile-declared command carries
// implicitly. A start, health or cleanup command has no plan predicates of its
// own, and its nonzero exit is a failure rather than the pass a predicate-free
// reduction would record.
func exitZeroPredicates() []live.Predicate {
	return []live.Predicate{{Form: "exit-code", Value: "0"}}
}

// runPhaseCommand executes one start or health command and reports whether the
// phase may continue. Readiness never yields a row verdict, so a command that
// completed and failed its expectation is recorded as an unbound failure; one
// that never completed leaves its execution error and nothing else.
func (r *liveRun) runPhaseCommand(phase string, index int, step live.Step) (*live.ReducedStep, bool) {
	res, reduced, _ := r.exec(phase, index, step)
	if res == nil {
		return reduced, false
	}
	if pass, _ := live.EvaluatePredicates(step.Predicates, res); !pass {
		r.recordUnboundFailure(phase, index, step.Expectation)
		return reduced, false
	}
	return reduced, true
}

// runPlanStep executes one plan step, merging its outcome into every row it
// binds only when applyVerdicts: a cleanup step of a walkthrough that never
// ran proves nothing about its rows. A step no row verdict can carry — no
// bindings, or bindings whose application is suppressed — keeps its failure
// visible through unboundFailures instead.
func (r *liveRun) runPlanStep(phase string, index int, step live.Step, applyVerdicts bool) *live.ReducedStep {
	res, reduced, barrier := r.exec(phase, index, step)
	if !applyVerdicts || len(step.SCBindings) == 0 {
		if res != nil {
			if pass, _ := live.EvaluatePredicates(step.Predicates, res); !pass {
				r.recordUnboundFailure(phase, index, step.Expectation)
			}
		}
		return reduced
	}

	verdict := verdictInconclusive
	if res != nil {
		if pass, _ := live.EvaluatePredicates(step.Predicates, res); pass {
			verdict = verdictPass
		} else {
			verdict = verdictFail
		}
	}
	for _, id := range step.SCBindings {
		r.mergeVerdict(id, verdict)
		if barrier {
			r.barriers[id] = true
		}
	}
	return reduced
}

// runCleanupCommands runs every cleanup command the descriptor declares under
// phase — "cleanup" for the end-of-run removal, "recovery" for the same
// commands run first by a restarted attempt. Each carries the implicit
// exit-code 0 and the configured last one carries cleanupEvidence as well, so
// what the caller reads back is the profile author's own proof that nothing was
// left behind. completedAll is false when any command never completed and
// failed is true when any that did failed its predicates; the caller turns that
// pair into its own tri-state.
func (r *liveRun) runCleanupCommands(phase string) (steps []live.ReducedStep, completedAll, failed bool) {
	steps = []live.ReducedStep{}
	completedAll = true
	last := len(r.descriptor.Cleanup) - 1
	for i, argv := range r.descriptor.Cleanup {
		step := live.Step{
			Expectation: fmt.Sprintf("profile %s command %d", phase, i+1),
			Command:     argv,
			Predicates:  exitZeroPredicates(),
		}
		if i == last {
			// cleanupEvidence leads, so the decisive output stays the removal
			// proof the profile author chose rather than the implicit exit.
			step.Predicates = append(append([]live.Predicate{}, r.descriptor.CleanupEvidence...), step.Predicates...)
		}
		res, reduced, _ := r.exec(phase, i+1, step)
		steps = appendReduced(steps, reduced)
		if res == nil {
			completedAll = false
			continue
		}
		if pass, _ := live.EvaluatePredicates(step.Predicates, res); !pass {
			failed = true
		}
	}
	return steps, completedAll, failed
}

// provedRemoval is the one rule both cleanup phases are judged by: every
// declared command completed with exit 0, the configured last one also met
// cleanupEvidence, and the profile declared a cleanup at all — one that
// declares none proves no removal either.
func (r *liveRun) provedRemoval(completedAll, failed bool) bool {
	return completedAll && !failed && len(r.descriptor.Cleanup) > 0
}

// runProfileCleanup runs the end-of-run cleanup and records its tri-state: a
// known failure of any command outranks a command that never completed, which
// in turn keeps cleanupOK from claiming a clean run. Neither set means the
// removal is simply unproven, and the cause is in executionErrors.
func (r *liveRun) runProfileCleanup() {
	steps, completedAll, failed := r.runCleanupCommands("cleanup")
	r.cleanupSteps = append(r.cleanupSteps, steps...)
	r.cleanupFailed = failed
	r.cleanupOK = r.provedRemoval(completedAll, failed)
}

// withheldExpectation stands in for planner text the redaction barrier refuses.
const withheldExpectation = "[withheld by redaction barrier]"

// recordUnboundFailure lists a completed failure that reached no row verdict.
// The expectation is planner text copied into the evidence, so it passes the
// same barrier a transcript does; the failure itself is a fact independent of
// the text and is listed either way. A hit here can only happen on a step exec
// already recorded an error against: the same barrier, over the same parent
// environment, refused that step's transcript before it could be reduced.
func (r *liveRun) recordUnboundFailure(phase string, index int, expectation string) {
	if live.RedactionBarrierHit(expectation, r.parentEnv, r.descriptor.EnvAllowlist) {
		expectation = withheldExpectation
	}
	r.unboundFailures = append(r.unboundFailures, liveStepFailure{
		Phase:       phase,
		Index:       index,
		Expectation: expectation,
	})
}

// exec runs one command in the worktree with the sanitized environment. res is
// nil when the process never ran; reduced is nil both then and when the
// redaction barrier withheld the transcript, which barrier distinguishes.
func (r *liveRun) exec(phase string, index int, step live.Step) (res *live.StepResult, reduced *live.ReducedStep, barrier bool) {
	dir := filepath.Join(r.runDir, fmt.Sprintf("%s-%d", phase, index))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.recordError(phase, index, err)
		return nil, nil, false
	}

	// Recovery is the profile's cleanup under another name; it gets the same
	// independent deadline the end-of-run cleanup does.
	cleanup := phase == "cleanup" || phase == "plan-cleanup" || phase == "recovery"
	res, err := live.ExecuteStep(step, r.env, r.workDir, dir, r.descriptor.Limits(cleanup))
	if res != nil {
		r.recordRaw(res)
	}
	if err != nil {
		r.recordError(phase, index, err)
		return nil, nil, false
	}

	reduced, err = live.ReduceTranscript(step, res, r.parentEnv, r.descriptor.EnvAllowlist)
	if err != nil {
		r.recordError(phase, index, err)
		return res, nil, true
	}
	return res, reduced, false
}

func (r *liveRun) recordRaw(res *live.StepResult) {
	for _, path := range []string{res.StdoutPath, res.StderrPath, res.CombinedPath} {
		sum, err := evidence.SHA256File(path)
		if err != nil {
			r.executionErrors = append(r.executionErrors, fmt.Sprintf("hash %s: %v", path, err))
			continue
		}
		r.rawArtifacts = append(r.rawArtifacts, liveRawArtifact{Path: path, SHA256: sum})
	}
}

func (r *liveRun) recordError(phase string, index int, err error) {
	r.executionErrors = append(r.executionErrors, fmt.Sprintf("%s[%d]: %v", phase, index, err))
}

var verdictRank = map[string]int{verdictPass: 1, verdictInconclusive: 2, verdictFail: 3}

func (r *liveRun) mergeVerdict(rowID, verdict string) {
	if current, ok := r.verdicts[rowID]; ok && verdictRank[current] >= verdictRank[verdict] {
		return
	}
	r.verdicts[rowID] = verdict
}

// rowVerdicts yields exactly one entry per applicable row, in coverage-source
// order. A row no step ever reached is LIVE-INCONCLUSIVE, never absent, and a
// row layer 1 reported as a gap earns the same real verdict as any other: the
// gap is annotation, not a substitute for what the run observed.
func (r *liveRun) rowVerdicts(applicable []coverage.Row, gaps map[string]bool) []liveRowVerdict {
	out := make([]liveRowVerdict, 0, len(applicable))
	for _, row := range applicable {
		verdict, ok := r.verdicts[row.ID]
		if !ok {
			verdict = verdictInconclusive
		}
		out = append(out, liveRowVerdict{
			RowID:            row.ID,
			Verdict:          verdict,
			RedactionBarrier: r.barriers[row.ID],
			Layer1Gap:        gaps[row.ID],
		})
	}
	return out
}

// recovery is the run's recovery record, nil when the run had no prior attempt
// to recover from and so performed none.
func (r *liveRun) recovery() *liveRecovery {
	if !r.recover {
		return nil
	}
	return &liveRecovery{Performed: true, OK: !r.recoveryFailed, Steps: r.recoverySteps}
}

func appendReduced(steps []live.ReducedStep, reduced *live.ReducedStep) []live.ReducedStep {
	if reduced == nil {
		return steps
	}
	return append(steps, *reduced)
}
