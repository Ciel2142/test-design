package main

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
	"test-design/internal/trace"
	"test-design/internal/worktree"
)

const baselineEvidenceFile = "baseline.json"

type baselineOutput struct {
	evidence.Envelope
	Source         string            `json:"source"`
	BaselineCommit string            `json:"baselineCommit"`
	Counts         gotestjson.Counts `json:"counts"`
	ExitStatus     int               `json:"exitStatus"`
	EvidencePath   string            `json:"evidencePath"`
	EvidenceSHA256 string            `json:"evidenceSha256"`
	RunDir         string            `json:"runDir"`
}

func baselineHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeVerifyRequest("baseline", requestBody)
	if cliErr != nil {
		return cliErr
	}
	src, cliErr := loadVerifiedSource("baseline", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}
	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("baseline: %v", err)}
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("baseline: %v", err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "dirty-repository",
			Message: fmt.Sprintf("baseline: %v: %s", worktree.ErrDirtyBeforeSnapshot, describeDirty(req.RepoRoot)),
		}
	}

	runDir, err := ctrl.NewRunDir(req.RunID)
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("baseline: %v", err)}
	}

	wt, err := ctrl.Create(src.BaselineCommit)
	if err != nil {
		return &cliError{Code: "worktree-create-failed", Message: fmt.Sprintf("baseline: %v", err)}
	}
	disposed := false
	dispose := func() *cliError {
		if disposed {
			return nil
		}
		disposed = true
		if err := ctrl.Dispose(wt); err != nil {
			return &cliError{Code: "worktree-dispose-failed", Message: fmt.Sprintf("baseline: %v", err)}
		}
		return nil
	}
	defer func() { _ = dispose() }()

	suiteDir := filepath.Join(wt.Path, filepath.FromSlash(src.Runner.Root))
	sr, cliErr := runSuite("baseline", suiteDir, runDir, src.Runner.Command)
	if cliErr != nil {
		return cliErr
	}

	b := &trace.Baseline{
		Envelope:           evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RepoCommit:         src.BaselineCommit,
		RunnerKind:         src.Runner.Kind,
		Command:            redactCommand(src.Runner.Command),
		ToolVersions:       toolVersions(suiteDir, src.Runner.Command),
		StartedAt:          sr.startedAt,
		FinishedAt:         sr.finishedAt,
		ExitStatus:         sr.exitStatus,
		NormalizationError: sr.normalizationError,
	}
	if sr.report != nil {
		canonicalizeReport(sr.report)
		reportSHA, cliErr := writeNormalizedReport("baseline", runDir, sr.report)
		if cliErr != nil {
			return cliErr
		}
		b.ReportSHA256 = reportSHA
		b.Counts = sr.report.Counts
		b.Failed = identities(sr.report.Records, runrecord.Failed)
		b.Skipped = identities(sr.report.Records, runrecord.Skipped)
	}

	evidencePath := filepath.Join(runDir, baselineEvidenceFile)
	if err := trace.WriteBaseline(evidencePath, b); err != nil {
		return &cliError{Code: "baseline-write-failed", Message: fmt.Sprintf("baseline: %v", err)}
	}
	if sr.normalizationError != "" {
		return &cliError{
			Code:    "normalization-failed",
			Message: fmt.Sprintf("baseline: cannot publish an unnormalizable baseline: %s", sr.normalizationError),
		}
	}

	if cliErr := dispose(); cliErr != nil {
		return cliErr
	}

	records, err := ctrl.Publish(before, []worktree.PublishFile{{
		RepoRelPath: filepath.FromSlash(src.BaselineEvidencePath),
		SourcePath:  evidencePath,
	}})
	if err != nil {
		return &cliError{Code: "publish-failed", Message: fmt.Sprintf("baseline: %v", err)}
	}
	if len(records) != 1 {
		return &cliError{Code: "publish-failed", Message: fmt.Sprintf("baseline: publish returned %d records, want 1", len(records))}
	}

	out := baselineOutput{
		Envelope:       evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		Source:         src.Path,
		BaselineCommit: src.BaselineCommit,
		Counts:         b.Counts,
		ExitStatus:     b.ExitStatus,
		EvidencePath:   src.BaselineEvidencePath,
		EvidenceSHA256: records[0].SHA256,
		RunDir:         runDir,
	}
	return writeStdout(stdout, out)
}

func identities(records []runrecord.Record, want runrecord.Status) []trace.BaselineIdentity {
	ids := make([]trace.BaselineIdentity, 0)
	for _, rec := range records {
		if rec.Status != want {
			continue
		}
		ids = append(ids, trace.BaselineIdentity{Scope: rec.Scope, Name: rec.Name})
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].Scope != ids[j].Scope {
			return ids[i].Scope < ids[j].Scope
		}
		return ids[i].Name < ids[j].Name
	})
	return ids
}

// toolVersions is best-effort provenance: a toolchain that cannot report its
// version does not invalidate the suite run that already succeeded with it.
func toolVersions(dir string, command []string) map[string]string {
	versions := map[string]string{}
	cmd := exec.Command(command[0], "version")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return versions
	}
	versions[filepath.Base(command[0])] = strings.TrimSpace(string(out))
	return versions
}
