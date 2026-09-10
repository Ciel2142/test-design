package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/evidence"
	"test-design/internal/report"
	"test-design/internal/runstate"
)

const finalizeApproval = "approved by tester at the coverage gate"

type finalizeRequestJSON struct {
	SchemaVersion  string `json:"schemaVersion"`
	RepoRoot       string `json:"repoRoot"`
	CoverageSource string `json:"coverageSource"`
	RunID          string `json:"runID"`
	finalizeRequestBody
}

// seededGroup is one review-group record the fixture writes into the run's
// ledger. An empty Outcome leaves the group started, which is how a proof
// asks what finalize does about an obligation the run opened and never closed.
type seededGroup struct {
	ID       string
	Rows     []string
	Verdicts []report.CriticVerdict
	Outcome  string
}

// seededMutation is one mutation-row record. The outcome decides which
// document the record points at: evidence for a row that ran, the unappliable
// document for one that could not be applied, the attempt for one the
// environment refused. An empty Outcome leaves the row started.
type seededMutation struct{ RowID, Outcome, EvidencePath, AttemptEvidencePath, Reason string }

// seededLive is the run's layer-3 record, absent when the fixture seeds none.
type seededLive struct{ Outcome, EvidencePath, AttemptEvidencePath string }

type finalizeFixture struct {
	t              *testing.T
	repo           string
	coverageSource string
	src            *coverage.Source
	head           string
	runID          string
	dir            string
	tracePath      string
	dispatchPath   string
	trace          *report.TraceEvidence
	body           finalizeRequestBody

	// identityCommit is the commit the ledger is opened at. It is HEAD unless a
	// proof moves it, which is how a run whose repository moved under it is
	// asked for a report without rewriting the subject's history.
	identityCommit string
	groups         []seededGroup
	mutations      []seededMutation
	live           *seededLive
	// unseeded runs finalize against a run that never opened a ledger.
	unseeded bool
	seeded   bool
}

// newFinalizeFixture builds a repository plus synthesized layer evidence whose
// finalize verdict is PASS; each test bends one input away from that shape.
func newFinalizeFixture(t *testing.T) *finalizeFixture {
	t.Helper()
	return newFinalizeFixtureFor(t, fixtureRepo(t), fixtureCoverageSource)
}

// newLiveFinalizeFixture is the same shape over fixtures/live-project, whose
// smoke and e2e rows make layer 3 applicable.
func newLiveFinalizeFixture(t *testing.T) *finalizeFixture {
	t.Helper()
	return newFinalizeFixtureFor(t, liveFixtureRepo(t), liveFixtureCoverageSource)
}

func newFinalizeFixtureFor(t *testing.T, repo, coverageSource string) *finalizeFixture {
	t.Helper()
	src, cliErr := loadVerifiedSource("finalize", repo, coverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	head, cliErr := headCommit("finalize", repo)
	if cliErr != nil {
		t.Fatalf("headCommit: %v", cliErr)
	}

	f := &finalizeFixture{
		t: t, repo: repo, coverageSource: coverageSource, src: src, head: head, runID: "fin-1", dir: t.TempDir(),
	}
	f.identityCommit = head

	active := report.ActiveRows(src)
	rowResults := make([]report.TraceRowResult, 0, len(active))
	for _, row := range active {
		rowResults = append(rowResults, report.TraceRowResult{
			RowID: row.ID, Scope: "fixturemod/orders", Name: "Test" + strings.ReplaceAll(row.ID, "-", ""), Status: "passed",
		})
	}
	f.trace = &report.TraceEvidence{
		Envelope:       evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:          f.runID,
		Source:         src.Path,
		SourceSHA256:   src.SHA256,
		RepoCommit:     head,
		BaselineCommit: src.BaselineCommit,
		BaselineStatus: "ok",
		Counts:         report.SuiteCounts{Passed: len(active)},
		RowAccounting:  report.RowAccounting{ActiveRows: len(active), WithResult: len(active), Unaccounted: []string{}},
		FindingCounts:  map[string]int{},
		RowResults:     rowResults,
		Findings:       []report.TraceFinding{},
		NewFailures:    []report.TraceNewFailure{},
	}
	f.tracePath = filepath.Join(f.dir, traceEvidenceFile)
	f.writeJSON(f.tracePath, f.trace)
	f.dispatchPath = filepath.Join(f.dir, dispatchInputsFile)

	// One group over every active row, all HOLDS: the run answered layer 2 in
	// full, which is the shape a proof bends one record away from.
	rows := make([]string, 0, len(active))
	verdicts := make([]report.CriticVerdict, 0, len(active))
	for _, row := range active {
		rows = append(rows, row.ID)
		verdicts = append(verdicts, report.CriticVerdict{RowID: row.ID, Verdict: "HOLDS"})
	}
	f.groups = []seededGroup{{ID: "g1", Rows: rows, Verdicts: verdicts, Outcome: "holds"}}

	for _, row := range active {
		if row.Risk != "high" {
			continue
		}
		path := filepath.Join(f.dir, "mutation-"+row.ID+".json")
		f.writeJSON(path, &report.MutationEvidence{
			Envelope:      evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
			RunID:         f.runID,
			RepoCommit:    f.head,
			Source:        f.src.Path,
			SourceSHA256:  f.src.SHA256,
			RowID:         row.ID,
			Sites:         []report.MutationSiteOutcome{{Site: "orders/orders.go:12", Outcome: "caught"}},
			SnapshotEqual: true,
		})
		f.mutations = append(f.mutations, seededMutation{RowID: row.ID, Outcome: "caught", EvidencePath: path})
	}

	f.body = finalizeRequestBody{
		Approval:  finalizeApproval,
		Freshness: report.Freshness{Result: "fresh"},
	}
	return f
}

func (f *finalizeFixture) writeJSON(path string, doc any) {
	f.t.Helper()
	if err := evidence.WriteJSON(path, doc); err != nil {
		f.t.Fatalf("write %s: %v", path, err)
	}
}

// writeDispatch writes the dispatch inputs the trace phase would have produced
// for this fixture's synthesized trace: every active row bound, none a gap.
func (f *finalizeFixture) writeDispatch() {
	f.t.Helper()
	payload := dispatchInputs{
		Envelope:     f.trace.Envelope,
		RunID:        f.runID,
		Source:       f.src.Path,
		SourceSHA256: f.src.SHA256,
		RepoCommit:   f.trace.RepoCommit,
		Rows:         make([]dispatchRow, 0, len(f.src.Rows)),
		Runner: evidence.RunnerIdentity{
			Kind: f.src.Runner.Kind, Root: f.src.Runner.Root,
			Command: f.src.Runner.Command, ReportGlobs: f.src.Runner.ReportGlobs,
		},
		ToolVersions: map[string]string{},
	}
	for _, row := range report.ActiveRows(f.src) {
		entry := dispatchRow{
			RowID: row.ID, Behavior: row.Behavior, Expected: row.Expected,
			Level: row.Level, Risk: row.Risk, VerificationMutation: row.Mutation,
			Status: "passed", Scope: "fixturemod/orders",
			TestName: "Test" + strings.ReplaceAll(row.ID, "-", ""),
			File:     "orders/orders_test.go", Line: 1, EndLine: 2,
			TestSource: "func Test" + strings.ReplaceAll(row.ID, "-", "") + "(t *testing.T) {}",
		}
		entry.TestSourceSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(entry.TestSource)))
		entry.ClaimSHA256 = evidence.ClaimDigest(claimOf(entry))
		payload.Rows = append(payload.Rows, entry)
	}
	f.writeJSON(f.dispatchPath, &payload)
}

