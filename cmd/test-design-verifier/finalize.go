package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/worktree"
)

const coverageSourceSuffix = "-coverage.md"

type finalizeLayer2 struct {
	Batches []report.Batch `json:"batches"`
}

type finalizeLayer3 struct {
	EvidencePath        string `json:"evidencePath,omitempty"`
	AttemptEvidencePath string `json:"attemptEvidencePath,omitempty"`
}

type finalizeLayer4 struct {
	Rows []report.MutationInput `json:"rows"`
}

type finalizeRequestBody struct {
	Approval        string           `json:"approval"`
	TraceEvidence   string           `json:"traceEvidence"`
	Freshness       report.Freshness `json:"freshness"`
	Layer2          finalizeLayer2   `json:"layer2"`
	Layer3          *finalizeLayer3  `json:"layer3,omitempty"`
	Layer4          finalizeLayer4   `json:"layer4"`
	Acknowledgement string           `json:"acknowledgement,omitempty"`
}

type finalizeRequest struct {
	verifyRequest
	finalizeRequestBody
}

type finalizeOutput struct {
	evidence.Envelope
	Verdict        string          `json:"verdict"`
	Acknowledged   bool            `json:"acknowledged"`
	Reasons        []report.Reason `json:"reasons"`
	JSONPath       string          `json:"jsonPath"`
	MarkdownPath   string          `json:"markdownPath"`
	TranscriptsDir string          `json:"transcriptsDir"`
	RunDir         string          `json:"runDir"`
}

func newFinalizeFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	addRequestFlag(fs)
	return fs
}

func finalizeHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	req, cliErr := decodeFinalizeRequest("finalize", requestBody)
	if cliErr != nil {
		return cliErr
	}
	return finalizeOp(req, stdout, nil)
}

func decodeFinalizeRequest(op string, requestBody []byte) (*finalizeRequest, *cliError) {
	base, cliErr := decodeVerifyRequest(op, requestBody)
	if cliErr != nil {
		return nil, cliErr
	}
	if base.RunID == "" {
		return nil, &cliError{Code: "missing-run-id", Message: fmt.Sprintf("%s: runID is required", op)}
	}

	var body finalizeRequestBody
	if err := json.Unmarshal(requestBody, &body); err != nil {
		return nil, &cliError{Code: "malformed-request", Message: fmt.Sprintf("%s: decode request: %v", op, err)}
	}
	if body.TraceEvidence == "" {
		return nil, &cliError{Code: "missing-trace-evidence", Message: fmt.Sprintf("%s: traceEvidence is required", op)}
	}
	if body.Layer3 != nil && body.Layer3.EvidencePath != "" && body.Layer3.AttemptEvidencePath != "" {
		return nil, &cliError{
			Code:    "malformed-request",
			Message: fmt.Sprintf("%s: layer3 names both evidencePath and attemptEvidencePath", op),
		}
	}
	return &finalizeRequest{verifyRequest: *base, finalizeRequestBody: body}, nil
}

