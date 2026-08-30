package junitxml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/runrecord"
)

func fixturePath(elem ...string) string {
	return filepath.Join(append([]string{"..", "..", "fixtures", "reports", "jvm"}, elem...)...)
}

func openFixture(t *testing.T, elem ...string) *os.File {
	t.Helper()
	f, err := os.Open(fixturePath(elem...))
	if err != nil {
		t.Fatalf("open fixture %v: %v", elem, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestNormalizeFileMavenSurefireNestedCasesMapsAllFiveStatuses(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "maven-surefire", "TEST-com.example.verifier.SimpleTest$NestedCases.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Failed, ClassName: "com.example.verifier.SimpleTest", RawName: "failingAssertionTest"},
		{Status: runrecord.Skipped, ClassName: "com.example.verifier.SimpleTest", RawName: "disabledTest"},
		{Status: runrecord.Failed, ClassName: "com.example.verifier.SimpleTest", RawName: "throwingExceptionTest"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.SimpleTest", RawName: "passingTest"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.SimpleTest$NestedCases", RawName: "nestedPassingTest"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	for i, c := range want {
		if doc.Cases[i] != c {
			t.Errorf("Cases[%d] = %+v, want %+v", i, doc.Cases[i], c)
		}
	}
}

func TestNormalizeFileMavenSurefireOuterFileWithZeroTestsIsEmptyDocument(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "maven-surefire", "TEST-com.example.verifier.SimpleTest.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}
	if len(doc.Cases) != 0 {
		t.Errorf("Cases = %+v, want none", doc.Cases)
	}
}

func TestNormalizeFileMavenFailsafeNestedCasesMapsAllFiveStatuses(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "maven-failsafe", "TEST-com.example.verifier.SimpleIT$NestedCases.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Failed, ClassName: "com.example.verifier.SimpleIT", RawName: "failingAssertionTest"},
		{Status: runrecord.Skipped, ClassName: "com.example.verifier.SimpleIT", RawName: "disabledTest"},
		{Status: runrecord.Failed, ClassName: "com.example.verifier.SimpleIT", RawName: "throwingExceptionTest"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.SimpleIT", RawName: "passingTest"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.SimpleIT$NestedCases", RawName: "nestedPassingTest"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	for i, c := range want {
		if doc.Cases[i] != c {
			t.Errorf("Cases[%d] = %+v, want %+v", i, doc.Cases[i], c)
		}
	}
}

func TestNormalizeFileGradleJavaSimpleTestFoldsErrorIntoFailure(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "gradle-java", "TEST-com.example.verifier.SimpleTest.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Failed, ClassName: "com.example.verifier.SimpleTest", RawName: "failingAssertionTest()"},
		{Status: runrecord.Skipped, ClassName: "com.example.verifier.SimpleTest", RawName: "disabledTest()"},
		{Status: runrecord.Failed, ClassName: "com.example.verifier.SimpleTest", RawName: "throwingExceptionTest()"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.SimpleTest", RawName: "passingTest()"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	for i, c := range want {
		if doc.Cases[i] != c {
			t.Errorf("Cases[%d] = %+v, want %+v", i, doc.Cases[i], c)
		}
	}
}

func TestNormalizeFileGradleJavaParamTestKeepsBracketedInvocationNamesVerbatim(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "gradle-java", "TEST-com.example.verifier.ParamTest.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Passed, ClassName: "com.example.verifier.ParamTest", RawName: "[1] 1"},
		{Status: runrecord.Failed, ClassName: "com.example.verifier.ParamTest", RawName: "[2] 2"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.ParamTest", RawName: "[3] 3"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.ParamTest", RawName: "[1] 1"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.ParamTest", RawName: "[2] 2"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.ParamTest", RawName: "[3] 3"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	for i, c := range want {
		if doc.Cases[i] != c {
			t.Errorf("Cases[%d] = %+v, want %+v", i, doc.Cases[i], c)
		}
	}
}

func TestNormalizeFileGradleKotlinBacktickNamesKeptVerbatim(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "gradle-kotlin", "TEST-com.example.verifier.BacktickTest.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Passed, ClassName: "com.example.verifier.BacktickTest", RawName: "passes with backticks and spaces()"},
		{Status: runrecord.Passed, ClassName: "com.example.verifier.BacktickTest", RawName: "alsoBackticked()"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	for i, c := range want {
		if doc.Cases[i] != c {
			t.Errorf("Cases[%d] = %+v, want %+v", i, doc.Cases[i], c)
		}
	}
}

func TestNormalizeFileNamespacedFixtureMatchesByLocalName(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "namespaced.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Passed, ClassName: "com.example.verifier.NamespacedTest", RawName: "passingTest"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	if doc.Cases[0] != want[0] {
		t.Errorf("Cases[0] = %+v, want %+v", doc.Cases[0], want[0])
	}
}

func TestNormalizeFileTestsuitesRootRecursesIntoNestedTestsuites(t *testing.T) {
	input := `<?xml version="1.0"?>
<testsuites>
  <testsuite name="a" tests="1">
    <testcase classname="pkg.A" name="one"/>
  </testsuite>
  <testsuite name="b" tests="1">
    <testcase classname="pkg.B" name="two"/>
  </testsuite>
</testsuites>`

	doc, err := NormalizeFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}

	want := []Case{
		{Status: runrecord.Passed, ClassName: "pkg.A", RawName: "one"},
		{Status: runrecord.Passed, ClassName: "pkg.B", RawName: "two"},
	}
	if len(doc.Cases) != len(want) {
		t.Fatalf("Cases = %+v, want %d entries", doc.Cases, len(want))
	}
	for i, c := range want {
		if doc.Cases[i] != c {
			t.Errorf("Cases[%d] = %+v, want %+v", i, doc.Cases[i], c)
		}
	}
}

func TestNormalizeFileEmptyTestsuiteProducesEmptyDocument(t *testing.T) {
	doc, err := NormalizeFile(strings.NewReader(`<testsuite name="empty" tests="0"/>`))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}
	if len(doc.Cases) != 0 {
		t.Errorf("Cases = %+v, want none", doc.Cases)
	}
}

func TestNormalizeFileDTDEntityFixtureRejected(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "dtd-entity.xml"))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "doctype") && !strings.Contains(lower, "entity") {
		t.Errorf("error = %q, want it to name DOCTYPE or ENTITY", err.Error())
	}
}

func TestNormalizeFileInlineDoctypeWithoutEntityRejected(t *testing.T) {
	input := `<?xml version="1.0"?><!DOCTYPE testsuite><testsuite name="x" tests="0"/>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "doctype") {
		t.Errorf("error = %q, want it to name DOCTYPE", err.Error())
	}
}

func TestNormalizeFileMalformedFixtureErrorsWithPositionContext(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "malformed.xml"))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("error = %q, want it to name a line for position context", err.Error())
	}
}

func TestNormalizeFileMissingClassnameFixtureErrors(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "missing-classname.xml"))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(err.Error(), "classname") {
		t.Errorf("error = %q, want it to name the missing classname attribute", err.Error())
	}
}

func TestNormalizeFileDoctypeInsideTestsuiteBodyRejected(t *testing.T) {
	input := `<testsuite name="s" tests="1"><!DOCTYPE evil><testcase classname="pkg.A" name="one"/></testsuite>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "doctype") {
		t.Errorf("error = %q, want it to name DOCTYPE", err.Error())
	}
}

