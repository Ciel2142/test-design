package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

type runnerOutput struct {
	Kind        string   `json:"kind"`
	Root        string   `json:"root"`
	Command     []string `json:"command"`
	ReportGlobs []string `json:"reportGlobs"`
}

type layerCounts struct {
	Layer1 int `json:"layer1"`
	Layer2 int `json:"layer2"`
	Layer3 int `json:"layer3"`
	Layer4 int `json:"layer4"`
}

type preflightOutput struct {
	evidence.Envelope
	Source               string       `json:"source"`
	SourceSHA256         string       `json:"sourceSha256"`
	BaselineCommit       string       `json:"baselineCommit"`
	BaselineEvidencePath string       `json:"baselineEvidencePath"`
	BaselineSHA256       string       `json:"baselineSha256"`
	Runner               runnerOutput `json:"runner"`
	Layers               layerCounts  `json:"layers"`
	HighRiskCount        int          `json:"highRiskCount"`
	LiveProfileRequired  bool         `json:"liveProfileRequired"`
	LiveProfile          string       `json:"liveProfile"`
}

func preflightHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	repoRoot := flagString(fs, "repo")

	if _, err := coverage.BuildRegistry(repoRoot); err != nil {
		return &cliError{Code: "registry-error", Message: fmt.Sprintf("preflight: %v", err)}
	}

	sourcePath := flagString(fs, "source")
	if sourcePath == "" {
		return &cliError{Code: "missing-source", Message: "preflight: --source is required"}
	}
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(repoRoot, sourcePath)
	}
	relSource, cliErr := repoRelative(repoRoot, sourcePath)
	if cliErr != nil {
		return cliErr
	}

	src, err := coverage.ParseSource(sourcePath)
	if err != nil {
		return &cliError{Code: "parse-source-failed", Message: fmt.Sprintf("preflight: %v", err)}
	}

	layers, highRisk := computeLayers(src.Rows, src.Retired)

	out := preflightOutput{
		Envelope:             evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		Source:               relSource,
		SourceSHA256:         src.SHA256,
		BaselineCommit:       src.BaselineCommit,
		BaselineEvidencePath: src.BaselineEvidencePath,
		BaselineSHA256:       src.BaselineSHA256,
		Runner: runnerOutput{
			Kind:        src.Runner.Kind,
			Root:        src.Runner.Root,
			Command:     src.Runner.Command,
			ReportGlobs: src.Runner.ReportGlobs,
		},
		Layers:              layers,
		HighRiskCount:       highRisk,
		LiveProfileRequired: layers.Layer3 > 0,
		LiveProfile:         src.LiveProfile,
	}
	return writeStdout(stdout, out)
}

// repoRelative renders sourcePath in the verifier's one path vocabulary:
// repo-root-relative and slash-separated, the form coverage.Owner.DocPath
// carries and wave `trace` compares against.
func repoRelative(repoRoot, sourcePath string) (string, *cliError) {
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", &cliError{Code: "invalid-repo", Message: fmt.Sprintf("preflight: resolve repo %s: %v", repoRoot, err)}
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", &cliError{Code: "invalid-source", Message: fmt.Sprintf("preflight: resolve source %s: %v", sourcePath, err)}
	}
	rel, err := filepath.Rel(absRepo, absSource)
	if err != nil {
		return "", &cliError{
			Code:    "source-outside-repo",
			Message: fmt.Sprintf("preflight: source %s is not under repo %s: %v", sourcePath, repoRoot, err),
		}
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &cliError{
			Code:    "source-outside-repo",
			Message: fmt.Sprintf("preflight: source %s is not under repo %s", sourcePath, repoRoot),
		}
	}
	return filepath.ToSlash(rel), nil
}

func computeLayers(rows []coverage.Row, retired []string) (layerCounts, int) {
	retiredSet := make(map[string]bool, len(retired))
	for _, id := range retired {
		retiredSet[id] = true
	}

	var lc layerCounts
	highRisk := 0
	for _, r := range rows {
		if retiredSet[r.ID] {
			continue
		}
		lc.Layer1++
		lc.Layer2++
		if r.Level == "smoke" || r.Level == "e2e" {
			lc.Layer3++
		}
		if r.Risk == "high" {
			lc.Layer4++
			highRisk++
		}
	}
	return lc, highRisk
}
