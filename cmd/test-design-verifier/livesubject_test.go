package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/report"
)

// This file holds the machinery every pipeline proof shares: a temporary Git
// subject repository built through the real baseline op, a driver for the real
// trace, prepare-live and execute-live ops over it, and the finalize request
// assembled from the evidence those ops emitted. No proof writes an evidence
// document by hand; each one reads only the files the ops produced.

// liveSubjectBaselineRunID names the run the builder gives the baseline op. It
// belongs to building the subject, never to a live run a proof then drives.
const liveSubjectBaselineRunID = "live-subject-baseline"

// liveSubject is a temporary Git repository the pipeline proofs run against, at
// the clean, frozen commit every op in the pipeline must agree on. coverageSource
// and profilePath are the subject's own paths, relative to repo.
type liveSubject struct {
	repo, head, coverageSource, profilePath string
	priorCommits                            []string
}

func writeSubjectFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func rewriteSubjectDoc(t *testing.T, repo, coverageSource string, replace func(string) string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(coverageSource))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read subject coverage doc: %v", err)
	}
	if err := os.WriteFile(path, []byte(replace(string(data))), 0o644); err != nil {
		t.Fatalf("write subject coverage doc: %v", err)
	}
}

func subjectHead(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
}

// newLiveSubject builds the subject repository and runs the real baseline op
// against it, so the trace that follows compares against a verified baseline and
// reports baselineStatus ok rather than a PARTIAL condition.
//
// commits are applied in order, each map being path (slash-separated, relative
// to the repository root) to content; a path listed in executable is made 0o755
// before it is added. After the last one the builder pins BASELINE_COMMIT to the
// last content commit, runs baseline, pins BASELINE_SHA256 to the evidence digest
// it published, commits that, and freezes head. priorCommits holds the SHA of
// every commit before the freeze, oldest first, so a stale-checkout harness can
// deploy an earlier revision of the subject.
func newLiveSubject(t *testing.T, coverageSource, profilePath string, commits []map[string]string, executable []string) *liveSubject {
	t.Helper()
	if len(commits) == 0 {
		t.Fatalf("newLiveSubject: want at least one commit of subject files")
	}
	isolateEnv(t)
	repo := t.TempDir()
	mustGit(t, repo, "init", "-q")

	executableSet := make(map[string]bool, len(executable))
	for _, relPath := range executable {
		executableSet[relPath] = true
	}

	s := &liveSubject{repo: repo, coverageSource: coverageSource, profilePath: profilePath}
	for i, files := range commits {
		for _, relPath := range slices.Sorted(maps.Keys(files)) {
			writeSubjectFile(t, repo, relPath, files[relPath])
			if !executableSet[relPath] {
				continue
			}
			if err := os.Chmod(filepath.Join(repo, filepath.FromSlash(relPath)), 0o755); err != nil {
				t.Fatalf("chmod %s: %v", relPath, err)
			}
		}
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-q", "-m", fmt.Sprintf("live subject commit %d", i+1))
		s.priorCommits = append(s.priorCommits, subjectHead(t, repo))
	}

	rewriteSubjectDoc(t, repo, coverageSource, func(doc string) string {
		return strings.ReplaceAll(doc, "BASELINE_COMMIT", s.priorCommits[len(s.priorCommits)-1])
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline commit")
	s.priorCommits = append(s.priorCommits, subjectHead(t, repo))

	code, stdout, stderr := runVerifyOp(t, "baseline", liveRequestJSON(t, repo, coverageSource, liveSubjectBaselineRunID))
	if code != exitOK {
		t.Fatalf("baseline exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var baseOut baselineOutput
	if err := json.Unmarshal([]byte(stdout), &baseOut); err != nil {
		t.Fatalf("baseline stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	// The baseline op leaves its published evidence untracked; committing it
	// together with the pinned digest returns the subject to a clean HEAD.
	rewriteSubjectDoc(t, repo, coverageSource, func(doc string) string {
		return strings.ReplaceAll(doc, "BASELINE_SHA256", baseOut.EvidenceSHA256)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "pin baseline evidence")

	s.head = subjectHead(t, repo)
	return s
}

func liveRequestJSON(t *testing.T, repo, coverageSource, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       repo,
		"coverageSource": coverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func liveExecuteRequestJSON(t *testing.T, repo, coverageSource, runDir, planPath, tracePath string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       repo,
		"coverageSource": coverageSource,
		"runDir":         runDir,
		"plan":           planPath,
		"traceEvidence":  tracePath,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

// runLivePipeline drives the real trace, prepare-live and execute-live ops over
// the subject under one run ID, the way the workflow does, and fails on any
// refusal so a regression surfaces with the op's own error code. plan is the
// caller's plan document, written outside the repository the way the live-runner
// hands one to the op. beforeExecute, when non-nil, runs between prepare-live and
// execute-live with the prepare output, so a proof can reach the run dir or the
// worktree before anything is started.
func runLivePipeline(t *testing.T, s *liveSubject, runID string, plan map[string]any, beforeExecute func(prepareLiveOutput)) (traceOutput, prepareLiveOutput, executeLiveOutput) {
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

	if beforeExecute != nil {
		beforeExecute(prep)
	}

	code, stdout, stderr = runVerifyOp(t, "execute-live",
		liveExecuteRequestJSON(t, s.repo, s.coverageSource, prep.RunDir, writePlanFile(t, plan), tr.EvidencePath))
	if code != exitOK {
		t.Fatalf("execute-live exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var exec executeLiveOutput
	if err := json.Unmarshal([]byte(stdout), &exec); err != nil {
		t.Fatalf("execute-live stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return tr, prep, exec
}

// layer2ForSubject reuses the bookkeeping shape the e2e finalize proof builds —
// one caught synthesized canary, its row re-dispatched alone against the real
// source, the rest inheriting the catch, gap rows MISSING — and stamps this
// run's own id, which staleness validation ties every batch to.
func layer2ForSubject(runID string, active []coverage.Row, gaps map[string]bool) []report.Batch {
	batches := layer2FromTrace(active, gaps)
	for i := range batches {
		batches[i].RunID = runID
	}
	return batches
}

// liveFinalizeRequestJSON assembles the finalize request around the evidence
// the pipeline actually produced: the trace file, the layer-2 verdicts it
// implies, and the live evidence path execute-live wrote.
func liveFinalizeRequestJSON(t *testing.T, s *liveSubject, runID, tracePath, livePath string) string {
	t.Helper()
	src, cliErr := loadVerifiedSource("finalize", s.repo, s.coverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	tr, err := report.LoadTrace(tracePath)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	active := report.ActiveRows(src)
	gaps := report.Layer1Gaps(tr)
	// The layer-2 canary rides on the first row layer 1 bound to a tagged test;
	// a trace reporting every active row as a gap would leave that batch with no
	// row at all, which is a broken subject rather than a finalize verdict.
	canaried := false
	for _, row := range active {
		if !gaps[row.ID] {
			canaried = true
			break
		}
	}
	if !canaried {
		t.Fatalf("trace gaps = %v over active rows %+v, want at least one bound row to carry the layer-2 canary", gaps, active)
	}

	body, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:  evidence.SchemaVersion,
		RepoRoot:       s.repo,
		CoverageSource: s.coverageSource,
		RunID:          runID,
		finalizeRequestBody: finalizeRequestBody{
			Approval:      finalizeApproval,
			TraceEvidence: tracePath,
			Freshness:     report.Freshness{Result: "fresh"},
			Layer2:        finalizeLayer2{Batches: layer2ForSubject(runID, active, gaps)},
			Layer3:        &finalizeLayer3{EvidencePath: livePath},
		},
	})
	if err != nil {
		t.Fatalf("marshal finalize request: %v", err)
	}
	return string(body)
}

// liveRunID returns "<prefix>-<pid>". The process ID makes the run unique across
// concurrent and repeated test processes — a run root one of them left under
// /tmp must not collide with a later run — and keeps the id inside the run-ID
// class [A-Za-z0-9][A-Za-z0-9-]{0,39} the harness recipes accept.
func liveRunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, os.Getpid())
}

func readPublishedReport(t *testing.T, repo, jsonPath string) *report.Report {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(jsonPath)))
	if err != nil {
		t.Fatalf("read published report: %v", err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("decode published report: %v", err)
	}
	return &rep
}

func publishedRow(t *testing.T, rep *report.Report, rowID string) report.RowReport {
	t.Helper()
	for _, row := range rep.Rows {
		if row.RowID == rowID {
			return row
		}
	}
	t.Fatalf("report rows = %+v, want an entry for %s", rep.Rows, rowID)
	return report.RowReport{}
}

func publishedTranscript(t *testing.T, rep *report.Report, path string) report.TranscriptRef {
	t.Helper()
	for _, ref := range rep.Transcripts {
		if ref.Path == path {
			return ref
		}
	}
	t.Fatalf("report transcripts = %+v, want an entry for %s", rep.Transcripts, path)
	return report.TranscriptRef{}
}

func requireReason(t *testing.T, reasons []report.Reason, layer, rowID, code string) {
	t.Helper()
	for _, r := range reasons {
		if r.Layer == layer && r.RowID == rowID && r.Code == code {
			return
		}
	}
	t.Errorf("reasons = %+v, want a layer-%s %q reason for %s", reasons, layer, code, rowID)
}
