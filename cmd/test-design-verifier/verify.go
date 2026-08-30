package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
)

type verifyRequest struct {
	SchemaVersion  string `json:"schemaVersion"`
	RepoRoot       string `json:"repoRoot"`
	CoverageSource string `json:"coverageSource"`
	RunID          string `json:"runID"`
}

func decodeVerifyRequest(op string, requestBody []byte) (*verifyRequest, *cliError) {
	if requestBody == nil {
		return nil, &cliError{Code: "missing-request", Message: fmt.Sprintf("%s: --request is required", op)}
	}
	var req verifyRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if req.RepoRoot == "" {
		return nil, &cliError{Code: "missing-repo-root", Message: fmt.Sprintf("%s: repoRoot is required", op)}
	}
	if req.CoverageSource == "" {
		return nil, &cliError{Code: "missing-coverage-source", Message: fmt.Sprintf("%s: coverageSource is required", op)}
	}
	rel, cliErr := confinedRelPath(op, "coverageSource", "source-outside-repo", req.CoverageSource)
	if cliErr != nil {
		return nil, cliErr
	}
	req.CoverageSource = rel
	return &req, nil
}

// confinedRelPath guards a join: coverageSource onto the repo root,
// runner-root onto the worktree.
func confinedRelPath(op, field, code, value string) (string, *cliError) {
	if value == "" {
		return "", &cliError{Code: code, Message: fmt.Sprintf("%s: %s is empty", op, field)}
	}
	if filepath.IsAbs(value) {
		return "", &cliError{
			Code:    code,
			Message: fmt.Sprintf("%s: %s %q escapes its root: absolute path", op, field, value),
		}
	}
	cleaned := filepath.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", &cliError{
			Code:    code,
			Message: fmt.Sprintf("%s: %s %q escapes its root: traverses above it", op, field, value),
		}
	}
	return filepath.ToSlash(cleaned), nil
}

