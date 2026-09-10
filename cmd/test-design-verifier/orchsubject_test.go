package main

import (
	"testing"
)

// This file holds the subject the orchestration proof runs against: a
// five-package Go repository, one row and one review group per package, so a
// single edit refuses exactly one group and leaves the other four — and the
// mutation rows they carry — adoptable. It deploys a binary as well, which is
// what gives the run a live obligation nothing may adopt.

const (
	orchCoverageSource = "docs/superpowers/coverage/2026-09-07-orch-subject-coverage.md"
	orchProfilePath    = ".test-design/live-profiles/orch-local.json"

	// The subject's five rows. SC-01 is the one the live journey walks;
	// SC-04 and SC-05 are high risk and therefore carry layer-4 obligations.
	orchGreetRow = "SC-01"
	orchCountRow = "SC-02"
	orchFlagRow  = "SC-03"
	orchLimitRow = "SC-04"
	orchTagRow   = "SC-05"

	// One group per row, so a change to one package's test file is isolated
	// to the one group that was handed it.
	orchGreetGroup = "G1"
	orchCountGroup = "G2"
	orchFlagGroup  = "G3"
	orchLimitGroup = "G4"
	orchTagGroup   = "G5"

	// The greeting the deployed binary prints, which is what the live
	// journey's predicate settles on.
	orchGreeting = "hello"
)

const orchGoMod = `module orchlive

go 1.26
`

const orchAlphaSource = `package alpha

// Greet is the greeting the deployed binary prints.
func Greet() string { return "hello" }
`

const orchBetaSource = `package beta

// Count is how many items the subject holds.
func Count() int { return 3 }
`

const orchGammaSource = `package gamma

// Flag is whether the subject's feature is on.
func Flag() bool { return true }
`

const orchDeltaSource = `package delta

import "fmt"

// Limit refuses a bound that is not positive.
func Limit(n int) error {
	if n <= 0 {
		return fmt.Errorf("limit must be positive, got %d", n)
	}
	return nil
}
`

const orchEpsilonSource = `package epsilon

// Tag is the subject's release tag.
func Tag() string { return "e1" }
`

// The subject's own tagged tests. Their coverage tags cite the subject
// document's rows, never a row of this repository: the text lives inside a
// string literal here and only becomes a comment in the temporary repository
// the builder writes.

const orchAlphaTestSource = `package alpha

import "testing"

// covers: SC-01
func TestGreet(t *testing.T) {
	if got := Greet(); got != "hello" {
		t.Fatalf("Greet() = %q, want hello", got)
	}
}
`

const orchBetaTestSource = `package beta

import "testing"

// covers: SC-02
func TestCount(t *testing.T) {
	if got := Count(); got != 3 {
		t.Fatalf("Count() = %d, want 3", got)
	}
}
`

const orchGammaTestSource = `package gamma

import "testing"

// covers: SC-03
func TestFlag(t *testing.T) {
	if got := Flag(); got != true {
		t.Fatalf("Flag() = %v, want true", got)
	}
}
`

const orchDeltaTestSource = `package delta

import "testing"

// covers: SC-04
func TestLimitRejectsZero(t *testing.T) {
	if err := Limit(0); err == nil {
		t.Fatalf("Limit(0) = nil, want the boundary refused")
	}
}
`

const orchEpsilonTestSource = `package epsilon

import "testing"

// covers: SC-05
func TestTag(t *testing.T) {
	if got := Tag(); got != "e1" {
		t.Fatalf("Tag() = %q, want e1", got)
	}
}
`

const orchMainSource = `package main

import (
	"fmt"

	"orchlive/alpha"
)

func main() { fmt.Println(alpha.Greet()) }
`

// orchMutatedDeltaSource is SC-04's verification mutation applied: the guard
// accepts zero. One line moves, so the diff is the single hunk the mutation
// ops admit.
const orchMutatedDeltaSource = `package delta

import "fmt"

// Limit refuses a bound that is not positive.
func Limit(n int) error {
	if n < 0 {
		return fmt.Errorf("limit must be positive, got %d", n)
	}
	return nil
}
`