// seed records what the run's ops would have recorded: the trace phase with its
// dispatch inputs, every seeded review group, every seeded mutation row and the
// live record the proof configured. run calls it once; a proof that wants the
// records damaged after the fact calls it itself first.
func (f *finalizeFixture) seed() {
	f.t.Helper()
	if f.seeded {
		return
	}
	f.seeded = true
	f.writeDispatch()

	req := &verifyRequest{
		SchemaVersion: evidence.SchemaVersion, RepoRoot: f.repo,
		CoverageSource: f.coverageSource, RunID: f.runID,
	}
	store, cliErr := openLedger("finalize-fixture", req)
	if cliErr != nil {
		f.t.Fatalf("openLedger: %s", cliErr.Message)
	}
	identity := ledgerIdentity(req, f.repo, f.identityCommit, f.src)

	digest, err := evidence.SHA256File(f.dispatchPath)
	if err != nil {
		f.t.Fatalf("digest dispatch inputs: %v", err)
	}
	dispatch := readDispatchInputs(f.t, f.dispatchPath)

	err = store.Update(identity, func(l *runstate.Ledger) error {
		if _, err := l.Start(runstate.KindPhase, tracePhaseID, phaseInputs{RunDir: f.dir}, "", ""); err != nil {
			return err
		}
		if _, err := l.Complete(runstate.KindPhase, tracePhaseID, "ok", map[string]string{
			artifactEvidence: f.tracePath, artifactDispatchInputs: f.dispatchPath,
		}, ""); err != nil {
			return err
		}

		for _, group := range f.groups {
			inputs, ok := reviewGroupInputsFor(&dispatch, group.Rows, digest)
			if !ok {
				return fmt.Errorf("seed group %s: the dispatch inputs name no such row among %v", group.ID, group.Rows)
			}
			rec, err := l.Start(runstate.KindReviewGroup, group.ID, inputs, "", "")
			if err != nil {
				return err
			}
			if group.Outcome == "" {
				continue
			}
			path := reviewEvidencePath(store.Dir(), group.ID, rec.Seq)
			f.writeJSON(path, &report.ReviewArtifact{
				Envelope:     f.trace.Envelope,
				RunID:        f.runID,
				RepoCommit:   f.trace.RepoCommit,
				Source:       f.src.Path,
				SourceSHA256: f.src.SHA256,
				GroupID:      group.ID,
				Rows:         group.Rows,
				Verdicts:     group.Verdicts,
			})
			if _, err := l.Complete(runstate.KindReviewGroup, group.ID, group.Outcome,
				map[string]string{artifactEvidence: path}, ""); err != nil {
				return err
			}
		}

		for _, row := range f.mutations {
			inputs := mutationRowInputs{
				Identity:   mutationRowIdentity{Scope: "fixturemod/orders", Name: "Test" + strings.ReplaceAll(row.RowID, "-", "")},
				RunnerKind: f.src.Runner.Kind, RunDir: f.dir,
			}
			if _, err := l.Start(runstate.KindMutationRow, row.RowID, inputs, "", ""); err != nil {
				return err
			}
			if row.Outcome == "" {
				continue
			}
			artifacts := map[string]string{artifactEvidence: row.EvidencePath}
			if row.AttemptEvidencePath != "" {
				artifacts = map[string]string{artifactAttempt: row.AttemptEvidencePath}
			}
			if _, err := l.Complete(runstate.KindMutationRow, row.RowID, row.Outcome, artifacts, ""); err != nil {
				return err
			}
		}

		if f.live == nil {
			return nil
		}
		inputs := liveAttemptInputs{RunDir: f.dir, ProfilePath: f.src.LiveProfile}
		if _, err := l.Start(runstate.KindLiveAttempt, liveAttemptID, inputs, "", ""); err != nil {
			return err
		}
		if f.live.Outcome == "" {
			return nil
		}
		artifacts := map[string]string{artifactEvidence: f.live.EvidencePath}
		if f.live.AttemptEvidencePath != "" {
			artifacts = map[string]string{artifactAttempt: f.live.AttemptEvidencePath}
		}
		_, err := l.Complete(runstate.KindLiveAttempt, liveAttemptID, f.live.Outcome, artifacts, "")
		return err
	})
	if err != nil {
		f.t.Fatalf("seed ledger: %v", err)
	}
}

