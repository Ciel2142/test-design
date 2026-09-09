package main

import (
	"fmt"
	"testing"
)

// This file holds the subject the reuse proofs run against: a two-package Go
// repository small enough that a whole run — baseline, trace, review, mutation
// — is cheap, and split so each row's tagged test lives in a file no other row
// cites. That isolation is what lets one row's evidence be adopted while
// another's is refused, which is the whole point of the proofs above it.

const (
	readyCoverageSource = "docs/superpowers/coverage/2026-09-07-ready-subject-coverage.md"
	readyProfilePath    = ".test-design/live-profiles/ready-local.json"

	// The subject's two rows: SC-01 is med risk and carries no layer-4
	// obligation, SC-02 is high risk and therefore always carries one.
	readyMarkerRow  = "SC-01"
	readyVersionRow = "SC-02"

	// The groups the proofs bind those rows into, one row each, so a group can
	// be adopted or refused without dragging the other row with it.
	readyMarkerGroup  = "G1"
	readyVersionGroup = "G2"
)

const readyGoMod = `module readylive

go 1.26
`

const readyMarkerSource = `package ready

// Marker is the string the deployed binary prints once it is running.
func Marker() string { return "ready-v1" }
`

const readyVersionSource = `package ready

// Version is the subject's schema version.
func Version() int { return 1 }
`

// readyMarkerTestSource and readyVersionTestSource are the subject's own tagged
// tests. Their coverage tags cite the subject document's rows, never a row of
// this repository: the text lives inside a string literal here and only becomes
// a comment in the temporary repository the builder writes.
const readyMarkerTestSource = `package ready

import "testing"

// covers: SC-01
func TestMarker(t *testing.T) {
	if got := Marker(); got != "ready-v1" {
		t.Fatalf("Marker() = %q, want ready-v1", got)
	}
}
`

const readyVersionTestSource = `package ready

import "testing"

// covers: SC-02
func TestVersion(t *testing.T) {
	if got := Version(); got != 1 {
		t.Fatalf("Version() = %d, want 1", got)
	}
}
`

const readyMainSource = `package main

import (
	"fmt"

	"readylive/ready"
)

func main() { fmt.Println(ready.Marker()) }
`

// readyProfile is the committed local profile the required decision names: it
// builds the subject's binary under the run's own name, proves the build
// landed, walks the binary, and removes it with a search that proves the
// removal. GOCACHE and the rest of the Go environment are on the allowlist
// because the start step compiles.
const readyProfile = `{
  "schemaVersion": "1",
  "classification": "local",
  "start": [["go", "build", "-o", "ready-{run-id}", "./cmd/ready"]],
  "health": [["ls", "ready-{run-id}"]],
  "walkthrough": [["./ready-{run-id}"]],
  "cleanup": [
    ["rm", "-f", "ready-{run-id}"],
    ["find", ".", "-maxdepth", "1", "-name", "ready-{run-id}"]
  ],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "not-matches", "stream": "stdout", "value": "ready-"}
  ],
  "envAllowlist": ["PATH", "HOME", "GOCACHE", "GOMODCACHE", "GOFLAGS", "GOTOOLCHAIN"],
  "resourcePrefixes": ["ready-{run-id}"],
  "endpoints": [],
  "timeoutSeconds": 120
}
`

// readyCoverageDocTemplate is the subject's coverage document with the two
// halves the live decision changes left open: the ## Verification key that
// names the profile, and the ## Live decision section itself. BASELINE_COMMIT
// and BASELINE_SHA256 are pinned by the shared subject builder.
const readyCoverageDocTemplate = `# Coverage: ready reuse subject

Spec: docs/superpowers/specs/2026-09-07-ready-subject.md
Baseline: BASELINE_COMMIT · 2 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-ready-subject-baseline.json
- baseline-sha256: BASELINE_SHA256
%s- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-ready-subject-coverage.md"]

## Live decision

%s
## Scenarios

### Behavior: ready marker and version

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the package reports its ready marker | Marker() returns ready-v1 | unit | marker string changed | med |
| SC-02 | the package reports its schema version | Version() returns 1 | unit | version constant bumped | high |
`

const readyNotApplicableDecision = `- decision: not-applicable
- reason: both behaviors are observable in process; the subject deploys no surface this run must reach
`

const readyRequiredDecision = `- decision: required
- reason: the marker is only observable from the built binary

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | Against the deployed binary the walkthrough prints ready-v1 |
`

// readySubject builds the reuse proofs' subject repository through the shared
// live-subject builder, which commits it, runs the real baseline op and freezes
// HEAD. decision is the ## Live decision the document carries: not-applicable
// leaves layer 3 with no obligation at all, required names the committed local
// profile and one journey over SC-01.
func readySubject(t *testing.T, decision string) *liveSubject {
	t.Helper()

	files := map[string]string{
		"go.mod":                readyGoMod,
		"ready/ready.go":        readyMarkerSource,
		"ready/version.go":      readyVersionSource,
		"ready/ready_test.go":   readyMarkerTestSource,
		"ready/version_test.go": readyVersionTestSource,
		"cmd/ready/main.go":     readyMainSource,
	}

	profilePath, profileKey, decisionBlock := "", "", readyNotApplicableDecision
	switch decision {
	case "not-applicable":
	case "required":
		profilePath = readyProfilePath
		profileKey = "- live-profile: " + readyProfilePath + "\n"
		decisionBlock = readyRequiredDecision
		files[readyProfilePath] = readyProfile
	default:
		t.Fatalf("readySubject: live decision %q is neither not-applicable nor required", decision)
	}
	files[readyCoverageSource] = fmt.Sprintf(readyCoverageDocTemplate, profileKey, decisionBlock)

	return newLiveSubject(t, readyCoverageSource, profilePath, []map[string]string{files}, nil)
}