func TestNormalizeFileDoctypeInsideTestcaseBodyRejected(t *testing.T) {
	input := `<testsuite name="s" tests="1"><testcase classname="pkg.A" name="one"><!DOCTYPE evil><failure/></testcase></testsuite>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "doctype") {
		t.Errorf("error = %q, want it to name DOCTYPE", err.Error())
	}
}

func TestNormalizeFileTestcaseMissingNameErrors(t *testing.T) {
	input := `<testsuite name="s" tests="1"><testcase classname="pkg.A"/></testsuite>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want it to name the missing name attribute", err.Error())
	}
}

func TestNormalizeFileUnrecognizedStatusChildErrors(t *testing.T) {
	input := `<testsuite name="s" tests="1"><testcase classname="pkg.A" name="one"><incomplete/></testcase></testsuite>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error = %q, want it to name the unrecognized child %q", err.Error(), "incomplete")
	}
}

func TestNormalizeFileBothFailureAndErrorChildrenYieldsFailed(t *testing.T) {
	input := `<testsuite name="s" tests="1"><testcase classname="pkg.A" name="one"><failure message="f"/><error message="e"/></testcase></testsuite>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}
	if len(doc.Cases) != 1 {
		t.Fatalf("Cases = %+v, want 1 entry", doc.Cases)
	}
	if doc.Cases[0].Status != runrecord.Failed {
		t.Errorf("Status = %q, want %q", doc.Cases[0].Status, runrecord.Failed)
	}
}

func TestNormalizeFileRecognizedSkipFormsMapToSkipped(t *testing.T) {
	tests := []struct {
		name  string
		child string
	}{
		{"skipped", "skipped"},
		{"disabled", "disabled"},
		{"ignored", "ignored"},
		{"pending", "pending"},
		{"aborted", "aborted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `<testsuite name="s" tests="1"><testcase classname="pkg.A" name="one"><` + tt.child + `/></testcase></testsuite>`
			doc, err := NormalizeFile(strings.NewReader(input))
			if err != nil {
				t.Fatalf("NormalizeFile: %v", err)
			}
			if len(doc.Cases) != 1 {
				t.Fatalf("Cases = %+v, want 1 entry", doc.Cases)
			}
			if doc.Cases[0].Status != runrecord.Skipped {
				t.Errorf("Status = %q, want %q", doc.Cases[0].Status, runrecord.Skipped)
			}
		})
	}
}

func TestNormalizeFileIgnorableChildrenDoNotAffectStatus(t *testing.T) {
	input := `<testsuite name="s" tests="1"><properties><property name="k" value="v"/></properties>` +
		`<testcase classname="pkg.A" name="one"><system-out>out</system-out><system-err>err</system-err></testcase>` +
		`<system-out>suite out</system-out><system-err>suite err</system-err></testsuite>`
	doc, err := NormalizeFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}
	if len(doc.Cases) != 1 {
		t.Fatalf("Cases = %+v, want 1 entry", doc.Cases)
	}
	if doc.Cases[0].Status != runrecord.Passed {
		t.Errorf("Status = %q, want %q", doc.Cases[0].Status, runrecord.Passed)
	}
}

func TestNormalizeFileEmptyInputErrors(t *testing.T) {
	doc, err := NormalizeFile(strings.NewReader(""))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
}

func TestNormalizeFileUnknownRootElementErrors(t *testing.T) {
	doc, err := NormalizeFile(strings.NewReader(`<report/>`))
	if err == nil {
		t.Fatalf("NormalizeFile returned nil error with %+v, want error", doc)
	}
	if doc != nil {
		t.Errorf("NormalizeFile returned %+v alongside error, want nil *Document", doc)
	}
}
