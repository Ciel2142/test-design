package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
)

func newBaselineFixture() *Baseline {
	return &Baseline{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RepoCommit:   "abc123",
		RunnerKind:   "go",
		Command:      []string{"go", "test", "./..."},
		ToolVersions: map[string]string{"go": "go1.26"},
		Counts:       gotestjson.Counts{Passed: 2, Failed: 1, Skipped: 0},
		Failed: []BaselineIdentity{
			{Scope: "pkg", Name: "TestB"},
			{Scope: "pkg", Name: "TestA"},
		},
		Skipped:      []BaselineIdentity{{Scope: "pkg", Name: "TestC"}},
		ReportSHA256: strings.Repeat("a", 64),
		StartedAt:    "2026-08-30T00:00:00Z",
		FinishedAt:   "2026-08-30T00:01:00Z",
		ExitStatus:   1,
	}
}

func TestWriteBaselineSortsIdentitiesForByteIdenticalOutput(t *testing.T) {
	dir := t.TempDir()

	unsorted := newBaselineFixture()
	unsorted.Failed = []BaselineIdentity{
		{Scope: "pkg", Name: "TestB"},
		{Scope: "pkg", Name: "TestA"},
	}
	pathUnsorted := filepath.Join(dir, "unsorted.json")
	if err := WriteBaseline(pathUnsorted, unsorted); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}

	sorted := newBaselineFixture()
	sorted.Failed = []BaselineIdentity{
		{Scope: "pkg", Name: "TestA"},
		{Scope: "pkg", Name: "TestB"},
	}
	pathSorted := filepath.Join(dir, "sorted.json")
	if err := WriteBaseline(pathSorted, sorted); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}

	dataUnsorted, err := os.ReadFile(pathUnsorted)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	dataSorted, err := os.ReadFile(pathSorted)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(dataUnsorted) != string(dataSorted) {
		t.Errorf("WriteBaseline output differs for same logical content with different input order:\nunsorted-input: %s\nsorted-input: %s", dataUnsorted, dataSorted)
	}

	idxA := strings.Index(string(dataSorted), "TestA")
	idxB := strings.Index(string(dataSorted), "TestB")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Errorf("WriteBaseline did not sort Failed identities: %s", dataSorted)
	}
}

func TestWriteBaselineDoesNotMutateCallersIdentitySlices(t *testing.T) {
	dir := t.TempDir()
	b := newBaselineFixture()
	original := append([]BaselineIdentity(nil), b.Failed...)

	if err := WriteBaseline(filepath.Join(dir, "b.json"), b); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}
	for i := range original {
		if b.Failed[i] != original[i] {
			t.Errorf("WriteBaseline mutated caller's Failed slice: got %+v, want %+v", b.Failed, original)
			break
		}
	}
}

func TestLoadBaselineRoundTripsWithMatchingHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	want := newBaselineFixture()

	if err := WriteBaseline(path, want); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}
	sum, err := evidence.SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}

	got, err := LoadBaseline(path, sum)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got.RepoCommit != want.RepoCommit || got.RunnerKind != want.RunnerKind {
		t.Errorf("LoadBaseline = %+v, want fields matching %+v", got, want)
	}
	if len(got.Failed) != 2 || got.Failed[0].Name != "TestA" || got.Failed[1].Name != "TestB" {
		t.Errorf("LoadBaseline.Failed = %+v, want sorted TestA, TestB", got.Failed)
	}
	if got.Counts != want.Counts {
		t.Errorf("LoadBaseline.Counts = %+v, want %+v", got.Counts, want.Counts)
	}
}

func TestLoadBaselineEmptyWantSHA256SkipsHashCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	if err := WriteBaseline(path, newBaselineFixture()); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}

	got, err := LoadBaseline(path, "")
	if err != nil {
		t.Fatalf("LoadBaseline with empty wantSHA256: %v", err)
	}
	if got == nil {
		t.Fatalf("LoadBaseline returned nil Baseline with nil error")
	}
}

func TestLoadBaselineHashMismatchErrorsNamingPathExpectedActual(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")
	if err := WriteBaseline(path, newBaselineFixture()); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}
	actual, err := evidence.SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	wrongWant := strings.Repeat("f", 64)

	got, err := LoadBaseline(path, wrongWant)
	if err == nil {
		t.Fatalf("LoadBaseline returned nil error with %+v, want error", got)
	}
	if got != nil {
		t.Errorf("LoadBaseline returned non-nil Baseline %+v alongside an error", got)
	}
	if !strings.Contains(err.Error(), "baseline evidence hash mismatch") {
		t.Errorf("LoadBaseline error %q does not name the hash-mismatch condition", err.Error())
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("LoadBaseline error %q does not name path %q", err.Error(), path)
	}
	if !strings.Contains(err.Error(), wrongWant) {
		t.Errorf("LoadBaseline error %q does not name expected hash %q", err.Error(), wrongWant)
	}
	if !strings.Contains(err.Error(), actual) {
		t.Errorf("LoadBaseline error %q does not name actual hash %q", err.Error(), actual)
	}
}

func TestLoadBaselineMissingFileErrorsNamingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	got, err := LoadBaseline(path, "")
	if err == nil {
		t.Fatalf("LoadBaseline returned nil error with %+v, want error", got)
	}
	if got != nil {
		t.Errorf("LoadBaseline returned non-nil Baseline %+v alongside an error", got)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("LoadBaseline error %q does not name path %q", err.Error(), path)
	}
}

