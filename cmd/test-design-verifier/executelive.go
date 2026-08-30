package main

import (
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
	"test-design/internal/worktree"
)

const liveEvidenceFile = "live-evidence.json"

const (
	verdictPass         = "LIVE-PASS"
	verdictFail         = "LIVE-FAIL"
	verdictInconclusive = "LIVE-INCONCLUSIVE"
	verdictLayer1Gap    = "LIVE-N/A (layer 1 gap)"
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
}

type liveRawArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type liveEvidence struct {
	evidence.Envelope
	RunID           string             `json:"runId"`
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
	RawArtifacts    []liveRawArtifact  `json:"rawArtifacts"`
	StartedAt       string             `json:"startedAt"`
	FinishedAt      string             `json:"finishedAt"`
	// stamped only after the compare; blocked run leaves evidence behind and
	// finalize must not read it as a good run
	SnapshotEqual bool `json:"snapshotEqual"`
}

type executeLiveOutput struct {
	evidence.Envelope
	RunID         string           `json:"runID"`
	EvidencePath  string           `json:"evidencePath"`
	RowVerdicts   []liveRowVerdict `json:"rowVerdicts"`
	CleanupOK     bool             `json:"cleanupOK"`
	CleanupFailed bool             `json:"cleanupFailed"`
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

// executeLiveOp is the execute-live op body. between runs after the worktree is
// disposed of and before the final Compare; it is nil in production and exists
// so a test can mutate the original repository at exactly the moment the op is
// blind to it.
func executeLiveOp(req *executeLiveRequest, stdout io.Writer, between func()) *cliError {
	src, cliErr := loadVerifiedSource("execute-live", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
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

	gaps, cliErr := layer1GapRows(req.TraceEvidence)
	if cliErr != nil {
		return cliErr
	}
	applicable := applicableLiveRows(src)

	planData, err := os.ReadFile(req.PlanPath)
	if err != nil {
		return &cliError{Code: "plan-unreadable", Message: fmt.Sprintf("execute-live: read plan: %v", err)}
	}
	pl, err := live.DecodePlan(planData)
	if err != nil {
		return &cliError{Code: "plan-decode-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	if err := pl.Validate(&descriptor, applicable, gaps); err != nil {
		return &cliError{Code: planRefusalCode(err), Message: fmt.Sprintf("execute-live: %v", err)}
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}
	wt, err := ctrl.Attach(st.WorktreePath)
	if err != nil {
		return &cliError{Code: "worktree-attach-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}

	run := newLiveRun(req.RunDir, wt.Path, &descriptor)
	run.execute(pl)

	ev := liveEvidence{
		Envelope:        evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:           st.RunID,
		Source:          src.Path,
		SourceSHA256:    src.SHA256,
		ProfilePath:     st.ProfilePath,
		RowVerdicts:     run.rowVerdicts(applicable, gaps),
		StartSteps:      run.startSteps,
		HealthSteps:     run.healthSteps,
		Steps:           run.steps,
		CleanupSteps:    run.cleanupSteps,
		CleanupOK:       run.cleanupOK,
		CleanupFailed:   !run.cleanupOK,
		ExecutionErrors: run.executionErrors,
		RawArtifacts:    run.rawArtifacts,
		StartedAt:       run.startedAt,
		FinishedAt:      run.finishedAt,
	}
	evidencePath := filepath.Join(req.RunDir, liveEvidenceFile)
	if err := evidence.WriteJSON(evidencePath, &ev); err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("execute-live: %v", err)}
	}

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

	out := executeLiveOutput{
		Envelope:      ev.Envelope,
		RunID:         ev.RunID,
		EvidencePath:  evidencePath,
		RowVerdicts:   ev.RowVerdicts,
		CleanupOK:     ev.CleanupOK,
		CleanupFailed: ev.CleanupFailed,
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

// layer1GapRows reads the trace run's evidence: a row named by a gap or
// ambiguity finding has no honest layer-1 result, so layer 3 reports it as
// LIVE-N/A rather than running anything for it.
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

func applicableLiveRows(src *coverage.Source) []coverage.Row {
	retired := make(map[string]bool, len(src.Retired))
	for _, id := range src.Retired {
		retired[id] = true
	}
	rows := make([]coverage.Row, 0, len(src.Rows))
	for _, row := range src.Rows {
		if retired[row.ID] {
			continue
		}
		if row.Level == "smoke" || row.Level == "e2e" {
			rows = append(rows, row)
		}
	}
	return rows
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

	verdicts map[string]string
	barriers map[string]bool

	startFailed  bool
	healthFailed bool
	cleanupOK    bool
	startedAt    string
	finishedAt   string
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
		rawArtifacts:    []liveRawArtifact{},
		executionErrors: []string{},
		verdicts:        map[string]string{},
		barriers:        map[string]bool{},
	}
}

func (r *liveRun) execute(pl *live.Plan) {
	r.startedAt = time.Now().UTC().Format(time.RFC3339Nano)
	defer func() { r.finishedAt = time.Now().UTC().Format(time.RFC3339Nano) }()

	for i, argv := range r.descriptor.Start {
		step := live.Step{Expectation: fmt.Sprintf("profile start command %d", i+1), Command: argv}
		res, reduced, _ := r.exec("start", i+1, step)
		r.startSteps = appendReduced(r.startSteps, reduced)
		if res == nil || res.ExitStatus != 0 {
			r.startFailed = true
			break
		}
	}

	if !r.startFailed {
		for i, argv := range r.descriptor.Health {
			step := live.Step{Expectation: fmt.Sprintf("profile health command %d", i+1), Command: argv}
			res, reduced, _ := r.exec("health", i+1, step)
			r.healthSteps = appendReduced(r.healthSteps, reduced)
			if res == nil || res.ExitStatus != 0 {
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

// runPlanStep executes one plan step, merging its outcome into every row it
// binds only when applyVerdicts: a cleanup step of a walkthrough that never
// ran proves nothing about its rows.
func (r *liveRun) runPlanStep(phase string, index int, step live.Step, applyVerdicts bool) *live.ReducedStep {
	res, reduced, barrier := r.exec(phase, index, step)
	if !applyVerdicts {
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

// runProfileCleanup runs the descriptor's cleanup commands and evaluates
// cleanupEvidence against the last one, so cleanupOK is the profile author's
// own proof that the run left nothing behind.
func (r *liveRun) runProfileCleanup() {
	last := len(r.descriptor.Cleanup) - 1
	for i, argv := range r.descriptor.Cleanup {
		step := live.Step{Expectation: fmt.Sprintf("profile cleanup command %d", i+1), Command: argv}
		if i == last {
			step.Predicates = r.descriptor.CleanupEvidence
		}
		res, reduced, _ := r.exec("cleanup", i+1, step)
		r.cleanupSteps = appendReduced(r.cleanupSteps, reduced)
		if i == last && res != nil {
			r.cleanupOK, _ = live.EvaluatePredicates(r.descriptor.CleanupEvidence, res)
		}
	}
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

	res, err := live.ExecuteStep(step, r.env, r.workDir, dir)
	if err != nil {
		r.recordError(phase, index, err)
		return nil, nil, false
	}
	r.recordRaw(res)

	reduced, err = live.ReduceTranscript(step, res, r.parentEnv, r.descriptor.EnvAllowlist)
	if err != nil {
		r.recordError(phase, index, err)
		return res, nil, true
	}
	return res, reduced, false
}

func (r *liveRun) recordRaw(res *live.StepResult) {
	for _, path := range []string{res.StdoutPath, res.StderrPath} {
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
// order. A row no step ever reached is LIVE-INCONCLUSIVE, never absent.
func (r *liveRun) rowVerdicts(applicable []coverage.Row, gaps map[string]bool) []liveRowVerdict {
	out := make([]liveRowVerdict, 0, len(applicable))
	for _, row := range applicable {
		if gaps[row.ID] {
			out = append(out, liveRowVerdict{RowID: row.ID, Verdict: verdictLayer1Gap})
			continue
		}
		verdict, ok := r.verdicts[row.ID]
		if !ok {
			verdict = verdictInconclusive
		}
		out = append(out, liveRowVerdict{RowID: row.ID, Verdict: verdict, RedactionBarrier: r.barriers[row.ID]})
	}
	return out
}

func appendReduced(steps []live.ReducedStep, reduced *live.ReducedStep) []live.ReducedStep {
	if reduced == nil {
		return steps
	}
	return append(steps, *reduced)
}
