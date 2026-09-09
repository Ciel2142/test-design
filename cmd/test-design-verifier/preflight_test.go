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
	source := filepath.Join("docs", "superpowers", "coverage", "2026-08-26-widgets-coverage.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodePreflight(t, &stdout)

	wantSource := "docs/superpowers/coverage/2026-08-26-widgets-coverage.md"
	if got.Source != wantSource {
		t.Errorf("Source = %q, want the repo-root-relative slash-separated form %q", got.Source, wantSource)
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
		t.Errorf("LiveProfileRequired = false, want true (decision required)")
	}
	if got.LiveProfile != ".test-design/live-profiles/local.json" {
		t.Errorf("LiveProfile = %q", got.LiveProfile)
	}
	if got.LiveDecision.Decision != "required" {
		t.Errorf("LiveDecision.Decision = %q, want %q", got.LiveDecision.Decision, "required")
	}
	if len(got.LiveDecision.Journeys) != 1 {
		t.Fatalf("LiveDecision.Journeys = %+v, want exactly 1 journey", got.LiveDecision.Journeys)
	}
	j := got.LiveDecision.Journeys[0]
	if j.ID != "J-1" || len(j.Scenarios) != 2 || j.Scenarios[0] != "SC-03" || j.Scenarios[1] != "SC-04" {
		t.Errorf("LiveDecision.Journeys[0] = %+v, want J-1 over SC-03, SC-04", j)
	}
}

// TestPreflightHandler_RequiredDecisionMissingLiveProfileRefused replaces the
// pre-SC-06 TestPreflightHandler_MissingLiveProfileReportsRequiredWithEmptyPath:
// under the decision-derived rule, a required decision with no live-profile
// key is no longer a successful output with an empty LiveProfile — it is
// refused by ValidateLiveDecision's target-missing code before layers are
// computed.
func TestPreflightHandler_RequiredDecisionMissingLiveProfileRefused(t *testing.T) {
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

## Live decision

- decision: required

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | a downed dependency answers 503 on the deployed revision |

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

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want %q", got.Code, "live-decision-incomplete")
	}
	if !strings.Contains(got.Message, "target-missing") {
		t.Errorf("error message = %q, want it to contain %q", got.Message, "target-missing")
	}
	if !strings.Contains(got.Message, "live-profile") {
		t.Errorf("error message = %q, want it to name %q", got.Message, "live-profile")
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
	if got.Code != "missing-source" {
		t.Errorf("error code = %q, want %q", got.Code, "missing-source")
	}
}

func TestPreflightHandler_RegistryCollisionSurfacesBeforeMissingSourceFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "collision-repo")
	code := run([]string{"preflight", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "registry-error" {
		t.Errorf("error code = %q, want %q (the registry is validated before --source)", got.Code, "registry-error")
	}
	if !strings.Contains(got.Message, "SC-03") {
		t.Errorf("error message = %q, want it to name the colliding id SC-03", got.Message)
	}
}

func TestPreflightHandler_RetiredRowExcludedFromLayerCounts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "retired-row-repo")
	source := filepath.Join("docs", "superpowers", "coverage", "2026-08-26-retired-coverage.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodePreflight(t, &stdout)

	// SC-02 sits in the scenario table and under ## Retired; only SC-01 is active.
	if got.Layers.Layer1 != 1 || got.Layers.Layer2 != 1 {
		t.Errorf("Layers = %+v, want layer1 1 and layer2 1 (retired SC-02 excluded)", got.Layers)
	}
	if got.Layers.Layer3 != 0 {
		t.Errorf("Layers.Layer3 = %d, want 0 (retired SC-02 is the only e2e row)", got.Layers.Layer3)
	}
	if got.Layers.Layer4 != 0 {
		t.Errorf("Layers.Layer4 = %d, want 0 (retired SC-02 is the only high-risk row)", got.Layers.Layer4)
	}
	if got.HighRiskCount != 0 {
		t.Errorf("HighRiskCount = %d, want 0", got.HighRiskCount)
	}
	if got.LiveProfileRequired {
		t.Errorf("LiveProfileRequired = true, want false (decision not-applicable: the only e2e row, SC-02, is retired)")
	}
	if got.LiveDecision.Decision != "not-applicable" {
		t.Errorf("LiveDecision.Decision = %q, want %q", got.LiveDecision.Decision, "not-applicable")
	}
	if !strings.Contains(stdout.String(), `"journeys":[]`) {
		t.Errorf("stdout = %s, want liveDecision.journeys rendered as [] (not null) when there are no journeys", stdout.String())
	}
}

