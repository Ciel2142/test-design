// Package report defines the finalize op's evidence loaders, verdict
// computation, and canonical report document.
package report

import (
	"encoding/json"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
)

type SuiteCounts struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type RowAccounting struct {
	ActiveRows  int      `json:"activeRows"`
	WithResult  int      `json:"withResult"`
	Unaccounted []string `json:"unaccounted"`
}

type TraceRowResult struct {
	RowID  string `json:"rowId"`
	Scope  string `json:"scope"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type TraceFinding struct {
	Kind   string `json:"kind"`
	RowID  string `json:"rowId"`
	Detail string `json:"detail"`
}

type TraceNewFailure struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

type TraceEvidence struct {
	evidence.Envelope
	RunID          string            `json:"runId"`
	Source         string            `json:"source"`
	SourceSHA256   string            `json:"sourceSha256"`
	RepoCommit     string            `json:"repoCommit"`
	BaselineCommit string            `json:"baselineCommit"`
	BaselineStatus string            `json:"baselineStatus"`
	ExitStatus     int               `json:"exitStatus"`
	ReportSHA256   string            `json:"reportSha256"`
	Counts         SuiteCounts       `json:"counts"`
	RowAccounting  RowAccounting     `json:"rowAccounting"`
	FindingCounts  map[string]int    `json:"findingCounts"`
	RowResults     []TraceRowResult  `json:"rowResults"`
	Findings       []TraceFinding    `json:"findings"`
	NewFailures    []TraceNewFailure `json:"newFailures"`
}

type LiveRowVerdict struct {
	RowID            string `json:"rowId"`
	Verdict          string `json:"verdict"`
	RedactionBarrier bool   `json:"redactionBarrier,omitempty"`
}

// LiveStepFailure names one command that completed, failed its predicates
// (the implicit exit-code 0 predicate on start and health), and whose
// outcome reached no row verdict: a start or health command, a plan step
// with no scBindings, or a plan-cleanup step with no scBindings or whose
// verdict application was suppressed because the walkthrough never ran.
// Profile cleanup commands are never listed here; CleanupOK/CleanupFailed
// carry those. Expectation is the planner's own text as the producer published
// it: the same redaction barrier that governs a reduced transcript scanned it,
// and on a hit the producer wrote the literal "[withheld by redaction barrier]"
// here in place of the text. A finalize reason may repeat it as it stands.
type LiveStepFailure struct {
	Phase       string `json:"phase"`
	Index       int    `json:"index"`
	Expectation string `json:"expectation"`
}

type LiveEvidence struct {
	evidence.Envelope
	RunID           string            `json:"runId"`
	RepoCommit      string            `json:"repoCommit"`
	Source          string            `json:"source"`
	SourceSHA256    string            `json:"sourceSha256"`
	RowVerdicts     []LiveRowVerdict  `json:"rowVerdicts"`
	CleanupOK       bool              `json:"cleanupOK"`
	CleanupFailed   bool              `json:"cleanupFailed"`
	ExecutionErrors []string          `json:"executionErrors"`
	SnapshotEqual   bool              `json:"snapshotEqual"`
	UnboundFailures []LiveStepFailure `json:"unboundFailures"`
}

type LiveAttempt struct {
	evidence.Envelope
	RunID        string `json:"runID"`
	RepoCommit   string `json:"repoCommit"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
	Profile      string `json:"profile"`
	Refusal      string `json:"refusal"`
}

// Attempt is the envelope-carrying shape shared by the attempt documents a
// refused layer-4 run leaves behind; finalize checks it before publishing one.
type Attempt struct {
	evidence.Envelope
	RunID        string `json:"runID"`
	RowID        string `json:"rowID"`
	RepoCommit   string `json:"repoCommit"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"sourceSha256"`
	Refusal      string `json:"refusal"`
}

type MutationSiteOutcome struct {
	Site              string `json:"site"`
	Outcome           string `json:"outcome"`
	DiffPath          string `json:"diffPath"`
	DiffSHA256        string `json:"diffSha256"`
	MutatedRunDir     string `json:"mutatedRunDir"`
	MutatedExitStatus int    `json:"mutatedExitStatus"`
	CleanRunDir       string `json:"cleanRunDir"`
	CleanExitStatus   int    `json:"cleanExitStatus"`
}

