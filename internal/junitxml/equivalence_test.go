package junitxml

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"test-design/internal/runrecord"
)

// loadProducerReport merges every "TEST-*.xml" file in a captured producer
// directory into one Report. Loading the whole directory — including a
// Surefire/Failsafe outer-class file with zero testcases whose methods
// were routed into a sibling $NestedCases file — is what makes that
// nested-routing divergence invisible at the merged-Report level.
func loadProducerReport(t *testing.T, producer string) *Report {
	t.Helper()
	dir := fixturePath(producer)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}

	var docs []*Document
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "TEST-") || !strings.HasSuffix(e.Name(), ".xml") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("open %q: %v", e.Name(), err)
		}
		doc, err := NormalizeFile(f)
		f.Close()
		if err != nil {
			t.Fatalf("NormalizeFile(%q): %v", e.Name(), err)
		}
		docs = append(docs, doc)
	}

	report, err := Merge(docs)
	if err != nil {
		t.Fatalf("Merge(%s): %v", producer, err)
	}
	return report
}

// comparableFamily is a Family reduced to what cross-producer equivalence
// compares: identity and outcome, not raw evidence or case order.
type comparableFamily struct {
	className   string
	position    string // Method for a derivable family, RawName for an Unsupported one
	unsupported bool
	status      runrecord.Status
}

// canonicalFamilies reduces a Report's Families to comparableFamily,
// remapping each ClassName through classNameMap (for the Maven
// Surefire/Failsafe *Test/*IT class-name suffix divergence) and dropping
// any family whose remapped ClassName is in exclude.
func canonicalFamilies(report *Report, classNameMap map[string]string, exclude map[string]bool) []comparableFamily {
	var out []comparableFamily
	for _, f := range report.Families {
		className := f.Identity.ClassName
		if mapped, ok := classNameMap[className]; ok {
			className = mapped
		}
		if exclude[className] {
			continue
		}
		cf := comparableFamily{className: className, status: f.Status, unsupported: f.Identity.Unsupported != ""}
		if cf.unsupported {
			cf.position = f.Cases[0].RawName
		} else {
			cf.position = f.Identity.Method
		}
		out = append(out, cf)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].className != out[j].className {
			return out[i].className < out[j].className
		}
		return out[i].position < out[j].position
	})
	return out
}

var failsafeToSurefireClassNames = map[string]string{
	"com.example.verifier.SimpleIT":             "com.example.verifier.SimpleTest",
	"com.example.verifier.SimpleIT$NestedCases": "com.example.verifier.SimpleTest$NestedCases",
	"com.example.verifier.ParamIT":              "com.example.verifier.ParamTest",
	"com.example.verifier.DisplayNameIT":        "com.example.verifier.DisplayNameTest",
	"com.example.verifier.AlphaIT":              "com.example.verifier.AlphaTest",
	"com.example.verifier.BetaIT":               "com.example.verifier.BetaTest",
}

// divergentClasses are excluded from the identical-derivable-set
// comparison because they derive differently across producers by design
// (see TestMergeCrossProducerParameterizedAndDisplayNameFamiliesDivergeAsExpected).
var divergentClasses = map[string]bool{
	"com.example.verifier.ParamTest":       true,
	"com.example.verifier.DisplayNameTest": true,
}

func TestMergeCrossProducerDerivableSharedMatrixFamiliesAreIdentical(t *testing.T) {
	surefire := canonicalFamilies(loadProducerReport(t, "maven-surefire"), nil, divergentClasses)
	failsafe := canonicalFamilies(loadProducerReport(t, "maven-failsafe"), failsafeToSurefireClassNames, divergentClasses)
	gradleJava := canonicalFamilies(loadProducerReport(t, "gradle-java"), nil, divergentClasses)

	want := []comparableFamily{
		{className: "com.example.verifier.AlphaTest", position: "sharedMethod", status: runrecord.Passed},
		{className: "com.example.verifier.BetaTest", position: "sharedMethod", status: runrecord.Passed},
		{className: "com.example.verifier.SimpleTest", position: "disabledTest", status: runrecord.Skipped},
		{className: "com.example.verifier.SimpleTest", position: "failingAssertionTest", status: runrecord.Failed},
		{className: "com.example.verifier.SimpleTest", position: "passingTest", status: runrecord.Passed},
		{className: "com.example.verifier.SimpleTest", position: "throwingExceptionTest", status: runrecord.Failed},
		{className: "com.example.verifier.SimpleTest$NestedCases", position: "nestedPassingTest", status: runrecord.Passed},
	}

	for name, got := range map[string][]comparableFamily{"maven-surefire": surefire, "maven-failsafe": failsafe, "gradle-java": gradleJava} {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s canonical families = %+v, want %+v", name, got, want)
		}
	}
}

