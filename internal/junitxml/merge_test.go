package junitxml

import (
	"reflect"
	"testing"

	"test-design/internal/runrecord"
)

func TestMergeNoDocumentsProducesEmptyReport(t *testing.T) {
	report, err := Merge(nil)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 0 {
		t.Errorf("Families = %+v, want none", report.Families)
	}
	if report.Counts != (Counts{}) {
		t.Errorf("Counts = %+v, want zero value", report.Counts)
	}
}

func TestMergeSingleDocumentGroupsCasesByClassAndMethod(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "one"},
		{Status: runrecord.Failed, ClassName: "pkg.A", RawName: "one"},
		{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "two"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 2 {
		t.Fatalf("Families = %+v, want 2 entries", report.Families)
	}

	one := report.Families[0]
	if one.Identity.Method != "one" || one.Status != runrecord.Failed || len(one.Cases) != 2 {
		t.Errorf("Families[0] = %+v, want method %q status %q with 2 cases", one, "one", runrecord.Failed)
	}
	two := report.Families[1]
	if two.Identity.Method != "two" || two.Status != runrecord.Passed || len(two.Cases) != 1 {
		t.Errorf("Families[1] = %+v, want method %q status %q with 1 case", two, "two", runrecord.Passed)
	}
}

func TestMergeConcatenatesCasesAcrossDocumentsInInputOrder(t *testing.T) {
	doc1 := &Document{Cases: []Case{{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "one"}}}
	doc2 := &Document{Cases: []Case{{Status: runrecord.Failed, ClassName: "pkg.A", RawName: "one"}}}

	report, err := Merge([]*Document{doc1, doc2})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 1 {
		t.Fatalf("Families = %+v, want 1 entry", report.Families)
	}
	want := []Case{doc1.Cases[0], doc2.Cases[0]}
	if !reflect.DeepEqual(report.Families[0].Cases, want) {
		t.Errorf("Cases = %+v, want %+v (doc1 before doc2)", report.Families[0].Cases, want)
	}
	if report.Families[0].Status != runrecord.Failed {
		t.Errorf("Status = %q, want %q", report.Families[0].Status, runrecord.Failed)
	}
}

