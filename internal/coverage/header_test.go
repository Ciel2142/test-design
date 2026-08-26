package coverage

import "testing"

func TestParseHeaderLinesExtractsSpecAndBaselineCommit(t *testing.T) {
	lines := []string{
		"# Coverage: order creation",
		"",
		"Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md",
		"Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped",
		"",
		"## Verification",
	}

	spec, baseline, err := parseHeaderLines("path.md", lines)
	if err != nil {
		t.Fatalf("parseHeaderLines: %v", err)
	}
	if spec != "docs/superpowers/specs/2026-07-30-order-creation-design.md" {
		t.Errorf("spec = %q", spec)
	}
	if baseline != "a1b2c3d" {
		t.Errorf("baseline = %q, want first whitespace-delimited token", baseline)
	}
}

func TestParseHeaderLinesMissingSpecErrors(t *testing.T) {
	lines := []string{
		"# Coverage: order creation",
		"",
		"Baseline: a1b2c3d · 1 passed, 0 failed, 0 skipped",
	}

	_, _, err := parseHeaderLines("path.md", lines)
	wantErrContains(t, err, "path.md")
	wantErrContains(t, err, "Spec")
}

func TestParseHeaderLinesMissingBaselineErrors(t *testing.T) {
	lines := []string{
		"# Coverage: order creation",
		"",
		"Spec: docs/superpowers/specs/2026-07-30-order-creation-design.md",
	}

	_, _, err := parseHeaderLines("path.md", lines)
	wantErrContains(t, err, "path.md")
	wantErrContains(t, err, "Baseline")
}