// ledgerDir is where the seeded run's checkpoint and review artifacts live.
func (f *finalizeFixture) ledgerDir() string {
	f.t.Helper()
	store, cliErr := openLedger("finalize-fixture", &verifyRequest{
		SchemaVersion: evidence.SchemaVersion, RepoRoot: f.repo,
		CoverageSource: f.coverageSource, RunID: f.runID,
	})
	if cliErr != nil {
		f.t.Fatalf("openLedger: %s", cliErr.Message)
	}
	return store.Dir()
}

// writeLive writes live evidence carrying verdicts and returns its path.
func (f *finalizeFixture) writeLive(verdicts []report.LiveRowVerdict) string {
	f.t.Helper()
	path := filepath.Join(f.dir, liveEvidenceFile)
	f.writeJSON(path, &report.LiveEvidence{
		Envelope:      evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:         f.runID,
		Source:        f.src.Path,
		SourceSHA256:  f.src.SHA256,
		RepoCommit:    f.head,
		RowVerdicts:   verdicts,
		CleanupOK:     true,
		SnapshotEqual: true,
	})
	return path
}

func (f *finalizeFixture) writeLiveAttempt(profile, refusal string) string {
	f.t.Helper()
	path := filepath.Join(f.dir, liveAttemptEvidenceFile)
	f.writeJSON(path, &report.LiveAttempt{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:        f.runID,
		RepoCommit:   f.head,
		Source:       f.src.Path,
		SourceSHA256: f.src.SHA256,
		Profile:      profile,
		Refusal:      refusal,
	})
	return path
}

// writeMutationAttempt writes the attempt document a row the environment
// refused points at.
func (f *finalizeFixture) writeMutationAttempt(rowID, refusal string) string {
	f.t.Helper()
	path := filepath.Join(f.dir, "mutation-attempt-"+rowID+".json")
	f.writeJSON(path, &report.Attempt{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:        f.runID,
		RowID:        rowID,
		RepoCommit:   f.head,
		Source:       f.src.Path,
		SourceSHA256: f.src.SHA256,
		Refusal:      refusal,
	})
	return path
}

// writeMutationUnappliable writes the document record-mutation retains for a
// row whose mutation could not be applied.
func (f *finalizeFixture) writeMutationUnappliable(rowID, reason string) string {
	f.t.Helper()
	path := filepath.Join(f.dir, "mutation-unappliable-"+rowID+".json")
	f.writeJSON(path, &mutationUnappliableEvidence{
		Envelope:     evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:        f.runID,
		RowID:        rowID,
		RepoCommit:   f.head,
		Source:       f.src.Path,
		SourceSHA256: f.src.SHA256,
		Reason:       reason,
		RunDir:       f.dir,
	})
	return path
}

// mutation returns the seeded row so a proof can bend it.
func (f *finalizeFixture) mutation(rowID string) *seededMutation {
	f.t.Helper()
	for i := range f.mutations {
		if f.mutations[i].RowID == rowID {
			return &f.mutations[i]
		}
	}
	f.t.Fatalf("the fixture seeds no mutation row %s", rowID)
	return nil
}

// dropMutation removes one seeded row, leaving the obligation never started.
func (f *finalizeFixture) dropMutation(rowID string) {
	f.t.Helper()
	kept := make([]seededMutation, 0, len(f.mutations))
	for _, row := range f.mutations {
		if row.RowID != rowID {
			kept = append(kept, row)
		}
	}
	if len(kept) == len(f.mutations) {
		f.t.Fatalf("the fixture seeds no mutation row %s", rowID)
	}
	f.mutations = kept
}

// publishedReport decodes the verification report finalize wrote into the repo.
func (f *finalizeFixture) publishedReport(jsonPath string) *report.Report {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.repo, filepath.FromSlash(jsonPath)))
	if err != nil {
		f.t.Fatalf("read published report: %v", err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		f.t.Fatalf("decode published report: %v", err)
	}
	return &rep
}

func (f *finalizeFixture) requestJSON() string {
	f.t.Helper()
	body, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:       evidence.SchemaVersion,
		RepoRoot:            f.repo,
		CoverageSource:      f.coverageSource,
		RunID:               f.runID,
		finalizeRequestBody: f.body,
	})
	if err != nil {
		f.t.Fatalf("marshal finalize request: %v", err)
	}
	return string(body)
}

// legacyRequestJSON is the request document a parent of the previous protocol
// sent: this run's three fields plus one retired envelope field, exactly as it
// would have arrived on the wire.
func (f *finalizeFixture) legacyRequestJSON(field string, value any) string {
	f.t.Helper()
	body := map[string]any{
		"schemaVersion":  evidence.SchemaVersion,
		"repoRoot":       f.repo,
		"coverageSource": f.coverageSource,
		"runID":          f.runID,
		"approval":       f.body.Approval,
		"freshness":      f.body.Freshness,
		field:            value,
	}
	data, err := json.Marshal(body)
	if err != nil {
		f.t.Fatalf("marshal finalize request: %v", err)
	}
	return string(data)
}

func (f *finalizeFixture) request() *finalizeRequest {
	f.t.Helper()
	f.seed()
	req, cliErr := decodeFinalizeRequest("finalize", []byte(f.requestJSON()))
	if cliErr != nil {
		f.t.Fatalf("decodeFinalizeRequest: %v", cliErr)
	}
	return req
}

func (f *finalizeFixture) run() (int, string, string) {
	f.t.Helper()
	if !f.unseeded {
		f.seed()
	}
	return runVerifyOp(f.t, "finalize", f.requestJSON())
}

