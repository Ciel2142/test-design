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
	"slices"
	"sort"
	"strings"
	"time"

	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/runstate"
	"test-design/internal/worktree"
)

const coverageSourceSuffix = "-coverage.md"

// outcomeEnvironmental is the mutation-row outcome record-attempt closes a row
// with when the environment, not the mutant, refused the measurement.
const outcomeEnvironmental = "environmental"

// The two refusals adopted evidence adds to finalize's vocabulary. The report
// package owns the other two (reuse-not-admitted, reuse-identity-mismatch),
// which it also raises for itself once the report is computed.
const (
	// codeReusedArtifactChanged is a prior run's artifact whose bytes no
	// longer digest to what the reuse reference recorded for them.
	codeReusedArtifactChanged = "reused-artifact-changed"
	// codeReusedRowRepasted is a row this run's ledger already answers for
	// that the parent pasted an answer for as well.
	codeReusedRowRepasted = "reused-row-repasted"
)

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

	reusedBatches, reusedRows, reusedCopies, cliErr := reusedInputs("finalize", req)
	if cliErr != nil {
		return cliErr
	}

	in := &report.Inputs{
		Source:          src,
		RunID:           req.RunID,
		HeadCommit:      head,
		Trace:           tr,
		Freshness:       req.Freshness,
		Batches:         slices.Concat(req.Layer2.Batches, reusedBatches),
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
			attempt, err := report.LoadAttempt(m.AttemptEvidencePath)
			if err != nil {
				return evidenceLoadError(err)
			}
			m.Attempt = attempt
			transcripts = append(transcripts, worktree.PublishFile{
				RepoRelPath: repoPath(artifactDir, "transcripts", req.RunID, "mutation-attempt-"+m.RowID+".json"),
				SourcePath:  m.AttemptEvidencePath,
			})
		}
	}

	// The adopted entries arrive already loaded and already hashed, so they
	// join the parent's after the loading loop rather than passing through it
	// a second time under this run's transcript names. Their transcripts are
	// not published from where the prior run left them: they are staged into
	// this run's directory and verified there, once the report is written.
	in.Mutations = slices.Concat(in.Mutations, reusedRows)

	rep, err := report.Compute(in)
	if err != nil {
		var refusal *report.RefusalError
		if errors.As(err, &refusal) {
			return &cliError{Code: refusal.Code, Message: fmt.Sprintf("finalize: %s", refusal.Detail)}
		}
		return &cliError{Code: "compute-failed", Message: fmt.Sprintf("finalize: %v", err)}
	}

	// The report names every transcript by the digest of the bytes that will
	// be published: this run's own are hashed here, an adopted one is named by
	// the digest its reuse reference recorded, which the staged copy is held
	// to before anything is published.
	rep.Transcripts = make([]report.TranscriptRef, 0, len(transcripts)+len(reusedCopies))
	for _, tf := range transcripts {
		sum, err := evidence.SHA256File(tf.SourcePath)
		if err != nil {
			return &cliError{Code: "evidence-unreadable", Message: fmt.Sprintf("finalize: %v", err)}
		}
		rep.Transcripts = append(rep.Transcripts, report.TranscriptRef{Path: tf.RepoRelPath, SHA256: sum})
	}
	for _, ref := range reusedCopies {
		rep.Transcripts = append(rep.Transcripts, report.TranscriptRef{Path: ref.RepoRelPath, SHA256: ref.SHA256})
	}
	sort.Slice(rep.Transcripts, func(i, j int) bool { return rep.Transcripts[i].Path < rep.Transcripts[j].Path })
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
	// Everything between the ledger read and this line is time in which a
	// prior run's artifact could have changed under the report that quotes it.
	// Each adopted artifact is copied into this run's directory, the copy is
	// held to the reference digest, and the copy is what is published: the
	// bytes verified are the bytes that land, and a change to the original
	// after this point reaches nothing.
	staged, cliErr := stageReusedArtifacts("finalize", runDir, req.RunID, reusedCopies)
	if cliErr != nil {
		return cliErr
	}
	files = append(files, staged...)
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

