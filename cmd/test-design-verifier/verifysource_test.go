package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSourceDoc(t *testing.T, replacements map[string]string) (string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "go-project",
		"docs", "superpowers", "coverage", "2026-08-30-fixture-coverage.md"))
	if err != nil {
		t.Fatalf("read fixture doc: %v", err)
	}
	s := string(data)
	for old, new := range replacements {
		if !strings.Contains(s, old) {
			t.Fatalf("fixture doc does not contain %q", old)
		}
		s = strings.ReplaceAll(s, old, new)
	}
	root := t.TempDir()
	rel := "docs/superpowers/coverage/doc.md"
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	return root, rel
}

func TestLoadVerifiedSource_MavenKindAccepted(t *testing.T) {
	root, rel := writeSourceDoc(t, map[string]string{
		"- runner-kind: go": "- runner-kind: maven",
		`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["mvn", "-B", "test"]`,
		"- report-globs: []": `- report-globs: ["target/surefire-reports/*.xml"]`,
	})
	src, cliErr := loadVerifiedSource("baseline", root, rel)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	if src.Runner.Kind != "maven" {
		t.Errorf("Kind = %q, want maven", src.Runner.Kind)
	}
}

func TestLoadVerifiedSource_GradleWithoutRerunFlagRefused(t *testing.T) {
	root, rel := writeSourceDoc(t, map[string]string{
		"- runner-kind: go": "- runner-kind: gradle",
		`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["gradle", "test"]`,
		"- report-globs: []": `- report-globs: ["build/test-results/test/*.xml"]`,
	})
	_, cliErr := loadVerifiedSource("baseline", root, rel)
	if cliErr == nil {
		t.Fatal("want missing-rerun-flag refusal, got nil")
	}
	if cliErr.Code != "missing-rerun-flag" {
		t.Errorf("Code = %q, want missing-rerun-flag", cliErr.Code)
	}
}

func TestLoadVerifiedSource_GradleRerunFlagAccepted(t *testing.T) {
	for _, flag := range []string{"--rerun", "--rerun-tasks"} {
		root, rel := writeSourceDoc(t, map[string]string{
			"- runner-kind: go": "- runner-kind: gradle",
			`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["gradle", "test", "` + flag + `"]`,
			"- report-globs: []": `- report-globs: ["build/test-results/test/*.xml"]`,
		})
		src, cliErr := loadVerifiedSource("baseline", root, rel)
		if cliErr != nil {
			t.Fatalf("%s: loadVerifiedSource: %v", flag, cliErr)
		}
		if src.Runner.Kind != "gradle" {
			t.Errorf("%s: Kind = %q, want gradle", flag, src.Runner.Kind)
		}
	}
}

func TestLoadVerifiedSource_JvmKindWithoutReportGlobsRefused(t *testing.T) {
	root, rel := writeSourceDoc(t, map[string]string{
		"- runner-kind: go": "- runner-kind: maven",
		`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["mvn", "-B", "test"]`,
	})
	_, cliErr := loadVerifiedSource("baseline", root, rel)
	if cliErr == nil {
		t.Fatal("want missing-report-globs refusal, got nil")
	}
	if cliErr.Code != "missing-report-globs" {
		t.Errorf("Code = %q, want missing-report-globs", cliErr.Code)
	}
}

func TestLoadVerifiedSource_ReportGlobEscapingRootRefused(t *testing.T) {
	for _, glob := range []string{"../outside/*.xml", "/abs/reports/*.xml"} {
		root, rel := writeSourceDoc(t, map[string]string{
			"- runner-kind: go": "- runner-kind: maven",
			`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["mvn", "-B", "test"]`,
			"- report-globs: []": `- report-globs: ["` + glob + `"]`,
		})
		_, cliErr := loadVerifiedSource("baseline", root, rel)
		if cliErr == nil {
			t.Fatalf("glob %q: want report-glob-escapes-root refusal, got nil", glob)
		}
		if cliErr.Code != "report-glob-escapes-root" {
			t.Errorf("glob %q: Code = %q, want report-glob-escapes-root", glob, cliErr.Code)
		}
	}
}

func TestLoadVerifiedSource_JvmKindDoesNotRequireCountOne(t *testing.T) {
	root, rel := writeSourceDoc(t, map[string]string{
		"- runner-kind: go": "- runner-kind: maven",
		`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["mvn", "-B", "test"]`,
		"- report-globs: []": `- report-globs: ["target/surefire-reports/*.xml"]`,
	})
	if _, cliErr := loadVerifiedSource("baseline", root, rel); cliErr != nil {
		t.Fatalf("maven command without -count=1 refused: %v", cliErr)
	}
}

func TestLoadVerifiedSource_GoStillRequiresCountOne(t *testing.T) {
	root, rel := writeSourceDoc(t, map[string]string{
		`- runner-command: ["go", "test", "-count=1", "-json", "./..."]`: `- runner-command: ["go", "test", "-json", "./..."]`,
	})
	_, cliErr := loadVerifiedSource("baseline", root, rel)
	if cliErr == nil || cliErr.Code != "missing-count-one" {
		t.Fatalf("cliErr = %v, want missing-count-one", cliErr)
	}
}
