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

// dispatchContributor is one bound tag site handed to the critic, carrying
// its own declaration range and the exact source read from that range.
type dispatchContributor struct {
	Scope            string `json:"scope"`
	TestName         string `json:"testName"`
	Status           string `json:"status"`
	File             string `json:"file"`
	Line             int    `json:"line"`
	EndLine          int    `json:"endLine"`
	TestSource       string `json:"testSource"`
	TestSourceSHA256 string `json:"testSourceSha256"`
}

// dispatchRow is one active row handed to the critic. Its scalar identity and
// source come from the row's primary contributor; Contributors lists every
// bound site with its own source.
type dispatchRow struct {
	RowID                string                `json:"rowId"`
	Behavior             string                `json:"behavior"`
	Expected             string                `json:"expected"`
	Level                string                `json:"level"`
	Risk                 string                `json:"risk"`
	VerificationMutation string                `json:"verificationMutation"`
	Status               string                `json:"status"`
	Scope                string                `json:"scope,omitempty"`
	TestName             string                `json:"testName,omitempty"`
	File                 string                `json:"file,omitempty"`
	Line                 int                   `json:"line,omitempty"`
	EndLine              int                   `json:"endLine,omitempty"`
	TestSource           string                `json:"testSource,omitempty"`
	TestSourceSHA256     string                `json:"testSourceSha256,omitempty"`
	Contributors         []dispatchContributor `json:"contributors,omitempty"`
	ClaimSHA256          string                `json:"claimSha256"`
}

type dispatchInputs struct {
	evidence.Envelope
	RunID        string                  `json:"runId"`
	Source       string                  `json:"source"`
	SourceSHA256 string                  `json:"sourceSha256"`
	RepoCommit   string                  `json:"repoCommit"`
	Rows         []dispatchRow           `json:"rows"`
	Runner       evidence.RunnerIdentity `json:"runner"`
	ToolVersions map[string]string       `json:"toolVersions"`
}

// claimOf returns the approved semantics row carries as an evidence.Claim,
// suitable for ClaimDigest. It reads only the row's claim-identity fields:
// a row's status, source binding or contributors never affect its digest.
func claimOf(row dispatchRow) evidence.Claim {
	return evidence.Claim{
		ID:       row.RowID,
		Behavior: row.Behavior,
		Expected: row.Expected,
		Level:    row.Level,
		Mutation: row.VerificationMutation,
		Risk:     row.Risk,
	}
}

// toolVersionsFor is the seam writeDispatchInputs calls through to capture
// toolchain provenance; production code never reassigns it. Tests swap it to
// avoid depending on a real toolchain binary and restore it with t.Cleanup.
var toolVersionsFor = toolVersions

func writeDispatchInputs(src *coverage.Source, ev *traceEvidence, suiteDir, runDir string) (string, error) {
	bindings := make(map[string]traceRowResult, len(ev.RowResults))
	for _, result := range ev.RowResults {
		bindings[result.RowID] = result
	}
	gaps := make(map[string]string)
	reds := make(map[string]bool)
	for _, finding := range ev.Findings {
		if finding.RowID == "" {
			continue
		}
		switch finding.Kind {
		case "red":
			reds[finding.RowID] = true
		case "gap", "ambiguity":
			gaps[finding.RowID] = finding.Kind
		}
	}
	payload := dispatchInputs{
		Envelope: ev.Envelope, RunID: ev.RunID, Source: ev.Source,
		SourceSHA256: ev.SourceSHA256, RepoCommit: ev.RepoCommit,
		Rows: make([]dispatchRow, 0, len(src.Rows)),
		Runner: evidence.RunnerIdentity{
			Kind: src.Runner.Kind, Root: src.Runner.Root,
			Command: src.Runner.Command, ReportGlobs: src.Runner.ReportGlobs,
		},
		ToolVersions: toolVersionsFor(suiteDir, src.Runner.Kind, src.Runner.Command),
	}
	read := newRangeReader(suiteDir)
	for _, row := range src.Rows {
		if slices.Contains(src.Retired, row.ID) {
			continue
		}
		entry := dispatchRow{RowID: row.ID, Behavior: row.Behavior, Expected: row.Expected,
			Level: row.Level, Risk: row.Risk, VerificationMutation: row.Mutation, Status: "gap"}
		entry.ClaimSHA256 = evidence.ClaimDigest(claimOf(entry))
		if binding, bound := bindings[row.ID]; bound && binding.File != "" {
			entry.Status, entry.Scope, entry.TestName = binding.Status, binding.Scope, binding.Name
			entry.File, entry.Line, entry.EndLine = binding.File, binding.Line, binding.EndLine
			primary, err := read(binding.File, binding.Line, binding.EndLine)
			if err != nil {
				return "", err
			}
			entry.TestSource, entry.TestSourceSHA256 = primary.text, primary.hash
			for _, c := range binding.Contributors {
				text, err := read(c.File, c.Line, c.EndLine)
				if err != nil {
					return "", err
				}
				entry.Contributors = append(entry.Contributors, dispatchContributor{
					Scope: c.Scope, TestName: c.Name, Status: c.Status,
					File: c.File, Line: c.Line, EndLine: c.EndLine,
					TestSource: text.text, TestSourceSHA256: text.hash,
				})
			}
		}
		// A row-bound gap or ambiguity names the row's status, unless a red
		// finding names the same row: the report ranks a row's findings red
		// over gap over ambiguity, and one row must not be red there and gap
		// here. It never removes the source: a row that has a binding still
		// hands the critic its primary contributor's declaration, and only a
		// row with no bound site is sourceless.
		if gap, ok := gaps[row.ID]; ok && !reds[row.ID] {
			entry.Status = gap
		}
		payload.Rows = append(payload.Rows, entry)
	}
	path := filepath.Join(runDir, dispatchInputsFile)
	if err := evidence.WriteJSON(path, &payload); err != nil {
		return "", err
	}
	return path, nil
}

// sourceText is one bound declaration exactly as the disposable worktree
// holds it, with the SHA-256 of those bytes.
type sourceText struct{ text, hash string }

// newRangeReader returns a reader over declaration ranges under suiteDir. It
// reads each file once and each distinct line range once, so the several
// contributors of one row never re-read the same bytes.
func newRangeReader(suiteDir string) func(file string, first, last int) (sourceText, error) {
	type sourceRange struct {
		file        string
		first, last int
	}
	files := make(map[string][]byte)
	ranges := make(map[sourceRange]sourceText)

	return func(file string, first, last int) (sourceText, error) {
		key := sourceRange{file, first, last}
		if text, cached := ranges[key]; cached {
			return text, nil
		}
		data, cached := files[file]
		if !cached {
			rel, cliErr := confinedRelPath("trace", "binding file", "source-outside-repo", file)
			if cliErr != nil {
				return sourceText{}, fmt.Errorf("dispatch source: %s", cliErr.Message)
			}
			var err error
			data, err = os.ReadFile(filepath.Join(suiteDir, filepath.FromSlash(rel)))
			if err != nil {
				return sourceText{}, fmt.Errorf("dispatch source %s: %w", rel, err)
			}
			files[file] = data
		}
		snippet, err := sourceLineRange(data, first, last)
		if err != nil {
			return sourceText{}, fmt.Errorf("dispatch source %s: %w", file, err)
		}
		text := sourceText{string(snippet), fmt.Sprintf("%x", sha256.Sum256(snippet))}
		ranges[key] = text
		return text, nil
	}
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