func (f *finalizeFixture) wantRefusal(code string) cliError {
	f.t.Helper()
	exit, stdout, stderr := f.run()
	if exit != exitError {
		f.t.Fatalf("exit code = %d, want %d; stdout=%s", exit, exitError, stdout)
	}
	var got cliError
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		f.t.Fatalf("stderr did not decode as a cliError: %v; stderr=%s", err, stderr)
	}
	if got.Code != code {
		f.t.Errorf("refusal code = %q, want %q; message=%s", got.Code, code, got.Message)
	}
	if lines := gitStatusLines(f.t, f.repo); len(lines) != 0 {
		f.t.Errorf("git status = %v, want a refusal to publish nothing", lines)
	}
	return got
}

func decodeFinalizeOutput(t *testing.T, stdout string) finalizeOutput {
	t.Helper()
	var out finalizeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("finalize stdout did not decode: %v; stdout=%s", err, stdout)
	}
	return out
}

// workItem returns the report's work entry for one obligation.
func workItem(t *testing.T, rep *report.Report, kind, id string) report.WorkItem {
	t.Helper()
	for _, item := range rep.Work.Items {
		if item.Kind == kind && item.ID == id {
			return item
		}
	}
	t.Fatalf("work items = %+v, want an entry for %s %s", rep.Work.Items, kind, id)
	return report.WorkItem{}
}

func TestFinalize_DecodesRequestDocument(t *testing.T) {
	body := `{
	  "schemaVersion": "1",
	  "repoRoot": "/repo",
	  "coverageSource": "docs/superpowers/coverage/x-coverage.md",
	  "runID": "run-9",
	  "approval": "approved by tester",
	  "freshness": {"result": "stale", "stale": [{"hunk": "spec.md:12", "behavior": "new refund rule"}]},
	  "acknowledgement": "PARTIAL accepted by tester — docker unavailable"
	}`

	req, cliErr := decodeFinalizeRequest("finalize", []byte(body))
	if cliErr != nil {
		t.Fatalf("decodeFinalizeRequest: %v", cliErr)
	}

	if req.RepoRoot != "/repo" || req.CoverageSource != "docs/superpowers/coverage/x-coverage.md" || req.RunID != "run-9" {
		t.Errorf("base request = %+v, want the verify fields decoded", req.verifyRequest)
	}
	if req.Approval != "approved by tester" {
		t.Errorf("approval = %q", req.Approval)
	}
	wantFreshness := report.Freshness{Result: "stale", Stale: []report.StaleFinding{{Hunk: "spec.md:12", Behavior: "new refund rule"}}}
	if fmt.Sprint(req.Freshness) != fmt.Sprint(wantFreshness) {
		t.Errorf("freshness = %+v, want %+v", req.Freshness, wantFreshness)
	}
	if req.Acknowledgement != "PARTIAL accepted by tester — docker unavailable" {
		t.Errorf("acknowledgement = %q", req.Acknowledgement)
	}
}

func TestFinalize_OrdinaryInlineSpecPublishes(t *testing.T) {
	repo := fixtureRepo(t)
	source := "docs/superpowers/coverage/ordinary-spec.md"
	original, err := os.ReadFile(filepath.Join(repo, fixtureCoverageSource))
	if err != nil {
		t.Fatal(err)
	}
	original = []byte(strings.ReplaceAll(string(original), fixtureCoverageSource, source))
	original = []byte(strings.ReplaceAll(string(original), "Spec: docs/superpowers/specs/2026-08-30-fixture-design.md", "Spec: "+source))
	if err := os.WriteFile(filepath.Join(repo, source), original, 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", source)
	mustGit(t, repo, "commit", "-m", "add inline coverage source")
	f := newFinalizeFixtureFor(t, repo, source)
	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("finalize inline source: %s", stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.JSONPath != "docs/superpowers/coverage/ordinary-spec-verification.json" || out.Verdict != report.VerdictPass {
		t.Fatalf("inline result = %+v", out)
	}
	f.publishedReport(out.JSONPath)
}

func TestFinalize_DirtyRepositoryRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.seed()
	if err := os.WriteFile(filepath.Join(f.repo, "orders", "orders.go"), []byte("package orders\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exit, _, stderr := f.run()

	if exit != exitError {
		t.Fatalf("exit code = %d, want %d", exit, exitError)
	}
	if !strings.Contains(stderr, "source-not-reproducible") || !strings.Contains(stderr, "orders/orders.go") {
		t.Errorf("stderr = %q, want the source-not-reproducible refusal naming the dirty path", stderr)
	}
}

func TestFinalize_MissingApprovalRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.body.Approval = ""

	f.wantRefusal("missing-approval")
}

// TestFinalize_UnreadableEvidenceRefused holds finalize to resume's accounting
// for a record whose artifact went missing after it was recorded: the run owes
// the phase again, and finalize says so in resume's words rather than reading
// past it.
func TestFinalize_UnreadableEvidenceRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.seed()
	if err := os.Remove(f.tracePath); err != nil {
		t.Fatalf("remove trace evidence: %v", err)
	}

	got := f.wantRefusal(codeRunIncomplete)

	if !strings.Contains(got.Message, "phase trace: artifact-missing:"+artifactEvidence) {
		t.Errorf("message = %q, want it to name phase trace: artifact-missing:%s", got.Message, artifactEvidence)
	}
}

func TestFinalize_SchemaMismatchEvidenceRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.trace.SchemaVersion = "99"
	f.writeJSON(f.tracePath, f.trace)

	f.wantRefusal("evidence-schema-mismatch")
}