// transcriptCopy is one adopted record's artifact: the original file, still
// where the prior run wrote it, the copy of it this run publishes, and the
// digest the reuse reference recorded for those bytes.
type transcriptCopy struct{ SourcePath, RepoRelPath, SHA256 string }

// stageReusedArtifacts copies every adopted artifact into
// <runDir>/transcripts/<runID>/ under the name it is published as, and holds
// each copy to the digest its reference recorded. What comes back publishes
// from the copies. A copy that does not digest to the reference is the
// original having changed since the ledger was read, and nothing is published.
func stageReusedArtifacts(op, runDir, runID string, copies []transcriptCopy) ([]worktree.PublishFile, *cliError) {
	if len(copies) == 0 {
		return nil, nil
	}
	dir := filepath.Join(runDir, "transcripts", runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	staged := make([]worktree.PublishFile, 0, len(copies))
	for _, ref := range copies {
		data, err := os.ReadFile(ref.SourcePath)
		if err != nil {
			return nil, reusedArtifactError(op, ref.SourcePath, fmt.Errorf("%s: %w: %v", ref.SourcePath, report.ErrEvidenceUnreadable, err))
		}
		target := filepath.Join(dir, filepath.Base(filepath.FromSlash(ref.RepoRelPath)))
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return nil, &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("%s: stage %s: %v", op, ref.RepoRelPath, err)}
		}
		if err := report.VerifyEvidenceDigest(target, ref.SHA256); err != nil {
			return nil, reusedArtifactError(op, ref.SourcePath, err)
		}
		staged = append(staged, worktree.PublishFile{RepoRelPath: ref.RepoRelPath, SourcePath: target})
	}
	return staged, nil
}

