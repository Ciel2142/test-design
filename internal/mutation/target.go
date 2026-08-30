package mutation

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"test-design/internal/coverage"
)

// Target is the command mutation testing runs for one Identity: the row's
// bound test family alone.
type Target struct {
	Command     []string
	ReportGlobs []string // JVM only; task-scoped subset. nil for go.
}

// GoTarget builds the go test invocation for id from cfg.Command: it keeps
// -count=1 (erroring if absent, even though upstream source validation
// already guarantees it — this package fails closed on its own), targets
// exactly id.Scope's package, and anchors -run to id.Name and its subtests.
func GoTarget(cfg coverage.RunnerConfig, id Identity) (Target, error) {
	if cfg.Kind != "go" {
		return Target{}, fmt.Errorf("mutation: GoTarget: kind must be go, got %q", cfg.Kind)
	}
	if id.Scope == "" || id.Name == "" {
		return Target{}, fmt.Errorf("mutation: GoTarget: identity scope and name must not be empty")
	}
	if !slices.Contains(cfg.Command, "-count=1") {
		return Target{}, fmt.Errorf("mutation: GoTarget: command %v is missing -count=1", cfg.Command)
	}

	cmd := slices.Clone(cfg.Command)
	// package pattern = sole non-flag token after "go test". 2+ non-flag
	// tokens is ambiguous (e.g. two-token "-timeout 300s" flag shape) —
	// guessing would risk clobbering a flag's value, so fail closed.
	pkgIdx := -1
	nonFlagCount := 0
	for i := 2; i < len(cmd); i++ {
		if !strings.HasPrefix(cmd[i], "-") {
			nonFlagCount++
			pkgIdx = i
		}
	}
	if nonFlagCount > 1 {
		return Target{}, fmt.Errorf("mutation: GoTarget: command %v has %d non-flag arguments after go test; expected at most one package pattern", cfg.Command, nonFlagCount)
	}
	switch {
	case pkgIdx == -1:
		cmd = append(cmd, id.Scope)
	case cmd[pkgIdx] != id.Scope:
		cmd[pkgIdx] = id.Scope
	}
	cmd = append(cmd, "-run", "^"+regexp.QuoteMeta(id.Name)+"$")
	return Target{Command: cmd}, nil
}

// JVMTarget builds the maven or gradle invocation for id from cfg.Command,
// selecting exactly the bound class+method and the report-glob subset that
// can attest to that one task's run alone.
func JVMTarget(cfg coverage.RunnerConfig, id Identity) (Target, error) {
	if id.Scope == "" || id.Name == "" {
		return Target{}, fmt.Errorf("mutation: JVMTarget: identity scope and name must not be empty")
	}
	switch cfg.Kind {
	case "maven":
		return mavenTarget(cfg, id)
	case "gradle":
		return gradleTarget(cfg, id)
	default:
		return Target{}, fmt.Errorf("mutation: JVMTarget: unsupported kind %q", cfg.Kind)
	}
}

func mavenTarget(cfg coverage.RunnerConfig, id Identity) (Target, error) {
	// A "verify" phase runs Failsafe (integration tests); anything else is
	// Surefire's "test" phase. The two write to disjoint report directories.
	verify := slices.Contains(cfg.Command, "verify")
	flag := fmt.Sprintf("-Dtest=%s#%s", id.Scope, id.Name)
	marker := "surefire-reports"
	if verify {
		flag = fmt.Sprintf("-Dit.test=%s#%s", id.Scope, id.Name)
		marker = "failsafe-reports"
	}

	var globs []string
	for _, g := range cfg.ReportGlobs {
		if strings.Contains(g, marker) {
			globs = append(globs, g)
		}
	}
	if len(globs) == 0 {
		return Target{}, fmt.Errorf("mutation: JVMTarget: report-globs %v name no %s path for %s#%s", cfg.ReportGlobs, marker, id.Scope, id.Name)
	}

	cmd := append(slices.Clone(cfg.Command), flag)
	return Target{Command: cmd, ReportGlobs: globs}, nil
}

func gradleTarget(cfg coverage.RunnerConfig, id Identity) (Target, error) {
	if !slices.Contains(cfg.Command, "--rerun") && !slices.Contains(cfg.Command, "--rerun-tasks") {
		return Target{}, fmt.Errorf("mutation: JVMTarget: gradle command %v is missing --rerun or --rerun-tasks", cfg.Command)
	}
	cmd := append(slices.Clone(cfg.Command), "--tests", fmt.Sprintf("%s.%s", id.Scope, id.Name))
	return Target{Command: cmd, ReportGlobs: slices.Clone(cfg.ReportGlobs)}, nil
}
