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
	"time"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
	"test-design/internal/jvmrun"
	"test-design/internal/mutation"
	"test-design/internal/runrecord"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

type evaluateMutationRequest struct {
	verifyRequest
	RunDir   string   `json:"runDir"`
	Site     string   `json:"site"`
	LastSite bool     `json:"lastSite"`
	Sites    []string `json:"sites,omitempty"`
}

type diffSummaryOutput struct {
	File  string `json:"file"`
	Hunks int    `json:"hunks"`
}

type evaluateMutationOutput struct {
	evidence.Envelope
	RunID             string                 `json:"runId"`
	RowID             string                 `json:"rowID"`
	RepoCommit        string                 `json:"repoCommit"`
	Source            string                 `json:"source"`
	SourceSHA256      string                 `json:"sourceSha256"`
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

type mutationDiscardEvidence struct {
	Site       string `json:"site"`
	Reason     string `json:"reason"`
	DiffPath   string `json:"diffPath"`
	DiffSHA256 string `json:"diffSha256"`
}

type mutationEvidence struct {
	evidence.Envelope
	RunID           string                    `json:"runId"`
	RowID           string                    `json:"rowID"`
	RepoCommit      string                    `json:"repoCommit"`
	Source          string                    `json:"source"`
	SourceSHA256    string                    `json:"sourceSha256"`
	Identity        identityOutput            `json:"identity"`
	RunnerKind      string                    `json:"runnerKind"`
	SuiteCommand    []string                  `json:"suiteCommand"`
	TargetCommand   []string                  `json:"targetCommand"`
	Sites           []mutationSiteEvidence    `json:"sites"`
	SnapshotEqual   bool                      `json:"snapshotEqual"`
	EnumeratedSites []string                  `json:"enumeratedSites,omitempty"`
	Discarded       []mutationDiscardEvidence `json:"discarded,omitempty"`
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
		RunDir   string   `json:"runDir"`
		Site     string   `json:"site"`
		LastSite bool     `json:"lastSite"`
		Sites    []string `json:"sites"`
	}
	if err := json.Unmarshal(requestBody, &extra); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}

	if cliErr := validateRunDir(op, extra.RunDir); cliErr != nil {
		return nil, cliErr
	}

	if extra.Site == "" {
		return nil, &cliError{Code: "missing-site", Message: fmt.Sprintf("%s: site is required", op)}
	}
	if strings.ContainsAny(extra.Site, `/\`) {
		return nil, &cliError{Code: "invalid-site", Message: fmt.Sprintf("%s: site %q must not contain a path separator", op, extra.Site)}
	}

	return &evaluateMutationRequest{verifyRequest: *base, RunDir: extra.RunDir, Site: extra.Site, LastSite: extra.LastSite, Sites: extra.Sites}, nil
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
	head, cliErr := headCommit("evaluate-mutation", req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	if st.RunID != req.RunID || st.RepoCommit != head || st.Source != src.Path || st.SourceSHA256 != src.SHA256 {
		return &cliError{Code: "stale-evidence", Message: "evaluate-mutation: prepared state does not match request run/source/HEAD; prepare again"}
	}
	if cliErr := resolveRowSites("evaluate-mutation", st, req.Sites, req.Site); cliErr != nil {
		return cliErr
	}
	// Nothing is measured on a workspace of unknown cleanliness: the marker an
	// interrupted evaluation left behind is cleared by recover-mutation, never
	// by measuring over it.
	if st.Started != nil {
		return &cliError{
			Code: "site-interrupted",
			Message: fmt.Sprintf("evaluate-mutation: site %q was interrupted during its %s run; recover-mutation must restore %s before another site is measured",
				st.Started.Site, st.Started.Phase, st.WorktreePath),
		}
	}
	for _, so := range st.Sites {
		if so.Site == req.Site {
			return &cliError{
				Code:    "duplicate-site",
				Message: fmt.Sprintf("evaluate-mutation: site %q is already recorded in %s", req.Site, req.RunDir),
			}
		}
	}

	store, cliErr := openLedger("evaluate-mutation", &req.verifyRequest)
	if cliErr != nil {
		return cliErr
	}
	ledger, cliErr := loadLedgerFor("evaluate-mutation", runstate.KindMutationRow, store)
	if cliErr != nil {
		return cliErr
	}
	// The row must be the one prepare-mutation opened and nothing has answered
	// yet: a row with no record carries no obligation this measurement could
	// satisfy, and one already answered would have its verdict overwritten.
	switch current := runstate.Current(ledger, runstate.KindMutationRow, st.RowID); {
	case current == nil:
		return &cliError{
			Code:    "mutation-row-not-started",
			Message: fmt.Sprintf("evaluate-mutation: %s was never prepared for this run", st.RowID),
		}
	case current.Status == runstate.StatusCompleted:
		return completedRecordRefusal("evaluate-mutation", runstate.KindMutationRow, st.RowID)
	}
	if cliErr := checkSitesRemaining("evaluate-mutation", st, req.Site, req.LastSite); cliErr != nil {
		return cliErr
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

	// markSite names the site about to run before the run starts, so a process
	// killed inside it leaves the workspace's uncertainty on disk instead of a
	// state document that reads as if nothing had happened.
	markSite := func(phase string) *cliError {
		st.Started = &mutation.SiteAttempt{
			Site:      req.Site,
			Phase:     phase,
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if err := mutation.WriteState(req.RunDir, st); err != nil {
			return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
		}
		return nil
	}

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
	if cliErr := markSite("mutated"); cliErr != nil {
		return restoreOnFail(cliErr)
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
	if cliErr := markSite("clean"); cliErr != nil {
		return restoreOnFail(cliErr)
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
	// The site is finished, so the marker goes out in the same write that
	// records it: the workspace is clean and the state says so in one step.
	st.Started = nil
	st.Sites = append(st.Sites, so)
	if err := mutation.WriteState(req.RunDir, st); err != nil {
		return &cliError{Code: "state-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	out := evaluateMutationOutput{
		Envelope:          evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:             st.RunID,
		RowID:             st.RowID,
		RepoCommit:        st.RepoCommit,
		Source:            st.Source,
		SourceSHA256:      st.SourceSHA256,
		Site:              req.Site,
		Outcome:           outcome,
		Diff:              diffSummaryOutput{File: summary.File, Hunks: summary.Hunks},
		MutatedRunDir:     mutatedDir,
		MutatedExitStatus: mutatedExit,
		CleanRunDir:       cleanDir,
		CleanExitStatus:   cleanExit,
	}
	if !mutation.RowComplete(st.Enumerated, evaluatedSiteLabels(st.Sites)) {
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
		Envelope:        out.Envelope,
		RunID:           st.RunID,
		RepoCommit:      st.RepoCommit,
		Source:          st.Source,
		SourceSHA256:    st.SourceSHA256,
		RowID:           st.RowID,
		Identity:        identityOutput{Scope: st.Identity.Scope, Name: st.Identity.Name},
		RunnerKind:      src.Runner.Kind,
		SuiteCommand:    redactCommand(src.Runner.Command),
		TargetCommand:   redactCommand(target.Command),
		Sites:           siteRecords,
		SnapshotEqual:   true,
		EnumeratedSites: st.Enumerated,
		Discarded:       discardEvidenceRecords(st.Discarded),
	}
	if err := evidence.WriteJSON(evidencePath, &ev); err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("evaluate-mutation: %v", err)}
	}

	// The row is answered only once its evidence exists and the repository has
	// been proved untouched. One site that was not caught answers the whole
	// row: a guard the mutant walked past is not made sound by the sites that
	// did catch one.
	rowOutcome := "caught"
	for _, site := range st.Sites {
		if site.Outcome != "caught" {
			rowOutcome = "survived"
			break
		}
	}
	if _, err := store.Complete(ledgerIdentity(&req.verifyRequest, req.RepoRoot, head, src),
		runstate.KindMutationRow, st.RowID, rowOutcome,
		map[string]string{artifactEvidence: evidencePath}, ""); err != nil {
		cliErr := ledgerError("evaluate-mutation", runstate.KindMutationRow, err)
		cliErr.Message = fmt.Sprintf("%s; the evidence stands at %s", cliErr.Message, evidencePath)
		return cliErr
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
		// Before selection: a non-compiling mutant yields zero records too, but
		// that's not zero-selection.
		if len(sr.report.BuildFailed) > 0 {
			return "", sr.exitStatus, buildFailedError(op, sr.report)
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

// buildFailedDetailLimit caps the quoted compiler output, runes, so one bad
// build dump cannot flood the refusal message.
const buildFailedDetailLimit = 2000

func buildFailedError(op string, rep *gotestjson.Report) *cliError {
	return &cliError{
		Code:    "mutant-build-failed",
		Message: fmt.Sprintf("%s: mutant did not compile: %s", op, buildFailedDetail(rep)),
	}
}

func buildFailedDetail(rep *gotestjson.Report) string {
	failed := make(map[string]bool, len(rep.BuildFailed))
	for _, pkg := range rep.BuildFailed {
		failed[pkg] = true
	}
	var parts []string
	for _, f := range rep.Failures {
		if failed[f.Package] {
			parts = append(parts, strings.TrimSpace(f.Detail))
		}
	}
	detail := strings.Join(parts, "\n")
	if runes := []rune(detail); len(runes) > buildFailedDetailLimit {
		detail = string(runes[:buildFailedDetailLimit]) + "...(truncated)"
	}
	return detail
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

func discardEvidenceRecords(discards []mutation.DiscardRecord) []mutationDiscardEvidence {
	out := make([]mutationDiscardEvidence, 0, len(discards))
	for _, d := range discards {
		out = append(out, mutationDiscardEvidence{
			Site:       d.Site,
			Reason:     d.Reason,
			DiffPath:   d.DiffPath,
			DiffSHA256: d.DiffSHA256,
		})
	}
	return out
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
