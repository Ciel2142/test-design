package mutation

import (
	"errors"
	"strings"
	"testing"

	"test-design/internal/gotestjson"
	"test-design/internal/junitxml"
	"test-design/internal/jvmrun"
	"test-design/internal/runrecord"
)

func rec(status runrecord.Status, scope, name string) runrecord.Record {
	return runrecord.Record{Status: status, Scope: scope, Name: name}
}

func TestVerifyGoSelection(t *testing.T) {
	id := Identity{Scope: "test-design/internal/widget", Name: "TestAdd"}

	tests := []struct {
		name      string
		rep       *gotestjson.Report
		wantErr   error
		wantNames []string
	}{
		{
			name: "accepts the family and its subtests",
			rep: &gotestjson.Report{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "TestAdd"),
				rec(runrecord.Passed, id.Scope, "TestAdd/case1"),
				rec(runrecord.Failed, id.Scope, "TestAdd/case2"),
			}},
		},
		{
			name: "accepts a failure in id's own package",
			rep: &gotestjson.Report{
				Records:  []runrecord.Record{rec(runrecord.Failed, id.Scope, "TestAdd")},
				Failures: []gotestjson.RunnerFailure{{Package: id.Scope, Detail: "build failed"}},
			},
		},
		{
			name: "rejects a record in a foreign package",
			rep: &gotestjson.Report{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "TestAdd"),
				rec(runrecord.Passed, "test-design/internal/other", "TestSub"),
			}},
			wantErr:   ErrForeignIdentity,
			wantNames: []string{"test-design/internal/other"},
		},
		{
			name: "rejects a foreign test outside the family in id's own package",
			rep: &gotestjson.Report{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "TestAdd"),
				rec(runrecord.Passed, id.Scope, "TestSub"),
			}},
			wantErr:   ErrForeignIdentity,
			wantNames: []string{"TestSub"},
		},
		{
			name: "rejects a RunnerFailure for a foreign package",
			rep: &gotestjson.Report{
				Records:  []runrecord.Record{rec(runrecord.Passed, id.Scope, "TestAdd")},
				Failures: []gotestjson.RunnerFailure{{Package: "test-design/internal/other", Detail: "boom"}},
			},
			wantErr:   ErrForeignIdentity,
			wantNames: []string{"test-design/internal/other"},
		},
		{
			name:    "rejects zero records",
			rep:     &gotestjson.Report{},
			wantErr: ErrZeroSelection,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyGoSelection(tc.rep, id)
			checkSelectionErr(t, err, tc.wantErr, tc.wantNames)
		})
	}
}

func TestVerifyJVMSelection(t *testing.T) {
	id := Identity{Scope: "com.example.ATest", Name: "adds"}

	tests := []struct {
		name      string
		res       *jvmrun.Result
		wantErr   error
		wantNames []string
	}{
		{
			name: "accepts an exact match",
			res:  &jvmrun.Result{Records: []runrecord.Record{rec(runrecord.Passed, id.Scope, "adds")}},
		},
		{
			name: "accepts a nested class",
			res: &jvmrun.Result{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "adds"),
				rec(runrecord.Passed, id.Scope+"$NestedCases", "adds"),
			}},
		},
		{
			name: "accepts a parameterized decoration of the name",
			res: &jvmrun.Result{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "adds(int)[1]"),
				rec(runrecord.Failed, id.Scope, "adds(int)[2]"),
			}},
		},
		{
			name: "rejects a foreign class",
			res: &jvmrun.Result{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "adds"),
				rec(runrecord.Passed, "com.example.BTest", "adds"),
			}},
			wantErr:   ErrForeignIdentity,
			wantNames: []string{"com.example.BTest"},
		},
		{
			name: "rejects a foreign method in id's own class",
			res: &jvmrun.Result{Records: []runrecord.Record{
				rec(runrecord.Passed, id.Scope, "adds"),
				rec(runrecord.Passed, id.Scope, "subtracts"),
			}},
			wantErr:   ErrForeignIdentity,
			wantNames: []string{"subtracts"},
		},
		{
			name: "an Unsupported family within id.Scope maps to ErrUnsupportedIdentity",
			res: &jvmrun.Result{
				Records: []runrecord.Record{rec(runrecord.Passed, id.Scope, "[1] 1")},
				Unsupported: []junitxml.Family{{Identity: junitxml.Identity{
					ClassName:   id.Scope,
					Unsupported: `gradle parameterized invocation decoration ("[index] ...") does not encode the declared method name`,
				}}},
			},
			wantErr:   ErrUnsupportedIdentity,
			wantNames: []string{id.Scope},
		},
		{
			name: "an Unsupported family outside id.Scope is ignored",
			res: &jvmrun.Result{
				Records: []runrecord.Record{rec(runrecord.Passed, id.Scope, "adds")},
				Unsupported: []junitxml.Family{{Identity: junitxml.Identity{
					ClassName: "com.example.OtherTest", Unsupported: "custom display name",
				}}},
			},
		},
		{
			name:    "rejects zero records",
			res:     &jvmrun.Result{},
			wantErr: ErrZeroSelection,
		},
		{
			name:      "a NormalizationError preempts any verdict",
			res:       &jvmrun.Result{Records: []runrecord.Record{rec(runrecord.Passed, id.Scope, "adds")}, NormalizationError: "no fresh report file matched report-globs after the run"},
			wantNames: []string{"no fresh report file matched report-globs after the run"},
		},
		{
			name:    "a NormalizationError preempts ErrZeroSelection and ErrUnsupportedIdentity too",
			res:     &jvmrun.Result{NormalizationError: "unreadable report"},
			wantErr: nil, // plain error, not a selection sentinel
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyJVMSelection(tc.res, id)
			if tc.res.NormalizationError != "" {
				if err == nil {
					t.Fatalf("VerifyJVMSelection() = nil, want error")
				}
				if errors.Is(err, ErrZeroSelection) || errors.Is(err, ErrUnsupportedIdentity) || errors.Is(err, ErrForeignIdentity) {
					t.Errorf("VerifyJVMSelection() error = %v, want a plain error, not a selection sentinel", err)
				}
				for _, n := range tc.wantNames {
					if !strings.Contains(err.Error(), n) {
						t.Errorf("VerifyJVMSelection() error = %q, want it to contain %q", err.Error(), n)
					}
				}
				return
			}
			checkSelectionErr(t, err, tc.wantErr, tc.wantNames)
		})
	}
}

func checkSelectionErr(t *testing.T, err error, wantErr error, wantNames []string) {
	t.Helper()
	if wantErr == nil {
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("error = nil, want errors.Is(_, %v)", wantErr)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want errors.Is(_, %v)", err, wantErr)
	}
	for _, n := range wantNames {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), n)
		}
	}
}

func TestSelectionSentinelsAreDistinct(t *testing.T) {
	if errors.Is(ErrZeroSelection, ErrForeignIdentity) ||
		errors.Is(ErrZeroSelection, ErrUnsupportedIdentity) ||
		errors.Is(ErrForeignIdentity, ErrUnsupportedIdentity) {
		t.Error("selection sentinels must not be errors.Is-equal to each other")
	}
}
