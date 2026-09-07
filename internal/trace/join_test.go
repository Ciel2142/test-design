package trace

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
)

func buildJoinTestRegistry(t *testing.T) (*coverage.Registry, string) {
	t.Helper()
	root := filepath.Join("testdata", "join-fixture")
	reg, err := coverage.BuildRegistry(root)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	thisDoc := filepath.Join("docs", "superpowers", "coverage", "2026-01-01-basic-coverage.md")
	return reg, thisDoc
}

func basicSource(thisDoc string, rows ...coverage.Row) *coverage.Source {
	return &coverage.Source{
		Path:    thisDoc,
		Rows:    rows,
		Retired: []string{"SC-04"},
	}
}

func TestJoinSingleBindingPassedProducesRowResultOnly(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, EndLine: 9, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, classes, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	wantResults := []RowResult{{
		RowID: "SC-01", Scope: "pkg", Name: "TestFoo", Status: runrecord.Passed, File: "pkg/foo_test.go", Line: 5, EndLine: 9,
		Contributors: []Contributor{
			{Scope: "pkg", Name: "TestFoo", Status: runrecord.Passed, Executed: true, File: "pkg/foo_test.go", Line: 5, EndLine: 9},
		},
	}}
	if !reflect.DeepEqual(results, wantResults) {
		t.Errorf("results = %+v, want %+v", results, wantResults)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
	if len(classes) != 0 {
		t.Errorf("classes = %+v, want none", classes)
	}
}

func TestJoinSingleBindingFailedProducesRedFinding(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Failed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 || results[0].Status != runrecord.Failed {
		t.Fatalf("results = %+v, want one failed row result", results)
	}
	if len(findings) != 1 || findings[0].Kind != "red" || findings[0].RowID != "SC-01" {
		t.Errorf("findings = %+v, want one red finding for SC-01", findings)
	}
}

func TestJoinSingleBindingSkippedProducesGapFinding(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Skipped, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 || results[0].Status != runrecord.Skipped {
		t.Fatalf("results = %+v, want one skipped row result", results)
	}
	if len(findings) != 1 || findings[0].Kind != "gap" || findings[0].RowID != "SC-01" {
		t.Errorf("findings = %+v, want one gap finding for SC-01", findings)
	}
}

func TestJoinNoSiteBindsRowProducesGap(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	rep := &gotestjson.Report{}

	results, findings, _, err := Join(src, reg, nil, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none", results)
	}
	want := []Finding{{Kind: "gap", RowID: "SC-01", Detail: "no test cites SC-01"}}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %+v, want %+v", findings, want)
	}
}

func TestJoinSiteBindsRowButFamilyEmptyProducesGap(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none", results)
	}
	if len(findings) != 1 || findings[0].Kind != "gap" || findings[0].RowID != "SC-01" {
		t.Fatalf("findings = %+v, want one gap finding for SC-01", findings)
	}
	if !strings.Contains(findings[0].Detail, "pkg/foo_test.go:5") {
		t.Errorf("Detail = %q, want it to name the site file:line", findings[0].Detail)
	}
}

func TestJoinTwoSitesBindSameRowProduceOneResultWithBothContributors(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, EndLine: 9, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestBar", File: "pkg/bar_test.go", Line: 9, EndLine: 14, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestBar"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	wantResults := []RowResult{{
		RowID: "SC-01", Scope: "pkg", Name: "TestBar", Status: runrecord.Passed, File: "pkg/bar_test.go", Line: 9, EndLine: 14,
		Contributors: []Contributor{
			{Scope: "pkg", Name: "TestBar", Status: runrecord.Passed, Executed: true, File: "pkg/bar_test.go", Line: 9, EndLine: 14},
			{Scope: "pkg", Name: "TestFoo", Status: runrecord.Passed, Executed: true, File: "pkg/foo_test.go", Line: 5, EndLine: 9},
		},
	}}
	if !reflect.DeepEqual(results, wantResults) {
		t.Errorf("results = %+v, want %+v", results, wantResults)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none: two sites are two contributors, never ambiguity", findings)
	}
}

func TestJoinDuplicateIDInOneSiteIsSingleBindingNoDuplicateFinding(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01", "SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one row result", results)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

func TestJoinRetiredIDProducesDriftNoBinding(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-04"}},
	}
	rep := &gotestjson.Report{}

	results, findings, classes, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none", results)
	}
	if len(classes) != 0 {
		t.Errorf("classes = %+v, want none for retired", classes)
	}
	wantFindings := []Finding{
		{Kind: "drift", RowID: "", Detail: "retired ID SC-04 at pkg/foo_test.go:5"},
		{Kind: "gap", RowID: "SC-01", Detail: "no test cites SC-01"},
	}
	if !reflect.DeepEqual(findings, wantFindings) {
		t.Errorf("findings = %+v, want %+v", findings, wantFindings)
	}
}

