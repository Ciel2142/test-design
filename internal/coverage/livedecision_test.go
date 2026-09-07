package coverage

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// covers: SC-06
func TestValidateLiveDecisionIncompleteForms(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		wantCode   string
		wantDetail []string
	}{
		{name: "no section at all", file: "live-incomplete-missing.md", wantCode: "missing"},
		{name: "not-applicable without a reason", file: "live-incomplete-reason-missing.md", wantCode: "reason-missing"},
		{name: "not-applicable with an active e2e row", file: "live-incomplete-contradiction-rows.md", wantCode: "contradiction", wantDetail: []string{"SC-02"}},
		{name: "not-applicable with journeys", file: "live-incomplete-contradiction-journeys.md", wantCode: "contradiction", wantDetail: []string{"J-1"}},
		{name: "required with no journeys", file: "live-incomplete-journeys-missing.md", wantCode: "journeys-missing"},
		{name: "journey cites an absent id", file: "live-incomplete-journey-unknown-row.md", wantCode: "journey-unknown-row", wantDetail: []string{"J-1", "SC-99"}},
		{name: "active smoke row in no journey", file: "live-incomplete-row-unjourneyed.md", wantCode: "live-row-unjourneyed", wantDetail: []string{"orders/SC-05"}},
		{name: "required without a live profile", file: "live-incomplete-target-missing.md", wantCode: "target-missing", wantDetail: []string{"live-profile"}},
		{name: "complete required decision", file: "live-required-source.md", wantCode: ""},
		{name: "justified not-applicable", file: "live-na-source.md", wantCode: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			src, err := ParseSource(path)
			if err != nil {
				t.Fatalf("ParseSource(%s): %v", path, err)
			}

			err = ValidateLiveDecision(src)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateLiveDecision(%s) = %v, want nil", tc.file, err)
				}
				return
			}

			var lde *LiveDecisionError
			if !errors.As(err, &lde) {
				t.Fatalf("ValidateLiveDecision(%s) = %v, want *LiveDecisionError with code %q", tc.file, err, tc.wantCode)
			}
			if lde.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q (detail %q)", lde.Code, tc.wantCode, lde.Detail)
			}
			if lde.Path != path {
				t.Errorf("path = %q, want %q", lde.Path, path)
			}
			for _, want := range tc.wantDetail {
				if !strings.Contains(lde.Detail, want) {
					t.Errorf("detail %q does not name %q", lde.Detail, want)
				}
			}
			if !strings.Contains(lde.Error(), tc.wantCode) || !strings.Contains(lde.Error(), path) {
				t.Errorf("Error() = %q, want it to name the path and the code", lde.Error())
			}
		})
	}
}

func TestParseSourceWithoutLiveDecisionSectionYieldsNilDecision(t *testing.T) {
	path := filepath.Join("testdata", "valid-source.md")

	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if src.LiveDecision != nil {
		t.Fatalf("src.LiveDecision = %+v, want nil for a document without the section", src.LiveDecision)
	}
	if len(src.Rows) != 3 {
		t.Errorf("src.Rows has %d rows, want the legacy document to keep parsing", len(src.Rows))
	}
}

func TestParseSourceLiveDecisionParsesBulletsAndJourneys(t *testing.T) {
	path := filepath.Join("testdata", "live-required-source.md")

	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	d := src.LiveDecision
	if d == nil {
		t.Fatalf("src.LiveDecision = nil, want a parsed decision")
	}
	if d.Decision != LiveRequired {
		t.Errorf("Decision = %q, want %q", d.Decision, LiveRequired)
	}
	if d.Reason != "the checkout path is only observable on a deployed revision" {
		t.Errorf("Reason = %q", d.Reason)
	}
	if len(d.Journeys) != 1 {
		t.Fatalf("Journeys = %+v, want one journey", d.Journeys)
	}
	j := d.Journeys[0]
	if j.ID != "J-1" {
		t.Errorf("journey ID = %q, want J-1", j.ID)
	}
	wantScenarios := []string{"SC-02", "orders/SC-05"}
	if !reflect.DeepEqual(j.Scenarios, wantScenarios) {
		t.Errorf("journey Scenarios = %v, want %v (a prefixed id is accepted)", j.Scenarios, wantScenarios)
	}
	if !strings.Contains(j.Expected, "deployed ledger") {
		t.Errorf("journey Expected = %q", j.Expected)
	}
}

