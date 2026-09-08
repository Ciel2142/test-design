package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"test-design/internal/evidence"
)

func sampleProfileJSON(t *testing.T, overrides map[string]any) []byte {
	t.Helper()
	doc := map[string]any{
		"schemaVersion":  evidence.SchemaVersion,
		"classification": "local",
		"start":          [][]string{{"./scripts/start.sh"}},
		"health":         [][]string{{"./scripts/health.sh"}},
		"walkthrough":    [][]string{{"./scripts/walk.sh"}},
		"cleanup":        [][]string{{"./scripts/cleanup.sh"}},
		"cleanupEvidence": []map[string]string{
			{"form": "exit-code", "stream": "combined", "value": "0"},
		},
		"envAllowlist":     []string{"PATH"},
		"resourcePrefixes": []string{"fixture-{run-id}"},
		"endpoints":        []string{"http://127.0.0.1:8099"},
	}
	for k, v := range overrides {
		doc[k] = v
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal profile fixture: %v", err)
	}
	return data
}

func writeProfile(t *testing.T, dir string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestProfileRejectsNegativeExecutionLimits(t *testing.T) {
	for _, field := range []string{"timeoutSeconds", "cleanupTimeoutSeconds", "maxOutputBytes"} {
		t.Run(field, func(t *testing.T) {
			var p Profile
			if err := json.Unmarshal(sampleProfileJSON(t, map[string]any{field: -1}), &p); err != nil {
				t.Fatal(err)
			}
			if err := p.ValidateTemplate(); err == nil {
				t.Fatalf("negative %s accepted", field)
			}
		})
	}
}

func TestProfileSeparatesExecutionAndCleanupLimits(t *testing.T) {
	var p Profile
	if err := json.Unmarshal(sampleProfileJSON(t, map[string]any{
		"timeoutSeconds": 1, "cleanupTimeoutSeconds": 3, "maxOutputBytes": 128,
	}), &p); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateTemplate(); err != nil {
		t.Fatal(err)
	}
	if got := p.Limits(false); got.Timeout != time.Second || got.OutputBytes != 128 {
		t.Fatalf("execution limits = %+v", got)
	}
	if got := p.Limits(true); got.Timeout != 3*time.Second || got.OutputBytes != 128 {
		t.Fatalf("cleanup limits = %+v", got)
	}
}

func TestLoadProfileMissingFileFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	p, err := LoadProfile(path)
	if err == nil {
		t.Fatal("LoadProfile returned nil error, want a fail-closed error")
	}
	if p != nil {
		t.Errorf("Profile = %+v, want nil on error", p)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name the path", err.Error())
	}
}

func TestLoadProfileUnreadableFileFailsClosed(t *testing.T) {
	dir := t.TempDir()

	p, err := LoadProfile(dir)
	if err == nil {
		t.Fatal("LoadProfile returned nil error, want a fail-closed error")
	}
	if p != nil {
		t.Errorf("Profile = %+v, want nil on error", p)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error = %q, want it to name the path", err.Error())
	}
}

func TestLoadProfileMalformedJSONFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := writeProfile(t, dir, []byte("{not json"))

	p, err := LoadProfile(path)
	if err == nil {
		t.Fatal("LoadProfile returned nil error, want a fail-closed error")
	}
	if p != nil {
		t.Errorf("Profile = %+v, want nil on error", p)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name the path", err.Error())
	}
	if !strings.Contains(err.Error(), "malformed JSON") {
		t.Errorf("error = %q, want it to mention malformed JSON", err.Error())
	}
}

func TestLoadProfileWrongSchemaVersionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := writeProfile(t, dir, sampleProfileJSON(t, map[string]any{"schemaVersion": "999"}))

	p, err := LoadProfile(path)
	if err == nil {
		t.Fatal("LoadProfile returned nil error, want a fail-closed error")
	}
	if p != nil {
		t.Errorf("Profile = %+v, want nil on error", p)
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error = %q, want it to name the got schemaVersion", err.Error())
	}
	if !strings.Contains(err.Error(), evidence.SchemaVersion) {
		t.Errorf("error = %q, want it to name the want schemaVersion", err.Error())
	}
}

func TestLoadProfileSuccess(t *testing.T) {
	dir := t.TempDir()
	path := writeProfile(t, dir, sampleProfileJSON(t, nil))

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.SchemaVersion != evidence.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", p.SchemaVersion, evidence.SchemaVersion)
	}
	if p.Classification != "local" {
		t.Errorf("Classification = %q, want %q", p.Classification, "local")
	}
	if len(p.Start) != 1 || len(p.Start[0]) != 1 || p.Start[0][0] != "./scripts/start.sh" {
		t.Errorf("Start = %+v, want one entry [./scripts/start.sh]", p.Start)
	}
	if len(p.CleanupEvidence) != 1 || p.CleanupEvidence[0] != (Predicate{Form: "exit-code", Stream: "combined", Value: "0"}) {
		t.Errorf("CleanupEvidence = %+v, want one exit-code predicate", p.CleanupEvidence)
	}
	if len(p.ResourcePrefixes) != 1 || p.ResourcePrefixes[0] != "fixture-{run-id}" {
		t.Errorf("ResourcePrefixes = %+v, want [fixture-{run-id}]", p.ResourcePrefixes)
	}
	if len(p.Endpoints) != 1 || p.Endpoints[0] != "http://127.0.0.1:8099" {
		t.Errorf("Endpoints = %+v, want [http://127.0.0.1:8099]", p.Endpoints)
	}
}

