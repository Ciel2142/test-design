package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
	"test-design/internal/trace"
	"test-design/internal/worktree"
)

const traceEvidenceFile = "trace-evidence.json"

type traceRowResult struct {
	RowID  string `json:"rowId"`
	Scope  string `json:"scope"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type traceFinding struct {
	Kind   string `json:"kind"`
	RowID  string `json:"rowId"`
	Detail string `json:"detail"`
}

type traceTagClass struct {
	ID    string `json:"id"`
	Class string `json:"class"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

type traceNewFailure struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

type rowAccounting struct {
	ActiveRows  int      `json:"activeRows"`
	WithResult  int      `json:"withResult"`
	Unaccounted []string `json:"unaccounted"`
}

type traceEvidence struct {
	evidence.Envelope
	RunID          string            `json:"runId"`
	Source         string            `json:"source"`
	SourceSHA256   string            `json:"sourceSha256"`
	RepoCommit     string            `json:"repoCommit"`
	BaselineCommit string            `json:"baselineCommit"`
	BaselineStatus string            `json:"baselineStatus"`
	ModulePath     string            `json:"modulePath"`
	Command        []string          `json:"command"`
	ExitStatus     int               `json:"exitStatus"`
	ReportSHA256   string            `json:"reportSha256"`
	Counts         gotestjson.Counts `json:"counts"`
	RowAccounting  rowAccounting     `json:"rowAccounting"`
	FindingCounts  map[string]int    `json:"findingCounts"`
	RowResults     []traceRowResult  `json:"rowResults"`
	Findings       []traceFinding    `json:"findings"`
	TagClasses     []traceTagClass   `json:"tagClasses"`
	NewFailures    []traceNewFailure `json:"newFailures"`
	StartedAt      string            `json:"startedAt"`
	FinishedAt     string            `json:"finishedAt"`
}

type traceOutput struct {
	evidence.Envelope
	Source         string         `json:"source"`
	RepoCommit     string         `json:"repoCommit"`
	BaselineStatus string         `json:"baselineStatus"`
	FindingCounts  map[string]int `json:"findingCounts"`
	RowAccounting  rowAccounting  `json:"rowAccounting"`
	EvidencePath   string         `json:"evidencePath"`
	RunDir         string         `json:"runDir"`
}

func traceHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeVerifyRequest("trace", requestBody)
	if cliErr != nil {
		return cliErr
	}
	return traceOp(req, stdout, nil)
}

// traceOp is the trace op body. between runs after the suite has run and
// before the final Compare; it is nil in production and exists so a test can
// mutate the original repository at exactly the moment the op is blind to it.
func traceOp(req *verifyRequest, stdout io.Writer, between func()) *cliError {
	src, cliErr := loadVerifiedSource("trace", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("trace: %v", err)}
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("trace: %v", err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "source-not-reproducible",
			Message: fmt.Sprintf("trace: source not reproducible: %s", describeDirty(req.RepoRoot)),
		}
	}

	baselineDoc, baselineStatus := loadBaselineCondition(req.RepoRoot, src)

	repoCommit, cliErr := headCommit("trace", req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}
	runDir, err := ctrl.NewRunDir(req.RunID)
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("trace: %v", err)}
	}

	wt, err := ctrl.Create(repoCommit)
	if err != nil {
		return &cliError{Code: "worktree-create-failed", Message: fmt.Sprintf("trace: %v", err)}
	}
	disposed := false
	dispose := func() *cliError {
		if disposed {
			return nil
		}
		disposed = true
		if err := ctrl.Dispose(wt); err != nil {
			return &cliError{Code: "worktree-dispose-failed", Message: fmt.Sprintf("trace: %v", err)}
		}
		return nil
	}
	defer func() { _ = dispose() }()

	suiteDir := filepath.Join(wt.Path, filepath.FromSlash(src.Runner.Root))
	modulePath, cliErr := readModulePath(suiteDir)
	if cliErr != nil {
		return cliErr
	}

	sites, err := trace.ScanTags(suiteDir, modulePath)
	if err != nil {
		return &cliError{Code: "scan-tags-failed", Message: fmt.Sprintf("trace: %v", err)}
	}

	sr, cliErr := runSuite("trace", suiteDir, runDir, src.Runner.Command)
	if cliErr != nil {
		return cliErr
	}
	if between != nil {
		between()
	}
	if sr.normalizationError != "" {
		return &cliError{Code: "normalization-failed", Message: fmt.Sprintf("trace: %v", sr.normalizationError)}
	}
	if sr.exitStatus != 0 && sr.report.Counts.Failed == 0 && len(sr.report.Failures) == 0 {
		return &cliError{
			Code: "unexplained-runner-failure",
			Message: fmt.Sprintf("trace: runner exited %d with no failed test and no runner failure; see %s",
				sr.exitStatus, filepath.Join(runDir, stderrFile)),
		}
	}
	canonicalizeReport(sr.report)
	reportSHA, cliErr := writeNormalizedReport("trace", runDir, sr.report)
	if cliErr != nil {
		return cliErr
	}

	reg, err := coverage.BuildRegistry(wt.Path)
	if err != nil {
		return &cliError{Code: "registry-error", Message: fmt.Sprintf("trace: %v", err)}
	}

	results, findings, classes, err := trace.Join(src, reg, sites, sr.report)
	if err != nil {
		return &cliError{Code: "join-failed", Message: fmt.Sprintf("trace: %v", err)}
	}

	var newFailures []traceNewFailure
	if baselineStatus == baselineOK {
		findings = append(findings, trace.CompareBaseline(baselineDoc, sr.report)...)
		newFailures = newFailuresAgainst(baselineDoc, sr.report)
	}
	sortFindings(findings)

	ev := traceEvidence{
		Envelope:       evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:          req.RunID,
		Source:         src.Path,
		SourceSHA256:   src.SHA256,
		RepoCommit:     repoCommit,
		BaselineCommit: src.BaselineCommit,
		BaselineStatus: baselineStatus,
		ModulePath:     modulePath,
		Command:        redactCommand(src.Runner.Command),
		ExitStatus:     sr.exitStatus,
		ReportSHA256:   reportSHA,
		Counts:         sr.report.Counts,
		RowAccounting:  accountRows(src, results, findings),
		FindingCounts:  countFindings(findings),
		RowResults:     rowResults(results),
		Findings:       renderFindings(findings),
		TagClasses:     tagClasses(classes),
		NewFailures:    newFailures,
		StartedAt:      sr.startedAt,
		FinishedAt:     sr.finishedAt,
	}
	if ev.NewFailures == nil {
		ev.NewFailures = []traceNewFailure{}
	}

	evidencePath := filepath.Join(runDir, traceEvidenceFile)
	if err := evidence.WriteJSON(evidencePath, &ev); err != nil {
		return &cliError{Code: "evidence-write-failed", Message: fmt.Sprintf("trace: %v", err)}
	}

	if cliErr := dispose(); cliErr != nil {
		return cliErr
	}

	after, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("trace: %v", err)}
	}
	if equal, differing := worktree.Compare(before, after); !equal {
		return &cliError{
			Code: "source-changed-during-verification",
			Message: fmt.Sprintf("trace: source repository changed during verification: %s; commit or discard the concurrent change, then rerun",
				strings.Join(differing, ", ")),
		}
	}

	out := traceOutput{
		Envelope:       ev.Envelope,
		Source:         ev.Source,
		RepoCommit:     ev.RepoCommit,
		BaselineStatus: ev.BaselineStatus,
		FindingCounts:  ev.FindingCounts,
		RowAccounting:  ev.RowAccounting,
		EvidencePath:   evidencePath,
		RunDir:         runDir,
	}
	return writeStdout(stdout, out)
}