func TestFinalize_EvidenceFieldMissingRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.trace.RunID = ""
	f.writeJSON(f.tracePath, f.trace)

	exit, _, stderr := f.run()

	if exit != exitError {
		t.Fatalf("exit code = %d, want %d", exit, exitError)
	}
	if !strings.Contains(stderr, "evidence-field-missing") || !strings.Contains(stderr, "runId") {
		t.Errorf("stderr = %q, want evidence-field-missing naming runId", stderr)
	}
}

func TestFinalize_HeadMovedRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.identityCommit = strings.Repeat("a", 40)

	f.wantRefusal("head-moved")
}

func TestFinalize_ValidateRefusalKeepsItsCode(t *testing.T) {
	f := newFinalizeFixture(t)
	f.body.Freshness = report.Freshness{}

	f.wantRefusal(report.CodeMissingFreshness)
}

func TestFinalize_MutationEvidenceForAnotherRunRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	row := f.mutations[0]
	f.writeJSON(row.EvidencePath, &report.MutationEvidence{
		Envelope:      evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RepoCommit:    f.head,
		Source:        f.src.Path,
		SourceSHA256:  f.src.SHA256,
		RunID:         "some-other-run",
		RowID:         row.RowID,
		Sites:         []report.MutationSiteOutcome{{Site: "orders/orders.go:12", Outcome: "caught"}},
		SnapshotEqual: true,
	})

	f.wantRefusal(report.CodeStaleEvidence)
}

