package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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

// Follow the same explicit Markdown reference graph a skill reader follows. A
// missing phase file must fail even when the core skill remains readable.
var docLinkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

func linkedSkillDocs(t *testing.T, root string, entries ...string) map[string]string {
	t.Helper()
	docs := map[string]string{}
	var visit func(string)
	visit = func(rel string) {
		rel = filepath.Clean(rel)
		if _, ok := docs[rel]; ok {
			return
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read linked document %s: %v", rel, err)
		}
		docs[rel] = string(data)
		for _, match := range docLinkPattern.FindAllStringSubmatch(string(data), -1) {
			target := strings.SplitN(match[1], "#", 2)[0]
			if target == "" || strings.Contains(target, "://") || !strings.HasSuffix(target, ".md") {
				continue
			}
			next := filepath.Clean(filepath.Join(filepath.Dir(rel), target))
			if filepath.IsAbs(target) || next == ".." || strings.HasPrefix(next, ".."+string(filepath.Separator)) {
				t.Fatalf("%s: linked document escapes plugin: %s", rel, target)
			}
			visit(next)
		}
	}
	for _, entry := range entries {
		visit(entry)
	}
	return docs
}

func TestPluginDocs_PhasesReachable(t *testing.T) {
	docs := linkedSkillDocs(t, repoRoot(t), "skills/coverage-verification/SKILL.md")
	for _, phase := range []string{"critic", "live", "mutation", "finalize-recovery"} {
		path := filepath.Join("skills", "coverage-verification", "references", phase+".md")
		if _, ok := docs[path]; !ok {
			t.Errorf("verification cannot route a reader to %s", path)
		}
	}
}

func TestPluginDocs_ExplicitOptIn(t *testing.T) {
	for _, skill := range []string{"coverage-verification", "test-coverage-design"} {
		path := filepath.Join(repoRoot(t), "skills", skill, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(string(data), "---", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
			t.Fatalf("%s: missing frontmatter", path)
		}
		disabled := false
		for _, line := range strings.Split(parts[1], "\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.TrimSpace(key) == "disable-model-invocation" {
				disabled = strings.TrimSpace(value) == "true"
			}
		}
		if !disabled {
			t.Errorf("%s: automatic invocation must remain disabled", path)
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

func TestPluginDocs_AgentToolRestrictions(t *testing.T) {
	root := repoRoot(t)
	for _, agent := range []struct {
		path    string
		allowed map[string]bool
	}{
		{"agents/mutator.md", map[string]bool{"Read": true, "Grep": true, "Glob": true, "Edit": true}},
		{"agents/live-runner.md", map[string]bool{"Read": true, "Grep": true, "Glob": true}},
		{"agents/coverage-critic.md", map[string]bool{"Read": true, "Grep": true, "Glob": true}},
		{"agents/scenario-reviewer.md", map[string]bool{"Read": true, "Grep": true, "Glob": true}},
	} {
		tools := agentTools(t, root, agent.path)
		for _, tool := range tools {
			if !agent.allowed[tool] {
				t.Errorf("%s: tool %q escapes the agent's execution/edit boundary", agent.path, tool)
			}
		}
	}
}

func TestPluginDocs_MutatorSearchScopedToWorktree(t *testing.T) {
	root := repoRoot(t)
	tools := agentTools(t, root, "agents/mutator.md")
	hasGrep, hasGlob := false, false
	for _, tool := range tools {
		switch tool {
		case "Grep":
			hasGrep = true
		case "Glob":
			hasGlob = true
		}
	}
	if !hasGrep || !hasGlob {
		t.Errorf("agents/mutator.md: tools = %v, want Grep and Glob so enumeration can search", tools)
	}

	data, err := os.ReadFile(filepath.Join(root, "agents", "mutator.md"))
	if err != nil {
		t.Fatalf("read agents/mutator.md: %v", err)
	}
	if !strings.Contains(string(data), "set `path` to the worktree") {
		t.Errorf("agents/mutator.md: missing instruction to set `path` to the worktree on every Grep/Glob call")
	}
}

var opTokenPattern = regexp.MustCompile(`test-design-verifier ([a-zA-Z0-9<>-]+)`)

func TestPluginDocs_DocumentedCommandsExist(t *testing.T) {
	docs := linkedSkillDocs(t, repoRoot(t),
		"skills/coverage-verification/SKILL.md",
		"skills/test-coverage-design/SKILL.md")
	for path, text := range docs {
		for _, match := range opTokenPattern.FindAllStringSubmatch(text, -1) {
			op := match[1]
			if op == "<op>" {
				continue
			}
			if _, ok := ops[op]; !ok {
				t.Errorf("%s: documented command %q is not executable", path, op)
			}
		}
	}
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
	if err := coverage.ValidateLiveDecision(src); err != nil {
		t.Errorf("ValidateLiveDecision(%s) = %v, want no error", path, err)
	}
}

func TestPluginDocs_MutationSitesAreAFloor(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"agents/mutator.md",
		filepath.Join("skills", "coverage-verification", "references", "mutation.md"),
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(data), "floor") {
			t.Errorf("%s: missing the sites-are-a-floor rule", rel)
		}
	}

	for _, dir := range []string{"agents", "skills"} {
		walkErr := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), "and no other") {
				t.Errorf("%s: uses \"and no other\" as site-exclusion wording; named sites are a floor, not a ceiling", path)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}
}