// MutationDiscard mirrors internal/mutation.DiscardRecord: one rejected
// mutator edit a row's confirmation step discarded before evaluating it.
type MutationDiscard struct {
	Site       string `json:"site"`
	Reason     string `json:"reason"`
	DiffPath   string `json:"diffPath"`
	DiffSHA256 string `json:"diffSha256"`
}

type MutationEvidence struct {
	evidence.Envelope
	RunID           string                `json:"runId"`
	RowID           string                `json:"rowID"`
	RepoCommit      string                `json:"repoCommit"`
	Source          string                `json:"source"`
	SourceSHA256    string                `json:"sourceSha256"`
	Sites           []MutationSiteOutcome `json:"sites"`
	SnapshotEqual   bool                  `json:"snapshotEqual"`
	EnumeratedSites []string              `json:"enumeratedSites,omitempty"`
	Discarded       []MutationDiscard     `json:"discarded,omitempty"`
}

type Freshness struct {
	Result string         `json:"result"`
	Stale  []StaleFinding `json:"stale"`
}

type StaleFinding struct {
	Hunk     string `json:"hunk"`
	Behavior string `json:"behavior"`
}

type CriticVerdict struct {
	RowID   string `json:"rowId"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

// ReviewArtifact is the retained review-<groupId>-<seq>.json document a
// prior run left behind; a reusing run decodes it after verifying its digest.
type ReviewArtifact struct {
	evidence.Envelope
	RunID        string          `json:"runId"`
	RepoCommit   string          `json:"repoCommit"`
	Source       string          `json:"source"`
	SourceSHA256 string          `json:"sourceSha256"`
	GroupID      string          `json:"groupId"`
	Rows         []string        `json:"rows"`
	Verdicts     []CriticVerdict `json:"verdicts"`
}

type Batch struct {
	BatchID  string          `json:"batchId"`
	RunID    string          `json:"runId"`
	Rows     []string        `json:"rows"`
	Verdicts []CriticVerdict `json:"verdicts"`
	// Reuse is set when this batch was adopted from an earlier run; it names
	// that run's identity, sequence number, outcome and retained artifact.
	Reuse *evidence.ReuseReference `json:"reuse,omitempty"`
}

type MutationInput struct {
	RowID               string            `json:"rowId"`
	EvidencePath        string            `json:"evidencePath,omitempty"`
	Outcome             string            `json:"outcome,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	AttemptEvidencePath string            `json:"attemptEvidencePath,omitempty"`
	Evidence            *MutationEvidence `json:"-"`
	Attempt             *Attempt          `json:"-"`
	// Reuse is set when this entry was adopted from an earlier run.
	Reuse *evidence.ReuseReference `json:"reuse,omitempty"`
}

type Inputs struct {
	Source          *coverage.Source
	RunID           string
	HeadCommit      string
	Trace           *TraceEvidence
	Freshness       Freshness
	Batches         []Batch
	Live            *LiveEvidence
	LiveAttempt     *LiveAttempt
	Mutations       []MutationInput
	Acknowledgement string
}

