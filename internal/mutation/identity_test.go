package mutation

import (
	"errors"
	"strings"
	"testing"

	"test-design/internal/jvmsource"
	"test-design/internal/trace"
)

func goSite(importPath, testName, file string, line int, ids []string) trace.TagSite {
	return trace.TagSite{ImportPath: importPath, TestName: testName, File: file, Line: line, IDs: ids}
}

func goAmbiguousSite(importPath, file string, line int, ids []string, detail string) trace.TagSite {
	return trace.TagSite{ImportPath: importPath, File: file, Line: line, IDs: ids, Ambiguous: true, Detail: detail}
}

func jvmSite(className, method, file string, line int, ids []string) jvmsource.TagSite {
	return jvmsource.TagSite{ClassName: className, Method: method, File: file, Line: line, IDs: ids}
}

func jvmAmbiguousSite(className, file string, line int, ids []string, detail string) jvmsource.TagSite {
	return jvmsource.TagSite{ClassName: className, File: file, Line: line, IDs: ids, Ambiguous: true, Detail: detail}
}

func TestResolveGoIdentity(t *testing.T) {
	tests := []struct {
		name      string
		sites     []trace.TagSite
		rowID     string
		want      Identity
		wantErr   error
		wantNames []string
	}{
		{
			name:  "single clean binding",
			sites: []trace.TagSite{goSite("test-design/internal/widget", "TestAdd", "widget_test.go", 10, []string{"SC-01"})},
			rowID: "SC-01",
			want:  Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
		},
		{
			name:      "zero citations",
			sites:     []trace.TagSite{goSite("test-design/internal/widget", "TestAdd", "widget_test.go", 10, []string{"SC-02"})},
			rowID:     "SC-01",
			wantErr:   ErrRowNotBound,
			wantNames: []string{"SC-01"},
		},
		{
			name:      "ambiguous citing site alone",
			sites:     []trace.TagSite{goAmbiguousSite("test-design/internal/widget", "widget_test.go", 12, []string{"SC-01"}, "comment is free-floating")},
			rowID:     "SC-01",
			wantErr:   ErrRowAmbiguous,
			wantNames: []string{"widget_test.go:12", "comment is free-floating"},
		},
		{
			name: "ambiguous citing site poisons even with a second clean site",
			sites: []trace.TagSite{
				goAmbiguousSite("test-design/internal/widget", "widget_test.go", 12, []string{"SC-01"}, "comment is free-floating"),
				goSite("test-design/internal/widget", "TestAdd", "widget_test.go", 20, []string{"SC-01"}),
			},
			rowID:     "SC-01",
			wantErr:   ErrRowAmbiguous,
			wantNames: []string{"widget_test.go:12", "comment is free-floating"},
		},
		{
			name: "two distinct identities",
			sites: []trace.TagSite{
				goSite("test-design/internal/widget", "TestAdd", "widget_test.go", 10, []string{"SC-01"}),
				goSite("test-design/internal/other", "TestSub", "other_test.go", 30, []string{"SC-01"}),
			},
			rowID:   "SC-01",
			wantErr: ErrRowAmbiguous,
			wantNames: []string{
				"test-design/internal/widget.TestAdd", "widget_test.go:10",
				"test-design/internal/other.TestSub", "other_test.go:30",
			},
		},
		{
			name: "duplicate same-identity sites succeed",
			sites: []trace.TagSite{
				goSite("test-design/internal/widget", "TestAdd", "widget_test.go", 10, []string{"SC-01"}),
				goSite("test-design/internal/widget", "TestAdd", "widget_test.go", 10, []string{"SC-01", "SC-02"}),
			},
			rowID: "SC-01",
			want:  Identity{Scope: "test-design/internal/widget", Name: "TestAdd"},
		},
		{
			name:      "empty TestName counts as ambiguous",
			sites:     []trace.TagSite{goSite("test-design/internal/widget", "", "widget_test.go", 40, []string{"SC-01"})},
			rowID:     "SC-01",
			wantErr:   ErrRowAmbiguous,
			wantNames: []string{"widget_test.go:40"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveGoIdentity(tc.sites, tc.rowID)
			checkIdentityResult(t, got, err, tc.want, tc.wantErr, tc.wantNames)
		})
	}
}

