package main

import (
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
)

const finalizeApproval = "approved by tester at the coverage gate"

type finalizeRequestJSON struct {
	SchemaVersion  string `json:"schemaVersion"`
	RepoRoot       string `json:"repoRoot"`
	CoverageSource string `json:"coverageSource"`
	RunID          string `json:"runID"`
	finalizeRequestBody
}

type finalizeFixture struct {
	t              *testing.T
	repo           string
	coverageSource string
	src            *coverage.Source
	head           string
	runID          string
	dir            string
	tracePath      string
	trace          *report.TraceEvidence
	body           finalizeRequestBody
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
	f.tracePath = filepath.Join(f.dir, "trace-evidence.json")
	f.writeJSON(f.tracePath, f.trace)

	var mutations []report.MutationInput
	for _, row := range active {
		if row.Risk != "high" {
			continue
		}
		path := filepath.Join(f.dir, "mutation-"+row.ID+".json")
		f.writeJSON(path, &report.MutationEvidence{
			Envelope:      evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
			RunID:         f.runID,
			RowID:         row.ID,
			Sites:         []report.MutationSiteOutcome{{Site: "orders/orders.go:12", Outcome: "caught"}},
			SnapshotEqual: true,
		})
		mutations = append(mutations, report.MutationInput{RowID: row.ID, EvidencePath: path})
	}

	f.body = finalizeRequestBody{
		Approval:      finalizeApproval,
		TraceEvidence: f.tracePath,
		Freshness:     report.Freshness{Result: "fresh"},
		Layer2:        finalizeLayer2{Batches: holdingBatches(f.runID, active)},
		Layer4:        finalizeLayer4{Rows: mutations},
	}
	return f
}

// holdingBatches is the all-green layer-2 shape: a synthesized canary caught on
// the first row, that row re-dispatched alone against the real source, and the
// rest in a canary-free batch inheriting the catch.
func holdingBatches(runID string, active []coverage.Row) []report.Batch {
	first := active[0].ID
	rest := make([]string, 0, len(active)-1)
	verdicts := make([]report.CriticVerdict, 0, len(active)-1)
	for _, row := range active[1:] {
		rest = append(rest, row.ID)
		verdicts = append(verdicts, report.CriticVerdict{RowID: row.ID, Verdict: "HOLDS"})
	}
	return []report.Batch{
		{
			BatchID: "cb1", DispatchID: "d1", RunID: runID, Rows: []string{first},
			Canary: &report.Canary{RowID: first, Kind: "synthesized", Weakening: "dropped the boundary assertion", Caught: true},
		},
		{
			BatchID: "b1", DispatchID: "d2", RunID: runID, Rows: []string{first}, InheritsFrom: "cb1",
			Verdicts: []report.CriticVerdict{{RowID: first, Verdict: "HOLDS"}},
		},
		{
			BatchID: "b2", DispatchID: "d3", RunID: runID, Rows: rest, InheritsFrom: "cb1",
			Verdicts: verdicts,
		},
	}
}

func (f *finalizeFixture) writeJSON(path string, doc any) {
	f.t.Helper()
	if err := evidence.WriteJSON(path, doc); err != nil {
		f.t.Fatalf("write %s: %v", path, err)
	}
}

// writeLive writes live evidence carrying verdicts and returns its path.
func (f *finalizeFixture) writeLive(verdicts []report.LiveRowVerdict) string {
	f.t.Helper()
	path := filepath.Join(f.dir, "live-evidence.json")
	f.writeJSON(path, &report.LiveEvidence{
		Envelope:      evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:         f.runID,
		Source:        f.src.Path,
		SourceSHA256:  f.src.SHA256,
		RowVerdicts:   verdicts,
		CleanupOK:     true,
		SnapshotEqual: true,
	})
	return path
}

