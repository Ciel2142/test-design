package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/report"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

var forbiddenTokens = []string{
	"Bug it catches",
	"runner-output.md",
	"vitest",
	"jest",
	"pytest",
	"git checkout --",
	"git stash",
	"throwaway branch",
	"mutation-check-",
	"transcripts/SC-",
	"transcripts/W",
	"bring-up-attempt.txt",
	`grep -rn "covers`,
	`covers: SC-"`,
	"status\tscope\tname",
	".vitest-report",
	"sample-project",
	"-t '^",
	"-k '",
	"PIPESTATUS",
}

func scannedDocFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, d := range []string{"skills", "agents", ".claude-plugin"} {
		dir := filepath.Join(root, d)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	files = append(files,
		filepath.Join(root, "docs", "opt-in.md"),
		filepath.Join(root, "README.md"),
		filepath.Join(root, "fixtures", "README.md"),
	)
	sort.Strings(files)
	return files
}

func TestPluginDocs_NoForbiddenTokens(t *testing.T) {
	root := repoRoot(t)
	var failures []string
	for _, path := range scannedDocFiles(t, root) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(data), "\n") {
			for _, tok := range forbiddenTokens {
				if strings.Contains(line, tok) {
					failures = append(failures, fmt.Sprintf("%s:%d: forbidden token %q", rel, i+1, tok))
				}
			}
		}
	}
	for _, rel := range []string{
		filepath.Join("agents", "mutator.md"),
		filepath.Join("agents", "live-runner.md"),
	} {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "tools: Bash") {
				failures = append(failures, fmt.Sprintf("%s:%d: forbidden token %q", rel, i+1, "tools: Bash"))
			}
		}
	}
	if len(failures) != 0 {
		t.Errorf("forbidden tokens found:\n%s", strings.Join(failures, "\n"))
	}
}

func TestPluginDocs_RequiredTokens(t *testing.T) {
	root := repoRoot(t)
	requirements := []struct {
		path  string
		token string
	}{
		{"skills/coverage-verification/SKILL.md", "Verification mutation"},
		{"skills/test-coverage-design/SKILL.md", "Verification mutation"},
		{"skills/coverage-verification/references/example-verification.md", "Verification mutation"},
		{"skills/test-coverage-design/references/example-coverage-doc.md", "Verification mutation"},
		{"agents/coverage-critic.md", "Verification mutation"},
		{"skills/test-coverage-design/SKILL.md", "TDD RED"},
		{"skills/test-coverage-design/SKILL.md", "ALREADY-GREEN"},
		{"agents/mutator.md", "MUTATION-CAUGHT"},
		{"skills/coverage-verification/SKILL.md", "MUTATION-CAUGHT"},
		{"skills/coverage-verification/SKILL.md", "disable-model-invocation: true"},
		{"skills/test-coverage-design/SKILL.md", "disable-model-invocation: true"},
		{"skills/coverage-verification/SKILL.md", "go run -C"},
		{"skills/test-coverage-design/SKILL.md", "go run -C"},
	}
	for _, req := range requirements {
		path := filepath.Join(root, filepath.FromSlash(req.path))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", req.path, err)
		}
		if !strings.Contains(string(data), req.token) {
			t.Errorf("%s: missing required token %q", req.path, req.token)
		}
	}
}

func agentTools(t *testing.T, root, rel string) []string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if after, ok := strings.CutPrefix(line, "tools:"); ok {
			var tools []string
			for _, tool := range strings.Split(after, ",") {
				tools = append(tools, strings.TrimSpace(tool))
			}
			return tools
		}
	}
	t.Fatalf("%s: no tools: line found", rel)
	return nil
}

func TestPluginDocs_AgentsWithoutBash(t *testing.T) {
	root := repoRoot(t)

	for _, rel := range []string{"agents/mutator.md", "agents/live-runner.md"} {
		tools := agentTools(t, root, rel)
		for _, tool := range tools {
			if tool == "Bash" {
				t.Errorf("%s: tools line declares Bash: %v", rel, tools)
			}
		}
	}

	want := []string{"Read", "Grep", "Glob"}
	got := agentTools(t, root, "agents/coverage-critic.md")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("agents/coverage-critic.md: tools = %v, want %v", got, want)
	}
}

var opTokenPattern = regexp.MustCompile(`test-design-verifier ([a-zA-Z0-9<>-]+)`)