func validProfile() *Profile {
	return &Profile{
		SchemaVersion:  evidence.SchemaVersion,
		Classification: "local",
		Start:          [][]string{{"./scripts/start.sh"}},
		Health:         [][]string{{"./scripts/health.sh"}},
		Walkthrough:    [][]string{{"./scripts/walk.sh"}},
		Cleanup:        [][]string{{"./scripts/cleanup.sh"}},
		CleanupEvidence: []Predicate{
			{Form: "exit-code", Stream: "combined", Value: "0"},
		},
		EnvAllowlist:     []string{"PATH"},
		ResourcePrefixes: []string{"fixture-{run-id}"},
		Endpoints:        []string{"http://127.0.0.1:8099"},
	}
}

func TestValidateTemplateAcceptsWellFormedLocalProfile(t *testing.T) {
	p := validProfile()
	if err := p.ValidateTemplate(); err != nil {
		t.Errorf("ValidateTemplate() = %v, want nil", err)
	}
}

func TestValidateTemplateAcceptsEphemeralProfileWithNonLoopbackEndpoint(t *testing.T) {
	p := validProfile()
	p.Classification = "ephemeral"
	p.Endpoints = []string{"http://10.0.5.9:8099"}
	if err := p.ValidateTemplate(); err != nil {
		t.Errorf("ValidateTemplate() = %v, want nil (endpoint check is local-only)", err)
	}
}

func TestValidateTemplateAcceptsLoopbackEndpointVariants(t *testing.T) {
	loopbacks := []string{"http://127.0.0.1:8099", "http://localhost:8099", "http://[::1]:8099"}
	p := validProfile()
	p.Endpoints = loopbacks
	if err := p.ValidateTemplate(); err != nil {
		t.Errorf("ValidateTemplate() = %v, want nil for loopback endpoints %v", err, loopbacks)
	}
}

func TestValidateTemplateUnsupportedClassification(t *testing.T) {
	tests := []struct {
		name           string
		classification string
	}{
		{"production", "production"},
		{"empty", ""},
		{"unknown", "staging"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validProfile()
			p.Classification = tc.classification
			err := p.ValidateTemplate()
			if err == nil {
				t.Fatal("ValidateTemplate() returned nil error, want unsupported-classification")
			}
			if !strings.Contains(err.Error(), "unsupported-classification") {
				t.Errorf("error = %q, want it to contain unsupported-classification", err.Error())
			}
		})
	}
}

func TestValidateTemplateUndeclaredEndpointForLocalClassification(t *testing.T) {
	p := validProfile()
	p.Endpoints = []string{"http://198.51.100.7:8099"}

	err := p.ValidateTemplate()
	if err == nil {
		t.Fatal("ValidateTemplate() returned nil error, want undeclared-endpoint")
	}
	if !strings.Contains(err.Error(), "undeclared-endpoint") {
		t.Errorf("error = %q, want it to contain undeclared-endpoint", err.Error())
	}
	if !strings.Contains(err.Error(), "198.51.100.7") {
		t.Errorf("error = %q, want it to name the offending endpoint", err.Error())
	}
}

func TestValidateTemplateRunIDMissingFromResources(t *testing.T) {
	p := validProfile()
	p.ResourcePrefixes = []string{"fixture-{run-id}", "orphan-resource"}

	err := p.ValidateTemplate()
	if err == nil {
		t.Fatal("ValidateTemplate() returned nil error, want run-id-missing-from-resources")
	}
	if !strings.Contains(err.Error(), "run-id-missing-from-resources") {
		t.Errorf("error = %q, want it to contain run-id-missing-from-resources", err.Error())
	}
	if !strings.Contains(err.Error(), "orphan-resource") {
		t.Errorf("error = %q, want it to name the offending entry", err.Error())
	}
}

func TestValidateTemplateMissingCleanup(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(*Profile)
	}{
		{"empty cleanup", func(p *Profile) { p.Cleanup = nil }},
		{"empty cleanupEvidence", func(p *Profile) { p.CleanupEvidence = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validProfile()
			tc.corrupt(p)
			err := p.ValidateTemplate()
			if err == nil {
				t.Fatal("ValidateTemplate() returned nil error, want missing-cleanup")
			}
			if !strings.Contains(err.Error(), "missing-cleanup") {
				t.Errorf("error = %q, want it to contain missing-cleanup", err.Error())
			}
		})
	}
}

