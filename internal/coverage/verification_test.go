package coverage

import "testing"

func fullVerificationLines(overrides map[string]string) []string {
	values := map[string]string{
		"runner-kind":       "go-test-json",
		"runner-root":       ".",
		"runner-command":    `["go", "test", "-count=1", "-json", "./..."]`,
		"report-globs":      "[]",
		"baseline-evidence": "docs/superpowers/coverage/2026-07-30-order-creation-baseline.json",
		"baseline-sha256":   "3b1f2a9c",
		"live-profile":      ".test-design/live-profiles/local.json",
		"final-command":     `["/test-design:coverage-verification", "docs/superpowers/coverage/2026-07-30-order-creation-coverage.md"]`,
	}
	for k, v := range overrides {
		if v == "" {
			delete(values, k)
			continue
		}
		values[k] = v
	}

	order := []string{"runner-kind", "runner-root", "runner-command", "report-globs", "baseline-evidence", "baseline-sha256", "live-profile", "final-command"}
	lines := []string{"## Verification", ""}
	for _, k := range order {
		if v, ok := values[k]; ok {
			lines = append(lines, "- "+k+": "+v)
		}
	}
	lines = append(lines, "", "## Retired")
	return lines
}

func TestParseVerificationSectionExtractsAllFields(t *testing.T) {
	vf, err := parseVerificationSection("path.md", fullVerificationLines(nil))
	if err != nil {
		t.Fatalf("parseVerificationSection: %v", err)
	}
	if vf.Runner.Kind != "go-test-json" {
		t.Errorf("Runner.Kind = %q", vf.Runner.Kind)
	}
	if vf.Runner.Root != "." {
		t.Errorf("Runner.Root = %q", vf.Runner.Root)
	}
	wantCommand := []string{"go", "test", "-count=1", "-json", "./..."}
	if len(vf.Runner.Command) != len(wantCommand) {
		t.Fatalf("Runner.Command = %v, want %v", vf.Runner.Command, wantCommand)
	}
	for i, v := range wantCommand {
		if vf.Runner.Command[i] != v {
			t.Errorf("Runner.Command[%d] = %q, want %q", i, vf.Runner.Command[i], v)
		}
	}
	if len(vf.Runner.ReportGlobs) != 0 {
		t.Errorf("Runner.ReportGlobs = %v, want empty", vf.Runner.ReportGlobs)
	}
	if vf.BaselineEvidencePath != "docs/superpowers/coverage/2026-07-30-order-creation-baseline.json" {
		t.Errorf("BaselineEvidencePath = %q", vf.BaselineEvidencePath)
	}
	if vf.BaselineSHA256 != "3b1f2a9c" {
		t.Errorf("BaselineSHA256 = %q", vf.BaselineSHA256)
	}
	if vf.LiveProfile != ".test-design/live-profiles/local.json" {
		t.Errorf("LiveProfile = %q", vf.LiveProfile)
	}
	if len(vf.FinalCommand) != 2 {
		t.Fatalf("FinalCommand = %v, want 2 elements", vf.FinalCommand)
	}
}

func TestParseVerificationSectionLiveProfileIsOptional(t *testing.T) {
	vf, err := parseVerificationSection("path.md", fullVerificationLines(map[string]string{"live-profile": ""}))
	if err != nil {
		t.Fatalf("parseVerificationSection: %v", err)
	}
	if vf.LiveProfile != "" {
		t.Errorf("LiveProfile = %q, want empty when omitted", vf.LiveProfile)
	}
}

func TestParseVerificationSectionMissingSectionErrors(t *testing.T) {
	_, err := parseVerificationSection("path.md", []string{"# Title", "", "## Retired"})
	wantErrContains(t, err, "path.md")
	wantErrContains(t, err, "Verification")
}

func TestParseVerificationSectionRequiredKeyMissingNamesKeyAndPath(t *testing.T) {
	requiredKeys := []string{"runner-kind", "runner-root", "runner-command", "report-globs", "baseline-evidence", "baseline-sha256", "final-command"}
	for _, key := range requiredKeys {
		t.Run(key, func(t *testing.T) {
			_, err := parseVerificationSection("path.md", fullVerificationLines(map[string]string{key: ""}))
			wantErrContains(t, err, "path.md")
			wantErrContains(t, err, key)
		})
	}
}

func TestParseVerificationSectionNonArrayWhereArrayRequiredErrors(t *testing.T) {
	tests := []string{"runner-command", "report-globs", "final-command"}
	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := parseVerificationSection("path.md", fullVerificationLines(map[string]string{key: `"a-json-string-not-an-array"`}))
			wantErrContains(t, err, key)
		})
	}
}

func TestParseVerificationSectionEmptyRequiredArrayErrors(t *testing.T) {
	tests := []string{"runner-command", "final-command"}
	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := parseVerificationSection("path.md", fullVerificationLines(map[string]string{key: "[]"}))
			wantErrContains(t, err, key)
		})
	}
}

func TestParseVerificationSectionEmptyReportGlobsIsValid(t *testing.T) {
	vf, err := parseVerificationSection("path.md", fullVerificationLines(map[string]string{"report-globs": "[]"}))
	if err != nil {
		t.Fatalf("parseVerificationSection with empty report-globs: %v", err)
	}
	if len(vf.Runner.ReportGlobs) != 0 {
		t.Errorf("Runner.ReportGlobs = %v, want empty", vf.Runner.ReportGlobs)
	}
}