func TestPluginDocs_OpsCoverage(t *testing.T) {
	root := repoRoot(t)
	named := map[string]bool{}
	for _, rel := range []string{"skills/coverage-verification/SKILL.md", "skills/test-coverage-design/SKILL.md"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, m := range opTokenPattern.FindAllStringSubmatch(string(data), -1) {
			tok := m[1]
			if tok == "<op>" {
				continue
			}
			named[tok] = true
		}
	}

	registered := map[string]bool{}
	for op := range ops {
		registered[op] = true
	}

	var missingFromSkills, missingFromRegistry []string
	for op := range registered {
		if !named[op] {
			missingFromSkills = append(missingFromSkills, op)
		}
	}
	for op := range named {
		if !registered[op] {
			missingFromRegistry = append(missingFromRegistry, op)
		}
	}
	sort.Strings(missingFromSkills)
	sort.Strings(missingFromRegistry)
	if len(missingFromSkills) != 0 {
		t.Errorf("ops registered but not named by either skill: %v", missingFromSkills)
	}
	if len(missingFromRegistry) != 0 {
		t.Errorf("ops named by a skill but not registered: %v", missingFromRegistry)
	}
}

var kebabTokenPattern = regexp.MustCompile("`([a-z]+(?:-[a-z0-9]+)+)`")

func TestPluginDocs_RefusalCodesExist(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "skills", "coverage-verification", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	section, err := extractSection(string(data), "## Refusals")
	if err != nil {
		t.Fatal(err)
	}

	excluded := map[string]bool{}
	for op := range ops {
		excluded[op] = true
	}

	cited := map[string]bool{}
	for _, m := range kebabTokenPattern.FindAllStringSubmatch(section, -1) {
		tok := m[1]
		if excluded[tok] {
			continue
		}
		cited[tok] = true
	}
	if len(cited) == 0 {
		t.Fatal("no refusal codes found in the Refusals section; the extraction heuristic is broken")
	}

	literals, err := collectStringLiterals(
		filepath.Join(root, "cmd", "test-design-verifier"),
		filepath.Join(root, "internal", "report"),
	)
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for code := range cited {
		if !literals[code] {
			missing = append(missing, code)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Errorf("refusal codes cited in skills/coverage-verification/SKILL.md but not found as a Go string literal in cmd/test-design-verifier or internal/report: %v", missing)
	}
}

func extractSection(doc, heading string) (string, error) {
	start := strings.Index(doc, heading)
	if start == -1 {
		return "", fmt.Errorf("heading %q not found", heading)
	}
	rest := doc[start+len(heading):]
	nextHeading := regexp.MustCompile(`(?m)^## `)
	if loc := nextHeading.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest, nil
}

func collectStringLiterals(dirs ...string) (map[string]bool, error) {
	literals := map[string]bool{}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", dir, err)
		}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				ast.Inspect(file, func(n ast.Node) bool {
					lit, ok := n.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					value, err := stringLitValue(lit.Value)
					if err == nil {
						literals[value] = true
					}
					return true
				})
			}
		}
	}
	return literals, nil
}

func stringLitValue(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", fmt.Errorf("not a double-quoted string literal: %s", raw)
	}
	return raw[1 : len(raw)-1], nil
}

func TestPluginDocs_ExampleVerificationRendered(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "cmd", "test-design-verifier", "testdata", "example-report.json"))
	if err != nil {
		t.Fatalf("read testdata/example-report.json: %v", err)
	}
	var r report.Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal example-report.json: %v", err)
	}
	rendered, err := report.RenderMarkdown(&r)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	wantPath := filepath.Join(root, "skills", "coverage-verification", "references", "example-verification.md")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read %s: %v", wantPath, err)
	}
	if string(rendered) != string(want) {
		t.Errorf("RenderMarkdown(example-report.json) does not byte-match %s\n--- got ---\n%s\n--- want ---\n%s", wantPath, rendered, want)
	}
}

func TestPluginDocs_ExampleCoverageDocParses(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "skills", "test-coverage-design", "references", "example-coverage-doc.md")
	src, err := coverage.ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource(%s) = %v, want no error", path, err)
	}
	if len(src.Rows) == 0 {
		t.Errorf("ParseSource(%s) returned zero active rows, want at least one", path)
	}
}