func TestFinalize_ConcurrentEditBeforePublishRefused(t *testing.T) {
	f := newFinalizeFixture(t)

	cliErr := finalizeOp(f.request(), io.Discard, func() {
		if err := os.WriteFile(filepath.Join(f.repo, "orders", "sneaked.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	})

	if cliErr == nil {
		t.Fatal("finalizeOp returned no error, want the snapshot-mismatch refusal")
	}
	if cliErr.Code != "snapshot-mismatch" {
		t.Errorf("code = %q, want %q; message=%s", cliErr.Code, "snapshot-mismatch", cliErr.Message)
	}
	published := filepath.Join(f.repo, "docs", "superpowers", "coverage", "2026-08-30-fixture-verification.json")
	if _, err := os.Stat(published); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want the refusal to have published nothing", published, err)
	}

	_, runDir, ok := strings.Cut(cliErr.Message, "the report stays in ")
	if !ok {
		t.Fatalf("message = %q, want it to name the run directory the report was left in", cliErr.Message)
	}
	if _, err := os.Stat(filepath.Join(runDir, "2026-08-30-fixture-verification.json")); err != nil {
		t.Errorf("stat report in run dir: %v; a blocked publish must leave it for inspection", err)
	}
}

// TestFinalize_PassRunPublishesReportAndTranscripts is the whole assembly in
// one: a run whose ledger holds the trace, its review group and both mutation
// rows publishes a PASS whose transcripts are the documents those records
// point at, each named by the digest of the bytes that landed.
func TestFinalize_PassRunPublishesReportAndTranscripts(t *testing.T) {
	f := newFinalizeFixture(t)

	exit, stdout, stderr := f.run()

	if exit != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictPass {
		t.Errorf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictPass, out.Reasons)
	}
	if len(out.Reasons) != 0 {
		t.Errorf("reasons = %+v, want none on PASS", out.Reasons)
	}
	if out.Acknowledged {
		t.Error("acknowledged = true, want false on PASS")
	}
	wantJSON := "docs/superpowers/coverage/2026-08-30-fixture-verification.json"
	wantMD := "docs/superpowers/coverage/2026-08-30-fixture-verification.md"
	wantTranscripts := "docs/superpowers/coverage/transcripts/" + f.runID
	if out.JSONPath != wantJSON || out.MarkdownPath != wantMD || out.TranscriptsDir != wantTranscripts {
		t.Errorf("output paths = %q, %q, %q; want %q, %q, %q",
			out.JSONPath, out.MarkdownPath, out.TranscriptsDir, wantJSON, wantMD, wantTranscripts)
	}
	if out.RunDir == "" {
		t.Error("runDir is empty, want the finalize run directory named")
	}

	for _, rel := range []string{
		wantJSON, wantMD,
		wantTranscripts + "/trace-evidence.json",
		wantTranscripts + "/review-g1.json",
		wantTranscripts + "/mutation-SC-07.json",
		wantTranscripts + "/mutation-SC-10.json",
	} {
		if _, err := os.Stat(filepath.Join(f.repo, filepath.FromSlash(rel))); err != nil {
			t.Errorf("stat published %s: %v", rel, err)
		}
	}

	rep := f.publishedReport(wantJSON)
	if rep.Approval != finalizeApproval {
		t.Errorf("approval = %q, want %q", rep.Approval, finalizeApproval)
	}
	if rep.HeadCommit != f.head {
		t.Errorf("headCommit = %q, want %q", rep.HeadCommit, f.head)
	}
	if rep.FinishedAt == "" {
		t.Error("finishedAt is empty")
	}
	if rep.ToolVersions["go"] == "" {
		t.Errorf("toolVersions = %v, want a go version", rep.ToolVersions)
	}
	if len(rep.Transcripts) != 4 {
		t.Fatalf("transcripts = %+v, want four refs", rep.Transcripts)
	}
	for i, ref := range rep.Transcripts {
		if i > 0 && rep.Transcripts[i-1].Path >= ref.Path {
			t.Errorf("transcripts are not sorted by path: %+v", rep.Transcripts)
		}
		sum, err := evidence.SHA256File(filepath.Join(f.repo, filepath.FromSlash(ref.Path)))
		if err != nil {
			t.Fatalf("sha256 %s: %v", ref.Path, err)
		}
		if sum != ref.SHA256 {
			t.Errorf("%s: published sha256 = %s, want the recorded %s", ref.Path, sum, ref.SHA256)
		}
	}

	// The batch is the run's own review group, keyed by the group and stamped
	// with the run that judged it: nothing about layer 2 came from the request.
	if len(rep.Batches) != 1 || rep.Batches[0].BatchID != "g1" || rep.Batches[0].RunID != f.runID {
		t.Fatalf("batches = %+v, want the run's own g1", rep.Batches)
	}

	md, err := os.ReadFile(filepath.Join(f.repo, filepath.FromSlash(wantMD)))
	if err != nil {
		t.Fatalf("read published markdown: %v", err)
	}
	if !strings.Contains(string(md), "**Verdict: PASS**") {
		t.Errorf("markdown does not carry the JSON verdict:\n%s", md)
	}
}

// TestFinalize_WorkSummaryListsEverySeededRecord holds the published work
// summary to the ledger it was derived from: one item per completed record, in
// ledger order, each fresh, each standing as its outcome says and each proved
// by the transcript the run published for it.
func TestFinalize_WorkSummaryListsEverySeededRecord(t *testing.T) {
	f := newFinalizeFixture(t)

	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	rep := f.publishedReport(out.JSONPath)

	transcripts := "docs/superpowers/coverage/transcripts/" + f.runID
	// The expectation is the seeded ledger in the order it was written: the
	// summary is a reading of the records, so it lists them as they stand.
	want := []report.WorkItem{
		{Kind: string(runstate.KindPhase), ID: tracePhaseID, Rows: []string{}, Source: report.WorkFresh,
			Outcome: "ok", Standing: report.StandingOK,
			Artifact: report.TranscriptRef{Path: transcripts + "/trace-evidence.json"}},
		{Kind: string(runstate.KindReviewGroup), ID: "g1", Rows: f.groups[0].Rows, Source: report.WorkFresh,
			Outcome: "holds", Standing: report.StandingOK,
			Artifact: report.TranscriptRef{Path: transcripts + "/review-g1.json"}},
	}
	for _, row := range f.mutations {
		want = append(want, report.WorkItem{
			Kind: string(runstate.KindMutationRow), ID: row.RowID, Rows: []string{row.RowID},
			Source: report.WorkFresh, Outcome: "caught", Standing: report.StandingOK,
			Artifact: report.TranscriptRef{Path: transcripts + "/mutation-" + row.RowID + ".json"},
		})
	}
	if len(f.mutations) != 2 {
		t.Fatalf("the fixture seeds %d mutation rows, want the two high-risk rows", len(f.mutations))
	}
	if len(rep.Work.Items) != len(want) {
		t.Fatalf("work items = %+v, want %d entries", rep.Work.Items, len(want))
	}
	for i, item := range rep.Work.Items {
		if item.Kind != want[i].Kind || item.ID != want[i].ID || item.Source != want[i].Source ||
			item.Outcome != want[i].Outcome || item.Standing != want[i].Standing ||
			item.Artifact.Path != want[i].Artifact.Path || strings.Join(item.Rows, ",") != strings.Join(want[i].Rows, ",") {
			t.Errorf("work item %d = %+v, want %+v", i, item, want[i])
		}
		if item.Reuse != nil {
			t.Errorf("work item %d carries a reuse reference, want every record fresh", i)
		}
		ref := publishedTranscript(t, rep, item.Artifact.Path)
		if ref.SHA256 != item.Artifact.SHA256 {
			t.Errorf("work item %s %s artifact sha256 = %s, want the published %s",
				item.Kind, item.ID, item.Artifact.SHA256, ref.SHA256)
		}
	}
	if got := (report.WorkCounts{Fresh: 4}); rep.Work.Counts != got {
		t.Errorf("work counts = %+v, want %+v", rep.Work.Counts, got)
	}
}

// TestFinalize_StartedGroupRefusesTheRun keeps finalize from publishing while
// the run still owes an answer: the obligation is named in resume's words.
func TestFinalize_StartedGroupRefusesTheRun(t *testing.T) {
	f := newFinalizeFixture(t)
	f.groups[0].Outcome = ""

	got := f.wantRefusal(codeRunIncomplete)

	if !strings.Contains(got.Message, "review-group g1: "+reasonNotCompleted) {
		t.Errorf("message = %q, want it to name review-group g1: %s", got.Message, reasonNotCompleted)
	}
}

// TestFinalize_UncoveredRowRefusesTheRun proves the accounting covers rows no
// group claims, not merely the records the ledger holds.
func TestFinalize_UncoveredRowRefusesTheRun(t *testing.T) {
	f := newFinalizeFixture(t)
	dropped := f.groups[0].Rows[0]
	f.groups[0].Rows = f.groups[0].Rows[1:]
	f.groups[0].Verdicts = f.groups[0].Verdicts[1:]

	got := f.wantRefusal(codeRunIncomplete)

	if !strings.Contains(got.Message, "uncovered: "+dropped) {
		t.Errorf("message = %q, want it to name the uncovered row %s", got.Message, dropped)
	}
}

// TestFinalize_RequiredMutationRowNeverStartedRefusesTheRun proves a row the
// dispatch inputs require is owed even though the ledger holds no record of it.
func TestFinalize_RequiredMutationRowNeverStartedRefusesTheRun(t *testing.T) {
	f := newFinalizeFixture(t)
	f.dropMutation("SC-10")

	got := f.wantRefusal(codeRunIncomplete)

	if !strings.Contains(got.Message, "mutation-row SC-10: "+reasonNeverStarted) {
		t.Errorf("message = %q, want it to name mutation-row SC-10: %s", got.Message, reasonNeverStarted)
	}
}

// TestFinalize_RequiredLiveAttemptNeverStartedRefusesTheRun proves the live
// layer is owed from the coverage document alone.
func TestFinalize_RequiredLiveAttemptNeverStartedRefusesTheRun(t *testing.T) {
	f := newLiveFinalizeFixture(t)

	got := f.wantRefusal(codeRunIncomplete)

	if !strings.Contains(got.Message, "live-attempt live: "+reasonNeverStarted) {
		t.Errorf("message = %q, want it to name live-attempt live: %s", got.Message, reasonNeverStarted)
	}
}

// TestFinalize_UnappliableRowBlocksWithItsDocumentsReason proves a fresh row
// whose mutation could not be applied reaches the report from its own
// document, published under its own transcript name.
func TestFinalize_UnappliableRowBlocksWithItsDocumentsReason(t *testing.T) {
	f := newFinalizeFixture(t)
	const reason = "the order total has no site this mutation applies to"
	row := f.mutation("SC-07")
	row.Outcome = outcomeUnappliable
	row.EvidencePath = f.writeMutationUnappliable("SC-07", reason)

	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictBlocked {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictBlocked, out.Reasons)
	}
	rep := f.publishedReport(out.JSONPath)
	if got := publishedRow(t, rep, "SC-07").Layer4.Status; got != "MUTATION-UNAPPLIABLE" {
		t.Errorf("SC-07 layer4 status = %q, want MUTATION-UNAPPLIABLE", got)
	}
	found := false
	for _, r := range rep.Reasons {
		if r.Code == "MUTATION-UNAPPLIABLE" && strings.Contains(r.Detail, reason) {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %+v, want the document's own reason %q", rep.Reasons, reason)
	}
	published := "docs/superpowers/coverage/transcripts/" + f.runID + "/mutation-unappliable-SC-07.json"
	publishedTranscript(t, rep, published)
	if _, err := os.Stat(filepath.Join(f.repo, filepath.FromSlash(published))); err != nil {
		t.Errorf("stat %s: %v", published, err)
	}
	if got := workItem(t, rep, string(runstate.KindMutationRow), "SC-07"); got.Standing != report.StandingFailed {
		t.Errorf("SC-07 work standing = %q, want %q", got.Standing, report.StandingFailed)
	}
}

// TestFinalize_EnvironmentalRowQuotesTheAttemptsRefusal proves the third shape
// a fresh layer-4 record can carry: an attempt document, no mutation evidence,
// and the attempt's own refusal as the reason.
func TestFinalize_EnvironmentalRowQuotesTheAttemptsRefusal(t *testing.T) {
	f := newFinalizeFixture(t)
	const refusal = "the mutation runner could not start in this environment"
	row := f.mutation("SC-07")
	row.Outcome = outcomeEnvironmental
	row.EvidencePath = ""
	row.AttemptEvidencePath = f.writeMutationAttempt("SC-07", refusal)

	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictPartial {
		t.Fatalf("verdict = %q, want %s; reasons=%+v", out.Verdict, report.VerdictPartial, out.Reasons)
	}
	rep := f.publishedReport(out.JSONPath)
	if got := publishedRow(t, rep, "SC-07").Layer4.Status; got != "MUTATION-N/A (environmental)" {
		t.Errorf("SC-07 layer4 status = %q, want MUTATION-N/A (environmental)", got)
	}
	found := false
	for _, r := range rep.Reasons {
		if r.Code == "mutation-environmental" && strings.Contains(r.Detail, refusal) {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %+v, want the attempt's own refusal %q", rep.Reasons, refusal)
	}
	publishedTranscript(t, rep, "docs/superpowers/coverage/transcripts/"+f.runID+"/mutation-attempt-SC-07.json")
	if got := workItem(t, rep, string(runstate.KindMutationRow), "SC-07"); got.Standing != report.StandingIncomplete {
		t.Errorf("SC-07 work standing = %q, want %q", got.Standing, report.StandingIncomplete)
	}
}

func TestFinalize_LiveEvidencePublishedAndPerRow(t *testing.T) {
	f := newLiveFinalizeFixture(t)
	f.live = &seededLive{Outcome: "pass", EvidencePath: f.writeLive([]report.LiveRowVerdict{
		{RowID: "SC-11", Verdict: "LIVE-PASS"},
		{RowID: "SC-12", Verdict: "LIVE-PASS"},
		{RowID: "SC-13", Verdict: "LIVE-INCONCLUSIVE"},
	})}

	exit, stdout, stderr := f.run()

	if exit != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictBlocked {
		t.Errorf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictBlocked, out.Reasons)
	}

	published := "docs/superpowers/coverage/transcripts/" + f.runID + "/live-evidence.json"
	if _, err := os.Stat(filepath.Join(f.repo, filepath.FromSlash(published))); err != nil {
		t.Errorf("stat %s: %v", published, err)
	}

	rep := f.publishedReport(out.JSONPath)
	want := map[string]string{"SC-11": "LIVE-PASS", "SC-12": "LIVE-PASS", "SC-13": "LIVE-INCONCLUSIVE", "SC-14": "N/A"}
	for _, row := range rep.Rows {
		if got := row.Layer3; got != want[row.RowID] {
			t.Errorf("%s layer3 = %q, want %q", row.RowID, got, want[row.RowID])
		}
	}
	if rep.Layers.Layer3.Applicable != 3 || rep.Layers.Layer3.Pass != 2 || rep.Layers.Layer3.Inconclusive != 1 {
		t.Errorf("layer3 summary = %+v, want 3 applicable, 2 pass, 1 inconclusive", rep.Layers.Layer3)
	}

	ref := publishedTranscript(t, rep, published)
	sum, err := evidence.SHA256File(filepath.Join(f.repo, filepath.FromSlash(published)))
	if err != nil {
		t.Fatalf("sha256: %v", err)
	}
	if sum != ref.SHA256 {
		t.Errorf("%s: published sha256 = %s, want the recorded %s", published, sum, ref.SHA256)
	}
}

func TestFinalize_LiveAttemptPublishedAsPartial(t *testing.T) {
	f := newLiveFinalizeFixture(t)
	f.live = &seededLive{
		Outcome:             "unavailable",
		AttemptEvidencePath: f.writeLiveAttempt(liveFixtureProfilePath, "missing-profile"),
	}

	exit, stdout, stderr := f.run()

	if exit != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exit, exitOK, stderr)
	}
	out := decodeFinalizeOutput(t, stdout)
	if out.Verdict != report.VerdictPartial {
		t.Fatalf("verdict = %q, want %q; reasons=%+v", out.Verdict, report.VerdictPartial, out.Reasons)
	}
	found := false
	for _, r := range out.Reasons {
		if r.Code == "live-profile-unavailable" {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %+v, want a live-profile-unavailable reason", out.Reasons)
	}

	published := "docs/superpowers/coverage/transcripts/" + f.runID + "/live-attempt.json"
	if _, err := os.Stat(filepath.Join(f.repo, filepath.FromSlash(published))); err != nil {
		t.Errorf("stat %s: %v", published, err)
	}
	if _, err := os.Stat(filepath.Join(f.repo, "docs", "superpowers", "coverage", "transcripts", f.runID, "live-evidence.json")); !os.IsNotExist(err) {
		t.Errorf("live-evidence.json exists, want only the attempt transcript published")
	}
	if got := workItem(t, f.publishedReport(out.JSONPath), string(runstate.KindLiveAttempt), liveAttemptID); got.Standing != report.StandingIncomplete {
		t.Errorf("live work standing = %q, want %q", got.Standing, report.StandingIncomplete)
	}
}

func TestFinalize_LiveEvidenceWithoutApplicableRowsRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.live = &seededLive{Outcome: "pass", EvidencePath: f.writeLive(nil)}

	f.wantRefusal(report.CodeUnexpectedLiveEvidence)
}

