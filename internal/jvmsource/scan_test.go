package jvmsource

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanTags_FixtureTree(t *testing.T) {
	sites, err := ScanTags(filepath.Join("testdata", "scan"))
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	byID := map[string]TagSite{}
	for _, s := range sites {
		for _, id := range s.IDs {
			byID[id] = s
		}
	}
	if s := byID["SC-01"]; s.ClassName != "com.example.a.ATest" || s.Method != "sameName" || s.Ambiguous {
		t.Errorf("SC-01 = %+v", s)
	}
	if s := byID["SC-02"]; s.ClassName != "com.example.b.BTest$Nested" || s.Method != "sameName" {
		t.Errorf("SC-02 = %+v", s)
	}
	if !reflect.DeepEqual(byID["SC-02"].IDs, []string{"SC-02", "SC-03"}) {
		t.Errorf("SC-02 IDs = %v", byID["SC-02"].IDs)
	}
	if s := byID["SC-04"]; s.ClassName != "com.example.k.DeclaredNameTest" || s.Method != "binds by declared class" {
		t.Errorf("SC-04 = %+v (declared class must win over filename)", s)
	}
	if s := byID["SC-05"]; !s.Ambiguous || s.Detail == "" {
		t.Errorf("SC-05 = %+v, want ambiguous with detail", s)
	}
	if _, found := byID["SC-99"]; found {
		t.Error("SC-99 from notes.txt scanned; non-source files must be ignored")
	}
	if len(sites) != 4 {
		t.Errorf("sites = %d, want 4", len(sites))
	}
	for _, s := range sites {
		if filepath.IsAbs(s.File) || strings.HasPrefix(s.File, "testdata") {
			t.Errorf("File = %q, want root-relative", s.File)
		}
	}
}

func TestScanTags_DeterministicOrder(t *testing.T) {
	a, err := ScanTags(filepath.Join("testdata", "scan"))
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	b, err := ScanTags(filepath.Join("testdata", "scan"))
	if err != nil {
		t.Fatalf("ScanTags: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("two scans differ")
	}
	for i := 1; i < len(a); i++ {
		if a[i-1].File > a[i].File || (a[i-1].File == a[i].File && a[i-1].Line > a[i].Line) {
			t.Errorf("order violated at %d: %+v then %+v", i, a[i-1], a[i])
		}
	}
}

func TestScanTags_ParseErrorAbortsNamingFile(t *testing.T) {
	_, err := ScanTags(filepath.Join("testdata", "broken"))
	if err == nil {
		t.Fatal("want error for unbalanced file, got nil")
	}
	if !strings.Contains(err.Error(), "Broken.java") {
		t.Errorf("error %q does not name the file", err)
	}
}