// resolvedProfile returns the descriptor prepare-live produces for runID: the
// committed template with every {run-id} token already replaced.
func resolvedProfile(runID string) *Profile {
	p := validProfile()
	p.Start = [][]string{{"./scripts/start.sh", "--scratch", "fixture-" + runID}}
	p.ResourcePrefixes = []string{"fixture-" + runID}
	return p
}

func TestValidateResolvedAcceptsSubstitutedDescriptor(t *testing.T) {
	p := resolvedProfile("run-abc")
	if err := p.ValidateResolved("run-abc"); err != nil {
		t.Errorf("ValidateResolved() = %v, want nil for a descriptor produced by substitution", err)
	}
}

// TestValidateResolvedRefusesEmptyRunID closes the vacuous acceptance: every
// resourcePrefixes entry contains the empty string, so without the guard a
// caller that lost its run ID validates a descriptor no run owns.
func TestValidateResolvedRefusesEmptyRunID(t *testing.T) {
	p := resolvedProfile("run-abc")

	err := p.ValidateResolved("")
	if err == nil {
		t.Fatal("ValidateResolved(\"\") returned nil error, want run-id-missing")
	}
	if !strings.Contains(err.Error(), "run-id-missing:") {
		t.Errorf("error = %q, want it to contain run-id-missing:", err.Error())
	}
}

func TestValidateResolvedRunIDMissingFromResources(t *testing.T) {
	p := resolvedProfile("run-abc")
	p.ResourcePrefixes = []string{"fixture-run-abc", "orphan-resource"}

	err := p.ValidateResolved("run-abc")
	if err == nil {
		t.Fatal("ValidateResolved() returned nil error, want run-id-missing-from-resources")
	}
	if !strings.Contains(err.Error(), "run-id-missing-from-resources") {
		t.Errorf("error = %q, want it to contain run-id-missing-from-resources", err.Error())
	}
	if !strings.Contains(err.Error(), "orphan-resource") {
		t.Errorf("error = %q, want it to name the offending entry", err.Error())
	}
}

func TestValidateResolvedRefusesLeftoverRunIDToken(t *testing.T) {
	tests := []struct {
		field   string
		corrupt func(*Profile)
	}{
		{"start", func(p *Profile) { p.Start = [][]string{{"./scripts/start.sh", "fixture-{run-id}"}} }},
		{"health", func(p *Profile) { p.Health = [][]string{{"./scripts/health.sh", "fixture-{run-id}"}} }},
		{"walkthrough", func(p *Profile) { p.Walkthrough = [][]string{{"./scripts/walk.sh", "fixture-{run-id}"}} }},
		{"cleanup", func(p *Profile) { p.Cleanup = [][]string{{"./scripts/cleanup.sh", "fixture-{run-id}"}} }},
		// This prefix carries the run ID too, so it clears the
		// run-id-missing-from-resources check and reaches the token sweep.
		{"resourcePrefixes", func(p *Profile) { p.ResourcePrefixes = []string{"fixture-run-abc-{run-id}"} }},
		{"endpoints", func(p *Profile) { p.Endpoints = []string{"http://127.0.0.1:8099/{run-id}"} }},
	}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			p := resolvedProfile("run-abc")
			tc.corrupt(p)

			err := p.ValidateResolved("run-abc")
			if err == nil {
				t.Fatalf("ValidateResolved() returned nil error, want unresolved-run-id %s", tc.field)
			}
			if !strings.Contains(err.Error(), "unresolved-run-id") {
				t.Errorf("error = %q, want it to contain unresolved-run-id", err.Error())
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error = %q, want it to name the offending field %s", err.Error(), tc.field)
			}
		})
	}
}

func TestValidateResolvedKeepsTemplateInvariants(t *testing.T) {
	tests := []struct {
		name    string
		refusal string
		corrupt func(*Profile)
	}{
		{"negative limits", "invalid-limits", func(p *Profile) { p.TimeoutSeconds = -1 }},
		{"classification", "unsupported-classification", func(p *Profile) { p.Classification = "production" }},
		{"endpoint", "undeclared-endpoint", func(p *Profile) { p.Endpoints = []string{"http://198.51.100.7:8099"} }},
		{"cleanup", "missing-cleanup", func(p *Profile) { p.CleanupEvidence = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := resolvedProfile("run-abc")
			tc.corrupt(p)

			err := p.ValidateResolved("run-abc")
			if err == nil {
				t.Fatalf("ValidateResolved() returned nil error, want %s", tc.refusal)
			}
			if !strings.Contains(err.Error(), tc.refusal) {
				t.Errorf("error = %q, want it to contain %s", err.Error(), tc.refusal)
			}
		})
	}
}