// loadVerifiedSource parses the coverage source and applies the two gates both
// run ops share. Source.Path is rewritten to the repo-root-relative form
// coverage.Owner.DocPath carries, which tags.Classify compares verbatim.
func loadVerifiedSource(op, repoRoot, coverageSource string) (*coverage.Source, *cliError) {
	src, err := coverage.ParseSource(filepath.Join(repoRoot, filepath.FromSlash(coverageSource)))
	if err != nil {
		return nil, &cliError{Code: "parse-source-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	src.Path = coverageSource

	if src.Runner.Kind != "go" {
		return nil, &cliError{
			Code:    "unsupported-runner-kind",
			Message: fmt.Sprintf("%s: unsupported runner kind %s", op, src.Runner.Kind),
		}
	}
	if len(src.Runner.Command) == 0 {
		return nil, &cliError{Code: "empty-runner-command", Message: fmt.Sprintf("%s: runner command is empty", op)}
	}
	if !slices.Contains(src.Runner.Command, "-count=1") {
		return nil, &cliError{
			Code:    "missing-count-one",
			Message: fmt.Sprintf("%s: go suite command must include -count=1", op),
		}
	}
	root, cliErr := confinedRelPath(op, "runner-root", "runner-root-outside-worktree", src.Runner.Root)
	if cliErr != nil {
		return nil, cliErr
	}
	src.Runner.Root = root

	baselinePath, cliErr := confinedRelPath(op, "baseline-evidence", "baseline-evidence-outside-repo", src.BaselineEvidencePath)
	if cliErr != nil {
		return nil, cliErr
	}
	src.BaselineEvidencePath = baselinePath
	return src, nil
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s (in %s): %w: %s", strings.Join(args, " "), repoRoot, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func headCommit(op, repoRoot string) (string, *cliError) {
	out, err := gitOutput(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", &cliError{Code: "head-unresolved", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	return strings.TrimSpace(out), nil
}

// dirtyPaths names what a snapshot found unclean. worktree.Snapshot reports
// only Clean(); the paths come from a second, read-only status read.
func dirtyPaths(repoRoot string) []string {
	out, err := gitOutput(repoRoot, "--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		paths = append(paths, strings.TrimSpace(line[3:]))
	}
	sort.Strings(paths)
	return paths
}

func describeDirty(repoRoot string) string {
	paths := dirtyPaths(repoRoot)
	if len(paths) == 0 {
		return "uncommitted changes present"
	}
	return "uncommitted changes in " + strings.Join(paths, ", ")
}

type suiteRun struct {
	report             *gotestjson.Report
	exitStatus         int
	startedAt          string
	finishedAt         string
	normalizationError string
}

const (
	rawStreamFile  = "runner-stdout.jsonl"
	stderrFile     = "runner-stderr.log"
	normalizedFile = "normalized-report.json"
)

// runSuite executes the coverage source's runner command inside dir and
// normalizes its stdout. A nonzero exit is not an error here: callers decide
// what an unexplained failure means. A normalization failure leaves report nil
// and names itself in normalizationError.
func runSuite(op, dir, runDir string, command []string) (*suiteRun, *cliError) {
	stderrPath := filepath.Join(runDir, stderrFile)
	stderrOut, err := os.Create(stderrPath)
	if err != nil {
		return nil, &cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	defer stderrOut.Close()

	var stdout bytes.Buffer
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdout = &stdout
	cmd.Stderr = stderrOut

	started := time.Now().UTC()
	runErr := cmd.Run()
	finished := time.Now().UTC()

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
	default:
		return nil, &cliError{
			Code:    "runner-start-failed",
			Message: fmt.Sprintf("%s: run %s in %s: %v", op, strings.Join(command, " "), dir, runErr),
		}
	}

	rawPath := filepath.Join(runDir, rawStreamFile)
	if err := os.WriteFile(rawPath, stdout.Bytes(), 0o644); err != nil {
		return nil, &cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	sr := &suiteRun{
		exitStatus: cmd.ProcessState.ExitCode(),
		startedAt:  started.Format(time.RFC3339Nano),
		finishedAt: finished.Format(time.RFC3339Nano),
	}

	report, err := gotestjson.Normalize(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		sr.normalizationError = err.Error()
		return sr, nil
	}
	sr.report = report
	return sr, nil
}

// elapsedPattern matches only go test's trailing per-package result line
// ("FAIL\tpkg\t0.003s"), whose timing varies run to run. Anchored to that line
// so a duration quoted inside a panic or build message survives verbatim.
var elapsedPattern = regexp.MustCompile(`(?m)^((?:ok|FAIL|\?)[ \t]+\S+[ \t]+)[0-9]+\.[0-9]+s[ \t]*$`)

// canonicalizeReport makes a normalized report reproducible: go test emits
// packages in completion order, so both records and package-level failures are
// sorted and their elapsed times scrubbed.
func canonicalizeReport(rep *gotestjson.Report) {
	sort.SliceStable(rep.Records, func(i, j int) bool {
		if rep.Records[i].Scope != rep.Records[j].Scope {
			return rep.Records[i].Scope < rep.Records[j].Scope
		}
		if rep.Records[i].Name != rep.Records[j].Name {
			return rep.Records[i].Name < rep.Records[j].Name
		}
		return rep.Records[i].Status < rep.Records[j].Status
	})
	for i := range rep.Failures {
		rep.Failures[i].Detail = elapsedPattern.ReplaceAllString(rep.Failures[i].Detail, "${1}<elapsed>")
	}
	sort.SliceStable(rep.Failures, func(i, j int) bool {
		if rep.Failures[i].Package != rep.Failures[j].Package {
			return rep.Failures[i].Package < rep.Failures[j].Package
		}
		return rep.Failures[i].Detail < rep.Failures[j].Detail
	})
}

type normalizedReport struct {
	Records  []runrecord.Record         `json:"records"`
	Counts   gotestjson.Counts          `json:"counts"`
	Failures []gotestjson.RunnerFailure `json:"failures"`
}

func writeNormalizedReport(op, runDir string, rep *gotestjson.Report) (string, *cliError) {
	path := filepath.Join(runDir, normalizedFile)
	doc := normalizedReport{Records: rep.Records, Counts: rep.Counts, Failures: rep.Failures}
	if doc.Records == nil {
		doc.Records = []runrecord.Record{}
	}
	if doc.Failures == nil {
		doc.Failures = []gotestjson.RunnerFailure{}
	}
	if err := evidence.WriteJSON(path, &doc); err != nil {
		return "", &cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	sum, err := evidence.SHA256File(path)
	if err != nil {
		return "", &cliError{Code: "run-dir-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	return sum, nil
}

var (
	secretArgPattern  = regexp.MustCompile(`(?i)^(-{0,2}[A-Za-z0-9_.-]*(token|secret|password|passwd|api[_-]?key|credential)[A-Za-z0-9_.-]*)=.*$`)
	secretFlagPattern = regexp.MustCompile(`(?i)^-{1,2}[A-Za-z0-9_.-]*(token|secret|password|passwd|api[_-]?key|credential)[A-Za-z0-9_.-]*$`)
)

// redactCommand keeps the command reproducible in evidence without carrying a
// credential a coverage source may have inlined into an argument.
func redactCommand(command []string) []string {
	redacted := make([]string, len(command))
	for i := 0; i < len(command); i++ {
		arg := command[i]
		if m := secretArgPattern.FindStringSubmatch(arg); m != nil {
			redacted[i] = m[1] + "=REDACTED"
			continue
		}
		redacted[i] = arg
		if secretFlagPattern.MatchString(arg) && i+1 < len(command) {
			i++
			redacted[i] = "REDACTED"
		}
	}
	return redacted
}

func activeRowIDs(src *coverage.Source) []string {
	retired := make(map[string]bool, len(src.Retired))
	for _, id := range src.Retired {
		retired[id] = true
	}
	ids := make([]string, 0, len(src.Rows))
	for _, row := range src.Rows {
		if retired[row.ID] {
			continue
		}
		ids = append(ids, row.ID)
	}
	sort.Strings(ids)
	return ids
}
