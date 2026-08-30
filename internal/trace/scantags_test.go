package trace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTestFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScanTagsBindsDocCommentToTestFunc(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

import "testing"

// covers: SC-01
func TestRejectsZeroQuantity(t *testing.T) {
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	want := []TagSite{
		{
			ImportPath: "example.com/mod/pkg",
			TestName:   "TestRejectsZeroQuantity",
			File:       "pkg/foo_test.go",
			Line:       5,
			IDs:        []string{"SC-01"},
			Ambiguous:  false,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScanTags = %+v, want %+v", got, want)
	}
}

func TestScanTagsMultiLineDocGroupCollectsAllTokens(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

import "testing"

// this test asserts two things
// covers: SC-01, orders/SC-02
// see also the spec
func TestTwoThings(t *testing.T) {
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want 1 site", got)
	}
	want := []string{"SC-01", "orders/SC-02"}
	if !reflect.DeepEqual(got[0].IDs, want) {
		t.Errorf("IDs = %v, want %v", got[0].IDs, want)
	}
	if got[0].Line != 6 {
		t.Errorf("Line = %d, want 6 (the covers: line, not the doc-group start)", got[0].Line)
	}
}

func TestScanTagsDocCommentWithoutMarkerProducesNoSite(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

import "testing"

// TestOrdinary checks nothing special.
func TestOrdinary(t *testing.T) {
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanTags = %+v, want no sites", got)
	}
}

func TestScanTagsImportPathDirDotIsBareModulePath(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"foo_test.go": `package pkg

import "testing"

// covers: SC-01
func TestAtRoot(t *testing.T) {
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want 1 site", got)
	}
	if got[0].ImportPath != "example.com/mod" {
		t.Errorf("ImportPath = %q, want %q", got[0].ImportPath, "example.com/mod")
	}
}

func TestScanTagsExternalTestPackageSharesImportPathWithInternal(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/internal_test.go": `package pkg

import "testing"

// covers: SC-01
func TestInternal(t *testing.T) {
}
`,
		"pkg/external_test.go": `package pkg_test

import "testing"

// covers: SC-02
func TestExternal(t *testing.T) {
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanTags = %+v, want 2 sites", got)
	}
	for _, site := range got {
		if site.ImportPath != "example.com/mod/pkg" {
			t.Errorf("site %+v: ImportPath = %q, want %q", site, site.ImportPath, "example.com/mod/pkg")
		}
	}
}

func TestScanTagsWrongShapeFunctionsAreAmbiguous(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "helper function",
			src: `package pkg

// covers: SC-01
func helperSetup(t *testing.T) {
}
`,
		},
		{
			name: "benchmark",
			src: `package pkg

// covers: SC-01
func BenchmarkFoo(b *testing.B) {
}
`,
		},
		{
			name: "fuzz",
			src: `package pkg

// covers: SC-01
func FuzzFoo(f *testing.F) {
}
`,
		},
		{
			name: "example",
			src: `package pkg

// covers: SC-01
func ExampleFoo() {
}
`,
		},
		{
			name: "wrong signature extra param",
			src: `package pkg

// covers: SC-01
func TestFoo(t *testing.T, extra string) {
}
`,
		},
		{
			name: "wrong signature no params",
			src: `package pkg

// covers: SC-01
func TestFoo() {
}
`,
		},
		{
			name: "method receiver",
			src: `package pkg

type Suite struct{}

// covers: SC-01
func (s *Suite) TestFoo(t *testing.T) {
}
`,
		},
		{
			name: "lowercase after Test prefix",
			src: `package pkg

// covers: SC-01
func Testfoo(t *testing.T) {
}
`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeTestFiles(t, map[string]string{
				"pkg/foo_test.go": "package pkg\n\nimport \"testing\"\n\n" + strings.TrimPrefix(c.src, "package pkg\n\n"),
			})
			got, err := ScanTags(dir, "example.com/mod")
			if err != nil {
				t.Fatalf("ScanTags: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("ScanTags = %+v, want 1 site", got)
			}
			site := got[0]
			if !site.Ambiguous {
				t.Errorf("Ambiguous = false, want true")
			}
			if site.TestName != "" {
				t.Errorf("TestName = %q, want empty", site.TestName)
			}
			if site.Detail == "" {
				t.Errorf("Detail = empty, want a reason")
			}
			if !reflect.DeepEqual(site.IDs, []string{"SC-01"}) {
				t.Errorf("IDs = %v, want [SC-01]", site.IDs)
			}
		})
	}
}