func (f *finalizeFixture) writeLiveAttempt(profile, refusal string) string {
	f.t.Helper()
	path := filepath.Join(f.dir, "live-attempt.json")
	f.writeJSON(path, &report.LiveAttempt{
		Envelope: evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:    f.runID,
		Profile:  profile,
		Refusal:  refusal,
	})
	return path
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

func (f *finalizeFixture) request() *finalizeRequest {
	f.t.Helper()
	req, cliErr := decodeFinalizeRequest("finalize", []byte(f.requestJSON()))
	if cliErr != nil {
		f.t.Fatalf("decodeFinalizeRequest: %v", cliErr)
	}
	return req
}

func (f *finalizeFixture) run() (int, string, string) {
	f.t.Helper()
	return runVerifyOp(f.t, "finalize", f.requestJSON())
}

func (f *finalizeFixture) wantRefusal(code string) {
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
}

func decodeFinalizeOutput(t *testing.T, stdout string) finalizeOutput {
	t.Helper()
	var out finalizeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("finalize stdout did not decode: %v; stdout=%s", err, stdout)
	}
	return out
}

func TestFinalize_DecodesRequestDocument(t *testing.T) {
	body := `{
	  "schemaVersion": "1",
	  "repoRoot": "/repo",
	  "coverageSource": "docs/superpowers/coverage/x-coverage.md",
	  "runID": "run-9",
	  "approval": "approved by tester",
	  "traceEvidence": "/runs/run-9/trace-evidence.json",
	  "freshness": {"result": "stale", "stale": [{"hunk": "spec.md:12", "behavior": "new refund rule"}]},
	  "layer2": {"batches": [{
	    "batchId": "b1", "dispatchId": "d1", "runId": "run-9", "rows": ["SC-01"],
	    "canary": {"rowId": "SC-01", "kind": "synthesized", "weakening": "dropped assert", "caught": true},
	    "inheritsFrom": "cb0", "voided": true,
	    "verdicts": [{"rowId": "SC-01", "verdict": "WEAK", "reason": "asserts nothing"}]
	  }]},
	  "layer3": {"evidencePath": "/runs/run-9/live-evidence.json"},
	  "layer4": {"rows": [{
	    "rowId": "SC-02", "evidencePath": "/runs/m.json", "outcome": "environmental",
	    "reason": "docker unavailable", "attemptEvidencePath": "/runs/attempt.json"
	  }]},
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
	if req.TraceEvidence != "/runs/run-9/trace-evidence.json" {
		t.Errorf("traceEvidence = %q", req.TraceEvidence)
	}
	wantFreshness := report.Freshness{Result: "stale", Stale: []report.StaleFinding{{Hunk: "spec.md:12", Behavior: "new refund rule"}}}
	if fmt.Sprint(req.Freshness) != fmt.Sprint(wantFreshness) {
		t.Errorf("freshness = %+v, want %+v", req.Freshness, wantFreshness)
	}
	if len(req.Layer2.Batches) != 1 {
		t.Fatalf("layer2.batches = %+v, want one batch", req.Layer2.Batches)
	}
	b := req.Layer2.Batches[0]
	if b.BatchID != "b1" || b.DispatchID != "d1" || b.RunID != "run-9" || b.InheritsFrom != "cb0" || !b.Voided {
		t.Errorf("batch = %+v, want every batch field decoded", b)
	}
	if b.Canary == nil || b.Canary.RowID != "SC-01" || b.Canary.Kind != "synthesized" ||
		b.Canary.Weakening != "dropped assert" || !b.Canary.Caught {
		t.Errorf("canary = %+v, want every canary field decoded", b.Canary)
	}
	if len(b.Verdicts) != 1 || b.Verdicts[0].Verdict != "WEAK" || b.Verdicts[0].Reason != "asserts nothing" {
		t.Errorf("verdicts = %+v", b.Verdicts)
	}
	if req.Layer3 == nil || req.Layer3.EvidencePath != "/runs/run-9/live-evidence.json" {
		t.Errorf("layer3 = %+v", req.Layer3)
	}
	if len(req.Layer4.Rows) != 1 {
		t.Fatalf("layer4.rows = %+v, want one row", req.Layer4.Rows)
	}
	m := req.Layer4.Rows[0]
	if m.RowID != "SC-02" || m.EvidencePath != "/runs/m.json" || m.Outcome != "environmental" ||
		m.Reason != "docker unavailable" || m.AttemptEvidencePath != "/runs/attempt.json" {
		t.Errorf("layer4 row = %+v, want every field decoded", m)
	}
	if req.Acknowledgement != "PARTIAL accepted by tester — docker unavailable" {
		t.Errorf("acknowledgement = %q", req.Acknowledgement)
	}
}

func TestFinalize_SourceNameUnconventionalRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	body, err := json.Marshal(finalizeRequestJSON{
		SchemaVersion:       evidence.SchemaVersion,
		RepoRoot:            f.repo,
		CoverageSource:      "docs/superpowers/coverage/2026-08-30-fixture.md",
		RunID:               f.runID,
		finalizeRequestBody: f.body,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	exit, _, stderr := runVerifyOp(t, "finalize", string(body))

	if exit != exitError {
		t.Fatalf("exit code = %d, want %d", exit, exitError)
	}
	if !strings.Contains(stderr, "source-name-unconventional") {
		t.Errorf("stderr = %q, want the source-name-unconventional refusal", stderr)
	}
}

func TestFinalize_DirtyRepositoryRefused(t *testing.T) {
	f := newFinalizeFixture(t)
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

func TestFinalize_UnreadableEvidenceRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.body.TraceEvidence = filepath.Join(f.dir, "absent.json")

	f.wantRefusal("evidence-unreadable")
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
	f.trace.RepoCommit = strings.Repeat("a", 40)
	f.writeJSON(f.tracePath, f.trace)

	f.wantRefusal("head-moved")
}

func TestFinalize_ValidateRefusalKeepsItsCode(t *testing.T) {
	f := newFinalizeFixture(t)
	f.body.Freshness = report.Freshness{}

	f.wantRefusal(report.CodeMissingFreshness)
}

func TestFinalize_MutationEvidenceForAnotherRunRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	path := f.body.Layer4.Rows[0].EvidencePath
	f.writeJSON(path, &report.MutationEvidence{
		Envelope:      evidence.Envelope{SchemaVersion: evidence.SchemaVersion, VerifierVersion: evidence.VerifierVersion},
		RunID:         "some-other-run",
		RowID:         f.body.Layer4.Rows[0].RowID,
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
		wantTranscripts + "/mutation-SC-07.json",
		wantTranscripts + "/mutation-SC-10.json",
	} {
		if _, err := os.Stat(filepath.Join(f.repo, filepath.FromSlash(rel))); err != nil {
			t.Errorf("stat published %s: %v", rel, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(f.repo, filepath.FromSlash(wantJSON)))
	if err != nil {
		t.Fatalf("read published report: %v", err)
	}
	var rep report.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("decode published report: %v", err)
	}
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
	if len(rep.Transcripts) != 3 {
		t.Fatalf("transcripts = %+v, want three refs", rep.Transcripts)
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

	md, err := os.ReadFile(filepath.Join(f.repo, filepath.FromSlash(wantMD)))
	if err != nil {
		t.Fatalf("read published markdown: %v", err)
	}
	if !strings.Contains(string(md), "**Verdict: PASS**") {
		t.Errorf("markdown does not carry the JSON verdict:\n%s", md)
	}
}

func TestFinalize_LiveEvidencePublishedAndPerRow(t *testing.T) {
	f := newLiveFinalizeFixture(t)
	f.body.Layer3 = &finalizeLayer3{EvidencePath: f.writeLive([]report.LiveRowVerdict{
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

	var ref report.TranscriptRef
	for _, tr := range rep.Transcripts {
		if tr.Path == published {
			ref = tr
		}
	}
	if ref.Path == "" {
		t.Fatalf("transcripts = %+v, want a ref to %s", rep.Transcripts, published)
	}
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
	f.body.Layer3 = &finalizeLayer3{
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
}

func TestFinalize_LiveEvidenceWithoutApplicableRowsRefused(t *testing.T) {
	f := newFinalizeFixture(t)
	f.body.Layer3 = &finalizeLayer3{EvidencePath: f.writeLive(nil)}

	f.wantRefusal(report.CodeUnexpectedLiveEvidence)
}