func TestParseSourceLiveDecisionIgnoresProse(t *testing.T) {
	section := `Required, not N/A. The deployed checkout journey is the only observation point.

- decision: required
- reason: only observable on a deployed revision

The journey below is the single live selection source for the live handoff.

| Journey | Scenarios | Expected |
|---|---|---|
| J-1 | SC-01 | the ledger row exists |

Setup proof: docs/setup.json records the recipe execution.`

	path, _ := writeLiveDoc(t, section)

	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	d := src.LiveDecision
	if d == nil {
		t.Fatalf("src.LiveDecision = nil, want prose lines ignored and the bullets parsed")
	}
	if d.Decision != LiveRequired || d.Reason != "only observable on a deployed revision" {
		t.Errorf("decision = %q, reason = %q", d.Decision, d.Reason)
	}
	if len(d.Journeys) != 1 || d.Journeys[0].ID != "J-1" {
		t.Errorf("Journeys = %+v, want exactly J-1", d.Journeys)
	}
	if err := ValidateLiveDecision(src); err != nil {
		t.Errorf("ValidateLiveDecision = %v, want nil", err)
	}
}

func TestParseSourceLiveDecisionMalformedFormsFailClosed(t *testing.T) {
	cases := []struct {
		name      string
		section   string
		extraRows []string
		offending string
	}{
		{
			name:      "decision key absent",
			section:   "- reason: only observable on a deployed revision",
			offending: "## Live decision",
		},
		{
			name:      "decision value empty",
			section:   "- decision:",
			offending: "- decision:",
		},
		{
			name:      "decision value unknown",
			section:   "- decision: maybe",
			offending: "- decision: maybe",
		},
		{
			name:      "key repeats",
			section:   "- decision: required\n- decision: not-applicable",
			offending: "- decision: not-applicable",
		},
		{
			name:      "malformed journey token",
			section:   "- decision: required\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J1 | SC-01 | the ledger row exists |",
			offending: "| J1 | SC-01 | the ledger row exists |",
		},
		{
			name:      "empty scenarios cell",
			section:   "- decision: required\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-1 |  | the ledger row exists |",
			offending: "| J-1 |  | the ledger row exists |",
		},
		{
			name:      "scenario token is not an id",
			section:   "- decision: required\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-1 | SC-01, nope | the ledger row exists |",
			offending: "| J-1 | SC-01, nope | the ledger row exists |",
		},
		{
			name:      "empty expected cell",
			section:   "- decision: required\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-1 | SC-01 |  |",
			offending: "| J-1 | SC-01 |  |",
		},
		{
			name:      "journey row with too few columns",
			section:   "- decision: required\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-1 | SC-01 |",
			offending: "| J-1 | SC-01 |",
		},
		{
			name:      "second journeys table",
			section:   "- decision: required\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-1 | SC-01 | first |\n\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-2 | SC-01 | second |",
			offending: "| J-2 | SC-01 | second |",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, content := writeLiveDoc(t, tc.section, tc.extraRows...)

			src, err := ParseSource(path)
			if err == nil {
				t.Fatalf("ParseSource returned nil error with %+v, want a closed failure", src)
			}
			if src != nil {
				t.Errorf("ParseSource returned %+v alongside the error, want nil *Source", src)
			}
			wantErrContains(t, err, path)
			wantErrContains(t, err, lineRef(t, content, tc.offending))
		})
	}
}

