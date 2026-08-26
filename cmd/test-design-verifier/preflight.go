package main

import (
	"flag"
	"fmt"
	"io"

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
	sourcePath := flagString(fs, "source")
	if sourcePath == "" {
		return &cliError{Code: "missing-source", Message: "preflight: --source is required"}
	}

	if _, err := coverage.BuildRegistry(repoRoot); err != nil {
		return &cliError{Code: "registry-error", Message: fmt.Sprintf("preflight: %v", err)}
	}

	src, err := coverage.ParseSource(sourcePath)
	if err != nil {
		return &cliError{Code: "parse-source-failed", Message: fmt.Sprintf("preflight: %v", err)}
	}

	layers, highRisk := computeLayers(src.Rows)

	out := preflightOutput{
		Envelope:             evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		Source:               src.Path,
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

func computeLayers(rows []coverage.Row) (layerCounts, int) {
	var lc layerCounts
	highRisk := 0
	for _, r := range rows {
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
