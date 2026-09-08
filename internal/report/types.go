// Package report defines the finalize op's evidence loaders, verdict
// computation, and canonical report document.
package report

import (
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
// carry those. Expectation has already passed the redaction barrier at the
// producer, so it is safe to surface verbatim in a finalize reason.
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

type MutationEvidence struct {
	evidence.Envelope
	RunID         string                `json:"runId"`
	RowID         string                `json:"rowID"`
	RepoCommit    string                `json:"repoCommit"`
	Source        string                `json:"source"`
	SourceSHA256  string                `json:"sourceSha256"`
	Sites         []MutationSiteOutcome `json:"sites"`
	SnapshotEqual bool                  `json:"snapshotEqual"`
}

type Freshness struct {
	Result string         `json:"result"`
	Stale  []StaleFinding `json:"stale"`
}

type StaleFinding struct {
	Hunk     string `json:"hunk"`
	Behavior string `json:"behavior"`
}

type Canary struct {
	RowID     string `json:"rowId"`
	Kind      string `json:"kind"`
	Weakening string `json:"weakening"`
	Caught    bool   `json:"caught"`
}

type CriticVerdict struct {
	RowID   string `json:"rowId"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

type Batch struct {
	BatchID                 string          `json:"batchId"`
	DispatchID              string          `json:"dispatchId"`
	RunID                   string          `json:"runId"`
	Rows                    []string        `json:"rows"`
	Canary                  *Canary         `json:"canary,omitempty"`
	InheritsFrom            string          `json:"inheritsFrom,omitempty"`
	CanaryUnavailableReason string          `json:"canaryUnavailableReason,omitempty"`
	Voided                  bool            `json:"voided,omitempty"`
	Verdicts                []CriticVerdict `json:"verdicts"`
}

type MutationInput struct {
	RowID               string            `json:"rowId"`
	EvidencePath        string            `json:"evidencePath,omitempty"`
	Outcome             string            `json:"outcome,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	AttemptEvidencePath string            `json:"attemptEvidencePath,omitempty"`
	Evidence            *MutationEvidence `json:"-"`
	Attempt             *Attempt          `json:"-"`
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
	Verdict    string `json:"verdict"`
	BatchID    string `json:"batchId,omitempty"`
	DispatchID string `json:"dispatchId,omitempty"`
}

type MutationRef struct {
	Status string   `json:"status"`
	Sites  []string `json:"sites,omitempty"`
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
	BatchID                 string          `json:"batchId"`
	DispatchID              string          `json:"dispatchId"`
	RunID                   string          `json:"runId"`
	Rows                    []string        `json:"rows"`
	Canary                  *Canary         `json:"canary,omitempty"`
	InheritsFrom            string          `json:"inheritsFrom,omitempty"`
	CanaryUnavailableReason string          `json:"canaryUnavailableReason,omitempty"`
	Voided                  bool            `json:"voided"`
	Verdicts                []CriticVerdict `json:"verdicts"`
}

type TranscriptRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
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
	Transcripts     []TranscriptRef   `json:"transcripts"`
	ToolVersions    map[string]string `json:"toolVersions"`
	FinishedAt      string            `json:"finishedAt"`
}