// finalizeOp is the finalize op body. between runs after the report is written
// into the run dir and before Publish; it is nil in production and exists so a
// test can change the repository at the moment the snapshot gate must catch it.
func finalizeOp(req *finalizeRequest, stdout io.Writer, between func()) *cliError {
	artifactDir, stem, cliErr := artifactStem(req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	src, cliErr := loadVerifiedSource("finalize", req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "source-not-reproducible",
			Message: fmt.Sprintf("finalize: source not reproducible: %s", describeDirty(req.RepoRoot)),
		}
	}

	if req.Approval == "" {
		return &cliError{
			Code:    "missing-approval",
			Message: "finalize: approval is required; record the approval line from the coverage gate",
		}
	}

	head, cliErr := headCommit("finalize", req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}

	tr, err := report.LoadTrace(req.TraceEvidence)
	if err != nil {
		return evidenceLoadError(err)
	}
	if tr.RepoCommit != head {
		return &cliError{
			Code: "head-moved",
			Message: fmt.Sprintf("finalize: HEAD is %s but the trace ran against %s; a verification describes one commit, so rerun trace at HEAD",
				head, tr.RepoCommit),
		}
	}

	transcripts := []worktree.PublishFile{{
		RepoRelPath: repoPath(artifactDir, "transcripts", req.RunID, traceEvidenceFile),
		SourcePath:  req.TraceEvidence,
	}}

	in := &report.Inputs{
		Source:          src,
		RunID:           req.RunID,
		HeadCommit:      head,
		Trace:           tr,
		Freshness:       req.Freshness,
		Batches:         req.Layer2.Batches,
		Mutations:       req.Layer4.Rows,
		Acknowledgement: req.Acknowledgement,
	}

	if l3 := req.Layer3; l3 != nil {
		switch {
		case l3.EvidencePath != "":
			live, err := report.LoadLive(l3.EvidencePath)
			if err != nil {
				return evidenceLoadError(err)
			}
			in.Live = live
			transcripts = append(transcripts, worktree.PublishFile{
				RepoRelPath: repoPath(artifactDir, "transcripts", req.RunID, liveEvidenceFile),
				SourcePath:  l3.EvidencePath,
			})
		case l3.AttemptEvidencePath != "":
			attempt, err := report.LoadLiveAttempt(l3.AttemptEvidencePath)
			if err != nil {
				return evidenceLoadError(err)
			}
			in.LiveAttempt = attempt
			transcripts = append(transcripts, worktree.PublishFile{
				RepoRelPath: repoPath(artifactDir, "transcripts", req.RunID, liveAttemptEvidenceFile),
				SourcePath:  l3.AttemptEvidencePath,
			})
		}
	}

	for i := range in.Mutations {
		m := &in.Mutations[i]
		if m.EvidencePath != "" {
			ev, err := report.LoadMutation(m.EvidencePath)
			if err != nil {
				return evidenceLoadError(err)
			}
			m.Evidence = ev
			transcripts = append(transcripts, worktree.PublishFile{
				RepoRelPath: repoPath(artifactDir, "transcripts", req.RunID, "mutation-"+m.RowID+".json"),
				SourcePath:  m.EvidencePath,
			})
		}
		if m.AttemptEvidencePath != "" {
			if _, err := report.LoadAttempt(m.AttemptEvidencePath); err != nil {
				return evidenceLoadError(err)
			}
			transcripts = append(transcripts, worktree.PublishFile{
				RepoRelPath: repoPath(artifactDir, "transcripts", req.RunID, "mutation-attempt-"+m.RowID+".json"),
				SourcePath:  m.AttemptEvidencePath,
			})
		}
	}

	rep, err := report.Compute(in)
	if err != nil {
		var refusal *report.RefusalError
		if errors.As(err, &refusal) {
			return &cliError{Code: refusal.Code, Message: fmt.Sprintf("finalize: %s", refusal.Detail)}
		}
		return &cliError{Code: "compute-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}

	sort.Slice(transcripts, func(i, j int) bool { return transcripts[i].RepoRelPath < transcripts[j].RepoRelPath })
	rep.Transcripts = make([]report.TranscriptRef, 0, len(transcripts))
	for _, tf := range transcripts {
		sum, err := evidence.SHA256File(tf.SourcePath)
		if err != nil {
			return &cliError{Code: "evidence-unreadable", Message: fmt.Sprintf("finalize: %v", err)}
		}
		rep.Transcripts = append(rep.Transcripts, report.TranscriptRef{Path: tf.RepoRelPath, SHA256: sum})
	}
	rep.Approval = req.Approval
	rep.ToolVersions = map[string]string{"go": runtime.Version()}
	rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)

	// run dir keyed by the clock, not the run ID: NewRunDir refuses a name it
	// already allocated, and finalize must stay rerunnable over the same inputs.
	runDir, err := ctrl.NewRunDir(fmt.Sprintf("%s-finalize-%d", req.RunID, time.Now().UnixNano()))
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}
	jsonPath := filepath.Join(runDir, stem+"-verification.json")
	if err := evidence.WriteJSON(jsonPath, rep); err != nil {
		return &cliError{Code: "report-write-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}
	markdown, err := report.RenderMarkdown(rep)
	if err != nil {
		return &cliError{Code: "render-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}
	markdownPath := filepath.Join(runDir, stem+"-verification.md")
	if err := os.WriteFile(markdownPath, markdown, 0o644); err != nil {
		return &cliError{Code: "report-write-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}

	jsonRel := repoPath(artifactDir, stem+"-verification.json")
	markdownRel := repoPath(artifactDir, stem+"-verification.md")
	files := append([]worktree.PublishFile{
		{RepoRelPath: jsonRel, SourcePath: jsonPath},
		{RepoRelPath: markdownRel, SourcePath: markdownPath},
	}, transcripts...)

	if between != nil {
		between()
	}
	if _, err := ctrl.Publish(before, files); err != nil {
		code := "publish-failed"
		// Publish reports the concurrent-change refusal by message alone.
		if strings.Contains(err.Error(), "repository changed since snapshot") {
			code = "snapshot-mismatch"
		}
		return &cliError{Code: code, Message: fmt.Sprintf("finalize: %v; the report stays in %s", err, runDir)}
	}

	out := finalizeOutput{
		Envelope:       rep.Envelope,
		Verdict:        rep.Verdict,
		Acknowledged:   rep.Acknowledged,
		Reasons:        rep.Reasons,
		JSONPath:       jsonRel,
		MarkdownPath:   markdownRel,
		TranscriptsDir: repoPath(artifactDir, "transcripts", req.RunID),
		RunDir:         runDir,
	}
	return writeStdout(stdout, out)
}

// artifactStem derives the verification artifact names from the coverage
// source name; without the convention there is no name to publish under.
func artifactStem(coverageSource string) (dir, stem string, cliErr *cliError) {
	base := filepath.Base(filepath.FromSlash(coverageSource))
	stem, ok := strings.CutSuffix(base, coverageSourceSuffix)
	if !ok || stem == "" {
		return "", "", &cliError{
			Code:    "source-name-unconventional",
			Message: fmt.Sprintf("finalize: coverage source %q does not end in %q, so no artifact stem can be derived", base, coverageSourceSuffix),
		}
	}
	dir = filepath.ToSlash(filepath.Dir(filepath.FromSlash(coverageSource)))
	if dir == "." {
		dir = ""
	}
	return dir, stem, nil
}

func repoPath(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "/")
}

func evidenceLoadError(err error) *cliError {
	var missing *report.FieldMissingError
	switch {
	case errors.As(err, &missing):
		return &cliError{Code: "evidence-field-missing", Message: fmt.Sprintf("finalize: %v", err)}
	case errors.Is(err, report.ErrEvidenceSchemaMismatch):
		return &cliError{Code: "evidence-schema-mismatch", Message: fmt.Sprintf("finalize: %v", err)}
	default:
		return &cliError{Code: "evidence-unreadable", Message: fmt.Sprintf("finalize: %v", err)}
	}
}
