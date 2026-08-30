package mutation

import (
	"strings"
	"testing"

	"test-design/internal/coverage"
)

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGoTarget(t *testing.T) {
	tests := []struct {
		name        string
		cfg         coverage.RunnerConfig
		id          Identity
		wantCommand []string
		wantErr     string // substring; empty means want no error
	}{
		{
			name:        "substitutes a broader package pattern",
			cfg:         coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "-json", "./..."}},
			id:          Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
			wantCommand: []string{"go", "test", "-count=1", "-json", "test-design/internal/widget", "-run", "^TestAdd$"},
		},
		{
			name:        "appends the package when none is configured",
			cfg:         coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1"}},
			id:          Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
			wantCommand: []string{"go", "test", "-count=1", "test-design/internal/widget", "-run", "^TestAdd$"},
		},
		{
			name:        "leaves an already-explicit package in place",
			cfg:         coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "test-design/internal/widget"}},
			id:          Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
			wantCommand: []string{"go", "test", "-count=1", "test-design/internal/widget", "-run", "^TestAdd$"},
		},
		{
			name:        "never drops or reorders other configured flags",
			cfg:         coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "-json", "-v", "./..."}},
			id:          Identity{Scope: "pkg", Name: "TestX"},
			wantCommand: []string{"go", "test", "-count=1", "-json", "-v", "pkg", "-run", "^TestX$"},
		},
		{
			name:        "escapes regex metacharacters in the test name",
			cfg:         coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "./..."}},
			id:          Identity{Scope: "test-design/internal/widget", Name: "TestAdd[special]"},
			wantCommand: []string{"go", "test", "-count=1", "test-design/internal/widget", "-run", `^TestAdd\[special\]$`},
		},
		{
			name:    "rejects two non-flag tokens as ambiguous (e.g. a two-token flag value)",
			cfg:     coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "-timeout", "300s", "./..."}},
			id:      Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
			wantErr: "non-flag arguments",
		},
		{
			name:    "rejects a missing -count=1",
			cfg:     coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-json", "./..."}},
			id:      Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
			wantErr: "-count=1",
		},
		{
			name:    "rejects a non-go kind",
			cfg:     coverage.RunnerConfig{Kind: "maven", Command: []string{"go", "test", "-count=1"}},
			id:      Identity{Scope: "x", Name: "y"},
			wantErr: "kind must be go",
		},
		{
			name:    "rejects empty scope",
			cfg:     coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "./..."}},
			id:      Identity{Scope: "", Name: "TestAdd"},
			wantErr: "must not be empty",
		},
		{
			name:    "rejects empty name",
			cfg:     coverage.RunnerConfig{Kind: "go", Command: []string{"go", "test", "-count=1", "./..."}},
			id:      Identity{Scope: "pkg", Name: ""},
			wantErr: "must not be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GoTarget(tc.cfg, tc.id)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("GoTarget() error = %v, want nil", err)
				}
				if !equalStrings(got.Command, tc.wantCommand) {
					t.Errorf("Command = %v, want %v", got.Command, tc.wantCommand)
				}
				if got.ReportGlobs != nil {
					t.Errorf("ReportGlobs = %v, want nil", got.ReportGlobs)
				}
				return
			}
			if err == nil {
				t.Fatalf("GoTarget() = %+v, nil, want error containing %q", got, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("GoTarget() error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestJVMTarget(t *testing.T) {
	tests := []struct {
		name        string
		cfg         coverage.RunnerConfig
		id          Identity
		wantCommand []string
		wantGlobs   []string
		wantErr     string // substring; empty means want no error
	}{
		{
			name: "maven surefire (test phase) appends -Dtest and keeps only surefire globs",
			cfg: coverage.RunnerConfig{
				Kind:        "maven",
				Command:     []string{"mvn", "-B", "test"},
				ReportGlobs: []string{"target/surefire-reports/*.xml", "target/failsafe-reports/*.xml"},
			},
			id:          Identity{Scope: "com.example.ATest", Name: "adds"},
			wantCommand: []string{"mvn", "-B", "test", "-Dtest=com.example.ATest#adds"},
			wantGlobs:   []string{"target/surefire-reports/*.xml"},
		},
		{
			name: "maven failsafe (verify phase) appends -Dit.test and keeps only failsafe globs",
			cfg: coverage.RunnerConfig{
				Kind:        "maven",
				Command:     []string{"mvn", "-B", "verify"},
				ReportGlobs: []string{"target/surefire-reports/*.xml", "target/failsafe-reports/*.xml"},
			},
			id:          Identity{Scope: "com.example.AIT", Name: "adds"},
			wantCommand: []string{"mvn", "-B", "verify", "-Dit.test=com.example.AIT#adds"},
			wantGlobs:   []string{"target/failsafe-reports/*.xml"},
		},
		{
			name: "maven never drops or reorders existing flags",
			cfg: coverage.RunnerConfig{
				Kind:        "maven",
				Command:     []string{"mvn", "-B", "-q", "test"},
				ReportGlobs: []string{"target/surefire-reports/*.xml"},
			},
			id:          Identity{Scope: "com.example.ATest", Name: "adds"},
			wantCommand: []string{"mvn", "-B", "-q", "test", "-Dtest=com.example.ATest#adds"},
			wantGlobs:   []string{"target/surefire-reports/*.xml"},
		},
		{
			name: "maven empty glob subset errors",
			cfg: coverage.RunnerConfig{
				Kind:        "maven",
				Command:     []string{"mvn", "-B", "test"},
				ReportGlobs: []string{"target/failsafe-reports/*.xml"},
			},
			id:      Identity{Scope: "com.example.ATest", Name: "adds"},
			wantErr: "surefire-reports",
		},
		{
			name: "gradle appends --tests and keeps report-globs unchanged",
			cfg: coverage.RunnerConfig{
				Kind:        "gradle",
				Command:     []string{"gradle", "test", "--rerun"},
				ReportGlobs: []string{"build/test-results/test/TEST-*.xml"},
			},
			id:          Identity{Scope: "com.example.ATest", Name: "adds"},
			wantCommand: []string{"gradle", "test", "--rerun", "--tests", "com.example.ATest.adds"},
			wantGlobs:   []string{"build/test-results/test/TEST-*.xml"},
		},
		{
			name: "gradle accepts --rerun-tasks in place of --rerun",
			cfg: coverage.RunnerConfig{
				Kind:        "gradle",
				Command:     []string{"gradle", "test", "--rerun-tasks"},
				ReportGlobs: []string{"build/test-results/test/TEST-*.xml"},
			},
			id:          Identity{Scope: "com.example.ATest", Name: "adds"},
			wantCommand: []string{"gradle", "test", "--rerun-tasks", "--tests", "com.example.ATest.adds"},
			wantGlobs:   []string{"build/test-results/test/TEST-*.xml"},
		},
		{
			name: "gradle rejects a missing rerun flag",
			cfg: coverage.RunnerConfig{
				Kind:        "gradle",
				Command:     []string{"gradle", "test"},
				ReportGlobs: []string{"build/test-results/test/TEST-*.xml"},
			},
			id:      Identity{Scope: "com.example.ATest", Name: "adds"},
			wantErr: "--rerun",
		},
		{
			name:    "rejects an unknown kind, naming it",
			cfg:     coverage.RunnerConfig{Kind: "ant", Command: []string{"ant", "test"}},
			id:      Identity{Scope: "com.example.ATest", Name: "adds"},
			wantErr: "ant",
		},
		{
			name:    "rejects empty scope",
			cfg:     coverage.RunnerConfig{Kind: "maven", Command: []string{"mvn", "test"}, ReportGlobs: []string{"target/surefire-reports/*.xml"}},
			id:      Identity{Scope: "", Name: "adds"},
			wantErr: "must not be empty",
		},
		{
			name:    "rejects empty name",
			cfg:     coverage.RunnerConfig{Kind: "maven", Command: []string{"mvn", "test"}, ReportGlobs: []string{"target/surefire-reports/*.xml"}},
			id:      Identity{Scope: "com.example.ATest", Name: ""},
			wantErr: "must not be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := JVMTarget(tc.cfg, tc.id)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("JVMTarget() error = %v, want nil", err)
				}
				if !equalStrings(got.Command, tc.wantCommand) {
					t.Errorf("Command = %v, want %v", got.Command, tc.wantCommand)
				}
				if !equalStrings(got.ReportGlobs, tc.wantGlobs) {
					t.Errorf("ReportGlobs = %v, want %v", got.ReportGlobs, tc.wantGlobs)
				}
				return
			}
			if err == nil {
				t.Fatalf("JVMTarget() = %+v, nil, want error containing %q", got, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("JVMTarget() error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
