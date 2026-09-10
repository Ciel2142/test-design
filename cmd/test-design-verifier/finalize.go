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

// outcomeUnavailable is the live-attempt outcome an attempt the environment
// refused is closed with; like environmental it leaves the obligation
// incomplete rather than failed.
const outcomeUnavailable = "unavailable"

// codeReusedArtifactChanged is a prior run's artifact whose bytes no longer
// digest to what the reuse reference recorded for them. The report package
// owns the other adopted-evidence refusal (reuse-identity-mismatch), which it
// also raises for itself once the report is computed.
const codeReusedArtifactChanged = "reused-artifact-changed"

// The three refusals finalize raises about the run itself: an envelope the
// parent may no longer send, a run with no trace to assemble a report from, and
// a run that still owes an obligation.
const (
	codeEnvelopeNotAccepted = "envelope-not-accepted"
	codeTraceIncomplete     = "trace-incomplete"
	codeRunIncomplete       = "run-incomplete"
	// codePublicationUnverifiable is an own published path whose bytes are not
	// what this run wrote; every other dirty path is source-not-reproducible.
	codePublicationUnverifiable = "publication-unverifiable"
	// codeLedgerChanged is the run's own ledger having moved between the read
	// the report was assembled from and the transaction that opens the
	// publication. The ledger is not in the repository, so the snapshot gate
	// cannot see it; this is that gate's counterpart for the run's own state.
	codeLedgerChanged = "ledger-changed"
)

// finalizeRequestBody is everything the parent still hands finalize: the
// approval line, its answer about the specification's freshness and, for a
// PARTIAL it accepts, the acknowledgement. Every layer of evidence comes from
// the run's own ledger.
type finalizeRequestBody struct {
	Approval        string           `json:"approval"`
	Freshness       report.Freshness `json:"freshness"`
	Acknowledgement string           `json:"acknowledgement,omitempty"`

	// The retired envelope fields. They are decoded as raw JSON so a document
	// of the previous protocol is refused by name rather than silently read as
	// a request that pasted nothing: the layers it carries are answers the
	// ledger already holds, and reading past them would publish a report over
	// evidence nothing verified.
	TraceEvidence string          `json:"traceEvidence,omitempty"`
	Layer2        json.RawMessage `json:"layer2,omitempty"`
	Layer3        json.RawMessage `json:"layer3,omitempty"`
	Layer4        json.RawMessage `json:"layer4,omitempty"`
}

type finalizeRequest struct {
	verifyRequest
	finalizeRequestBody
}

type finalizeOutput struct {
	evidence.Envelope
	Verdict        string            `json:"verdict"`
	Acknowledged   bool              `json:"acknowledged"`
	Reasons        []report.Reason   `json:"reasons"`
	JSONPath       string            `json:"jsonPath"`
	MarkdownPath   string            `json:"markdownPath"`
	TranscriptsDir string            `json:"transcriptsDir"`
	RunDir         string            `json:"runDir"`
	MeasuredCommit string            `json:"measuredCommit"`
	Recovered      []recoveredFile   `json:"recovered"`
	Work           report.WorkCounts `json:"work"`
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
	if cliErr := refuseRetiredEnvelope(op, &body); cliErr != nil {
		return nil, cliErr
	}
	return &finalizeRequest{verifyRequest: *base, finalizeRequestBody: body}, nil
}

// refuseRetiredEnvelope names the first field of the previous protocol the
// request still carries. It is answered while the document is being decoded,
// before finalize reaches the repository at all: a parent still pasting layers
// is describing a run from outside it, and nothing it names is looked at.
func refuseRetiredEnvelope(op string, body *finalizeRequestBody) *cliError {
	refuse := func(field, was string) *cliError {
		return &cliError{
			Code: codeEnvelopeNotAccepted,
			Message: fmt.Sprintf("%s: the request carries %s, which finalize no longer accepts: %s comes from the run's own ledger, recorded by the op that produced it",
				op, field, was),
		}
	}
	if body.TraceEvidence != "" {
		return refuse("traceEvidence", "the trace phase")
	}
	for _, retired := range []struct {
		field, was string
		raw        json.RawMessage
	}{
		{"layer2", "every review group", body.Layer2},
		{"layer3", "the live attempt", body.Layer3},
		{"layer4", "every mutation row", body.Layer4},
	} {
		if carriesJSON(retired.raw) {
			return refuse(retired.field, retired.was)
		}
	}
	return nil
}