func TestJoinForeignIDProducesTagClassOnly(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-99"}},
	}
	rep := &gotestjson.Report{}

	_, findings, classes, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	for _, f := range findings {
		if f.Kind == "drift" {
			t.Errorf("unexpected finding for foreign id: %+v", f)
		}
	}
	wantClasses := []TagClass{{ID: "SC-99", Class: "foreign", File: "pkg/foo_test.go", Line: 5}}
	if !reflect.DeepEqual(classes, wantClasses) {
		t.Errorf("classes = %+v, want %+v", classes, wantClasses)
	}
}

func TestJoinUnknownIDProducesDriftAndTagClass(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-77"}},
	}
	rep := &gotestjson.Report{}

	_, findings, classes, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	found := false
	for _, f := range findings {
		if f.Kind == "drift" && f.Detail == "unknown ID SC-77 at pkg/foo_test.go:5" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want unknown-id drift finding", findings)
	}
	wantClasses := []TagClass{{ID: "SC-77", Class: "unknown", File: "pkg/foo_test.go", Line: 5}}
	if !reflect.DeepEqual(classes, wantClasses) {
		t.Errorf("classes = %+v, want %+v", classes, wantClasses)
	}
}

func TestJoinSiteWithThreeActiveIDsBindsEveryCitedRow(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc,
		coverage.Row{ID: "SC-01"},
		coverage.Row{ID: "SC-02"},
		coverage.Row{ID: "SC-03"},
	)
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, EndLine: 9, IDs: []string{"SC-01", "SC-02", "SC-03"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %+v, want three row results (no count limit on active ids per site)", results)
	}
	for i, wantID := range []string{"SC-01", "SC-02", "SC-03"} {
		got := results[i]
		if got.RowID != wantID {
			t.Errorf("results[%d].RowID = %q, want %q", i, got.RowID, wantID)
		}
		if got.Name != "TestFoo" || got.Status != runrecord.Passed {
			t.Errorf("results[%d] = %+v, want TestFoo passed", i, got)
		}
		if len(got.Contributors) != 1 || got.Contributors[0].Name != "TestFoo" || !got.Contributors[0].Executed {
			t.Errorf("results[%d].Contributors = %+v, want the one executed TestFoo contributor", i, got.Contributors)
		}
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

func TestJoinFailedContributorMarksRowRedAndKeepsBothIdentities(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestPass", File: "pkg/a_test.go", Line: 5, EndLine: 9, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestFail", File: "pkg/b_test.go", Line: 11, EndLine: 16, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestPass"},
		{Status: runrecord.Failed, Scope: "pkg", Name: "TestFail"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one row result", results)
	}
	got := results[0]
	if got.Status != runrecord.Failed {
		t.Errorf("results[0].Status = %q, want %q (failed dominates the collapse)", got.Status, runrecord.Failed)
	}
	wantContributors := []Contributor{
		{Scope: "pkg", Name: "TestPass", Status: runrecord.Passed, Executed: true, File: "pkg/a_test.go", Line: 5, EndLine: 9},
		{Scope: "pkg", Name: "TestFail", Status: runrecord.Failed, Executed: true, File: "pkg/b_test.go", Line: 11, EndLine: 16},
	}
	if !reflect.DeepEqual(got.Contributors, wantContributors) {
		t.Errorf("results[0].Contributors = %+v, want %+v (both identities retained)", got.Contributors, wantContributors)
	}
	if len(findings) != 1 || findings[0].Kind != "red" || findings[0].RowID != "SC-01" {
		t.Fatalf("findings = %+v, want exactly one red finding for SC-01", findings)
	}
	if !strings.Contains(findings[0].Detail, "pkg/b_test.go:11") || !strings.Contains(findings[0].Detail, "TestFail") {
		t.Errorf("Detail = %q, want it to name the failing contributor", findings[0].Detail)
	}
}

func TestJoinSkippedContributorIsGapEvenWhenAnotherPasses(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestPass", File: "pkg/a_test.go", Line: 5, EndLine: 9, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestSkip", File: "pkg/b_test.go", Line: 11, EndLine: 16, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestPass"},
		{Status: runrecord.Skipped, Scope: "pkg", Name: "TestSkip"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one row result", results)
	}
	got := results[0]
	if got.Status != runrecord.Skipped {
		t.Errorf("results[0].Status = %q, want %q (a skipped contributor withholds the pass)", got.Status, runrecord.Skipped)
	}
	wantContributors := []Contributor{
		{Scope: "pkg", Name: "TestPass", Status: runrecord.Passed, Executed: true, File: "pkg/a_test.go", Line: 5, EndLine: 9},
		{Scope: "pkg", Name: "TestSkip", Status: runrecord.Skipped, Executed: true, File: "pkg/b_test.go", Line: 11, EndLine: 16},
	}
	if !reflect.DeepEqual(got.Contributors, wantContributors) {
		t.Errorf("results[0].Contributors = %+v, want %+v (both contributors kept)", got.Contributors, wantContributors)
	}
	if len(findings) != 1 || findings[0].Kind != "gap" || findings[0].RowID != "SC-01" {
		t.Fatalf("findings = %+v, want exactly one gap finding for SC-01", findings)
	}
	if !strings.Contains(findings[0].Detail, "pkg/b_test.go:11") || !strings.Contains(findings[0].Detail, "TestSkip") {
		t.Errorf("Detail = %q, want it to name the skipped contributor", findings[0].Detail)
	}
}

func TestJoinUnexecutedContributorIsGapWithoutResultWhenNoneRan(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, EndLine: 9, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestBar", File: "pkg/bar_test.go", Line: 9, EndLine: 14, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestUnrelated"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none: no contributor executed, so nothing is credited", results)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want one gap per unexecuted contributor", findings)
	}
	var details []string
	for _, f := range findings {
		if f.Kind != "gap" || f.RowID != "SC-01" {
			t.Errorf("finding = %+v, want a gap bound to SC-01", f)
		}
		details = append(details, f.Detail)
	}
	joined := strings.Join(details, " | ")
	if !strings.Contains(joined, "pkg/foo_test.go:5") || !strings.Contains(joined, "pkg/bar_test.go:9") {
		t.Errorf("details = %q, want each unexecuted contributor named", joined)
	}
}

