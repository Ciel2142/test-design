package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// retiredModuleTestSource is the Go module under test: SC-01 is exercised by
// a passing test, and SC-02 (the highest scenario number in the document,
// retired) is still cited by a passing test so trace observes the citation
// rather than a gap.
const retiredModuleTestSource = `package retiredproof

import "testing"

// covers: SC-01
func TestActiveRowPasses(t *testing.T) {}

// covers: SC-02
func TestRetiredRowStillCited(t *testing.T) {}
`

// retiredHighestCoverageDoc retires SC-02, the highest scenario number this
// document ever defined; only SC-01 remains active.
const retiredHighestCoverageDoc = `# Coverage: retired highest id

Spec: docs/superpowers/specs/2026-09-07-retired-highest-design.md
Baseline: 0000000 · 0 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-retired-highest-baseline.json
- baseline-sha256: 0000000000000000000000000000000000000000000000000000000000000000
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-retired-highest-coverage.md"]

## Retired

- SC-02 superseded; kept for traceability and still cited by a drift-detection test

## Scenarios

### Behavior: retirement keeps the number reserved

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | active row is exercised | TestActiveRowPasses passes | unit | condition inverted | low |
`

// mustWriteFileAt writes contents to path, creating parent directories as
// needed, or fails the test.
func mustWriteFileAt(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// buildRetiredHighestRepo builds a temporary Git repository containing a
// minimal Go module and coverage source whose highest-numbered scenario ID,
// SC-02, is retired: SC-01 is the document's only active row.
func buildRetiredHighestRepo(t *testing.T) (repo, coverageSource string) {
	t.Helper()
	isolateEnv(t)
	repo = t.TempDir()

	mustWriteFileAt(t, filepath.Join(repo, "go.mod"), "module retiredproof\n\ngo 1.26\n")
	mustWriteFileAt(t, filepath.Join(repo, "retired_test.go"), retiredModuleTestSource)

	coverageSource = filepath.Join("docs", "superpowers", "coverage", "2026-09-07-retired-highest-coverage.md")
	mustWriteFileAt(t, filepath.Join(repo, coverageSource), retiredHighestCoverageDoc)

	mustGit(t, repo, "init", "-q")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "retired highest id fixture")

	return repo, filepath.ToSlash(coverageSource)
}

// covers: SC-01
func TestNextIDReservesRetiredHighestAndTraceExcludesRetired(t *testing.T) {
	repo, coverageSource := buildRetiredHighestRepo(t)

	// next-id must reserve the retired highest number: SC-02 is retired but
	// still occupies its number, so the next allocation is 3.
	var nextStdout, nextStderr bytes.Buffer
	code := run([]string{"next-id", "--repo", repo}, nil, &nextStdout, &nextStderr)
	if code != exitOK {
		t.Fatalf("next-id exit code = %d, want %d; stderr=%s", code, exitOK, nextStderr.String())
	}
	nextOut := decodeNextID(t, &nextStdout)
	unprefixed, ok := entryFor(nextOut.NextIDs, "")
	if !ok {
		t.Fatalf("no entry for unprefixed area in %+v", nextOut.NextIDs)
	}
	if unprefixed.Next != 3 {
		t.Errorf("next-id next = %d, want 3 (SC-02 retired must still occupy its number)", unprefixed.Next)
	}

	// trace must account only the active row and report the retired
	// citation as drift, never as a binding.
	requestBody, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": coverageSource,
		"runID":          "retired-highest",
	})
	if err != nil {
		t.Fatalf("marshal trace request: %v", err)
	}
	code, stdout, stderr := runVerifyOp(t, "trace", string(requestBody))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	out := decodeTraceOutput(t, stdout)
	ev := readTraceEvidence(t, out)

	if ev.RowAccounting.ActiveRows != 1 {
		t.Errorf("rowAccounting.activeRows = %d, want 1 (retired SC-02 excluded)", ev.RowAccounting.ActiveRows)
	}
	if _, ok := rowResultFor(ev, "SC-02"); ok {
		t.Error("retired SC-02 produced a RowResult, want none: retirement must not be creditable")
	}
	wantRowResult(t, ev, "SC-01", "passed")
	wantFinding(t, ev, "drift", "", "retired ID SC-02")
}