const (
	baselineOK      = "ok"
	baselineMissing = "missing"
)

// loadBaselineCondition never fails the op: a baseline that is absent or does
// not verify is a named condition in the evidence, which finalize turns into
// PARTIAL rather than a verdict.
func loadBaselineCondition(repoRoot string, src *coverage.Source) (*trace.Baseline, string) {
	if src.BaselineEvidencePath == "" {
		return nil, baselineMissing
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(src.BaselineEvidencePath))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, baselineMissing
		}
		return nil, "invalid: " + err.Error()
	}
	b, err := trace.LoadBaseline(path, src.BaselineSHA256)
	if err != nil {
		return nil, "invalid: " + err.Error()
	}
	return b, baselineOK
}

var modulePattern = regexp.MustCompile(`(?m)^module\s+(\S+)`)

func readModulePath(dir string) (string, *cliError) {
	path := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", &cliError{Code: "go-mod-unreadable", Message: fmt.Sprintf("trace: read go.mod at runner root: %v", err)}
	}
	m := modulePattern.FindSubmatch(data)
	if m == nil {
		return "", &cliError{Code: "go-mod-unreadable", Message: fmt.Sprintf("trace: %s has no module line", path)}
	}
	return string(m[1]), nil
}

func newFailuresAgainst(b *trace.Baseline, rep *gotestjson.Report) []traceNewFailure {
	known := make(map[trace.BaselineIdentity]bool)
	if b != nil {
		for _, id := range b.Failed {
			known[id] = true
		}
	}
	var out []traceNewFailure
	for _, rec := range rep.Records {
		if rec.Status != runrecord.Failed {
			continue
		}
		if known[trace.BaselineIdentity{Scope: rec.Scope, Name: rec.Name}] {
			continue
		}
		out = append(out, traceNewFailure{Scope: rec.Scope, Name: rec.Name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sortFindings(findings []trace.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].RowID != findings[j].RowID {
			return findings[i].RowID < findings[j].RowID
		}
		return findings[i].Detail < findings[j].Detail
	})
}

func renderFindings(findings []trace.Finding) []traceFinding {
	out := make([]traceFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, traceFinding{Kind: f.Kind, RowID: f.RowID, Detail: f.Detail})
	}
	return out
}

func rowResults(results []trace.RowResult) []traceRowResult {
	out := make([]traceRowResult, 0, len(results))
	for _, r := range results {
		out = append(out, traceRowResult{RowID: r.RowID, Scope: r.Scope, Name: r.Name, Status: string(r.Status)})
	}
	return out
}

func tagClasses(classes []trace.TagClass) []traceTagClass {
	out := make([]traceTagClass, 0, len(classes))
	for _, c := range classes {
		out = append(out, traceTagClass{ID: c.ID, Class: c.Class, File: c.File, Line: c.Line})
	}
	return out
}

func countFindings(findings []trace.Finding) map[string]int {
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Kind]++
	}
	return counts
}

func accountRows(src *coverage.Source, results []trace.RowResult, findings []trace.Finding) rowAccounting {
	seen := make(map[string]bool)
	for _, r := range results {
		seen[r.RowID] = true
	}
	for _, f := range findings {
		if f.RowID != "" {
			seen[f.RowID] = true
		}
	}

	active := activeRowIDs(src)
	unaccounted := []string{}
	for _, id := range active {
		if !seen[id] {
			unaccounted = append(unaccounted, id)
		}
	}
	return rowAccounting{ActiveRows: len(active), WithResult: len(results), Unaccounted: unaccounted}
}
