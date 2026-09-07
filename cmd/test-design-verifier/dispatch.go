package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

const dispatchInputsFile = "dispatch-inputs.json"

type dispatchRow struct {
	RowID                string `json:"rowId"`
	Behavior             string `json:"behavior"`
	Expected             string `json:"expected"`
	Level                string `json:"level"`
	Risk                 string `json:"risk"`
	VerificationMutation string `json:"verificationMutation"`
	Status               string `json:"status"`
	Scope                string `json:"scope,omitempty"`
	TestName             string `json:"testName,omitempty"`
	File                 string `json:"file,omitempty"`
	Line                 int    `json:"line,omitempty"`
	EndLine              int    `json:"endLine,omitempty"`
	TestSource           string `json:"testSource,omitempty"`
	TestSourceSHA256     string `json:"testSourceSha256,omitempty"`
}

type dispatchInputs struct {
	evidence.Envelope
	RunID        string        `json:"runId"`
	Source       string        `json:"source"`
	SourceSHA256 string        `json:"sourceSha256"`
	RepoCommit   string        `json:"repoCommit"`
	Rows         []dispatchRow `json:"rows"`
}

func writeDispatchInputs(src *coverage.Source, ev *traceEvidence, suiteDir, runDir string) (string, error) {
	bindings := make(map[string]traceRowResult, len(ev.RowResults))
	for _, result := range ev.RowResults {
		bindings[result.RowID] = result
	}
	gaps := make(map[string]string)
	for _, finding := range ev.Findings {
		if finding.RowID != "" && (finding.Kind == "gap" || finding.Kind == "ambiguity") {
			gaps[finding.RowID] = finding.Kind
		}
	}
	payload := dispatchInputs{
		Envelope: ev.Envelope, RunID: ev.RunID, Source: ev.Source,
		SourceSHA256: ev.SourceSHA256, RepoCommit: ev.RepoCommit,
		Rows: make([]dispatchRow, 0, len(src.Rows)),
	}
	files := make(map[string][]byte)
	type sourceRange struct { file string; first, last int }
	type sourceText struct { text, hash string }
	ranges := make(map[sourceRange]sourceText)
	for _, row := range src.Rows {
		if slices.Contains(src.Retired, row.ID) {
			continue
		}
		entry := dispatchRow{RowID: row.ID, Behavior: row.Behavior, Expected: row.Expected,
			Level: row.Level, Risk: row.Risk, VerificationMutation: row.Mutation, Status: "gap"}
		binding, bound := bindings[row.ID]
		if gap, ok := gaps[row.ID]; ok {
			entry.Status = gap
		} else if bound && binding.File != "" {
			entry.Status, entry.Scope, entry.TestName = binding.Status, binding.Scope, binding.Name
			entry.File, entry.Line, entry.EndLine = binding.File, binding.Line, binding.EndLine
			key := sourceRange{binding.File, binding.Line, binding.EndLine}
			text, cached := ranges[key]
			if !cached {
				data, cached := files[binding.File]
				if !cached {
					rel, cliErr := confinedRelPath("trace", "binding file", "source-outside-repo", binding.File)
					if cliErr != nil {
						return "", fmt.Errorf("dispatch source: %s", cliErr.Message)
					}
					var err error
					data, err = os.ReadFile(filepath.Join(suiteDir, filepath.FromSlash(rel)))
					if err != nil {
						return "", fmt.Errorf("dispatch source %s: %w", rel, err)
					}
					files[binding.File] = data
				}
				snippet, err := sourceLineRange(data, binding.Line, binding.EndLine)
				if err != nil {
					return "", fmt.Errorf("dispatch source %s: %w", binding.File, err)
				}
				text = sourceText{string(snippet), fmt.Sprintf("%x", sha256.Sum256(snippet))}
				ranges[key] = text
			}
			entry.TestSource, entry.TestSourceSHA256 = text.text, text.hash
		}
		payload.Rows = append(payload.Rows, entry)
	}
	path := filepath.Join(runDir, dispatchInputsFile)
	if err := evidence.WriteJSON(path, &payload); err != nil {
		return "", err
	}
	return path, nil
}

func sourceLineRange(data []byte, first, last int) ([]byte, error) {
	if first < 1 || last < first {
		return nil, fmt.Errorf("invalid bound range %d-%d", first, last)
	}
	start, offset := 0, 0
	for line := 1; line <= last; line++ {
		if offset >= len(data) {
			return nil, fmt.Errorf("bound range %d-%d exceeds source", first, last)
		}
		if line == first {
			start = offset
		}
		if next := bytes.IndexByte(data[offset:], '\n'); next >= 0 {
			offset += next + 1
		} else {
			offset = len(data)
		}
	}
	return data[start:offset], nil
}
