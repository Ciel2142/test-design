package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodePreflight(t *testing.T, stdout *bytes.Buffer) preflightOutput {
	t.Helper()
	var got preflightOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout did not decode as preflight output: %v\nstdout: %s", err, stdout.String())
	}
	return got
}

func TestPreflightHandler_ValidSourceReportsFullOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "valid-repo")
	source := filepath.Join(repo, "docs", "superpowers", "coverage", "2026-08-26-widgets-coverage.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodePreflight(t, &stdout)

	if got.Source != source {
		t.Errorf("Source = %q, want %q", got.Source, source)
	}
	if got.SourceSHA256 == "" {
		t.Errorf("SourceSHA256 empty")
	}
	if got.BaselineCommit != "a1b2c3d" {
		t.Errorf("BaselineCommit = %q, want a1b2c3d", got.BaselineCommit)
	}
	if got.BaselineEvidencePath != "docs/superpowers/coverage/2026-08-26-widgets-baseline.json" {
		t.Errorf("BaselineEvidencePath = %q", got.BaselineEvidencePath)
	}
	if got.BaselineSHA256 != "9f8e7d6c" {
		t.Errorf("BaselineSHA256 = %q", got.BaselineSHA256)
	}
	if got.Runner.Kind != "go-test-json" || got.Runner.Root != "." {
		t.Errorf("Runner = %+v", got.Runner)
	}
	if len(got.Runner.Command) != 5 {
		t.Errorf("Runner.Command = %v, want 5 elements", got.Runner.Command)
	}
	if len(got.Runner.ReportGlobs) != 1 || got.Runner.ReportGlobs[0] != "reports/*.json" {
		t.Errorf("Runner.ReportGlobs = %v", got.Runner.ReportGlobs)
	}

	// 4 rows: SC-01 unit/med, SC-02 integration/high, SC-03 smoke/med, SC-04 e2e/high.
	if got.Layers.Layer1 != 4 {
		t.Errorf("Layers.Layer1 = %d, want 4", got.Layers.Layer1)
	}
	if got.Layers.Layer2 != 4 {
		t.Errorf("Layers.Layer2 = %d, want 4", got.Layers.Layer2)
	}
	if got.Layers.Layer3 != 2 {
		t.Errorf("Layers.Layer3 = %d, want 2 (SC-03 smoke, SC-04 e2e)", got.Layers.Layer3)
	}
	if got.Layers.Layer4 != 2 {
		t.Errorf("Layers.Layer4 = %d, want 2 (SC-02, SC-04 high risk)", got.Layers.Layer4)
	}
	if got.HighRiskCount != 2 {
		t.Errorf("HighRiskCount = %d, want 2", got.HighRiskCount)
	}
	if !got.LiveProfileRequired {
		t.Errorf("LiveProfileRequired = false, want true (layer 3 count > 0)")
	}
	if got.LiveProfile != ".test-design/live-profiles/local.json" {
		t.Errorf("LiveProfile = %q", got.LiveProfile)
	}
}

func TestPreflightHandler_MissingLiveProfileReportsRequiredWithEmptyPath(t *testing.T) {
	dir := t.TempDir()
	coverageDir := filepath.Join(dir, "docs", "superpowers", "coverage")
	if err := os.MkdirAll(coverageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	source := filepath.Join(coverageDir, "2026-08-26-smoke-only-coverage.md")
	content := `# Coverage: smoke-only

Spec: docs/superpowers/specs/2026-08-26-smoke-only-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-smoke-only-baseline.json
- baseline-sha256: abc123
- final-command: ["/verify"]

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | dependency down answers 503 | 503 returned | smoke | error mapped wrong | med |
`
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"preflight", "--repo", dir, "--source", source}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodePreflight(t, &stdout)
	if !got.LiveProfileRequired {
		t.Errorf("LiveProfileRequired = false, want true")
	}
	if got.LiveProfile != "" {
		t.Errorf("LiveProfile = %q, want empty", got.LiveProfile)
	}
}

func TestPreflightHandler_MissingSourceFlagRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	code := run([]string{"preflight", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code == "" {
		t.Errorf("cliError.Code empty, want a code")
	}
}

func TestPreflightHandler_MissingSourceFileNamesPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	source := filepath.Join(repo, "does-not-exist.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if !strings.Contains(got.Message, source) {
		t.Errorf("error message = %q, want it to name %q", got.Message, source)
	}
}

func TestPreflightHandler_OldHeaderSourceRefusedWithMigrationMessage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "old-header-repo")
	source := filepath.Join(repo, "docs", "superpowers", "coverage", "2026-08-26-legacy-coverage.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if !strings.Contains(got.Message, source) {
		t.Errorf("error message = %q, want it to name %q", got.Message, source)
	}
	if !strings.Contains(got.Message, "Bug it catches") {
		t.Errorf("error message = %q, want it to name the old header \"Bug it catches\"", got.Message)
	}
}

func TestPreflightHandler_CrossDocumentCollisionErrorNamesBothPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "collision-repo")
	source := filepath.Join(repo, "docs", "superpowers", "coverage", "2026-08-26-x-coverage.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if !strings.Contains(got.Message, "2026-08-26-x-coverage.md") {
		t.Errorf("error message = %q, want it to name 2026-08-26-x-coverage.md", got.Message)
	}
	if !strings.Contains(got.Message, "2026-08-26-y-coverage.md") {
		t.Errorf("error message = %q, want it to name 2026-08-26-y-coverage.md", got.Message)
	}
}
