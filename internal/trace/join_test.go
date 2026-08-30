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
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, classes, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	wantResults := []RowResult{{RowID: "SC-01", Scope: "pkg", Name: "TestFoo", Status: runrecord.Passed}}
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

func TestJoinTwoSitesBindSameRowProducesAmbiguity(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc, coverage.Row{ID: "SC-01"})
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestBar", File: "pkg/bar_test.go", Line: 9, IDs: []string{"SC-01"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestBar"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none (fail closed, no winner)", results)
	}
	if len(findings) != 1 || findings[0].Kind != "ambiguity" || findings[0].RowID != "SC-01" {
		t.Fatalf("findings = %+v, want one ambiguity finding for SC-01", findings)
	}
	if !strings.Contains(findings[0].Detail, "pkg/foo_test.go:5") || !strings.Contains(findings[0].Detail, "pkg/bar_test.go:9") {
		t.Errorf("Detail = %q, want both sites named", findings[0].Detail)
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
		if f.Kind == "drift" || f.Kind == "overclaim" {
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

func TestJoinSiteWithMoreThanTwoActiveIDsOverclaims(t *testing.T) {
	reg, thisDoc := buildJoinTestRegistry(t)
	src := basicSource(thisDoc,
		coverage.Row{ID: "SC-01"},
		coverage.Row{ID: "SC-02"},
		coverage.Row{ID: "SC-03"},
	)
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01", "SC-02", "SC-03"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none (overclaim site binds nothing)", results)
	}
	overclaimCount := 0
	gapCount := 0
	for _, f := range findings {
		switch f.Kind {
		case "overclaim":
			overclaimCount++
			if !strings.Contains(f.Detail, "TestFoo") || !strings.Contains(f.Detail, "SC-01") || !strings.Contains(f.Detail, "SC-02") || !strings.Contains(f.Detail, "SC-03") {
				t.Errorf("overclaim Detail = %q, want test name and all IDs", f.Detail)
			}
		case "gap":
			gapCount++
		}
	}
	if overclaimCount != 1 {
		t.Errorf("overclaim findings = %d, want 1", overclaimCount)
	}
	if gapCount != 3 {
		t.Errorf("gap findings = %d, want 3 (each row unbound)", gapCount)
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
	wantAmbiguity := Finding{Kind: "ambiguity", RowID: "", Detail: "func TestFoo: has a receiver; go test only runs top-level functions"}
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
		coverage.Row{ID: "SC-01"}, // passed, bound -> RowResult only
		coverage.Row{ID: "SC-02"}, // no site -> gap only
		coverage.Row{ID: "SC-03"}, // site binds, family empty -> gap only
		coverage.Row{ID: "SC-05"}, // two sites bind -> ambiguity only
	)
	sites := []TagSite{
		{ImportPath: "pkg", TestName: "TestFoo", File: "pkg/foo_test.go", Line: 5, IDs: []string{"SC-01"}},
		{ImportPath: "pkg", TestName: "TestBar", File: "pkg/bar_test.go", Line: 9, IDs: []string{"SC-03"}},
		{ImportPath: "pkg", TestName: "TestBaz", File: "pkg/baz_test.go", Line: 12, IDs: []string{"SC-05"}},
		{ImportPath: "pkg", TestName: "TestQux", File: "pkg/qux_test.go", Line: 20, IDs: []string{"SC-05"}},
	}
	rep := &gotestjson.Report{Records: []runrecord.Record{
		{Status: runrecord.Passed, Scope: "pkg", Name: "TestFoo"},
	}}

	results, findings, _, err := Join(src, reg, sites, rep)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	rowIDs := []string{"SC-01", "SC-02", "SC-03", "SC-05"}
	for _, id := range rowIDs {
		inResults := false
		for _, r := range results {
			if r.RowID == id {
				inResults = true
			}
		}
		findingCount := 0
		for _, f := range findings {
			if f.RowID == id {
				findingCount++
			}
		}
		switch {
		case inResults && findingCount == 0:
		case !inResults && findingCount == 1:
		default:
			t.Errorf("row %s: inResults=%v findingCount=%d, want exactly one representation", id, inResults, findingCount)
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