func TestJoinContributorsSortedByFileThenLine(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestB", File: "pkg/b_test.go", Line: 4, EndLine: 8, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestALate", File: "pkg/a_test.go", Line: 30, EndLine: 36, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestAEarly", File: "pkg/a_test.go", Line: 10, EndLine: 14, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestB"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestALate"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestAEarly"},
	}}

	results, _, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly one row result", results)
	}
	got := results[0]
	wantOrder := []string{"TestAEarly", "TestALate", "TestB"}
	if len(got.Contributors) != len(wantOrder) {
		t.Fatalf("Contributors = %+v, want %d", got.Contributors, len(wantOrder))
	}
	for i, want := range wantOrder {
		if got.Contributors[i].Name != want {
			t.Errorf("Contributors[%d].Name = %q, want %q (sorted by File then Line)", i, got.Contributors[i].Name, want)
		}
	}
	if got.Name != "TestAEarly" || got.File != "pkg/a_test.go" || got.Line != 10 || got.EndLine != 14 {
		t.Errorf("primary identity = %+v, want the first contributor by File then Line", got)
	}
}

func TestJoinAmbiguousTagSiteProducesAmbiguityFinding(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}, Ambiguous: true, Detail: "func TestFoo: has a receiver; go test only runs top-level functions"},
	}
	rep := &gotestjson.Report{}

	_, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	wantAmbiguity := Finding{Kind: "ambiguity", RowID: "", Detail: "pkg/foo_test.go:5: func TestFoo: has a receiver; go test only runs top-level functions"}
	found := false
	for _, f := range findings {
		if f == wantAmbiguity {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want %+v present", findings, wantAmbiguity)
	}
}

func TestJoinRunnerFailuresBecomeRedFindingsNeverDropped(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	rep := &gotestjson.Report{
		Failures: []gotestjson.RunnerFailure{
			{Package: "pkg", Detail: "boom"},
			{Package: "pkg [pkg.test]", Detail: "build failed"},
		},
	}

	_, findings, _, err := Join(src, reg, nil, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	want := []Finding{
		{Kind: "red", RowID: "", Detail: "runner: pkg [pkg.test]: build failed"},
		{Kind: "red", RowID: "", Detail: "runner: pkg: boom"},
		{Kind: "gap", RowID: "SC-01", Detail: "no test cites SC-01"},
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].Kind != want[j].Kind {
			return want[i].Kind < want[j].Kind
		}
		if want[i].RowID != want[j].RowID {
			return want[i].RowID < want[j].RowID
		}
		return want[i].Detail < want[j].Detail
	})
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %+v, want %+v", findings, want)
	}
}

