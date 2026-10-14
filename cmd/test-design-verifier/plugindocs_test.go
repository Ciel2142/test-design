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

// CLAUDE_PLUGIN_ROOT is substituted into configuration files, not exported
// into the Bash tool's shell, so each skill must tell the reader how to
// resolve the plugin root before the first `go run -C`.
func TestPluginDocs_PluginRootResolution(t *testing.T) {
	root := repoRoot(t)
	for _, skill := range []string{"coverage-verification", "test-coverage-design"} {
		path := filepath.Join("skills", skill, "SKILL.md")
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		first := strings.Index(text, `go run -C "$CLAUDE_PLUGIN_ROOT"`)
		resolve := strings.Index(text, "installed_plugins.json")
		if resolve < 0 {
			t.Errorf("%s: no plugin-root resolution step naming installed_plugins.json", path)
			continue
		}
		if first >= 0 && first < resolve {
			t.Errorf("%s: first go run -C at offset %d precedes the resolution step at %d", path, first, resolve)
		}
	}
}

// Under OpenCode nothing substitutes CLAUDE_PLUGIN_ROOT; the adapter exports
// the root under its own name, and each skill must read that same name.
func TestPluginDocs_OpenCodeRootVariable(t *testing.T) {
	const name = "TEST_DESIGN_PLUGIN_ROOT"
	root := repoRoot(t)
	adapter, err := os.ReadFile(filepath.Join(root, "opencode", "plugin.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(adapter), "output.env."+name+" =") {
		t.Errorf("opencode/plugin.js: does not export %s into the shell", name)
	}
	for _, skill := range []string{"coverage-verification", "test-coverage-design"} {
		path := filepath.Join("skills", skill, "SKILL.md")
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "${"+name+":-") {
			t.Errorf("%s: plugin-root resolution does not read %s", path, name)
		}
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

	mutatorPath := "agents/mutator.md"
	mutatorData := readFileOrFatal(t, root, mutatorPath)
	for _, want := range []string{
		"not a ceiling",
		"evaluate each one",
		"`extra-N`",
		"is a decision, not an omission",
		"give a one-line argument",
	} {
		if !strings.Contains(mutatorData, want) {
			t.Errorf("%s: missing floor-rule text %q", mutatorPath, want)
		}
	}
	if idx := strings.Index(mutatorData, "## Output format"); idx == -1 {
		t.Fatalf("%s: missing ## Output format section", mutatorPath)
	} else if !strings.Contains(mutatorData[idx:], "excluded") {
		t.Errorf("%s: output format has no line shape for an excluded named site", mutatorPath)
	}

	mutationPath := filepath.Join("skills", "coverage-verification", "references", "mutation.md")
	mutationData := readFileOrFatal(t, root, mutationPath)
	for _, want := range []string{
		"not a ceiling",
		"answered by fewer sites than it names",
		"`extra-N`",
		"needs a one-line reason on the record",
	} {
		if !strings.Contains(mutationData, want) {
			t.Errorf("%s: missing floor-rule text %q", mutationPath, want)
		}
	}

	visited := map[string]int{}
	for _, dir := range []string{"agents", "skills"} {
		walkErr := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			visited[dir]++
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
		if visited[dir] == 0 {
			t.Fatalf("walk %s: visited no .md files, want at least one", dir)
		}
	}
}

func readFileOrFatal(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}