// orchRepairedGammaTestSource is the repair the second run stands on: the same
// declaration, asserting more than it did — the flag is true and it is the same
// answer every call. It is an edit to the one file only SC-03's group was
// handed.
const orchRepairedGammaTestSource = `package gamma

import "testing"

// covers: SC-03
func TestFlag(t *testing.T) {
	if got := Flag(); got != true {
		t.Fatalf("Flag() = %v, want true", got)
	}
	if first, second := Flag(), Flag(); first != second {
		t.Fatalf("Flag() = %v then %v, want one stable answer", first, second)
	}
}
`

// orchProfile is the committed local profile the required decision names,
// shaped like the reuse subject's: it builds the subject's binary under the
// run's own name, proves the build landed, walks the binary, and removes it
// with a search that proves the removal. HOME and the Go environment are on the
// allowlist because the start step compiles.
const orchProfile = `{
  "schemaVersion": "1",
  "classification": "local",
  "start": [["go", "build", "-o", "orch-{run-id}", "./cmd/orch"]],
  "health": [["ls", "orch-{run-id}"]],
  "walkthrough": [["./orch-{run-id}"]],
  "cleanup": [
    ["rm", "-f", "orch-{run-id}"],
    ["find", ".", "-maxdepth", "1", "-name", "orch-{run-id}"]
  ],
  "cleanupEvidence": [
    {"form": "exit-code", "value": "0"},
    {"form": "not-matches", "stream": "stdout", "value": "orch-"}
  ],
  "envAllowlist": ["PATH", "HOME", "GOCACHE", "GOMODCACHE", "GOFLAGS", "GOTOOLCHAIN"],
  "resourcePrefixes": ["orch-{run-id}"],
  "endpoints": [],
  "timeoutSeconds": 120
}
`

// orchCoverageDoc is the subject's coverage document: five rows in five files,
// three med and two high risk, and a required live decision with one journey
// over SC-01. BASELINE_COMMIT and BASELINE_SHA256 are pinned by the shared
// subject builder.
const orchCoverageDoc = `# Coverage: orchestration subject

Spec: docs/superpowers/specs/2026-09-07-orch-subject.md
Baseline: BASELINE_COMMIT · 5 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go
- runner-root: .
- runner-command: ["go", "test", "-count=1", "-json", "./..."]
- report-globs: []
- baseline-evidence: docs/superpowers/coverage/2026-09-07-orch-subject-baseline.json
- baseline-sha256: BASELINE_SHA256
- live-profile: .test-design/live-profiles/orch-local.json
- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-09-07-orch-subject-coverage.md"]

## Live decision

- decision: required
- reason: the greeting is only observable from the built binary

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | Against the deployed binary the walkthrough prints hello |

## Scenarios

### Behavior: the subject's five packages

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | the subject reports its greeting | Greet() returns hello | unit | greeting string changed | med |
| SC-02 | the subject reports how many items it holds | Count() returns 3 | unit | count constant bumped | med |
| SC-03 | the subject reports whether its feature is on | Flag() returns true | unit | flag inverted | med |
| SC-04 | the subject refuses a bound that is not positive | Limit(0) returns an error | unit | guard accepts zero | high |
| SC-05 | the subject reports its release tag | Tag() returns e1 | unit | tag string changed | high |
`

// orchestrationSubject builds the orchestration proof's subject repository
// through the shared live-subject builder, which commits it, runs the real
// baseline op and freezes HEAD. Five packages, one tagged test each, one
// committed local profile and a binary the live journey walks.
func orchestrationSubject(t *testing.T) *liveSubject {
	t.Helper()

	files := map[string]string{
		"go.mod":                  orchGoMod,
		"alpha/alpha.go":          orchAlphaSource,
		"alpha/alpha_test.go":     orchAlphaTestSource,
		"beta/beta.go":            orchBetaSource,
		"beta/beta_test.go":       orchBetaTestSource,
		"gamma/gamma.go":          orchGammaSource,
		"gamma/gamma_test.go":     orchGammaTestSource,
		"delta/delta.go":          orchDeltaSource,
		"delta/delta_test.go":     orchDeltaTestSource,
		"epsilon/epsilon.go":      orchEpsilonSource,
		"epsilon/epsilon_test.go": orchEpsilonTestSource,
		"cmd/orch/main.go":        orchMainSource,
		orchProfilePath:           orchProfile,
		orchCoverageSource:        orchCoverageDoc,
	}

	return newLiveSubject(t, orchCoverageSource, orchProfilePath, []map[string]string{files}, nil)
}