// reusedInputs collects the run's adopted records — the completed review-group
// and mutation-row records of this run's own ledger whose inputs carry a reuse
// reference — as report inputs. Each one's original artifact is re-hashed
// against its reference before anything inside it is believed, decoded, and
// checked to be the document the reference names; what comes back are the
// batches and layer-4 entries the adopting run answers with, plus the
// transcript copies it publishes. A run with no ledger has adopted nothing and
// yields nothing.
//
// It also draws the line around adoption from the other side: a reuse
// reference is written by the reuse op and read here, so a parent that pastes
// one has bypassed the staleness and canary rules a fresh answer is held to,
// and a parent that also pastes an answer for a row the ledger already answers
// for is describing one row from two sources. Both are refused before the
// report is computed.
func reusedInputs(op string, req *finalizeRequest) ([]report.Batch, []report.MutationInput, []transcriptCopy, *cliError) {
	if cliErr := refuseUnadmittedReuse(op, req); cliErr != nil {
		return nil, nil, nil, cliErr
	}
	artifactDir, _, cliErr := artifactStem(req.CoverageSource)
	if cliErr != nil {
		return nil, nil, nil, cliErr
	}
	// A run with no ledger adopted nothing, and is told so from what is on
	// disk: finalize creates no ledger directory for a run that never opened
	// one.
	dir, cliErr := locateLedgerDir(op, &req.verifyRequest)
	if cliErr != nil {
		return nil, nil, nil, cliErr
	}
	store, cliErr := openExistingLedger(op, dir)
	if cliErr != nil || store == nil {
		return nil, nil, nil, cliErr
	}
	ledger, cliErr := loadLedgerFor(op, runstate.KindReviewGroup, store)
	if cliErr != nil || ledger == nil {
		return nil, nil, nil, cliErr
	}

	transcript := func(name string) string {
		return repoPath(artifactDir, "transcripts", req.RunID, name)
	}

	var (
		batches  []report.Batch
		rows     []report.MutationInput
		copies   []transcriptCopy
		reviewed = map[string]bool{}
		mutated  = map[string]bool{}
	)
	// A record carrying no reuse reference is this run's own work, which the
	// parent pastes as it always has, and finalize reads nothing more of it;
	// only an adopted one is read here. An adopted record whose captured shape
	// cannot be decoded is neither: it is a checkpoint this run cannot read,
	// and reading it as "not adopted" would drop its rows out of what the
	// ledger is known to answer for.
	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Status != runstate.StatusCompleted {
			continue
		}
		carries, err := recordCarriesReuse(rec)
		if err != nil {
			return nil, nil, nil, unreadableRecordError(op, rec, err)
		}
		if !carries {
			continue
		}
		switch rec.Kind {
		case runstate.KindReviewGroup:
			var inputs reviewGroupInputs
			if err := recordInputs(rec, &inputs); err != nil {
				return nil, nil, nil, unreadableRecordError(op, rec, err)
			}
			if inputs.Reuse == nil {
				continue
			}
			ref := inputs.Reuse
			art, err := report.LoadReviewArtifact(ref.Artifact.Path, ref.Artifact.SHA256)
			if err != nil {
				return nil, nil, nil, reusedArtifactError(op, ref.Artifact.Path, err)
			}
			if cliErr := checkReusedIdentity(op, rec.ID, ref, art.RunID, art.RepoCommit, art.SourceSHA256); cliErr != nil {
				return nil, nil, nil, cliErr
			}
			// The batch keeps the run it was judged in; only the rows are
			// this run's, because they are what this run answers for.
			batches = append(batches, report.Batch{
				BatchID:  "reuse:" + rec.ID,
				RunID:    ref.PriorRunID,
				Rows:     inputs.Rows,
				Verdicts: art.Verdicts,
				Reuse:    ref,
			})
			copies = append(copies, transcriptCopy{
				SourcePath:  ref.Artifact.Path,
				RepoRelPath: transcript("reused-review-" + rec.ID + ".json"),
				SHA256:      ref.Artifact.SHA256,
			})
			for _, row := range inputs.Rows {
				reviewed[row] = true
			}
			// A verdict the artifact carries for a row outside the record's
			// own list is still an answer this run holds, and still not one
			// the parent may give again.
			for _, v := range art.Verdicts {
				reviewed[v.RowID] = true
			}

		case runstate.KindMutationRow:
			var inputs reusedMutationInputs
			if err := recordInputs(rec, &inputs); err != nil {
				return nil, nil, nil, unreadableRecordError(op, rec, err)
			}
			if inputs.Reuse == nil {
				continue
			}
			entry, artifact, cliErr := reusedMutationEntry(op, rec, inputs.Reuse, transcript)
			if cliErr != nil {
				return nil, nil, nil, cliErr
			}
			rows = append(rows, entry)
			copies = append(copies, artifact)
			mutated[rec.ID] = true
		}
	}

	for _, b := range req.Layer2.Batches {
		for _, row := range batchRows(b) {
			if reviewed[row] {
				return nil, nil, nil, &cliError{
					Code: codeReusedRowRepasted,
					Message: fmt.Sprintf("%s: batch %s names row %s, which an adopted review group already answers for",
						op, b.BatchID, row),
				}
			}
		}
	}
	for _, m := range req.Layer4.Rows {
		if mutated[m.RowID] {
			return nil, nil, nil, &cliError{
				Code: codeReusedRowRepasted,
				Message: fmt.Sprintf("%s: layer4 names row %s, which an adopted mutation row already answers for",
					op, m.RowID),
			}
		}
	}
	return batches, rows, copies, nil
}