func TestLoadBaselineMalformedJSONErrorsNamingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion": `), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := LoadBaseline(path, "")
	if err == nil {
		t.Fatalf("LoadBaseline returned nil error with %+v, want error", got)
	}
	if got != nil {
		t.Errorf("LoadBaseline returned non-nil Baseline %+v alongside an error", got)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("LoadBaseline error %q does not name path %q", err.Error(), path)
	}
}

func TestLoadBaselineUnsupportedSchemaVersionSurfacesDecodeEnvelopeError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"99","verifierVersion":"9.9.9"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := LoadBaseline(path, "")
	if err == nil {
		t.Fatalf("LoadBaseline returned nil error with %+v, want error", got)
	}
	if got != nil {
		t.Errorf("LoadBaseline returned non-nil Baseline %+v alongside an error", got)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("LoadBaseline error %q does not surface the unsupported version found", err.Error())
	}
	if !strings.Contains(err.Error(), evidence.SchemaVersion) {
		t.Errorf("LoadBaseline error %q does not surface the version supported", err.Error())
	}
}

func failedRecord(scope, name string) runrecord.Record {
	return runrecord.Record{Status: runrecord.Failed, Scope: scope, Name: name}
}

func passedRecord(scope, name string) runrecord.Record {
	return runrecord.Record{Status: runrecord.Passed, Scope: scope, Name: name}
}

func TestCompareBaselineNewFailureNotInBaselineIsRed(t *testing.T) {
	b := &Baseline{Failed: []BaselineIdentity{{Scope: "pkg", Name: "TestKnown"}}}
	rep := &gotestjson.Report{Records: []runrecord.Record{failedRecord("pkg", "TestNew")}}

	findings := CompareBaseline(b, rep)

	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want exactly one", findings)
	}
	if findings[0].Kind != "red" {
		t.Errorf("findings[0].Kind = %q, want %q", findings[0].Kind, "red")
	}
	want := "new failure pkg TestNew (not in baseline)"
	if findings[0].Detail != want {
		t.Errorf("findings[0].Detail = %q, want %q", findings[0].Detail, want)
	}
}

func TestCompareBaselineSameNameDifferentScopeIsNewRed(t *testing.T) {
	b := &Baseline{Failed: []BaselineIdentity{{Scope: "pkgA", Name: "TestShared"}}}
	rep := &gotestjson.Report{Records: []runrecord.Record{failedRecord("pkgB", "TestShared")}}

	findings := CompareBaseline(b, rep)

	if len(findings) != 1 || findings[0].Kind != "red" {
		t.Fatalf("findings = %+v, want one red finding for the pkgB pair", findings)
	}
}

func TestCompareBaselineSkippedInBaselineNowFailedIsRed(t *testing.T) {
	b := &Baseline{Skipped: []BaselineIdentity{{Scope: "pkg", Name: "TestWasSkipped"}}}
	rep := &gotestjson.Report{Records: []runrecord.Record{failedRecord("pkg", "TestWasSkipped")}}

	findings := CompareBaseline(b, rep)

	if len(findings) != 1 || findings[0].Kind != "red" {
		t.Fatalf("findings = %+v, want one red finding (skipped-in-baseline gives no exemption)", findings)
	}
}

func TestCompareBaselineStillFailingProducesNoFinding(t *testing.T) {
	b := &Baseline{Failed: []BaselineIdentity{{Scope: "pkg", Name: "TestKnownBad"}}}
	rep := &gotestjson.Report{Records: []runrecord.Record{failedRecord("pkg", "TestKnownBad")}}

	findings := CompareBaseline(b, rep)

	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none for a baseline-failed test still failing", findings)
	}
}

func TestCompareBaselineFailureNowPassingOrAbsentProducesNoFinding(t *testing.T) {
	b := &Baseline{Failed: []BaselineIdentity{{Scope: "pkg", Name: "TestFixed"}, {Scope: "pkg", Name: "TestGone"}}}
	rep := &gotestjson.Report{Records: []runrecord.Record{passedRecord("pkg", "TestFixed")}}

	findings := CompareBaseline(b, rep)

	if len(findings) != 0 {
		t.Errorf("findings = %+v, want none: baseline failures now passing or absent", findings)
	}
}

func TestCompareBaselineZeroFailuresProducesEmptyFindingsNoPanic(t *testing.T) {
	b := &Baseline{Failed: []BaselineIdentity{{Scope: "pkg", Name: "TestX"}}}
	rep := &gotestjson.Report{Records: []runrecord.Record{passedRecord("pkg", "TestX")}}

	findings := CompareBaseline(b, rep)

	if len(findings) != 0 {
		t.Errorf("findings = %+v, want empty", findings)
	}
}

func TestCompareBaselineNilBaselineEveryFailureIsNewRed(t *testing.T) {
	rep := &gotestjson.Report{Records: []runrecord.Record{
		failedRecord("pkg", "TestOne"),
		failedRecord("pkg", "TestTwo"),
	}}

	findings := CompareBaseline(nil, rep)

	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want two red findings", findings)
	}
	for _, f := range findings {
		if f.Kind != "red" {
			t.Errorf("finding %+v: Kind = %q, want %q", f, f.Kind, "red")
		}
	}
}

func TestCompareBaselineFindingsSortedByDetail(t *testing.T) {
	rep := &gotestjson.Report{Records: []runrecord.Record{
		failedRecord("pkg", "TestZ"),
		failedRecord("pkg", "TestA"),
	}}

	findings := CompareBaseline(nil, rep)

	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want two", findings)
	}
	if !strings.Contains(findings[0].Detail, "TestA") || !strings.Contains(findings[1].Detail, "TestZ") {
		t.Errorf("findings not sorted by Detail: %+v", findings)
	}
}