func TestMergeCrossProducerParameterizedAndDisplayNameFamiliesDivergeAsExpected(t *testing.T) {
	surefire := canonicalFamilies(loadProducerReport(t, "maven-surefire"), nil, nil)
	gradleJava := canonicalFamilies(loadProducerReport(t, "gradle-java"), nil, nil)

	findByClass := func(fams []comparableFamily, className string) []comparableFamily {
		var out []comparableFamily
		for _, f := range fams {
			if f.className == className {
				out = append(out, f)
			}
		}
		return out
	}

	// Maven's decoration carries the method name, so ParamTest derives into
	// exactly its two declared methods.
	wantMavenParam := []comparableFamily{
		{className: "com.example.verifier.ParamTest", position: "allPassing", status: runrecord.Passed},
		{className: "com.example.verifier.ParamTest", position: "oneFailing", status: runrecord.Failed},
	}
	if got := findByClass(surefire, "com.example.verifier.ParamTest"); !reflect.DeepEqual(got, wantMavenParam) {
		t.Errorf("maven-surefire ParamTest families = %+v, want %+v", got, wantMavenParam)
	}

	// Gradle's "[N] args" decoration never carries the method name (identity.go
	// rule 1), so ParamTest is Unsupported by design — the raw display names
	// collide across allPassing/oneFailing, collapsing to three raw-name
	// families instead of the two declared methods.
	wantGradleParam := []comparableFamily{
		{className: "com.example.verifier.ParamTest", position: "[1] 1", unsupported: true, status: runrecord.Passed},
		{className: "com.example.verifier.ParamTest", position: "[2] 2", unsupported: true, status: runrecord.Failed},
		{className: "com.example.verifier.ParamTest", position: "[3] 3", unsupported: true, status: runrecord.Passed},
	}
	if got := findByClass(gradleJava, "com.example.verifier.ParamTest"); !reflect.DeepEqual(got, wantGradleParam) {
		t.Errorf("gradle-java ParamTest families = %+v, want %+v", got, wantGradleParam)
	}
	for _, f := range findByClass(gradleJava, "com.example.verifier.ParamTest") {
		if !f.unsupported {
			t.Errorf("gradle-java ParamTest family %+v, want unsupported", f)
		}
	}

	// Maven's writer uses the technical name even under @DisplayName, so
	// DisplayNameTest.customDisplay derives.
	wantMavenDisplay := []comparableFamily{
		{className: "com.example.verifier.DisplayNameTest", position: "customDisplay", status: runrecord.Passed},
	}
	if got := findByClass(surefire, "com.example.verifier.DisplayNameTest"); !reflect.DeepEqual(got, wantMavenDisplay) {
		t.Errorf("maven-surefire DisplayNameTest families = %+v, want %+v", got, wantMavenDisplay)
	}

	// Gradle substitutes the @DisplayName text into the name attribute,
	// losing the source method identity, so it is Unsupported instead.
	wantGradleDisplay := []comparableFamily{
		{className: "com.example.verifier.DisplayNameTest", position: "custom method display", unsupported: true, status: runrecord.Passed},
	}
	if got := findByClass(gradleJava, "com.example.verifier.DisplayNameTest"); !reflect.DeepEqual(got, wantGradleDisplay) {
		t.Errorf("gradle-java DisplayNameTest families = %+v, want %+v", got, wantGradleDisplay)
	}
}

func TestMergeCrossProducerGradleKotlinMatchesGradleJavaAndAddsBacktickFamilies(t *testing.T) {
	gradleJava := loadProducerReport(t, "gradle-java")
	gradleKotlin := loadProducerReport(t, "gradle-kotlin")

	sharedClasses := map[string]bool{
		"com.example.verifier.AlphaTest":              true,
		"com.example.verifier.BetaTest":               true,
		"com.example.verifier.DisplayNameTest":        true,
		"com.example.verifier.ParamTest":              true,
		"com.example.verifier.SimpleTest":             true,
		"com.example.verifier.SimpleTest$NestedCases": true,
	}
	keep := func(className string) bool { return sharedClasses[className] }
	filterKept := func(fams []comparableFamily) []comparableFamily {
		var out []comparableFamily
		for _, f := range fams {
			if keep(f.className) {
				out = append(out, f)
			}
		}
		return out
	}

	javaFams := filterKept(canonicalFamilies(gradleJava, nil, nil))
	kotlinFams := filterKept(canonicalFamilies(gradleKotlin, nil, nil))
	if !reflect.DeepEqual(javaFams, kotlinFams) {
		t.Errorf("gradle-kotlin shared-matrix families = %+v, want identical to gradle-java's %+v", kotlinFams, javaFams)
	}

	var backtick []comparableFamily
	for _, f := range canonicalFamilies(gradleKotlin, nil, nil) {
		if f.className == "com.example.verifier.BacktickTest" {
			backtick = append(backtick, f)
		}
	}
	wantBacktick := []comparableFamily{
		{className: "com.example.verifier.BacktickTest", position: "alsoBackticked", status: runrecord.Passed},
		{className: "com.example.verifier.BacktickTest", position: "passes with backticks and spaces", status: runrecord.Passed},
	}
	if !reflect.DeepEqual(backtick, wantBacktick) {
		t.Errorf("gradle-kotlin BacktickTest families = %+v, want %+v", backtick, wantBacktick)
	}
	for _, f := range backtick {
		if f.unsupported {
			t.Errorf("BacktickTest family %+v, want derivable", f)
		}
	}
}