// recordCarriesReuse reports whether a record's captured inputs name a reuse
// reference, reading nothing else of them: the one question finalize asks of
// a record before deciding whether the record is its business at all. Every
// op writes inputs this decode accepts, so a decode that fails is a reuse
// field nothing can read — an adopted record in all but name — and is
// reported rather than passed over as this run's own work.
func recordCarriesReuse(rec *runstate.Record) (bool, error) {
	var carried struct {
		Reuse *evidence.ReuseReference `json:"reuse"`
	}
	if err := recordInputs(rec, &carried); err != nil {
		return false, err
	}
	return carried.Reuse != nil, nil
}

// reusedMutationEntry is one adopted mutation row as the report reads it. The
// record's outcome decides which document its artifact is: mutation evidence
// for a row that ran, the unappliable document for one that could not be
// applied, the attempt document for one the environment refused.
func reusedMutationEntry(
	op string, rec *runstate.Record, ref *evidence.ReuseReference, transcript func(string) string,
) (report.MutationInput, transcriptCopy, *cliError) {
	if err := report.VerifyEvidenceDigest(ref.Artifact.Path, ref.Artifact.SHA256); err != nil {
		return report.MutationInput{}, transcriptCopy{}, reusedArtifactError(op, ref.Artifact.Path, err)
	}
	entry := report.MutationInput{RowID: rec.ID, Reuse: ref}

	var name, runID, commit, sourceSHA string
	switch rec.Outcome {
	case outcomeUnappliable:
		doc, err := loadUnappliableEvidence(ref.Artifact.Path)
		if err != nil {
			return report.MutationInput{}, transcriptCopy{}, reusedArtifactError(op, ref.Artifact.Path, err)
		}
		entry.Outcome, entry.Reason = outcomeUnappliable, doc.Reason
		name, runID, commit, sourceSHA = "reused-mutation-unappliable-", doc.RunID, doc.RepoCommit, doc.SourceSHA256
	case outcomeEnvironmental:
		attempt, err := report.LoadAttempt(ref.Artifact.Path)
		if err != nil {
			return report.MutationInput{}, transcriptCopy{}, reusedArtifactError(op, ref.Artifact.Path, err)
		}
		entry.Outcome, entry.AttemptEvidencePath, entry.Attempt = outcomeEnvironmental, ref.Artifact.Path, attempt
		name, runID, commit, sourceSHA = "reused-mutation-attempt-", attempt.RunID, attempt.RepoCommit, attempt.SourceSHA256
	default:
		ev, err := report.LoadMutation(ref.Artifact.Path)
		if err != nil {
			return report.MutationInput{}, transcriptCopy{}, reusedArtifactError(op, ref.Artifact.Path, err)
		}
		entry.EvidencePath, entry.Evidence = ref.Artifact.Path, ev
		name, runID, commit, sourceSHA = "reused-mutation-", ev.RunID, ev.RepoCommit, ev.SourceSHA256
	}
	if cliErr := checkReusedIdentity(op, rec.ID, ref, runID, commit, sourceSHA); cliErr != nil {
		return report.MutationInput{}, transcriptCopy{}, cliErr
	}
	return entry, transcriptCopy{
		SourcePath:  ref.Artifact.Path,
		RepoRelPath: transcript(name + rec.ID + ".json"),
		SHA256:      ref.Artifact.SHA256,
	}, nil
}

// refuseUnadmittedReuse rejects a reuse reference the parent wrote itself.
// Only the reuse op may adopt a prior record, and it does so through the
// ledger; a pasted reference would otherwise carry a batch past the staleness
// and canary bookkeeping every fresh batch answers to.
func refuseUnadmittedReuse(op string, req *finalizeRequest) *cliError {
	for _, b := range req.Layer2.Batches {
		if b.Reuse != nil {
			return &cliError{
				Code: report.CodeReuseNotAdmitted,
				Message: fmt.Sprintf("%s: batch %s carries a reuse reference; adopted evidence reaches a report through the run's ledger, not the request",
					op, b.BatchID),
			}
		}
	}
	for _, m := range req.Layer4.Rows {
		if m.Reuse != nil {
			return &cliError{
				Code: report.CodeReuseNotAdmitted,
				Message: fmt.Sprintf("%s: layer4 row %s carries a reuse reference; adopted evidence reaches a report through the run's ledger, not the request",
					op, m.RowID),
			}
		}
	}
	return nil
}

