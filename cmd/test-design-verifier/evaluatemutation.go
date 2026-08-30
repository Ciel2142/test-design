package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/jvmrun"
	"test-design/internal/mutation"
	"test-design/internal/runrecord"
	"test-design/internal/worktree"
)

type evaluateMutationRequest struct {
	verifyRequest
	RunDir   string `json:"runDir"`
	Site     string `json:"site"`
	LastSite bool   `json:"lastSite"`
}

type diffSummaryOutput struct {
	File  string `json:"file"`
	Hunks int    `json:"hunks"`
}

type evaluateMutationOutput struct {
	evidence.Envelope
	Site              string                 `json:"site"`
	Outcome           string                 `json:"outcome"`
	Diff              diffSummaryOutput      `json:"diff"`
	MutatedRunDir     string                 `json:"mutatedRunDir"`
	MutatedExitStatus int                    `json:"mutatedExitStatus"`
	CleanRunDir       string                 `json:"cleanRunDir"`
	CleanExitStatus   int                    `json:"cleanExitStatus"`
	Sites             []mutation.SiteOutcome `json:"sites,omitempty"`
	EvidencePath      string                 `json:"evidencePath,omitempty"`
}

type mutationSiteEvidence struct {
	Site              string `json:"site"`
	Outcome           string `json:"outcome"`
	DiffPath          string `json:"diffPath"`
	DiffSHA256        string `json:"diffSha256"`
	MutatedRunDir     string `json:"mutatedRunDir"`
	MutatedExitStatus int    `json:"mutatedExitStatus"`
	CleanRunDir       string `json:"cleanRunDir"`
	CleanExitStatus   int    `json:"cleanExitStatus"`
}

type mutationEvidence struct {
	evidence.Envelope
	RowID         string                 `json:"rowID"`
	Identity      identityOutput         `json:"identity"`
	RunnerKind    string                 `json:"runnerKind"`
	SuiteCommand  []string               `json:"suiteCommand"`
	TargetCommand []string               `json:"targetCommand"`
	Sites         []mutationSiteEvidence `json:"sites"`
	SnapshotEqual bool                   `json:"snapshotEqual"`
}