func TestFinalize_MutationAttemptEvidenceSchemaMismatchRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	attemptPath := filepath.Join(f.dir, "mutation-attempt.json")
	if err := os.WriteFile(attemptPath, []byte(`{"rowID":"SC-10","refusal":"runner-start-failed"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	row := &f.mutations[0]
	row.EvidencePath = ""
	row.Outcome = outcomeEnvironmental
	row.Reason = "the mutation runner could not start"
	row.AttemptEvidencePath = attemptPath

	f.wantRefusal("evidence-schema-mismatch")
}

func TestFinalize_MutationAttemptEvidenceUnreadableRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	attemptPath := filepath.Join(f.dir, "mutation-attempt-broken.json")
	if err := os.WriteFile(attemptPath, []byte(`{"schemaVersion": `), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	row := &f.mutations[0]
	row.EvidencePath = ""
	row.Outcome = outcomeEnvironmental
	row.Reason = "the mutation runner could not start"
	row.AttemptEvidencePath = attemptPath

	f.wantRefusal("evidence-unreadable")
}

func TestFinalize_PartialAcknowledgementAfterDiscardingOwnArtifacts(t *testing.T) {
	f := newLiveFinalizeFixture(t)
	f.live = &seededLive{
		Outcome:             "unavailable",
		AttemptEvidencePath: f.writeLiveAttempt(liveFixtureProfilePath, "profile unavailable"),
	}
	exit, stdout, stderr := f.run()
	if exit != exitOK {
		t.Fatalf("initial finalize: %s", stderr)
	}
	first := decodeFinalizeOutput(t, stdout)
	rep := f.publishedReport(first.JSONPath)
	if first.Verdict != report.VerdictPartial || first.Acknowledged {
		t.Fatalf("initial result = %+v", first)
	}

	// The second run recovers exactly what the first published and republishes
	// over it; nothing is removed by hand.
	f.body.Acknowledgement = "PARTIAL accepted: live environment unavailable"
	exit, stdout, stderr = f.run()
	if exit != exitOK {
		t.Fatalf("same-HEAD acknowledged finalize: %s", stderr)
	}
	second := decodeFinalizeOutput(t, stdout)
	if _, err := os.Stat(filepath.Join(first.RunDir, filepath.Base(first.JSONPath))); err != nil {
		t.Fatalf("original report not preserved: %v", err)
	}
	ack := f.publishedReport(second.JSONPath)
	if !ack.Acknowledged || ack.Verdict != report.VerdictPartial || ack.HeadCommit != f.head {
		t.Fatalf("acknowledged report = %+v", ack)
	}
	for i, ref := range rep.Transcripts {
		if ack.Transcripts[i] != ref {
			t.Fatalf("evidence changed: %+v -> %+v", ref, ack.Transcripts[i])
		}
	}
}

func TestFinalize_MutationAttemptRequiresMatchingIdentity(t *testing.T) {
	for _, mismatch := range []string{"run", "row", "commit", "source", "sha", "missing"} {
		t.Run(mismatch, func(t *testing.T) {
			f := newFinalizeFixture(t)
			row := &f.mutations[0]
			attempt := &report.Attempt{
				Envelope: f.trace.Envelope,
				RunID:    f.runID, RowID: row.RowID,
				RepoCommit: f.head, Source: f.src.Path, SourceSHA256: f.src.SHA256,
				Refusal: "runner unavailable",
			}
			code := report.CodeStaleEvidence
			switch mismatch {
			case "run":
				attempt.RunID = "another-run"
			case "row":
				attempt.RowID = "SC-999"
				code = report.CodeMutationRowMismatch
			case "commit":
				attempt.RepoCommit = strings.Repeat("a", 40)
			case "source":
				attempt.Source = "different-spec.md"
			case "sha":
				attempt.SourceSHA256 = "different-source"
			case "missing":
				attempt.RepoCommit = ""
				code = "evidence-field-missing"
			}
			row.EvidencePath = ""
			row.Outcome = outcomeEnvironmental
			row.AttemptEvidencePath = filepath.Join(f.dir, "attempt.json")
			f.writeJSON(row.AttemptEvidencePath, attempt)
			f.wantRefusal(code)
		})
	}
}
