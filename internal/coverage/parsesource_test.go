package coverage

import (
	"os"
	"path/filepath"
	"testing"

	"test-design/internal/evidence"
)

func TestParseSourceValidDocument(t *testing.T) {
	path := filepath.Join("testdata", "valid-source.md")

	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if src.Path != path {
		t.Errorf("src.Path = %q, want %q", src.Path, path)
	}
	if src.SpecPath != "docs/superpowers/specs/2026-07-30-order-creation-design.md" {
		t.Errorf("src.SpecPath = %q", src.SpecPath)
	}
	if src.BaselineCommit != "a1b2c3d" {
		t.Errorf("src.BaselineCommit = %q", src.BaselineCommit)
	}
	if src.BaselineEvidencePath != "docs/superpowers/coverage/2026-07-30-order-creation-baseline.json" {
		t.Errorf("src.BaselineEvidencePath = %q", src.BaselineEvidencePath)
	}
	if src.BaselineSHA256 != "3b1f2a9c" {
		t.Errorf("src.BaselineSHA256 = %q", src.BaselineSHA256)
	}
	if src.LiveProfile != ".test-design/live-profiles/local.json" {
		t.Errorf("src.LiveProfile = %q", src.LiveProfile)
	}
	if src.Runner.Kind != "go-test-json" || src.Runner.Root != "." {
		t.Errorf("src.Runner = %+v", src.Runner)
	}
	if len(src.Runner.Command) != 5 {
		t.Errorf("src.Runner.Command = %v, want 5 elements", src.Runner.Command)
	}
	if len(src.Runner.ReportGlobs) != 0 {
		t.Errorf("src.Runner.ReportGlobs = %v, want empty", src.Runner.ReportGlobs)
	}
	if len(src.FinalCommand) != 2 {
		t.Errorf("src.FinalCommand = %v, want 2 elements", src.FinalCommand)
	}

	wantIDs := []string{"SC-01", "SC-02", "orders/SC-03"}
	if len(src.Rows) != len(wantIDs) {
		t.Fatalf("src.Rows has %d rows, want %d: %+v", len(src.Rows), len(wantIDs), src.Rows)
	}
	for i, id := range wantIDs {
		if src.Rows[i].ID != id {
			t.Errorf("src.Rows[%d].ID = %q, want %q", i, src.Rows[i].ID, id)
		}
	}

	if len(src.Retired) != 1 || src.Retired[0] != "SC-04" {
		t.Errorf("src.Retired = %v, want [SC-04]", src.Retired)
	}

	wantSHA, err := evidence.SHA256File(path)
	if err != nil {
		t.Fatalf("evidence.SHA256File: %v", err)
	}
	if src.SHA256 != wantSHA {
		t.Errorf("src.SHA256 = %q, want %q", src.SHA256, wantSHA)
	}
}

func TestParseSourceMissingFileErrorsNamingPath(t *testing.T) {
	path := filepath.Join("testdata", "does-not-exist.md")

	src, err := ParseSource(path)
	if err == nil {
		t.Fatalf("ParseSource returned nil error with %+v, want error", src)
	}
	if src != nil {
		t.Errorf("ParseSource returned %+v alongside error, want nil *Source", src)
	}
	wantErrContains(t, err, path)
}

func TestParseSourceOldHeaderRefusesWithMigrationMessage(t *testing.T) {
	path := filepath.Join("testdata", "old-header-source.md")

	src, err := ParseSource(path)
	if err == nil {
		t.Fatalf("ParseSource returned nil error with %+v, want migration refusal", src)
	}
	if src != nil {
		t.Errorf("ParseSource returned %+v alongside error, want nil *Source", src)
	}
	wantErrContains(t, err, path)
	wantErrContains(t, err, "Bug it catches")
}

func TestParseSourceDuplicateIDAcrossTablesErrors(t *testing.T) {
	path := filepath.Join("testdata", "duplicate-id-source.md")

	src, err := ParseSource(path)
	if err == nil {
		t.Fatalf("ParseSource returned nil error with %+v, want error", src)
	}
	if src != nil {
		t.Errorf("ParseSource returned %+v alongside error, want nil *Source", src)
	}
	wantErrContains(t, err, path)
	wantErrContains(t, err, "SC-01")
}

func TestParseSourceMissingVerificationSectionErrors(t *testing.T) {
	path := filepath.Join("testdata", "missing-verification-source.md")

	src, err := ParseSource(path)
	if err == nil {
		t.Fatalf("ParseSource returned nil error with %+v, want error", src)
	}
	if src != nil {
		t.Errorf("ParseSource returned %+v alongside error, want nil *Source", src)
	}
	wantErrContains(t, err, path)
}

func TestParseSourceMalformedRowInTableFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-row.md")
	content := `# Coverage: x

Spec: docs/x.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/baseline.json
- baseline-sha256: abc123
- final-command: ["/verify"]

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| not-an-id | b | e | unit | m | low |
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	src, err := ParseSource(path)
	if err == nil {
		t.Fatalf("ParseSource returned nil error with %+v, want error", src)
	}
	if src != nil {
		t.Errorf("ParseSource returned %+v alongside error, want nil *Source", src)
	}
	wantErrContains(t, err, "not-an-id")
}