func TestParseSourceJourneysTableIsInertToRowsAndRegistry(t *testing.T) {
	journeys := "\n| Journey | Scenarios | Expected |\n|---|---|---|\n| J-1 | SC-02 | the ledger row exists |"
	withPath, withContent := writeLiveDoc(t, "- decision: required"+journeys, rowE2E)
	withoutPath, _ := writeLiveDoc(t, "- decision: not-applicable\n- reason: no deployed surface")

	withSrc, err := ParseSource(withPath)
	if err != nil {
		t.Fatalf("ParseSource(with journeys): %v", err)
	}
	withoutSrc, err := ParseSource(withoutPath)
	if err != nil {
		t.Fatalf("ParseSource(without journeys): %v", err)
	}
	if len(withoutSrc.Rows) != 1 {
		t.Fatalf("control document has %d rows, want 1", len(withoutSrc.Rows))
	}
	if len(withSrc.Rows) != 2 || withSrc.Rows[0].ID != "SC-01" || withSrc.Rows[1].ID != "SC-02" {
		t.Errorf("rows = %+v, want only the scenario table harvested", withSrc.Rows)
	}

	root := t.TempDir()
	coverageDir := filepath.Join(root, "docs", "superpowers", "coverage")
	if err := os.MkdirAll(coverageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(coverageDir, "journeys-coverage.md"), []byte(withContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg, err := BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	if _, ok := reg.Owner("J-1"); ok {
		t.Errorf("Owner(J-1) found, want the journeys table inert to the registry")
	}
	for _, id := range []string{"SC-01", "SC-02"} {
		owner, ok := reg.Owner(id)
		if !ok {
			t.Fatalf("Owner(%s) not found", id)
		}
		if !owner.Active {
			t.Errorf("Owner(%s).Active = false, want true", id)
		}
	}
	next, err := reg.NextID("")
	if err != nil {
		t.Fatalf("NextID: %v", err)
	}
	if next != 3 {
		t.Errorf("NextID = %d, want 3 (the journeys table contributes no id)", next)
	}
}

func TestValidateLiveDecisionUnjourneyedRowsCountActiveRowsOnly(t *testing.T) {
	e2eRow := Row{ID: "SC-02", Level: "e2e"}
	smokeRow := Row{ID: "SC-03", Level: "smoke"}
	unitRow := Row{ID: "SC-01", Level: "unit"}

	cases := []struct {
		name     string
		src      *Source
		wantCode string
	}{
		{
			name: "every active live row is journeyed",
			src: &Source{
				Path: "doc.md", LiveProfile: "profiles/local.json",
				Rows: []Row{unitRow, e2eRow, smokeRow},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-02", "SC-03"}, Expected: "observed"},
				}},
			},
		},
		{
			name: "an unjourneyed live row that is retired is not required",
			src: &Source{
				Path: "doc.md", LiveProfile: "profiles/local.json",
				Rows: []Row{unitRow, e2eRow, smokeRow}, Retired: []string{"SC-03"},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-02"}, Expected: "observed"},
				}},
			},
		},
		{
			name: "an active smoke row in no journey fires",
			src: &Source{
				Path: "doc.md", LiveProfile: "profiles/local.json",
				Rows: []Row{unitRow, e2eRow, smokeRow},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-02"}, Expected: "observed"},
				}},
			},
			wantCode: "live-row-unjourneyed",
		},
		{
			name: "a journey citing a retired row is unknown",
			src: &Source{
				Path: "doc.md", LiveProfile: "profiles/local.json",
				Rows: []Row{unitRow, e2eRow, smokeRow}, Retired: []string{"SC-03"},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-02", "SC-03"}, Expected: "observed"},
				}},
			},
			wantCode: "journey-unknown-row",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantCodeOrNil(t, ValidateLiveDecision(tc.src), tc.wantCode)
		})
	}
}