// carriesJSON reports whether a raw field was present with anything in it; an
// absent field and an explicit null are both "the parent sent nothing".
func carriesJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

// finalizeOp is the finalize op body. between runs after the report is written
// into the run dir and before Publish; it is nil in production and exists so a
// test can change the repository at the moment the snapshot gate must catch it.
func finalizeOp(req *finalizeRequest, stdout io.Writer, between func()) *cliError {
	const op = "finalize"

	artifactDir, stem, cliErr := artifactStem(req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	src, cliErr := loadVerifiedSource(op, req.RepoRoot, req.CoverageSource)
	if cliErr != nil {
		return cliErr
	}

	head, cliErr := headCommit(op, req.RepoRoot)
	if cliErr != nil {
		return cliErr
	}

	// A run that never opened a ledger is told so from what is on disk:
	// finalize creates no ledger directory for it, and there is no trace to
	// assemble a report from.
	ledgerDir, cliErr := locateLedgerDir(op, &req.verifyRequest)
	if cliErr != nil {
		return cliErr
	}
	store, cliErr := openExistingLedger(op, ledgerDir)
	if cliErr != nil {
		return cliErr
	}
	var ledger *runstate.Ledger
	if store != nil {
		if ledger, cliErr = loadLedgerFor(op, runstate.KindPhase, store); cliErr != nil {
			return cliErr
		}
	}
	if ledger == nil {
		return &cliError{
			Code:    codeTraceIncomplete,
			Message: fmt.Sprintf("%s: run %s has no ledger: trace the run before finalizing it", op, req.RunID),
		}
	}

	if req.Approval == "" {
		return &cliError{
			Code:    "missing-approval",
			Message: op + ": approval is required; record the approval line from the coverage gate",
		}
	}
	// Everything the request itself can be refused for is refused here, before
	// the repository is touched. report.Compute would raise these for itself,
	// but only at the end — after recovery had already taken back the previous
	// publication for a call that was never going to produce a report to put in
	// its place, leaving the repository without the publication its own ledger
	// still describes as completed.
	if cliErr := requestFieldRefusal(op, &req.finalizeRequestBody); cliErr != nil {
		return cliErr
	}

	// The revision the report measures is the one the run was opened against.
	// Committing what a previous finalize published is not a change to the
	// thing the report describes, so the measured revision survives it; a
	// repository that moved for any other reason is a different subject,
	// whatever the ledger still holds about the old one.
	opened := ledger.Identity.RepoCommit
	measured, offending, err := measuredCommit(req.RepoRoot, artifactDir, stem, req.RunID, opened, head)
	if err != nil {
		return &cliError{
			Code: "head-moved",
			Message: fmt.Sprintf("%s: HEAD is %s and the run was opened at %s, but what changed between them could not be read: %v",
				op, head, opened, err),
		}
	}
	if offending != "" {
		return &cliError{
			Code: "head-moved",
			Message: fmt.Sprintf("%s: HEAD is %s but the run was opened at %s and %s changed between them; a verification describes one commit, so start a new run at HEAD, reusing this approval when the scope is unchanged",
				op, head, opened, offending),
		}
	}
	identity := ledgerIdentity(&req.verifyRequest, req.RepoRoot, measured, src)

	// resume is the accounting authority: what it says the run still owes is
	// what finalize refuses to publish over.
	state := buildResume(&req.verifyRequest, src, head, store.Dir(), ledger)
	if cliErr := pendingRefusal(op, state.Pending); cliErr != nil {
		return cliErr
	}

	// An earlier publication of this run is taken back before the snapshot
	// gate sees it: exactly the files that publication recorded, proved by
	// digest, and nothing else in the tree.
	recovered, cliErr := recoverOwnPublication(op, req.RepoRoot, runstate.Current(ledger, runstate.KindPhase, publishPhaseID))
	if cliErr != nil {
		return cliErr
	}

	ctrl, err := worktree.NewController(req.RepoRoot)
	if err != nil {
		return &cliError{Code: "controller-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	before, err := ctrl.TakeSnapshot()
	if err != nil {
		return &cliError{Code: "snapshot-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	if !before.Clean() {
		return &cliError{
			Code:    "source-not-reproducible",
			Message: fmt.Sprintf("%s: source not reproducible: %s", op, describeDirty(req.RepoRoot)),
		}
	}

	run, cliErr := assembleRun(op, req, artifactDir, ledger, measured)
	if cliErr != nil {
		return cliErr
	}

	in := &report.Inputs{
		Source:          src,
		RunID:           req.RunID,
		HeadCommit:      measured,
		Trace:           run.trace,
		Freshness:       req.Freshness,
		Batches:         run.batches,
		Live:            run.live,
		LiveAttempt:     run.liveAttempt,
		Mutations:       run.mutations,
		Acknowledgement: req.Acknowledgement,
	}

	rep, err := report.Compute(in)
	if err != nil {
		var refusal *report.RefusalError
		if errors.As(err, &refusal) {
			return &cliError{Code: refusal.Code, Message: fmt.Sprintf("%s: %s", op, refusal.Detail)}
		}
		return &cliError{Code: "compute-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	// The report names every transcript by the digest of the bytes that will
	// be published: this run's own are hashed here, an adopted one is named by
	// the digest its reuse reference recorded, which the staged copy is held
	// to before anything is published.
	digests := make(map[string]string, len(run.transcripts)+len(run.copies))
	rep.Transcripts = make([]report.TranscriptRef, 0, len(run.transcripts)+len(run.copies))
	for _, tf := range run.transcripts {
		sum, err := evidence.SHA256File(tf.SourcePath)
		if err != nil {
			return &cliError{Code: "evidence-unreadable", Message: fmt.Sprintf("%s: %v", op, err)}
		}
		digests[tf.RepoRelPath] = sum
		rep.Transcripts = append(rep.Transcripts, report.TranscriptRef{Path: tf.RepoRelPath, SHA256: sum})
	}
	for _, ref := range run.copies {
		digests[ref.RepoRelPath] = ref.SHA256
		rep.Transcripts = append(rep.Transcripts, report.TranscriptRef{Path: ref.RepoRelPath, SHA256: ref.SHA256})
	}
	sort.Slice(rep.Transcripts, func(i, j int) bool { return rep.Transcripts[i].Path < rep.Transcripts[j].Path })

	// Each work item is proved by the transcript published for its record, so
	// the summary names the same bytes the transcripts section does.
	for i := range run.work {
		run.work[i].Artifact.SHA256 = digests[run.work[i].Artifact.Path]
	}
	rep.Work = report.WorkSummary{Items: run.work, Counts: report.WorkCountsOf(run.work)}
	rep.Approval = req.Approval
	rep.ToolVersions = map[string]string{"go": runtime.Version()}
	rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)

	// run dir keyed by the clock, not the run ID: NewRunDir refuses a name it
	// already allocated, and finalize must stay rerunnable over the same inputs.
	runDir, err := ctrl.NewRunDir(fmt.Sprintf("%s-finalize-%d", req.RunID, time.Now().UnixNano()))
	if err != nil {
		return &cliError{Code: "run-dir-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	jsonPath := filepath.Join(runDir, stem+"-verification.json")
	if err := evidence.WriteJSON(jsonPath, rep); err != nil {
		return &cliError{Code: "report-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	markdown, err := report.RenderMarkdown(rep)
	if err != nil {
		return &cliError{Code: "render-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	markdownPath := filepath.Join(runDir, stem+"-verification.md")
	if err := os.WriteFile(markdownPath, markdown, 0o644); err != nil {
		return &cliError{Code: "report-write-failed", Message: fmt.Sprintf("%s: %v", op, err)}
	}

	jsonRel := repoPath(artifactDir, stem+"-verification.json")
	markdownRel := repoPath(artifactDir, stem+"-verification.md")
	files := append([]worktree.PublishFile{
		{RepoRelPath: jsonRel, SourcePath: jsonPath},
		{RepoRelPath: markdownRel, SourcePath: markdownPath},
	}, run.transcripts...)

	// The publication is recorded before it starts, naming every file it will
	// write with the digest of the bytes it will write there. A publication
	// that fails part way leaves this record started, which is what tells the
	// rerun which files are its own to take back.
	reportSHA, err := evidence.SHA256File(jsonPath)
	if err != nil {
		return &cliError{Code: "evidence-unreadable", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	markdownSHA, err := evidence.SHA256File(markdownPath)
	if err != nil {
		return &cliError{Code: "evidence-unreadable", Message: fmt.Sprintf("%s: %v", op, err)}
	}
	published := make([]evidence.FileRef, 0, 2+len(rep.Transcripts))
	published = append(published,
		evidence.FileRef{Path: jsonRel, SHA256: reportSHA},
		evidence.FileRef{Path: markdownRel, SHA256: markdownSHA})
	for _, ref := range rep.Transcripts {
		published = append(published, evidence.FileRef{Path: ref.Path, SHA256: ref.SHA256})
	}
	sort.Slice(published, func(i, j int) bool { return published[i].Path < published[j].Path })
	if cliErr := startPublication(op, store, identity, fingerprintLedger(ledger), publishInputs{
		MeasuredCommit: measured,
		RunDir:         runDir,
		ReportSHA256:   reportSHA,
		Acknowledged:   req.Acknowledgement != "",
		Files:          published,
	}); cliErr != nil {
		return cliErr
	}

	if between != nil {
		between()
	}
	// Everything between the ledger read and this line is time in which a
	// prior run's artifact could have changed under the report that quotes it.
	// Each adopted artifact is copied into this run's directory, the copy is
	// held to the reference digest, and the copy is what is published: the
	// bytes verified are the bytes that land, and a change to the original
	// after this point reaches nothing.
	staged, cliErr := stageReusedArtifacts(op, runDir, req.RunID, run.copies)
	if cliErr != nil {
		return cliErr
	}
	files = append(files, staged...)
	if _, err := ctrl.Publish(before, files); err != nil {
		code := "publish-failed"
		if errors.Is(err, worktree.ErrSnapshotChanged) {
			code = "snapshot-mismatch"
		}
		// The publish record stays started: what Publish wrote is a partially
		// written report set, and the rerun that repairs the destination is
		// what completes it.
		return &cliError{
			Code: code,
			Message: fmt.Sprintf("%s: %v; publication incomplete: repair the destination and rerun finalize; the report stays in %s",
				op, err, runDir),
		}
	}
	if cliErr := completePublication(op, store, identity); cliErr != nil {
		return cliErr
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
		MeasuredCommit: measured,
		Recovered:      recovered,
		Work:           rep.Work.Counts,
	}
	return writeStdout(stdout, out)
}

// requestFieldRefusal answers, from the request alone, every refusal
// report.Compute raises about the three fields the parent still sends.
// `approval` is finalize's own gate above and `acknowledgement` is free text
// the report only quotes, so freshness is the whole of it. The codes and the
// wording mirror internal/report/validate.go's validateFreshness exactly: a
// caller must see the same refusal whether it is decided here or there, and
// deciding it here is what keeps a request that can never produce a report from
// reaching recovery.
func requestFieldRefusal(op string, body *finalizeRequestBody) *cliError {
	refuse := func(code, format string, args ...any) *cliError {
		return &cliError{Code: code, Message: fmt.Sprintf("%s: %s", op, fmt.Sprintf(format, args...))}
	}
	switch f := body.Freshness; f.Result {
	case "":
		return refuse(report.CodeMissingFreshness, "freshness result is empty")
	case "fresh":
		return nil
	case "stale":
		if len(f.Stale) == 0 {
			return refuse(report.CodeInvalidFreshness, "freshness result is stale with no stale findings")
		}
		return nil
	default:
		return refuse(report.CodeInvalidFreshness, "freshness result %q outside fresh|stale", f.Result)
	}
}

// pendingRefusal turns resume's pending list into finalize's refusal. The two
// ops read one ledger with one set of rules, so what resume says a run still
// owes is exactly what finalize will not publish over; a run whose trace phase
// is not closed at all is told so by its own code, because there is nothing
// there to assemble a report from.
func pendingRefusal(op string, pending []resumePending) *cliError {
	entries := make([]string, 0, len(pending))
	code := codeRunIncomplete
	for _, owed := range pending {
		// The publication is the very thing this call is about to redo.
		if owed.Kind == runstate.KindPhase && owed.ID == publishPhaseID {
			continue
		}
		if owed.Kind == runstate.KindPhase && owed.ID == tracePhaseID &&
			(owed.Reason == reasonNeverStarted || owed.Reason == reasonNotCompleted) {
			code = codeTraceIncomplete
		}
		entries = append(entries, fmt.Sprintf("%s %s: %s", owed.Kind, owed.ID, owed.Reason))
	}
	if len(entries) == 0 {
		return nil
	}
	return &cliError{
		Code: code,
		Message: fmt.Sprintf("%s: the run still owes %s; answer it and rerun finalize. Nothing was published",
			op, strings.Join(entries, "; ")),
	}
}

// assembledRun is everything finalize reads from the ledger for one run: the
// report inputs, the documents it publishes as transcripts, and one work item
// per record. A fresh record and an adopted one travel the same path; the only
// difference is where the record's document lives and the name it is published
// under.
type assembledRun struct {
	trace       *report.TraceEvidence
	batches     []report.Batch
	live        *report.LiveEvidence
	liveAttempt *report.LiveAttempt
	mutations   []report.MutationInput
	transcripts []worktree.PublishFile
	copies      []transcriptCopy
	work        []report.WorkItem
}

// publish records the document one record answers with. This run's own are
// published from where the op wrote them; an adopted one is staged into the run
// directory and verified there before anything lands.
func (r *assembledRun) publish(adopted bool, sourcePath, repoRelPath, sha string) {
	if adopted {
		r.copies = append(r.copies, transcriptCopy{SourcePath: sourcePath, RepoRelPath: repoRelPath, SHA256: sha})
		return
	}
	r.transcripts = append(r.transcripts, worktree.PublishFile{RepoRelPath: repoRelPath, SourcePath: sourcePath})
}

// assembleRun reads the run's current completed records — the trace phase,
// every review group, the live attempt and every mutation row — verifies each
// record's artifacts against what the ledger digested, loads the documents
// through the report loaders and returns the report inputs, the transcripts to
// publish and the work items. It is called only once pendingRefusal has found
// nothing owed, so every obligation the run took on is answered here.
func assembleRun(
	op string, req *finalizeRequest, artifactDir string, ledger *runstate.Ledger, measured string,
) (*assembledRun, *cliError) {
	run := &assembledRun{work: []report.WorkItem{}}
	transcript := func(name string) string {
		return repoPath(artifactDir, "transcripts", req.RunID, name)
	}

	for i := range ledger.Records {
		rec := &ledger.Records[i]
		if rec.Status != runstate.StatusCompleted {
			continue
		}
		// The publication record is bookkeeping about the report, not an
		// obligation the report speaks for.
		if rec.Kind == runstate.KindPhase && rec.ID != tracePhaseID {
			continue
		}
		if reasons := runstate.Verify(rec); len(reasons) > 0 {
			return nil, &cliError{
				Code:    "ledger-invalid",
				Message: fmt.Sprintf("%s: %s %s: %s", op, rec.Kind, rec.ID, strings.Join(reasons, ", ")),
			}
		}
		adopted, err := recordCarriesReuse(rec)
		if err != nil {
			return nil, unreadableRecordError(op, rec, err)
		}
		item := report.WorkItem{
			Kind: string(rec.Kind), ID: rec.ID, Rows: []string{},
			Source: report.WorkFresh, Outcome: rec.Outcome, Standing: recordStanding(rec),
		}
		if adopted {
			item.Source = report.WorkReused
		}

		var cliErr *cliError
		switch rec.Kind {
		case runstate.KindPhase:
			cliErr = run.addTrace(op, rec, transcript, &item)
		case runstate.KindReviewGroup:
			cliErr = run.addReviewGroup(op, req.RunID, rec, transcript, &item)
		case runstate.KindLiveAttempt:
			cliErr = run.addLiveAttempt(op, rec, transcript, &item)
		case runstate.KindMutationRow:
			cliErr = run.addMutationRow(op, rec, transcript, &item)
		default:
			// A kind this op knows nothing about is not evidence it may quote.
			continue
		}
		if cliErr != nil {
			return nil, cliErr
		}
		run.work = append(run.work, item)
	}

	if run.trace == nil {
		return nil, &cliError{
			Code:    codeTraceIncomplete,
			Message: fmt.Sprintf("%s: run %s has no completed trace phase to report on", op, req.RunID),
		}
	}
	return run, nil
}

// recordStanding is how an obligation stands in the work summary: an outcome
// the environment decided leaves it incomplete rather than failed, because
// nothing was measured either way.
func recordStanding(rec *runstate.Record) string {
	switch rec.Outcome {
	case outcomeEnvironmental, outcomeUnavailable:
		return report.StandingIncomplete
	}
	if runstate.IsFailedOutcome(rec.Kind, rec.Outcome) {
		return report.StandingFailed
	}
	return report.StandingOK
}

func (r *assembledRun) addTrace(
	op string, rec *runstate.Record, transcript func(string) string, item *report.WorkItem,
) *cliError {
	path := rec.Artifacts[artifactEvidence].Path
	if path == "" {
		return missingArtifactError(op, rec, artifactEvidence)
	}
	tr, err := report.LoadTrace(path)
	if err != nil {
		return evidenceLoadError(err)
	}
	r.trace = tr
	item.Artifact.Path = transcript(traceEvidenceFile)
	r.publish(false, path, item.Artifact.Path, "")
	return nil
}

// addReviewGroup admits one group's verdicts as a batch. A fresh group is this
// run's own work and keeps this run's id; an adopted one keeps the run it was
// judged in, because that is whose answer it is.
func (r *assembledRun) addReviewGroup(
	op, runID string, rec *runstate.Record, transcript func(string) string, item *report.WorkItem,
) *cliError {
	var inputs reviewGroupInputs
	if err := recordInputs(rec, &inputs); err != nil {
		return unreadableRecordError(op, rec, err)
	}
	item.Rows = append(item.Rows, inputs.Rows...)

	batch := report.Batch{BatchID: rec.ID, RunID: runID, Rows: inputs.Rows}
	path, sha, name := rec.Artifacts[artifactEvidence].Path, rec.Artifacts[artifactEvidence].SHA256, "review-"+rec.ID+".json"
	ref := inputs.Reuse
	if ref != nil {
		path, sha, name = ref.Artifact.Path, ref.Artifact.SHA256, "reused-review-"+rec.ID+".json"
		batch.BatchID, batch.RunID, batch.Reuse = "reuse:"+rec.ID, ref.PriorRunID, ref
		item.Reuse = ref
	}
	if path == "" {
		return missingArtifactError(op, rec, artifactEvidence)
	}
	art, err := report.LoadReviewArtifact(path, sha)
	if err != nil {
		return artifactLoadError(op, path, ref != nil, err)
	}
	if ref != nil {
		if cliErr := checkReusedIdentity(op, rec.ID, ref, art.RunID, art.RepoCommit, art.SourceSHA256); cliErr != nil {
			return cliErr
		}
	} else if cliErr := checkFreshReviewIdentity(op, runID, rec.ID, inputs.Rows, art); cliErr != nil {
		return cliErr
	}
	batch.Verdicts = art.Verdicts
	r.batches = append(r.batches, batch)
	item.Artifact.Path = transcript(name)
	r.publish(ref != nil, path, item.Artifact.Path, sha)
	return nil
}

// addLiveAttempt reads the run's layer-3 answer from the record: the journey's
// evidence when there was one, the attempt document when the environment
// refused before there could be.
func (r *assembledRun) addLiveAttempt(
	op string, rec *runstate.Record, transcript func(string) string, item *report.WorkItem,
) *cliError {
	if path := rec.Artifacts[artifactEvidence].Path; path != "" {
		live, err := report.LoadLive(path)
		if err != nil {
			return evidenceLoadError(err)
		}
		r.live = live
		item.Artifact.Path = transcript(liveEvidenceFile)
		r.publish(false, path, item.Artifact.Path, "")
		return nil
	}
	path := rec.Artifacts[artifactAttempt].Path
	if path == "" {
		return missingArtifactError(op, rec, artifactAttempt)
	}
	attempt, err := report.LoadLiveAttempt(path)
	if err != nil {
		return evidenceLoadError(err)
	}
	r.liveAttempt = attempt
	item.Artifact.Path = transcript(liveAttemptEvidenceFile)
	r.publish(false, path, item.Artifact.Path, "")
	return nil
}

// addMutationRow reads one row's layer-4 answer. The record's outcome decides
// which document its artifact is: mutation evidence for a row that ran, the
// unappliable document for one that could not be applied, the attempt document
// for one the environment refused.
func (r *assembledRun) addMutationRow(
	op string, rec *runstate.Record, transcript func(string) string, item *report.WorkItem,
) *cliError {
	item.Rows = append(item.Rows, rec.ID)

	var adopted reusedMutationInputs
	if err := recordInputs(rec, &adopted); err != nil {
		return unreadableRecordError(op, rec, err)
	}
	if ref := adopted.Reuse; ref != nil {
		entry, copy, cliErr := reusedMutationEntry(op, rec, ref, transcript)
		if cliErr != nil {
			return cliErr
		}
		r.mutations = append(r.mutations, entry)
		r.copies = append(r.copies, copy)
		item.Artifact.Path, item.Reuse = copy.RepoRelPath, ref
		return nil
	}

	entry := report.MutationInput{RowID: rec.ID}
	role, name := artifactEvidence, "mutation-"+rec.ID+".json"
	if rec.Outcome == outcomeEnvironmental {
		role, name = artifactAttempt, "mutation-attempt-"+rec.ID+".json"
	} else if rec.Outcome == outcomeUnappliable {
		name = "mutation-unappliable-" + rec.ID + ".json"
	}
	path := rec.Artifacts[role].Path
	if path == "" {
		return missingArtifactError(op, rec, role)
	}

	switch rec.Outcome {
	case outcomeUnappliable:
		doc, err := loadUnappliableEvidence(path)
		if err != nil {
			return evidenceLoadError(err)
		}
		entry.Outcome, entry.Reason = outcomeUnappliable, doc.Reason
	case outcomeEnvironmental:
		attempt, err := report.LoadAttempt(path)
		if err != nil {
			return evidenceLoadError(err)
		}
		entry.Outcome, entry.AttemptEvidencePath, entry.Attempt = outcomeEnvironmental, path, attempt
		entry.Reason = attempt.Refusal
	default:
		ev, err := report.LoadMutation(path)
		if err != nil {
			return evidenceLoadError(err)
		}
		entry.EvidencePath, entry.Evidence = path, ev
	}
	r.mutations = append(r.mutations, entry)
	item.Artifact.Path = transcript(name)
	r.publish(false, path, item.Artifact.Path, "")
	return nil
}

// missingArtifactError refuses a completed record that points at no document:
// every completed record of the four kinds names the artifact its outcome
// rests on, so one that does not is a checkpoint nothing can be read from.
func missingArtifactError(op string, rec *runstate.Record, role string) *cliError {
	return &cliError{
		Code:    "ledger-invalid",
		Message: fmt.Sprintf("%s: %s %s completed %q with no %s artifact", op, rec.Kind, rec.ID, rec.Outcome, role),
	}
}

// artifactLoadError names what went wrong reading a record's document: an
// adopted one speaks the reuse vocabulary, this run's own the evidence one.
func artifactLoadError(op, path string, adopted bool, err error) *cliError {
	if adopted {
		return reusedArtifactError(op, path, err)
	}
	return evidenceLoadError(err)
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

// checkFreshReviewIdentity holds a fresh group's document to the record that
// points at it. The digest proves only that the bytes are the ones the record
// was completed over — a record whose evidence path and digest were both
// rewritten at a prior run's artifact passes it, and the batch is stamped with
// this run's id before the verdicts are read. So the document is asked whose
// answer it is: this run, this group, these rows. An adopted record answers the
// same three questions against its reuse reference (checkReusedIdentity); a
// fresh one has this run to answer to.
func checkFreshReviewIdentity(op, runID, groupID string, rows []string, art *report.ReviewArtifact) *cliError {
	refuse := func(format string, args ...any) *cliError {
		return &cliError{
			Code: "ledger-invalid",
			Message: fmt.Sprintf("%s: %s %s: %s", op, runstate.KindReviewGroup, groupID,
				fmt.Sprintf(format, args...)),
		}
	}
	switch {
	case art.RunID != runID:
		return refuse("its evidence answers for run %q, not this run %q", art.RunID, runID)
	case art.GroupID != groupID:
		return refuse("its evidence answers for group %q", art.GroupID)
	}
	// The rows are a set: the record fixes which rows the group covers, the
	// document says which it answered, and the order either lists them in is
	// nobody's contract.
	sorted := func(in []string) []string {
		out := slices.Clone(in)
		slices.Sort(out)
		return out
	}
	if bound, answered := sorted(rows), sorted(art.Rows); !slices.Equal(bound, answered) {
		return refuse("the record binds rows %v and its evidence answers rows %v", bound, answered)
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