func decodeEvaluateMutationRequest(op string, requestBody []byte) (*evaluateMutationRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	if base.RunID == "" {
		return nil, &cliError{Code: "missing-run-id", Message: fmt.Sprintf("%s: runID is required", op)}
	}

	var extra struct {
		RunDir   string `json:"runDir"`
		Site     string `json:"site"`
		LastSite bool   `json:"lastSite"`
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

	if extra.Site == "" {
		return nil, &cliError{Code: "missing-site", Message: fmt.Sprintf("%s: site is required", op)}
	}
	if strings.ContainsAny(extra.Site, `/\`) {
		return nil, &cliError{Code: "invalid-site", Message: fmt.Sprintf("%s: site %q must not contain a path separator", op, extra.Site)}
	}

	return &evaluateMutationRequest{verifyRequest: *base, RunDir: extra.RunDir, Site: extra.Site, LastSite: extra.LastSite}, nil
}

func evaluateMutationHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeEvaluateMutationRequest("evaluate-mutation", requestBody)
	if cliErr != nil {
		return cliErr
	}
	return evaluateMutationOp(req, stdout, nil)
}

// evaluateMutationOp is the evaluate-mutation op body. between runs after the
// last site's worktree disposal and before the final snapshot re-check; it
// is nil in production and exists so a test can mutate the original
// repository at exactly the moment the op is blind to it.
func evaluateMutationOp(req *evaluateMutationRequest, stdout io.Writer, between func()) *cliError {
	src, cliErr := loadVerifiedSource("evaluate-mutation", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	st, err := mutation.LoadState(req.RunDir)
	if err != nil {
		return &cliError{Code: "state-load-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}
	for _, so := range st.Sites {
		if so.Site == req.Site {
			return &cliError{
				Code:    "duplicate-site",
				Message: fmt.Sprintf("evaluate-mutation: site %q is already recorded in %s", req.Site, req.RunDir),
			}
		}
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}
	wt, err := ctrl.Attach(st.WorktreePath)
	if err != nil {
		return &cliError{Code: "worktree-attach-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	target, err := buildMutationTarget(src, st.Identity)
	if err != nil {
		return &cliError{Code: "target-build-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	restoreOnFail := func(cliErr *cliError) *cliError {
		if restoreErr := ctrl.RestoreTracked(wt); restoreErr != nil {
			cliErr.Message = fmt.Sprintf("%s; best-effort restore also failed: %v", cliErr.Message, restoreErr)
		}
		return cliErr
	}

	siteIndex := len(st.Sites) + 1

	diff, err := ctrl.Diff(wt)
	if err != nil {
		return restoreOnFail(&cliError{Code: "diff-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)})
	}
	summary, err := mutation.PoliceDiff(diff, src.Runner.Kind)
	if err != nil {
		return restoreOnFail(&cliError{Code: policeDiffCode(err), Message: fmt.Sprintf("evaluate-mutation: %v", err)})
	}
	diffPath := filepath.Join(req.RunDir, fmt.Sprintf("site-%d.diff", siteIndex))
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		return restoreOnFail(&cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("evaluate-mutation: write diff: %v", err)})
	}

	suiteDir := filepath.Join(wt.Path, filepath.FromSlash(src.Runner.Root))

	mutatedDir := filepath.Join(req.RunDir, fmt.Sprintf("site-%d-mutated", siteIndex))
	if err := os.MkdirAll(mutatedDir, 0o755); err != nil {
		return restoreOnFail(&cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)})
	}
	outcome, mutatedExit, cliErr := runTargetedAndVerify("evaluate-mutation", src, suiteDir, wt.Path, mutatedDir, target, st.Identity)
	if cliErr != nil {
		return restoreOnFail(cliErr)
	}

	if err := ctrl.RestoreTracked(wt); err != nil {
		return &cliError{Code: "restore-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	cleanDir := filepath.Join(req.RunDir, fmt.Sprintf("site-%d-clean", siteIndex))
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return restoreOnFail(&cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)})
	}
	cleanOutcome, cleanExit, cleanErr := runTargetedAndVerify("evaluate-mutation", src, suiteDir, wt.Path, cleanDir, target, st.Identity)
	if cleanErr != nil || cleanOutcome != "survived" {
		detail := "clean baseline unrepeatable, row outcomes void"
		switch {
		case cleanErr != nil:
			detail = fmt.Sprintf("%s: %v", detail, cleanErr)
		default:
			detail = fmt.Sprintf("%s: targeted family did not fully pass on the clean rerun", detail)
		}
		return restoreOnFail(&cliError{Code: "clean-run-failed", Message: fmt.Sprintf("evaluate-mutation: %s", detail)})
	}

	so := mutation.SiteOutcome{
		Site:              req.Site,
		Outcome:           outcome,
		DiffPath:          diffPath,
		MutatedRunDir:     mutatedDir,
		MutatedExitStatus: mutatedExit,
		CleanRunDir:       cleanDir,
		CleanExitStatus:   cleanExit,
	}
	st.Sites = append(st.Sites, so)
	if err := mutation.WriteState(req.RunDir, st); err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	out := evaluateMutationOutput{
		Envelope:          evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		Site:              req.Site,
		Outcome:           outcome,
		Diff:              diffSummaryOutput{File: summary.File, Hunks: summary.Hunks},
		MutatedRunDir:     mutatedDir,
		MutatedExitStatus: mutatedExit,
		CleanRunDir:       cleanDir,
		CleanExitStatus:   cleanExit,
	}
	if !req.LastSite {
		return writeStdout(stdout, out)
	}

	if err := ctrl.Dispose(wt); err != nil {
		return &cliError{Code: "cleanup-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	if between != nil {
		between()
	}

	var before worktree.Snapshot
	if err := json.Unmarshal(st.Snapshot, &before); err != nil {
		return &cliError{Code: "state-load-failed", Message: fmt.Sprintf("evaluate-mutation: decode snapshot: %v", err)}
	}
	after, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}
	if equal, differing := worktree.Compare(&before, after); !equal {
		return &cliError{
			Code: "snapshot-mismatch",
			Message: fmt.Sprintf("evaluate-mutation: original repository changed during verification: %s; commit or discard the concurrent change, then rerun",
				strings.Join(differing, ", ")),
		}
	}

	siteRecords, err := siteEvidenceRecords(st.Sites)
	if err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}
	evidencePath := filepath.Join(req.RunDir, "mutation-evidence.json")
	ev := mutationEvidence{
		Envelope:      out.Envelope,
		RowID:         st.RowID,
		Identity:      identityOutput{Scope: st.Identity.Scope, Name: st.Identity.Name},
		RunnerKind:    src.Runner.Kind,
		SuiteCommand:  redactCommand(src.Runner.Command),
		TargetCommand: redactCommand(target.Command),
		Sites:         siteRecords,
		SnapshotEqual: true,
	}
	if err := evidence.WriteJSON(evidencePath, &ev); err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	out.Sites = st.Sites
	out.EvidencePath = evidencePath
	return writeStdout(stdout, out)
}

func buildMutationTarget(src *coverage.Source, id mutation.Identity) (mutation.Target, error) {
	if src.Runner.Kind == "go" {
		return mutation.GoTarget(src.Runner, id)
	}
	return mutation.JVMTarget(src.Runner, id)
}

// runTargetedAndVerify runs target's command in dir (a fresh, call-specific
// subdirectory), verifies the run selected exactly id's family, and derives
// the caught/survived outcome. cliErr is non-nil, and outcome empty, on any
// selection-verification failure or a fully-skipped family.
func runTargetedAndVerify(op string, src *coverage.Source, suiteDir, wtPath, dir string, target mutation.Target, id mutation.Identity) (outcome string, exitStatus int, cliErr *cliError) {
	if src.Runner.Kind == "go" {
		sr, cliErr := runSuite(op, suiteDir, dir, target.Command)
		if cliErr != nil {
			return "", 0, cliErr
		}
		if sr.normalizationError != "" {
			return "", sr.exitStatus, &cliError{Code: "normalization-failed", Message: fmt.Sprintf("%s: %v", op, sr.normalizationError)}
		}
		if err := mutation.VerifyGoSelection(sr.report, id); err != nil {
			return "", sr.exitStatus, &cliError{Code: mapSelectionErr(err), Message: fmt.Sprintf("%s: %v", op, err)}
		}
		if allSkipped(sr.report.Records) {
			return "", sr.exitStatus, zeroSelectionError(op, id)
		}
		return goOutcome(sr), sr.exitStatus, nil
	}

	cfg := coverage.RunnerConfig{Kind: src.Runner.Kind, Root: src.Runner.Root, Command: target.Command, ReportGlobs: target.ReportGlobs}
	res, err := jvmrun.Run(wtPath, dir, cfg)
	if err != nil {
		return "", 0, &cliError{Code: "runner-start-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if err := mutation.VerifyJVMSelection(res, id); err != nil {
		return "", res.ExitStatus, &cliError{Code: mapSelectionErr(err), Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if allSkipped(res.Records) {
		return "", res.ExitStatus, zeroSelectionError(op, id)
	}
	jvmOutcome := recordsOutcome(res.Records)
	if jvmOutcome == "survived" && res.ExitStatus != 0 {
		return "", res.ExitStatus, &cliError{
			Code: "unexplained-runner-failure",
			Message: fmt.Sprintf("%s: runner exited %d with every %s.%s record passing; an auxiliary task failure still prevents a mutation verdict",
				op, res.ExitStatus, id.Scope, id.Name),
		}
	}
	return jvmOutcome, res.ExitStatus, nil
}

func zeroSelectionError(op string, id mutation.Identity) *cliError {
	return &cliError{
		Code: "zero-selection",
		Message: fmt.Sprintf("%s: %v: %s.%s ran but every record was skipped; skips prove nothing executed the guard",
			op, mutation.ErrZeroSelection, id.Scope, id.Name),
	}
}

func goOutcome(sr *suiteRun) string {
	if len(sr.report.Failures) > 0 {
		return "caught"
	}
	return recordsOutcome(sr.report.Records)
}

func recordsOutcome(records []runrecord.Record) string {
	for _, r := range records {
		if r.Status == runrecord.Failed {
			return "caught"
		}
	}
	return "survived"
}

func allSkipped(records []runrecord.Record) bool {
	if len(records) == 0 {
		return false
	}
	for _, r := range records {
		if r.Status != runrecord.Skipped {
			return false
		}
	}
	return true
}

func mapSelectionErr(err error) string {
	switch {
	case errors.Is(err, mutation.ErrZeroSelection):
		return "zero-selection"
	case errors.Is(err, mutation.ErrForeignIdentity):
		return "foreign-identity"
	case errors.Is(err, mutation.ErrUnsupportedIdentity):
		return "unsupported-test-identity"
	default:
		return "normalization-failed"
	}
}

func policeDiffCode(err error) string {
	switch {
	case errors.Is(err, mutation.ErrEmptyDiff):
		return "empty-diff"
	case errors.Is(err, mutation.ErrMultiSite):
		return "multi-site-diff"
	case errors.Is(err, mutation.ErrForeignDiff):
		return "foreign-diff"
	default:
		return "diff-police-failed"
	}
}

func siteEvidenceRecords(sites []mutation.SiteOutcome) ([]mutationSiteEvidence, error) {
	out := make([]mutationSiteEvidence, 0, len(sites))
	for _, s := range sites {
		sum, err := evidence.SHA256File(s.DiffPath)
		if err != nil {
			return nil, err
		}
		out = append(out, mutationSiteEvidence{
			Site:              s.Site,
			Outcome:           s.Outcome,
			DiffPath:          s.DiffPath,
			DiffSHA256:        sum,
			MutatedRunDir:     s.MutatedRunDir,
			MutatedExitStatus: s.MutatedExitStatus,
			CleanRunDir:       s.CleanRunDir,
			CleanExitStatus:   s.CleanExitStatus,
		})
	}
	return out, nil
}