func TestScanTagsCommentSeparatedByBlankLineIsNotDocAndSurfacesAmbiguous(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

import "testing"

func Helper() {}

// covers: SC-05

func TestSomething(t *testing.T) {
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want 1 site (TestSomething must not gain a bound site)", got)
	}
	site := got[0]
	if !site.Ambiguous {
		t.Errorf("Ambiguous = false, want true")
	}
	if site.TestName != "" {
		t.Errorf("TestName = %q, want empty", site.TestName)
	}
	if !strings.Contains(site.Detail, "pkg/foo_test.go:7") {
		t.Errorf("Detail = %q, want it to name file:line pkg/foo_test.go:7", site.Detail)
	}
}

func TestScanTagsCommentInsideFunctionBodyIsAmbiguous(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

import "testing"

func TestX(t *testing.T) {
	// covers: SC-07
	t.Log("hi")
}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want 1 site", got)
	}
	site := got[0]
	if !site.Ambiguous || site.TestName != "" {
		t.Errorf("site = %+v, want ambiguous with no bound test name", site)
	}
	if !strings.Contains(site.Detail, "function body") {
		t.Errorf("Detail = %q, want it to mention the comment is inside a function body", site.Detail)
	}
}

func TestScanTagsCommentAboveTypeDeclIsAmbiguous(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

// covers: SC-08
type Foo struct{}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want 1 site", got)
	}
	if !strings.Contains(got[0].Detail, "type/var/const") {
		t.Errorf("Detail = %q, want it to mention type/var/const", got[0].Detail)
	}
}

func TestScanTagsCommentAboveVarAndConstDeclIsAmbiguous(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

// covers: SC-08
var x = 1

// covers: SC-09
const y = 2
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanTags = %+v, want 2 sites", got)
	}
	for _, site := range got {
		if !site.Ambiguous || !strings.Contains(site.Detail, "type/var/const") {
			t.Errorf("site = %+v, want ambiguous naming type/var/const", site)
		}
	}
}

func TestScanTagsTrailingCommentAtEOFIsAmbiguous(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/foo_test.go": `package pkg

import "testing"

func TestY(t *testing.T) {
}

// covers: SC-09
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want 1 site", got)
	}
	if !strings.Contains(got[0].Detail, "end of file") {
		t.Errorf("Detail = %q, want it to mention end of file", got[0].Detail)
	}
}

func TestScanTagsNonTestFileNotScanned(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/prod.go": `package pkg

// covers: SC-10
func Something() {}
`,
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanTags = %+v, want no sites (production files out of scope)", got)
	}
}

func TestScanTagsUnparsableTestFileIsAnError(t *testing.T) {
	dir := writeTestFiles(t, map[string]string{
		"pkg/broken_test.go": `package pkg

func TestBroken( {
`,
	})

	_, err := ScanTags(dir, "example.com/mod")
	if err == nil {
		t.Fatal("ScanTags = nil error, want an error naming the broken file")
	}
	if !strings.Contains(err.Error(), "broken_test.go") {
		t.Errorf("error = %q, want it to name broken_test.go", err.Error())
	}
}

func TestScanTagsSkipsVendorTestdataHiddenAndNestedModuleDirs(t *testing.T) {
	tagged := func(name string) string {
		return "package pkg\n\nimport \"testing\"\n\n// covers: SC-01\nfunc " + name + "(t *testing.T) {\n}\n"
	}
	dir := writeTestFiles(t, map[string]string{
		"good_test.go":                 tagged("TestGood"),
		"vendor/skip_test.go":          tagged("TestVendor"),
		"testdata/skip_test.go":        tagged("TestTestdata"),
		".hidden/skip_test.go":         tagged("TestHidden"),
		"nested/go.mod":                "module nested\n\ngo 1.26\n",
		"nested/skip_test.go":          tagged("TestNested"),
		"deep/vendor/nested_test.go":   tagged("TestDeepVendor"),
		"deep/testdata/nested_test.go": tagged("TestDeepTestdata"),
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ScanTags = %+v, want only the root site", got)
	}
	if got[0].TestName != "TestGood" {
		t.Errorf("TestName = %q, want TestGood", got[0].TestName)
	}
}

func TestScanTagsOutputSortedByFileThenLine(t *testing.T) {
	tagged := func(name string, line int) string {
		pad := strings.Repeat("\n", line-5)
		return "package pkg\n\nimport \"testing\"\n" + pad + "\n// covers: SC-01\nfunc " + name + "(t *testing.T) {\n}\n"
	}
	dir := writeTestFiles(t, map[string]string{
		"b_test.go": tagged("TestB", 5),
		"a_test.go": tagged("TestA2", 9) + "\n// covers: SC-02\nfunc TestA1(t *testing.T) {\n}\n",
	})

	got, err := ScanTags(dir, "example.com/mod")
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ScanTags = %+v, want 3 sites", got)
	}
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if prev.File > cur.File || (prev.File == cur.File && prev.Line > cur.Line) {
			t.Errorf("not sorted at index %d: %+v then %+v", i, prev, cur)
		}
	}
}