// checkReusedIdentity holds an adopted artifact to the run it says it came
// from. The digest already proves the bytes are the ones the reference was
// written against; this proves the reference and the document agree about
// whose bytes they are.
func checkReusedIdentity(op, id string, ref *evidence.ReuseReference, runID, commit, sourceSHA string) *cliError {
	if runID == ref.PriorRunID && commit == ref.PriorRepoCommit && sourceSHA == ref.PriorSourceSHA256 {
		return nil
	}
	return &cliError{
		Code: report.CodeReuseIdentityMismatch,
		Message: fmt.Sprintf("%s: adopted %s names run %q at %q over source %q, but its reuse reference names run %q at %q over source %q",
			op, id, runID, commit, sourceSHA, ref.PriorRunID, ref.PriorRepoCommit, ref.PriorSourceSHA256),
	}
}

// reusedArtifactError names what went wrong with a prior run's artifact: bytes
// that changed under the reference are their own refusal, anything else is the
// evidence vocabulary finalize already speaks.
func reusedArtifactError(op, path string, err error) *cliError {
	if errors.Is(err, report.ErrEvidenceDigestMismatch) {
		return &cliError{
			Code: codeReusedArtifactChanged,
			Message: fmt.Sprintf("%s: adopted artifact %s no longer digests to what its reuse reference recorded; nothing was published",
				op, path),
		}
	}
	return evidenceLoadError(err)
}

// loadUnappliableEvidence reads back the document record-mutation retained for
// a row whose mutation could not be applied; its reason is what the report
// quotes.
func loadUnappliableEvidence(path string) (*mutationUnappliableEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %v", path, report.ErrEvidenceUnreadable, err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("%s: %w: invalid JSON", path, report.ErrEvidenceUnreadable)
	}
	// The envelope is checked before the body is trusted, the way every other
	// evidence loader checks it.
	if _, err := evidence.DecodeEnvelope(data); err != nil {
		return nil, fmt.Errorf("%s: %w: %v", path, report.ErrEvidenceSchemaMismatch, err)
	}
	var doc mutationUnappliableEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w: %v", path, report.ErrEvidenceUnreadable, err)
	}
	return &doc, nil
}

// unreadableRecordError refuses an adopted record of this run whose captured
// input identity cannot be decoded. It is not "not adopted": the record names
// a reuse reference, so nothing can be told about what it answers for, and the
// checkpoint is reported as unreadable rather than passed over.
func unreadableRecordError(op string, rec *runstate.Record, err error) *cliError {
	return &cliError{
		Code:    "ledger-invalid",
		Message: fmt.Sprintf("%s: %s %s: %v", op, rec.Kind, rec.ID, err),
	}
}

// batchRows names every row a batch speaks for: the rows it dispatched and the
// rows it returned a verdict on.
func batchRows(b report.Batch) []string {
	rows := slices.Clone(b.Rows)
	for _, v := range b.Verdicts {
		if !slices.Contains(rows, v.RowID) {
			rows = append(rows, v.RowID)
		}
	}
	return rows
}

// artifactStem keeps coverage-doc names compatible and derives inline-spec
// artifacts from the complete Markdown basename in the same directory.
func artifactStem(coverageSource string) (dir, stem string, cliErr *cliError) {
	base := filepath.Base(filepath.FromSlash(coverageSource))
	stem, ok := strings.CutSuffix(base, coverageSourceSuffix)
	if !ok {
		stem, ok = strings.CutSuffix(base, ".md")
	}
	if !ok || stem == "" {
		return "", "", &cliError{
			Code:    "source-name-unconventional",
			Message: fmt.Sprintf("finalize: coverage source %q must have a nonempty Markdown basename", base),
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