type Reason struct {
	Layer  string `json:"layer"`
	RowID  string `json:"rowId"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type LayerVerdict struct {
	Status string `json:"status"`
	Scope  string `json:"scope,omitempty"`
	Name   string `json:"name,omitempty"`
}

type CriticRef struct {
	Verdict string `json:"verdict"`
	BatchID string `json:"batchId,omitempty"`
}

type MutationRef struct {
	Status string                   `json:"status"`
	Sites  []string                 `json:"sites,omitempty"`
	Reuse  *evidence.ReuseReference `json:"reuse,omitempty"`
}

type RowReport struct {
	RowID  string       `json:"rowId"`
	Level  string       `json:"level"`
	Risk   string       `json:"risk"`
	Layer1 LayerVerdict `json:"layer1"`
	Layer2 CriticRef    `json:"layer2"`
	Layer3 string       `json:"layer3"`
	Layer4 MutationRef  `json:"layer4"`
}

type BatchReport struct {
	BatchID  string                   `json:"batchId"`
	RunID    string                   `json:"runId"`
	Rows     []string                 `json:"rows"`
	Verdicts []CriticVerdict          `json:"verdicts"`
	Reuse    *evidence.ReuseReference `json:"reuse,omitempty"`
}

type TranscriptRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

const (
	WorkFresh          = "fresh"
	WorkReused         = "reused"
	StandingOK         = "ok"
	StandingFailed     = "failed"
	StandingIncomplete = "incomplete"
)

// WorkItem is one obligation the run answered: which record, where its
// answer came from, how it stands, and the published transcript that
// proves it.
type WorkItem struct {
	Kind     string                   `json:"kind"` // phase | review-group | live-attempt | mutation-row
	ID       string                   `json:"id"`
	Rows     []string                 `json:"rows"`     // review groups: the group's rows; mutation rows: [rowId]; else []
	Source   string                   `json:"source"`   // fresh | reused
	Outcome  string                   `json:"outcome"`  // the record's outcome
	Standing string                   `json:"standing"` // ok | failed | incomplete
	Artifact TranscriptRef            `json:"artifact"` // repo-relative published path and sha256
	Reuse    *evidence.ReuseReference `json:"reuse,omitempty"`
}

// MarshalJSON normalizes a nil Rows to an empty array before encoding so the
// field is never emitted as null.
func (w WorkItem) MarshalJSON() ([]byte, error) {
	type alias WorkItem
	a := alias(w)
	if a.Rows == nil {
		a.Rows = []string{}
	}
	return json.Marshal(a)
}

type WorkCounts struct {
	Fresh      int `json:"fresh"`
	Reused     int `json:"reused"`
	Failed     int `json:"failed"`
	Incomplete int `json:"incomplete"`
}

type WorkSummary struct {
	Items  []WorkItem `json:"items"`
	Counts WorkCounts `json:"counts"`
}

// MarshalJSON normalizes a nil Items to an empty array before encoding so
// the field is never emitted as null.
func (s WorkSummary) MarshalJSON() ([]byte, error) {
	type alias WorkSummary
	a := alias(s)
	if a.Items == nil {
		a.Items = []WorkItem{}
	}
	return json.Marshal(a)
}

// WorkCountsOf tallies items: Fresh/Reused by Source, Failed/Incomplete by
// Standing.
func WorkCountsOf(items []WorkItem) WorkCounts {
	var c WorkCounts
	for _, it := range items {
		switch it.Source {
		case WorkFresh:
			c.Fresh++
		case WorkReused:
			c.Reused++
		}
		switch it.Standing {
		case StandingFailed:
			c.Failed++
		case StandingIncomplete:
			c.Incomplete++
		}
	}
	return c
}

type Layer1Summary struct {
	Status        string         `json:"status"`
	Suite         SuiteCounts    `json:"suite"`
	FindingCounts map[string]int `json:"findingCounts"`
	NewFailures   int            `json:"newFailures"`
}

type Layer2Summary struct {
	Status     string `json:"status"`
	Applicable int    `json:"applicable"`
	Holds      int    `json:"holds"`
	Weak       int    `json:"weak"`
	Vacuous    int    `json:"vacuous"`
	Missing    int    `json:"missing"`
}

type Layer3Summary struct {
	Status       string `json:"status"`
	Applicable   int    `json:"applicable"`
	Pass         int    `json:"pass"`
	Fail         int    `json:"fail"`
	Inconclusive int    `json:"inconclusive"`
	NA           int    `json:"na"`
}

type Layer4Summary struct {
	Status      string `json:"status"`
	Applicable  int    `json:"applicable"`
	Caught      int    `json:"caught"`
	Survived    int    `json:"survived"`
	Unappliable int    `json:"unappliable"`
	NA          int    `json:"na"`
}

type LayerSummary struct {
	Layer1 Layer1Summary `json:"layer1"`
	Layer2 Layer2Summary `json:"layer2"`
	Layer3 Layer3Summary `json:"layer3"`
	Layer4 Layer4Summary `json:"layer4"`
}

type Report struct {
	evidence.Envelope
	RunID           string            `json:"runId"`
	Source          string            `json:"source"`
	SourceSHA256    string            `json:"sourceSha256"`
	HeadCommit      string            `json:"headCommit"`
	BaselineCommit  string            `json:"baselineCommit"`
	BaselineStatus  string            `json:"baselineStatus"`
	Approval        string            `json:"approval"`
	Verdict         string            `json:"verdict"`
	Acknowledged    bool              `json:"acknowledged"`
	Acknowledgement string            `json:"acknowledgement,omitempty"`
	Reasons         []Reason          `json:"reasons"`
	Layers          LayerSummary      `json:"layers"`
	Rows            []RowReport       `json:"rows"`
	Batches         []BatchReport     `json:"batches"`
	Work            WorkSummary       `json:"work"`
	Transcripts     []TranscriptRef   `json:"transcripts"`
	ToolVersions    map[string]string `json:"toolVersions"`
	FinishedAt      string            `json:"finishedAt"`
}