func TestJoinFamilyIncludesSubtestsByPrefix(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo/subtest_one"},
		{Status: runrecord.Failed, Scope: "pkg", Name: "TestFoo/subtest_two"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 || results[0].Status != runrecord.Failed {
		t.Fatalf("results = %+v, want one failed row result (worst-first collapse)", results)
	}
	if len(findings) != 1 || findings[0].Kind != "red" {
		t.Errorf("findings = %+v, want one red finding", findings)
	}
}

func TestJoinFamilyScopeIsolatesSameNameInDifferentPackages(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg/a", TestName: "TestFoo", File: "pkg/a/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg/a", Name: "TestFoo"},
		{Status: runrecord.Failed, Scope: "pkg/b", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 1 || results[0].Status != runrecord.Passed {
		t.Fatalf("results = %+v, want passed row result unaffected by pkg/b's failure", results)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

func TestJoinPartitionInvariantOverActiveRows(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc,
		coverage.Row{ID: "SC-01"}, // passed, one site -> RowResult only
		coverage.Row{ID: "SC-02"}, // no site -> gap only
		coverage.Row{ID: "SC-03"}, // site binds, family empty -> gap only
		coverage.Row{ID: "SC-05"}, // two sites bind, both passed -> one RowResult, two contributors
	)
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestBar", File: "pkg/bar_test.go", Line: 9, IDs: []string{"SC-03"}},
		{ImportPath: "pkg", TestName: "TestBaz", File: "pkg/baz_test.go", Line: 12, IDs: []string{"SC-05"}},
		{ImportPath: "pkg", TestName: "TestQux", File: "pkg/qux_test.go", Line: 20, IDs: []string{"SC-05"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestBaz"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestQux"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	rowIDs := []string{"SC-01", "SC-02", "SC-03", "SC-05"}
	for _, id := range rowIDs {
		resultCount := 0
		for _, r := range results {
			if r.RowID == id {
				resultCount++
			}
		}
		findingCount := 0
		for _, f := range findings {
			if f.RowID == id {
				findingCount++
			}
		}
		if resultCount > 1 {
			t.Errorf("row %s appears %d times in results, want at most once", id, resultCount)
		}
		if resultCount == 0 && findingCount == 0 {
			t.Errorf("row %s appears neither in results nor in a row-bound finding", id)
		}
	}

	var sc05 *RowResult
	for i := range results {
		if results[i].RowID == "SC-05" {
			sc05 = &results[i]
		}
	}
	if sc05 == nil {
		t.Fatalf("results = %+v, want one credited SC-05 row result", results)
	}
	if len(sc05.Contributors) != 2 {
		t.Errorf("SC-05 contributors = %+v, want both bound sites", sc05.Contributors)
	}
	for _, f := range findings {
		if f.RowID == "SC-05" {
			t.Errorf("finding = %+v, want none for the two-site SC-05 row", f)
		}
	}
}

func TestJoinDeterministicSortOrder(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc,
		coverage.Row{ID: "SC-01"},
		coverage.Row{ID: "SC-02"},
		coverage.Row{ID: "SC-03"},
	)
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestC", File: "pkg/c_test.go", Line: 1, IDs: []string{"SC-03"}},
		{ImportPath: "pkg", TestName: "TestA", File: "pkg/a_test.go", Line: 1, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestB", File: "pkg/b_test.go", Line: 1, IDs: []string{"SC-02", "SC-77"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestC"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestA"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestB"},
	}}

	results, findings, classes, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	if !sort.SliceIsSorted(results, func(i, j int) bool { return results[i].RowID < results[j].RowID }) {
		t.Errorf("results not sorted by RowID: %+v", results)
	}
	if !sort.SliceIsSorted(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].RowID != findings[j].RowID {
			return findings[i].RowID < findings[j].RowID
		}
		return findings[i].Detail < findings[j].Detail
	}) {
		t.Errorf("findings not sorted by (Kind, RowID, Detail): %+v", findings)
	}
	if !sort.SliceIsSorted(classes, func(i, j int) bool {
		if classes[i].File != classes[j].File {
			return classes[i].File < classes[j].File
		}
		if classes[i].Line != classes[j].Line {
			return classes[i].Line < classes[j].Line
		}
		return classes[i].ID < classes[j].ID
	}) {
		t.Errorf("classes not sorted by (File, Line, ID): %+v", classes)
	}
}

func TestJoinErrorsOnNilSource(t *testing.T) {
	reg, _ := buildJoinTestRegistry(t)
	_, _, _, err := Join(nil, reg, nil, &gotestjson.Report{})
	if err == nil {
		t.Fatal("Join: want error for nil source")
	}
}

func TestJoinErrorsOnNilRegistry(t *testing.T) {
	_, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	_, _, _, err := Join(src, nil, nil, &gotestjson.Report{})
	if err == nil {
		t.Fatal("Join: want error for nil registry")
	}
}

func TestJoinErrorsOnNilReport(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	_, _, _, err := Join(src, reg, nil, nil)
	if err == nil {
		t.Fatal("Join: want error for nil report")
	}
}

func TestJoinErrorsOnRowWithEmptyID(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: ""})
	_, _, _, err := Join(src, reg, nil, &gotestjson.Report{})
	if err == nil {
		t.Fatal("Join: want error for row with empty id")
	}
}
