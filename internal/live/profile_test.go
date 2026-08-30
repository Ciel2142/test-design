package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestValidateAcceptsWellFormedLocalProfile(t *testing.T) {
	p := validProfile()
	if err := p.Validate("run-abc"); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidateAcceptsEphemeralProfileWithNonLoopbackEndpoint(t *testing.T) {
	p := validProfile()
	p.Classification = "ephemeral"
	p.Endpoints = []string{"http://10.0.5.9:8099"}
	if err := p.Validate("run-abc"); err != nil {
		t.Errorf("Validate() = %v, want nil (endpoint check is local-only)", err)
	}
}

func TestValidateAcceptsLoopbackEndpointVariants(t *testing.T) {
	loopbacks := []string{"http://127.0.0.1:8099", "http://localhost:8099", "http://[::1]:8099"}
	p := validProfile()
	p.Endpoints = loopbacks
	if err := p.Validate("run-abc"); err != nil {
		t.Errorf("Validate() = %v, want nil for loopback endpoints %v", err, loopbacks)
	}
}

func TestValidateUnsupportedClassification(t *testing.T) {
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
			err := p.Validate("run-abc")
			if err == nil {
				t.Fatal("Validate() returned nil error, want unsupported-classification")
			}
			if !strings.Contains(err.Error(), "unsupported-classification") {
				t.Errorf("error = %q, want it to contain unsupported-classification", err.Error())
			}
		})
	}
}

func TestValidateUndeclaredEndpointForLocalClassification(t *testing.T) {
	p := validProfile()
	p.Endpoints = []string{"http://198.51.100.7:8099"}

	err := p.Validate("run-abc")
	if err == nil {
		t.Fatal("Validate() returned nil error, want undeclared-endpoint")
	}
	if !strings.Contains(err.Error(), "undeclared-endpoint") {
		t.Errorf("error = %q, want it to contain undeclared-endpoint", err.Error())
	}
	if !strings.Contains(err.Error(), "198.51.100.7") {
		t.Errorf("error = %q, want it to name the offending endpoint", err.Error())
	}
}

func TestValidateRunIDMissingFromResources(t *testing.T) {
	p := validProfile()
	p.ResourcePrefixes = []string{"fixture-{run-id}", "orphan-resource"}

	err := p.Validate("run-abc")
	if err == nil {
		t.Fatal("Validate() returned nil error, want run-id-missing-from-resources")
	}
	if !strings.Contains(err.Error(), "run-id-missing-from-resources") {
		t.Errorf("error = %q, want it to contain run-id-missing-from-resources", err.Error())
	}
	if !strings.Contains(err.Error(), "orphan-resource") {
		t.Errorf("error = %q, want it to name the offending entry", err.Error())
	}
}

func TestValidateMissingCleanup(t *testing.T) {
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
			err := p.Validate("run-abc")
			if err == nil {
				t.Fatal("Validate() returned nil error, want missing-cleanup")
			}
			if !strings.Contains(err.Error(), "missing-cleanup") {
				t.Errorf("error = %q, want it to contain missing-cleanup", err.Error())
			}
		})
	}
}