func TestResolveJVMIdentity(t *testing.T) {
	tests := []struct {
		name      string
		sites     []jvmsource.TagSite
		rowID     string
		want      Identity
		wantErr   error
		wantNames []string
	}{
		{
			name:  "single clean binding",
			sites: []jvmsource.TagSite{jvmSite("com.example.a.ATest", "sameName", "ATest.java", 10, []string{"SC-01"})},
			rowID: "SC-01",
			want:  Identity{Scope: "com.example.a.ATest", Name: "sameName"},
		},
		{
			name:      "zero citations",
			sites:     []jvmsource.TagSite{jvmSite("com.example.a.ATest", "sameName", "ATest.java", 10, []string{"SC-02"})},
			rowID:     "SC-01",
			wantErr:   ErrRowNotBound,
			wantNames: []string{"SC-01"},
		},
		{
			name:      "ambiguous citing site alone",
			sites:     []jvmsource.TagSite{jvmAmbiguousSite("com.example.a.ATest", "ATest.java", 12, []string{"SC-01"}, "method not found")},
			rowID:     "SC-01",
			wantErr:   ErrRowAmbiguous,
			wantNames: []string{"ATest.java:12", "method not found"},
		},
		{
			name: "ambiguous citing site poisons even with a second clean site",
			sites: []jvmsource.TagSite{
				jvmAmbiguousSite("com.example.a.ATest", "ATest.java", 12, []string{"SC-01"}, "method not found"),
				jvmSite("com.example.a.ATest", "sameName", "ATest.java", 20, []string{"SC-01"}),
			},
			rowID:     "SC-01",
			wantErr:   ErrRowAmbiguous,
			wantNames: []string{"ATest.java:12", "method not found"},
		},
		{
			name: "two distinct identities",
			sites: []jvmsource.TagSite{
				jvmSite("com.example.a.ATest", "sameName", "ATest.java", 10, []string{"SC-01"}),
				jvmSite("com.example.b.BTest", "otherName", "BTest.java", 30, []string{"SC-01"}),
			},
			rowID:   "SC-01",
			wantErr: ErrRowAmbiguous,
			wantNames: []string{
				"com.example.a.ATest.sameName", "ATest.java:10",
				"com.example.b.BTest.otherName", "BTest.java:30",
			},
		},
		{
			name: "duplicate same-identity sites succeed",
			sites: []jvmsource.TagSite{
				jvmSite("com.example.a.ATest", "sameName", "ATest.java", 10, []string{"SC-01"}),
				jvmSite("com.example.a.ATest", "sameName", "ATest.java", 10, []string{"SC-01", "SC-02"}),
			},
			rowID: "SC-01",
			want:  Identity{Scope: "com.example.a.ATest", Name: "sameName"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveJVMIdentity(tc.sites, tc.rowID)
			checkIdentityResult(t, got, err, tc.want, tc.wantErr, tc.wantNames)
		})
	}
}

func checkIdentityResult(t *testing.T, got Identity, err error, want Identity, wantErr error, wantNames []string) {
	t.Helper()
	if wantErr == nil {
		if err != nil {
			t.Fatalf("Resolve*Identity() error = %v, want nil", err)
		}
		if got != want {
			t.Errorf("Resolve*Identity() = %+v, want %+v", got, want)
		}
		return
	}
	if err == nil {
		t.Fatalf("Resolve*Identity() = %+v, nil, want error", got)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Resolve*Identity() error = %v, want errors.Is(_, %v)", err, wantErr)
	}
	for _, name := range wantNames {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("Resolve*Identity() error = %q, want it to contain %q", err.Error(), name)
		}
	}
}

func TestResolveIdentitySentinelsAreDistinct(t *testing.T) {
	if errors.Is(ErrRowNotBound, ErrRowAmbiguous) {
		t.Error("ErrRowNotBound and ErrRowAmbiguous must not be errors.Is-equal")
	}
}