func TestValidateLiveDecisionReturnsTheFirstCodeInOrder(t *testing.T) {
	cases := []struct {
		name     string
		src      *Source
		wantCode string
	}{
		{
			name: "no decision outranks every other fault",
			src: &Source{
				Path: "doc.md",
				Rows: []Row{{ID: "SC-02", Level: "e2e"}},
			},
			wantCode: "missing",
		},
		{
			name: "a missing reason outranks the contradiction it sits on",
			src: &Source{
				Path:         "doc.md",
				Rows:         []Row{{ID: "SC-02", Level: "e2e"}},
				LiveDecision: &LiveDecision{Decision: LiveNotApplicable},
			},
			wantCode: "reason-missing",
		},
		{
			name: "missing journeys outrank a missing target",
			src: &Source{
				Path:         "doc.md",
				Rows:         []Row{{ID: "SC-02", Level: "e2e"}},
				LiveDecision: &LiveDecision{Decision: LiveRequired},
			},
			wantCode: "journeys-missing",
		},
		{
			name: "an unknown cited row outranks an unjourneyed row and a missing target",
			src: &Source{
				Path: "doc.md",
				Rows: []Row{{ID: "SC-02", Level: "e2e"}, {ID: "SC-03", Level: "smoke"}},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-99"}, Expected: "observed"},
				}},
			},
			wantCode: "journey-unknown-row",
		},
		{
			name: "an unjourneyed row outranks a missing target",
			src: &Source{
				Path: "doc.md",
				Rows: []Row{{ID: "SC-02", Level: "e2e"}, {ID: "SC-03", Level: "smoke"}},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-02"}, Expected: "observed"},
				}},
			},
			wantCode: "live-row-unjourneyed",
		},
		{
			name: "a blank live profile is a missing target",
			src: &Source{
				Path: "doc.md", LiveProfile: "   ",
				Rows: []Row{{ID: "SC-02", Level: "e2e"}},
				LiveDecision: &LiveDecision{Decision: LiveRequired, Journeys: []Journey{
					{ID: "J-1", Scenarios: []string{"SC-02"}, Expected: "observed"},
				}},
			},
			wantCode: "target-missing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantCodeOrNil(t, ValidateLiveDecision(tc.src), tc.wantCode)
		})
	}
}

func wantCodeOrNil(t *testing.T, err error, wantCode string) {
	t.Helper()
	if wantCode == "" {
		if err != nil {
			t.Fatalf("ValidateLiveDecision = %v, want nil", err)
		}
		return
	}
	var lde *LiveDecisionError
	if !errors.As(err, &lde) {
		t.Fatalf("ValidateLiveDecision = %v, want *LiveDecisionError with code %q", err, wantCode)
	}
	if lde.Code != wantCode {
		t.Fatalf("code = %q, want %q (detail %q)", lde.Code, wantCode, lde.Detail)
	}
}

const rowE2E = "| SC-02 | checkout completes on the deployed revision | order row present | e2e | commit skipped | high |"

// writeLiveDoc writes a coverage source whose ## Live decision section is
// section and whose scenario table is SC-01 plus extraRows, and returns its
// path and its content.
func writeLiveDoc(t *testing.T, section string, extraRows ...string) (path, content string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`# Coverage: inline live decision

Spec: docs/superpowers/specs/inline-design.md
Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped

## Verification

- runner-kind: go-test-json
- runner-root: .
- runner-command: ["go", "test"]
- report-globs: []
- baseline-evidence: docs/baseline.json
- baseline-sha256: abc123
- live-profile: .test-design/live-profiles/local.json
- final-command: ["/verify"]

## Live decision

`)
	b.WriteString(section)
	b.WriteString(`

## Scenarios

| ID | Behavior | Expected | Level | Verification mutation | Risk |
|----|----------|----------|-------|------------------------|------|
| SC-01 | quantity at the lower bound is rejected | throws, no row persisted | unit | guard uses < instead of <= | med |
`)
	for _, r := range extraRows {
		b.WriteString(r + "\n")
	}
	content = b.String()

	path = filepath.Join(t.TempDir(), "inline-coverage.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path, content
}

// lineRef returns the "line N:" reference a fail-closed parse error must
// carry for the offending line, which must occur exactly once in content.
func lineRef(t *testing.T, content, offending string) string {
	t.Helper()
	found := 0
	number := 0
	for i, line := range strings.Split(content, "\n") {
		if strings.TrimRight(line, " ") == strings.TrimRight(offending, " ") {
			found++
			number = i + 1
		}
	}
	if found != 1 {
		t.Fatalf("offending line %q occurs %d times in the document, want exactly once", offending, found)
	}
	return "line " + strconv.Itoa(number) + ":"
}
