package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"test-design/internal/evidence"
	"test-design/internal/gotestjson"
	"test-design/internal/runrecord"
)

// BaselineIdentity names one test by the (Scope, Name) pair CompareBaseline
// matches on exactly — never by Name alone.
type BaselineIdentity struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

// Baseline is the evidence document recording a suite run's outcome, kept so
// later runs can be compared against it via CompareBaseline.
type Baseline struct {
	evidence.Envelope
	RepoCommit         string             `json:"repoCommit"`
	BaselineCommit     string             `json:"baselineCommit"`
	RunnerKind         string             `json:"runnerKind"`
	Command            []string           `json:"command"`
	ToolVersions       map[string]string  `json:"toolVersions"`
	Counts             gotestjson.Counts  `json:"counts"`
	Failed             []BaselineIdentity `json:"failed"`
	Skipped            []BaselineIdentity `json:"skipped"`
	ReportSHA256       string             `json:"reportSha256"`
	StartedAt          string             `json:"startedAt"`
	FinishedAt         string             `json:"finishedAt"`
	ExitStatus         int                `json:"exitStatus"`
	NormalizationError string             `json:"normalizationError"`
}

// WriteBaseline writes b to path as canonical JSON via evidence.WriteJSON,
// sorting the Failed and Skipped identity lists first so byte-identical
// logical content always produces byte-identical output.
func WriteBaseline(path string, b *Baseline) error {
	out := *b
	out.Failed = sortedIdentities(b.Failed)
	out.Skipped = sortedIdentities(b.Skipped)
	if err := evidence.WriteJSON(path, &out); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

func sortedIdentities(ids []BaselineIdentity) []BaselineIdentity {
	sorted := append([]BaselineIdentity(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Scope != sorted[j].Scope {
			return sorted[i].Scope < sorted[j].Scope
		}
		return sorted[i].Name < sorted[j].Name
	})
	return sorted
}

// LoadBaseline reads and decodes the baseline evidence document at path. It
// fails closed: any error returns a nil *Baseline, never a partially
// populated one. When wantSHA256 is non-empty, path's content must hash to
// it; an empty wantSHA256 skips the check (the coverage header may omit it
// before a baseline exists).
func LoadBaseline(path, wantSHA256 string) (*Baseline, error) {
	if wantSHA256 != "" {
		got, err := evidence.SHA256File(path)
		if err != nil {
			return nil, fmt.Errorf("load baseline %s: %w", path, err)
		}
		if got != wantSHA256 {
			return nil, fmt.Errorf("load baseline %s: baseline evidence hash mismatch: want %s, got %s", path, wantSHA256, got)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load baseline %s: %w", path, err)
	}

	if _, err := evidence.DecodeEnvelope(data); err != nil {
		return nil, fmt.Errorf("load baseline %s: %w", path, err)
	}

	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("load baseline %s: %w", path, err)
	}
	return &b, nil
}

// CompareBaseline reports every currently failed record in rep whose exact
// (Scope, Name) identity is not among b.Failed as a new red Finding. A nil
// baseline has an empty Failed set, so every current failure is new. Being
// in b.Skipped grants no exemption: only b.Failed does.
func CompareBaseline(b *Baseline, rep *gotestjson.Report) []Finding {
	baselineFailed := make(map[BaselineIdentity]bool)
	if b != nil {
		for _, id := range b.Failed {
			baselineFailed[id] = true
		}
	}

	var findings []Finding
	for _, rec := range rep.Records {
		if rec.Status != runrecord.Failed {
			continue
		}
		id := BaselineIdentity{Scope: rec.Scope, Name: rec.Name}
		if baselineFailed[id] {
			continue
		}
		findings = append(findings, Finding{
			Kind:   "red",
			Detail: fmt.Sprintf("new failure %s %s (not in baseline)", id.Scope, id.Name),
		})
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].RowID != findings[j].RowID {
			return findings[i].RowID < findings[j].RowID
		}
		return findings[i].Detail < findings[j].Detail
	})
	return findings
}
