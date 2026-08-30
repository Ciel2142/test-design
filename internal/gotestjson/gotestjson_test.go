package gotestjson

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/runrecord"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "fixtures", "reports", "go", name)
}

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(fixturePath(name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestNormalizeHappyMixProducesRecordsForEachTestEvent(t *testing.T) {
	report, err := Normalize(openFixture(t, "happy-mix.json"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	want := []runrecord.Record{
		{Status: runrecord.Passed, Scope: "happyfixture", Name: "TestPass"},
		{Status: runrecord.Failed, Scope: "happyfixture", Name: "TestFail"},
		{Status: runrecord.Skipped, Scope: "happyfixture", Name: "TestSkip"},
		{Status: runrecord.Passed, Scope: "happyfixture", Name: "TestParent/childOK"},
		{Status: runrecord.Failed, Scope: "happyfixture", Name: "TestParent/childFail"},
		{Status: runrecord.Failed, Scope: "happyfixture", Name: "TestParent"},
	}
	if len(report.Records) != len(want) {
		t.Fatalf("Records = %+v, want %d entries", report.Records, len(want))
	}
	for i, r := range want {
		if report.Records[i] != r {
			t.Errorf("Records[%d] = %+v, want %+v", i, report.Records[i], r)
		}
	}
}

func TestNormalizeHappyMixSubtestNamesStayWhole(t *testing.T) {
	report, err := Normalize(openFixture(t, "happy-mix.json"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	found := false
	for _, r := range report.Records {
		if r.Name == "TestParent/childFail" {
			found = true
		}
		if strings.Contains(r.Name, "/") && r.Name != "TestParent/childOK" && r.Name != "TestParent/childFail" {
			t.Errorf("unexpected subtest name %q", r.Name)
		}
	}
	if !found {
		t.Fatalf("expected a record named TestParent/childFail, got %+v", report.Records)
	}
}

func TestNormalizeHappyMixCountsTallyByStatus(t *testing.T) {
	report, err := Normalize(openFixture(t, "happy-mix.json"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	want := Counts{Passed: 2, Failed: 3, Skipped: 1}
	if report.Counts != want {
		t.Errorf("Counts = %+v, want %+v", report.Counts, want)
	}
}

func TestNormalizeHappyMixPackageFailureBecomesRunnerFailure(t *testing.T) {
	report, err := Normalize(openFixture(t, "happy-mix.json"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if len(report.Failures) != 1 {
		t.Fatalf("Failures = %+v, want 1 entry", report.Failures)
	}
	got := report.Failures[0]
	if got.Package != "happyfixture" {
		t.Errorf("Failures[0].Package = %q, want %q", got.Package, "happyfixture")
	}
	wantDetail := "FAIL\nFAIL\thappyfixture\t0.003s\n"
	if got.Detail != wantDetail {
		t.Errorf("Failures[0].Detail = %q, want %q", got.Detail, wantDetail)
	}
}

func TestNormalizeBuildFailureBecomesRunnerFailures(t *testing.T) {
	report, err := Normalize(openFixture(t, "build-fail.json"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if len(report.Records) != 0 {
		t.Errorf("Records = %+v, want none", report.Records)
	}
	if len(report.Failures) != 2 {
		t.Fatalf("Failures = %+v, want 2 entries", report.Failures)
	}

	build := report.Failures[0]
	if build.Package != "buildfailfixture [buildfailfixture.test]" {
		t.Errorf("Failures[0].Package = %q", build.Package)
	}
	wantBuildDetail := "# buildfailfixture [buildfailfixture.test]\n" +
		"./buildfail_test.go:6:14: cannot use \"not an int\" (untyped string constant) as int value in variable declaration\n"
	if build.Detail != wantBuildDetail {
		t.Errorf("Failures[0].Detail = %q, want %q", build.Detail, wantBuildDetail)
	}

	pkgFail := report.Failures[1]
	if pkgFail.Package != "buildfailfixture" {
		t.Errorf("Failures[1].Package = %q, want %q", pkgFail.Package, "buildfailfixture")
	}
	wantPkgDetail := "FAIL\tbuildfailfixture [build failed]\n"
	if pkgFail.Detail != wantPkgDetail {
		t.Errorf("Failures[1].Detail = %q, want %q", pkgFail.Detail, wantPkgDetail)
	}

	if report.Counts != (Counts{}) {
		t.Errorf("Counts = %+v, want zero value", report.Counts)
	}
}

func TestNormalizeMalformedLineErrorsNamingLineNumber(t *testing.T) {
	data, err := os.ReadFile(fixturePath("happy-mix.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	lines[2] = "{not valid json"
	corrupted := strings.NewReader(strings.Join(lines, "\n") + "\n")

	report, err := Normalize(corrupted)
	if err == nil {
		t.Fatalf("Normalize returned nil error with %+v, want error", report)
	}
	if report != nil {
		t.Errorf("Normalize returned %+v alongside error, want nil *Report", report)
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %q, want it to name line 3", err.Error())
	}
}

func TestNormalizeFailWithNoCapturedOutputUsesFixedPhraseNamingPackage(t *testing.T) {
	report, err := Normalize(strings.NewReader(`{"Action":"fail","Package":"lonelypkg"}` + "\n"))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("Failures = %+v, want 1 entry", report.Failures)
	}
	got := report.Failures[0]
	if got.Package != "lonelypkg" {
		t.Errorf("Failures[0].Package = %q, want %q", got.Package, "lonelypkg")
	}
	if !strings.Contains(got.Detail, "lonelypkg") {
		t.Errorf("Failures[0].Detail = %q, want it to name the package", got.Detail)
	}
}

func TestNormalizeBlankLinesAreTolerated(t *testing.T) {
	input := "\n" +
		`{"Action":"pass","Package":"pkg","Test":"TestOK"}` + "\n" +
		"\n" +
		`{"Action":"run","Package":"pkg"}` + "\n" +
		"\n"

	report, err := Normalize(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(report.Records) != 1 {
		t.Fatalf("Records = %+v, want 1 entry", report.Records)
	}
}

func TestNormalizeEmptyStreamProducesEmptyReport(t *testing.T) {
	report, err := Normalize(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(report.Records) != 0 {
		t.Errorf("Records = %+v, want none", report.Records)
	}
	if len(report.Failures) != 0 {
		t.Errorf("Failures = %+v, want none", report.Failures)
	}
	if report.Counts != (Counts{}) {
		t.Errorf("Counts = %+v, want zero value", report.Counts)
	}
}

func TestNormalizeMetadataActionsProduceNoRecordAndNoError(t *testing.T) {
	input := strings.Join([]string{
		`{"Action":"start","Package":"pkg"}`,
		`{"Action":"run","Package":"pkg","Test":"TestOK"}`,
		`{"Action":"output","Package":"pkg","Test":"TestOK","Output":"=== RUN\n"}`,
		`{"Action":"pause","Package":"pkg","Test":"TestOK"}`,
		`{"Action":"cont","Package":"pkg","Test":"TestOK"}`,
		`{"Action":"bench","Package":"pkg","Test":"BenchmarkX"}`,
		`{"Action":"pass","Package":"pkg","Test":"TestOK"}`,
	}, "\n") + "\n"

	report, err := Normalize(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(report.Records) != 1 {
		t.Fatalf("Records = %+v, want 1 entry (bench produces none)", report.Records)
	}
	if len(report.Failures) != 0 {
		t.Errorf("Failures = %+v, want none", report.Failures)
	}
}