func TestPreflightHandler_SourceOutsideRepoNamesBothPaths(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	source := filepath.Join(outside, "2026-08-26-stray-coverage.md")
	if err := os.WriteFile(source, []byte("# stray\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "source-outside-repo" {
		t.Errorf("error code = %q, want %q", got.Code, "source-outside-repo")
	}
	if !strings.Contains(got.Message, source) || !strings.Contains(got.Message, repo) {
		t.Errorf("error message = %q, want it to name both %q and %q", got.Message, source, repo)
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
	source := filepath.Join("docs", "superpowers", "coverage", "2026-08-26-legacy-coverage.md")
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
	source := filepath.Join("docs", "superpowers", "coverage", "2026-08-26-x-coverage.md")
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

func TestPreflightHandler_LiveDecisionMissingRefused(t *testing.T) {
	dir := t.TempDir()
	coverageDir := filepath.Join(dir, "docs", "superpowers", "coverage")
	if err := os.MkdirAll(coverageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	source := filepath.Join(coverageDir, "2026-08-26-no-decision-coverage.md")
	content := `# Coverage: no-decision

Spec: docs/superpowers/specs/2026-08-26-no-decision-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-no-decision-baseline.json
- baseline-sha256: abc123
- final-command: ["/verify"]

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws error, no row persisted | unit | guard omits equality check | med |
`
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"preflight", "--repo", dir, "--source", source}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want %q", got.Code, "live-decision-incomplete")
	}
	if !strings.Contains(got.Message, "no ## Live decision section") {
		t.Errorf("message = %q, want the missing code's own detail, not any refusal carrying \"missing\"", got.Message)
	}
}

func TestPreflightHandler_NotApplicableContradictingSmokeRowRefused(t *testing.T) {
	dir := t.TempDir()
	coverageDir := filepath.Join(dir, "docs", "superpowers", "coverage")
	if err := os.MkdirAll(coverageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	source := filepath.Join(coverageDir, "2026-08-26-contradiction-coverage.md")
	content := `# Coverage: contradiction

Spec: docs/superpowers/specs/2026-08-26-contradiction-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-contradiction-baseline.json
- baseline-sha256: abc123
- final-command: ["/verify"]

## Live decision

- decision: not-applicable
- reason: every behavior is observable in process

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

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want %q", got.Code, "live-decision-incomplete")
	}
	if !strings.Contains(got.Message, "contradiction") {
		t.Errorf("error message = %q, want it to contain %q", got.Message, "contradiction")
	}
	if !strings.Contains(got.Message, "SC-01") {
		t.Errorf("error message = %q, want it to name the contradicting row SC-01", got.Message)
	}
}

// TestPreflightHandler_JourneyedIntegrationRowCountsInLayer3 locks the
// approval gate to the same layer-3 obligation the live ops carry: the count
// is the applicable set the live decision declares, so an integration row a
// required journey names is part of it even though no level filter would
// select it.
func TestPreflightHandler_JourneyedIntegrationRowCountsInLayer3(t *testing.T) {
	dir := t.TempDir()
	coverageDir := filepath.Join(dir, "docs", "superpowers", "coverage")
	if err := os.MkdirAll(coverageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	source := filepath.Join(coverageDir, "2026-08-26-journeyed-integration-coverage.md")
	content := `# Coverage: journeyed-integration

Spec: docs/superpowers/specs/2026-08-26-journeyed-integration-design.md
Baseline: a1b2c3d · 2 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-journeyed-integration-baseline.json
- baseline-sha256: abc123
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/verify"]

## Live decision

- decision: required
- reason: the checkout walkthrough is only observable on a deployed revision

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01, SC-02 | the order appears in the deployed ledger |

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the ledger records the accepted order | one ledger row per order | integration | write skipped | med |
| SC-02 | the deployed revision answers the health probe | 200 returned | smoke | probe route dropped | med |
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

	if got.Layers.Layer3 != 2 {
		t.Errorf("Layers.Layer3 = %d, want 2 (journeyed SC-01 integration plus SC-02 smoke)", got.Layers.Layer3)
	}
	if got.Layers.Layer1 != 2 || got.Layers.Layer2 != 2 {
		t.Errorf("Layers = %+v, want layer1 2 and layer2 2 unchanged by the live decision", got.Layers)
	}
}

// TestPreflightHandler_RequiredDecisionEmitsJourneysAndRequiresProfile locks
// that LiveProfileRequired is derived from the decision, not from a row's
// level: this document has no smoke or e2e row at all, yet its decision is
// required (its journey cites a unit row), so LiveProfileRequired is true and
// that journeyed unit row is the whole of layer 3.
func TestPreflightHandler_RequiredDecisionEmitsJourneysAndRequiresProfile(t *testing.T) {
	dir := t.TempDir()
	coverageDir := filepath.Join(dir, "docs", "superpowers", "coverage")
	if err := os.MkdirAll(coverageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	source := filepath.Join(coverageDir, "2026-08-26-required-no-live-rows-coverage.md")
	content := `# Coverage: required-no-live-rows

Spec: docs/superpowers/specs/2026-08-26-required-no-live-rows-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-08-26-required-no-live-rows-baseline.json
- baseline-sha256: abc123
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/verify"]

## Live decision

- decision: required
- reason: the checkout path is only observable on a deployed revision

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | the order appears in the deployed ledger |

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws error, no row persisted | unit | guard omits equality check | med |
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

	if got.Layers.Layer3 != 1 {
		t.Errorf("Layers.Layer3 = %d, want 1 (the journeyed unit row SC-01, no smoke or e2e row in this document)", got.Layers.Layer3)
	}
	if !got.LiveProfileRequired {
		t.Errorf("LiveProfileRequired = false, want true (decision required, even with no smoke/e2e row)")
	}
	if got.LiveDecision.Decision != "required" {
		t.Errorf("LiveDecision.Decision = %q, want %q", got.LiveDecision.Decision, "required")
	}
	if len(got.LiveDecision.Journeys) != 1 {
		t.Fatalf("LiveDecision.Journeys = %+v, want exactly 1 journey", got.LiveDecision.Journeys)
	}
	j := got.LiveDecision.Journeys[0]
	if j.ID != "J-1" {
		t.Errorf("Journeys[0].ID = %q, want %q", j.ID, "J-1")
	}
	if len(j.Scenarios) != 1 || j.Scenarios[0] != "SC-01" {
		t.Errorf("Journeys[0].Scenarios = %+v, want [\"SC-01\"]", j.Scenarios)
	}
	if j.Expected != "the order appears in the deployed ledger" {
		t.Errorf("Journeys[0].Expected = %q", j.Expected)
	}
}