func TestMergeSameMethodNameDifferentClassesProducesDistinctFamilies(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.Beta", RawName: "shared"},
		{Status: runrecord.Passed, ClassName: "pkg.Alpha", RawName: "shared"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 2 {
		t.Fatalf("Families = %+v, want 2 entries", report.Families)
	}
	if report.Families[0].Identity.ClassName != "pkg.Alpha" || report.Families[1].Identity.ClassName != "pkg.Beta" {
		t.Errorf("Families order = [%q, %q], want [pkg.Alpha, pkg.Beta] (class name sorted)",
			report.Families[0].Identity.ClassName, report.Families[1].Identity.ClassName)
	}
}

func TestMergeUnsupportedFamilyGroupsByRawNameKeepsDistinctNamesDistinct(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.ParamTest", RawName: "[1] 1"},
		{Status: runrecord.Failed, ClassName: "pkg.ParamTest", RawName: "[2] 2"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 2 {
		t.Fatalf("Families = %+v, want 2 entries (distinct raw names never merged)", report.Families)
	}
	for _, f := range report.Families {
		if f.Identity.Unsupported == "" {
			t.Errorf("Families Identity.Unsupported = %q, want non-empty", f.Identity.Unsupported)
		}
		if f.Identity.Method != "" {
			t.Errorf("Families Identity.Method = %q, want empty for an unsupported family", f.Identity.Method)
		}
	}
	if report.Families[0].Cases[0].RawName != "[1] 1" || report.Families[1].Cases[0].RawName != "[2] 2" {
		t.Errorf("Families raw names = [%q, %q], want [\"[1] 1\", \"[2] 2\"]",
			report.Families[0].Cases[0].RawName, report.Families[1].Cases[0].RawName)
	}
}

func TestMergeUnsupportedFamilyCollapsesWorstFirstAcrossRawNameCollisions(t *testing.T) {
	// gradle "[N] args" decoration repeats across methods; same raw name -> same family.
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.ParamTest", RawName: "[2] 2"},
		{Status: runrecord.Failed, ClassName: "pkg.ParamTest", RawName: "[2] 2"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 1 {
		t.Fatalf("Families = %+v, want 1 entry", report.Families)
	}
	f := report.Families[0]
	if f.Status != runrecord.Failed {
		t.Errorf("Status = %q, want %q", f.Status, runrecord.Failed)
	}
	if len(f.Cases) != 2 {
		t.Errorf("Cases = %+v, want 2 entries", f.Cases)
	}
	if f.Identity.Unsupported == "" {
		t.Errorf("Identity.Unsupported = %q, want non-empty", f.Identity.Unsupported)
	}
}

func TestMergeCollapsesWorstFirst(t *testing.T) {
	tests := []struct {
		name     string
		statuses []runrecord.Status
		want     runrecord.Status
	}{
		{"any failed wins", []runrecord.Status{runrecord.Passed, runrecord.Skipped, runrecord.Failed}, runrecord.Failed},
		{"skipped beats passed", []runrecord.Status{runrecord.Passed, runrecord.Skipped}, runrecord.Skipped},
		{"all passed", []runrecord.Status{runrecord.Passed, runrecord.Passed}, runrecord.Passed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cases []Case
			for _, s := range tt.statuses {
				cases = append(cases, Case{Status: s, ClassName: "pkg.A", RawName: "one"})
			}
			report, err := Merge([]*Document{{Cases: cases}})
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			if len(report.Families) != 1 {
				t.Fatalf("Families = %+v, want 1 entry", report.Families)
			}
			if report.Families[0].Status != tt.want {
				t.Errorf("Status = %q, want %q", report.Families[0].Status, tt.want)
			}
		})
	}
}

func TestMergeFamiliesOrderedByClassNameThenMethod(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.B", RawName: "zeta"},
		{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "zeta"},
		{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "alpha"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	var got [][2]string
	for _, f := range report.Families {
		got = append(got, [2]string{f.Identity.ClassName, f.Identity.Method})
	}
	want := [][2]string{{"pkg.A", "alpha"}, {"pkg.A", "zeta"}, {"pkg.B", "zeta"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestMergeUnsupportedFamilyOrderedByRawNameInMethodPosition(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.Mixed", RawName: "bravo"},
		{Status: runrecord.Passed, ClassName: "pkg.Mixed", RawName: "alpha unsupported text"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 2 {
		t.Fatalf("Families = %+v, want 2 entries", report.Families)
	}
	if report.Families[0].Identity.Unsupported == "" {
		t.Errorf("Families[0] = %+v, want the unsupported family first (RawName sorts before %q)", report.Families[0], "bravo")
	}
	if report.Families[1].Identity.Method != "bravo" {
		t.Errorf("Families[1].Identity.Method = %q, want %q", report.Families[1].Identity.Method, "bravo")
	}
}

func TestMergeCountsRawPerCaseTallyBeforeCollapse(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "one"},
		{Status: runrecord.Failed, ClassName: "pkg.A", RawName: "one"},
		{Status: runrecord.Skipped, ClassName: "pkg.B", RawName: "two"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	want := Counts{Passed: 1, Failed: 1, Skipped: 1}
	if report.Counts != want {
		t.Errorf("Counts = %+v, want %+v", report.Counts, want)
	}
	// Counts is pre-collapse: pkg.A.one collapses to Failed below, but its passing member still counts.
	if report.Families[0].Status != runrecord.Failed {
		t.Fatalf("Families[0].Status = %q, want %q", report.Families[0].Status, runrecord.Failed)
	}
}

func TestMergeNoDeduplicationAcrossFiles(t *testing.T) {
	doc1 := &Document{Cases: []Case{{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "one"}}}
	doc2 := &Document{Cases: []Case{{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "one"}}}

	report, err := Merge([]*Document{doc1, doc2})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 1 {
		t.Fatalf("Families = %+v, want 1 entry", report.Families)
	}
	if len(report.Families[0].Cases) != 2 {
		t.Errorf("Cases = %+v, want 2 entries (no cross-file dedup)", report.Families[0].Cases)
	}
	if report.Counts.Passed != 2 {
		t.Errorf("Counts.Passed = %d, want 2", report.Counts.Passed)
	}
}

func TestMergeUnsupportedMarkingPropagatesToFamilyIdentity(t *testing.T) {
	docs := []*Document{{Cases: []Case{
		{Status: runrecord.Passed, ClassName: "pkg.DisplayNameTest", RawName: "custom method display"},
	}}}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(report.Families) != 1 {
		t.Fatalf("Families = %+v, want 1 entry", report.Families)
	}
	want := DeriveIdentity(docs[0].Cases[0])
	if report.Families[0].Identity != want {
		t.Errorf("Identity = %+v, want %+v", report.Families[0].Identity, want)
	}
}
