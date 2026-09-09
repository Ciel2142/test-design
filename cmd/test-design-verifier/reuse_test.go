package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// reuseRequestJSON is the request the op decodes: a verify request plus the
// run to stand on and the critic policy this run would dispatch with.
func reuseRequestJSON(t *testing.T, repo, runID, priorRunID string, policy *reviewPolicy) string {
	t.Helper()
	extra := map[string]any{"priorRunId": priorRunID}
	if policy != nil {
		extra["policy"] = policy
	}
	return requestWith(t, repo, runID, extra)
}

// runReuse drives the real reuse op over repo's run and decodes its report.
func runReuse(t *testing.T, repo, runID, priorRunID string, policy *reviewPolicy) reuseOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "reuse", reuseRequestJSON(t, repo, runID, priorRunID, policy))
	if code != exitOK {
		t.Fatalf("reuse exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out reuseOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("reuse stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// refuseReuse drives reuse expecting an op-level refusal and returns it.
func refuseReuse(t *testing.T, repo, runID, priorRunID string, policy *reviewPolicy) cliError {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "reuse", reuseRequestJSON(t, repo, runID, priorRunID, policy))
	if code != exitError || stdout != "" {
		t.Fatalf("reuse was accepted: exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	return decodeCLIError(t, stderr)
}

// startReviewGroupWith opens groupID over rows under a named critic policy,
// through the real record-review op.
func startReviewGroupWith(t *testing.T, repo, runID, groupID string, policy *reviewPolicy, rows ...string) recordReviewOutput {
	t.Helper()
	extra := map[string]any{"groupId": groupID, "status": "started", "rows": rows}
	if policy != nil {
		extra["policy"] = policy
	}
	return runRecordReview(t, repo, runID, extra)
}

// completeReviewGroupWith closes groupID with one sound verdict per row and the
// critic's own account of what it read. A nil reads is an unrecorded read list,
// which is what leaves a manifest incomplete.
func completeReviewGroupWith(
	t *testing.T, repo, runID, groupID string, dispatch dispatchInputs, reads *[]string, rows ...string,
) recordReviewOutput {
	t.Helper()
	extra := map[string]any{"groupId": groupID, "status": "completed", "verdicts": soundVerdicts(t, dispatch, rows...)}
	if reads != nil {
		extra["reads"] = *reads
	}
	return runRecordReview(t, repo, runID, extra)
}

// reusePolicy is a critic identity backed by a real instruction file outside
// the repository, in the decoded shape reuse and record-review both take.
func reusePolicy(t *testing.T) *reviewPolicy {
	t.Helper()
	return &reviewPolicy{
		Critic:       "coverage-critic",
		Instructions: []string{writePolicyFile(t, "critic.md", "judge the assertion, not the name\n")},
	}
}

// unappliableRow leaves a completed mutation-row record for rowID: prepared the
// way the run prepares one, then closed as unappliable, whose evidence lives in
// the ledger directory rather than a run directory a later run may not find.
func unappliableRow(t *testing.T, repo, runID, rowID string) recordMutationOutput {
	t.Helper()
	preparedMutation(t, repo, runID, rowID)
	code, stdout, stderr := runVerifyOp(t, "record-mutation", requestWith(t, repo, runID, map[string]any{
		"rowId": rowID, "outcome": "unappliable", "reason": "no site applies to the fixture guard",
	}))
	if code != exitOK {
		t.Fatalf("record-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out recordMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("record-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// writePriorLedger creates a prior run's ledger carrying identity and nothing
// else, the way a run that opened a ledger and recorded nothing would.
func writePriorLedger(t *testing.T, repo, runID string, identity runstate.Identity) {
	t.Helper()
	store, cliErr := openLedger("test", fixtureVerifyRequest(repo, runID))
	if cliErr != nil {
		t.Fatalf("openLedger: %v", cliErr.Message)
	}
	if err := store.Update(identity, func(l *runstate.Ledger) error { return nil }); err != nil {
		t.Fatalf("write prior ledger: %v", err)
	}
}

// resolvedRepoRoot is the repository root the ledger records, symlinks resolved.
func resolvedRepoRoot(t *testing.T, repo string) string {
	t.Helper()
	root, err := resolveThroughSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve %s: %v", repo, err)
	}
	return root
}

// reuseDecisionFor returns the decision reuse reported for id.
func reuseDecisionFor(t *testing.T, decisions []reuseDecision, id string) reuseDecision {
	t.Helper()
	for _, decision := range decisions {
		if decision.ID == id {
			return decision
		}
	}
	t.Fatalf("no decision for %s in %+v", id, decisions)
	return reuseDecision{}
}

// reuseChangedPathFor returns how reuse classified path.
func reuseChangedPathFor(t *testing.T, out reuseOutput, path string) reuseChangedPath {
	t.Helper()
	for _, changed := range out.ChangedPaths {
		if changed.Path == path {
			return changed
		}
	}
	t.Fatalf("no classification of %s in %+v", path, out.ChangedPaths)
	return reuseChangedPath{}
}

// readReviewEvidence decodes a review group's retained evidence document.
func readReviewEvidence(t *testing.T, path string) reviewEvidence {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read review evidence %s: %v", path, err)
	}
	var doc reviewEvidence
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("review evidence did not decode: %v", err)
	}
	return doc
}

// recordedReviewInputs decodes the input identity a ledger record captured.
func recordedReviewInputs(t *testing.T, rec runstate.Record) reviewGroupInputs {
	t.Helper()
	var inputs reviewGroupInputs
	if err := json.Unmarshal(rec.Inputs, &inputs); err != nil {
		t.Fatalf("record inputs did not decode: %v", err)
	}
	return inputs
}

// appendToRepoFile appends text to an existing repository file, leaving every
// declaration already in it at the line it was on.
func appendToRepoFile(t *testing.T, repo, rel, text string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	writeRepoFile(t, repo, rel, string(data)+text)
}

// TestReuse_RefusesARequestItCannotAnswer walks the gates a reuse request
// clears before any prior evidence is read: a prior run must be named, must be
// another run, the run asking must already stand on its own trace, and the
// prior run must have left a ledger.
func TestReuse_RefusesARequestItCannotAnswer(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "reuse-refuses-request"

	if got := refuseReuse(t, repo, runID, "", nil); got.Code != "missing-prior-run-id" {
		t.Errorf("error code = %q, want missing-prior-run-id", got.Code)
	}
	if got := refuseReuse(t, repo, runID, runID, nil); got.Code != "prior-run-is-current" {
		t.Errorf("error code = %q, want prior-run-is-current", got.Code)
	}
	if got := refuseReuse(t, repo, runID, "some-earlier-run", nil); got.Code != "trace-incomplete" {
		t.Errorf("error code = %q, want trace-incomplete before the run's own trace", got.Code)
	}

	tracedDispatch(t, repo, runID)

	if got := refuseReuse(t, repo, runID, "never-ran", nil); got.Code != "prior-ledger-missing" {
		t.Errorf("error code = %q, want prior-ledger-missing", got.Code)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want a refused reuse to leave the repository untouched", lines)
	}
}

// TestReuse_RefusesAPriorRunItCannotStandOn holds reuse to a prior run of the
// same subject at a commit this repository still has, and to a current run
// whose own inputs have not moved since its trace.
func TestReuse_RefusesAPriorRunItCannotStandOn(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "reuse-refuses-prior"
	const copySource = "docs/superpowers/coverage/2026-08-30-fixture-coverage-copy.md"

	original, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(fixtureCoverageSource)))
	if err != nil {
		t.Fatalf("read coverage doc: %v", err)
	}
	copyPath := writeRepoFile(t, repo, copySource, string(original))
	commitAll(t, repo, "keep a renamed copy of the coverage document")
	tracedDispatch(t, repo, runID)
	head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	copyDigest, err := evidence.SHA256File(copyPath)
	if err != nil {
		t.Fatalf("digest %s: %v", copySource, err)
	}

	const otherSubject = "reuse-prior-other-source"
	writePriorLedger(t, repo, otherSubject, runstate.Identity{
		RunID: otherSubject, RepoRoot: resolvedRepoRoot(t, repo), RepoCommit: head,
		Source: copySource, SourceSHA256: copyDigest,
	})
	if got := refuseReuse(t, repo, runID, otherSubject, nil); got.Code != "prior-source-mismatch" {
		t.Errorf("error code = %q, want prior-source-mismatch", got.Code)
	}

	const lostCommit = "reuse-prior-lost-commit"
	writePriorLedger(t, repo, lostCommit, runstate.Identity{
		RunID: lostCommit, RepoRoot: resolvedRepoRoot(t, repo),
		RepoCommit: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Source:     fixtureCoverageSource, SourceSHA256: strings.Repeat("0", 64),
	})
	if got := refuseReuse(t, repo, runID, lostCommit, nil); got.Code != "prior-commit-unreachable" {
		t.Errorf("error code = %q, want prior-commit-unreachable", got.Code)
	}

	rewriteFixtureDoc(t, repo, func(s string) string { return s + "\n<!-- edited after the trace -->\n" })
	if got := refuseReuse(t, repo, runID, lostCommit, nil); got.Code != "stale-evidence" {
		t.Errorf("error code = %q, want stale-evidence once the document moved under the trace", got.Code)
	}
}

// TestReuse_AdoptsCompletedEvidenceFromThePriorRun is the whole of what reuse
// is for: a run that changed nothing but its own published report adopts the
// prior run's completed group and mutation row, pointing at the original files
// rather than copies, carrying the prior manifest and the provenance of what it
// stood on. The adopted records answer for the run exactly as records it made
// itself would — resume reports them completed and valid, no row may be grouped
// again — while the trace and the live attempt are never adopted, and a second
// reuse adds nothing.
func TestReuse_AdoptsCompletedEvidenceFromThePriorRun(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-adopt-prior"
	const runID = "reuse-adopt-new"
	rows := []string{"SC-01", "SC-02", "SC-10"}
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, rows...)
	group := completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, rows...)
	row := unappliableRow(t, repo, priorRunID, "SC-10")
	priorHead := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	priorGroupSeq := ledgerRecord(t, runResume(t, repo, priorRunID), runstate.KindReviewGroup, "A").Seq

	const publication = "docs/superpowers/coverage/2026-08-30-fixture-report.md"
	writeRepoFile(t, repo, publication, "# published report\n")
	head := commitAll(t, repo, "publish a report beside the coverage document")

	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)

	if out.RunID != runID || out.PriorRunID != priorRunID {
		t.Errorf("run ids = %q/%q, want %q/%q", out.RunID, out.PriorRunID, runID, priorRunID)
	}
	if out.PriorRepoCommit != priorHead || out.RepoCommit != head {
		t.Errorf("commits = %q/%q, want %q/%q", out.PriorRepoCommit, out.RepoCommit, priorHead, head)
	}
	if out.Trace.Reason != "fresh-required" || out.Trace.Reused || out.Trace.Kind != runstate.KindPhase {
		t.Errorf("trace = %+v, want the phase never adopted, reason fresh-required", out.Trace)
	}
	if out.Live.Reason != "fresh-required" || out.Live.Reused || out.Live.Kind != runstate.KindLiveAttempt {
		t.Errorf("live = %+v, want the live attempt never adopted, reason fresh-required", out.Live)
	}
	if len(out.ChangedPaths) != 1 {
		t.Fatalf("changedPaths = %+v, want only the published report", out.ChangedPaths)
	}
	if changed := reuseChangedPathFor(t, out, publication); changed.Class != "exempt" {
		t.Errorf("%s class = %q, want exempt: it lies under the coverage source's directory", publication, changed.Class)
	}

	adopted := reuseDecisionFor(t, out.Review, "A")
	if !adopted.Reused || adopted.Outcome != "holds" || adopted.Reason != "" {
		t.Fatalf("group A = %+v, want it reused with the prior outcome", adopted)
	}
	if !slices.Equal(adopted.Rows, rows) {
		t.Errorf("group A rows = %v, want %v", adopted.Rows, rows)
	}
	ref := adopted.Reference
	if ref == nil {
		t.Fatal("group A carries no reuse reference")
	}
	if ref.PriorRunID != priorRunID || ref.PriorRepoCommit != priorHead || ref.PriorSeq != priorGroupSeq {
		t.Errorf("reference = %+v, want the prior run, commit and sequence number", ref)
	}
	if ref.Outcome != "holds" || ref.Artifact.Path != group.EvidencePath {
		t.Errorf("reference artifact = %+v, want the original evidence at %s", ref.Artifact, group.EvidencePath)
	}
	if ref.Basis.PriorCommit != priorHead || len(ref.Basis.ChangedPaths) != 0 {
		t.Errorf("basis = %+v, want the prior commit and no changed path outside the exempt directory", ref.Basis)
	}
	if !slices.Equal(ref.Basis.ClaimRows, rows) || ref.Basis.ContextFiles != 2 {
		t.Errorf("basis = %+v, want the group's rows and its two context files", ref.Basis)
	}
	if ref.Basis.Policy != "matched" || ref.Basis.Toolchain != "not-applicable" {
		t.Errorf("basis = %+v, want policy matched and toolchain not-applicable for a review group", ref.Basis)
	}

	adoptedRow := reuseDecisionFor(t, out.Mutation, "SC-10")
	if !adoptedRow.Reused || adoptedRow.Outcome != "unappliable" || adoptedRow.Reason != "" {
		t.Fatalf("SC-10 = %+v, want it reused with the prior outcome", adoptedRow)
	}
	if adoptedRow.Reference == nil || adoptedRow.Reference.Artifact.Path != row.EvidencePath {
		t.Fatalf("SC-10 reference = %+v, want the original evidence at %s", adoptedRow.Reference, row.EvidencePath)
	}
	if basis := adoptedRow.Reference.Basis; basis.Policy != "not-applicable" || basis.Toolchain != "matched" {
		t.Errorf("basis = %+v, want policy not-applicable and toolchain matched for a mutation row", basis)
	}
	if want := (reuseCounts{Reused: 2, ReusedFailed: 1}); out.Counts != want {
		t.Errorf("counts = %+v, want %+v: an unappliable row is a failed outcome", out.Counts, want)
	}

	resumed := runResume(t, repo, runID)
	rec := ledgerRecord(t, resumed, runstate.KindReviewGroup, "A")
	if want := "reused from " + priorRunID + " seq " + strconv.Itoa(priorGroupSeq); rec.Detail != want {
		t.Errorf("detail = %q, want %q", rec.Detail, want)
	}
	if got := rec.Artifacts[artifactEvidence].Path; got != group.EvidencePath {
		t.Errorf("adopted artifact path = %q, want the original %q", got, group.EvidencePath)
	}
	if got := rec.Artifacts[artifactEvidence].SHA256; got != ref.Artifact.SHA256 {
		t.Errorf("adopted artifact digest = %q, want the digest the reference verified, %q", got, ref.Artifact.SHA256)
	}
	inputs := recordedReviewInputs(t, rec)
	prior := readReviewEvidence(t, group.EvidencePath)
	if !reflect.DeepEqual(inputs.Manifest, prior.Inputs.Manifest) {
		t.Errorf("adopted manifest = %+v, want the prior artifact's %+v", inputs.Manifest, prior.Inputs.Manifest)
	}
	if inputs.Reuse == nil || inputs.Reuse.Basis.PriorCommit != priorHead {
		t.Errorf("adopted reuse = %+v, want the reference with its basis", inputs.Reuse)
	}
	rowRec := ledgerRecord(t, resumed, runstate.KindMutationRow, "SC-10")
	if got := rowRec.Artifacts[artifactEvidence].Path; got != row.EvidencePath {
		t.Errorf("adopted row artifact path = %q, want the original %q", got, row.EvidencePath)
	}

	reported := resumeGroupFor(t, resumed, "A")
	if reported.Status != string(runstate.StatusCompleted) || reported.Reason != "" {
		t.Errorf("resumed group = %+v, want it completed and valid", reported)
	}
	for _, pending := range resumed.Pending {
		if pending.ID == "A" || pending.ID == "SC-10" {
			t.Errorf("pending = %+v, want the adopted records never offered again", resumed.Pending)
		}
	}

	regroup := refuseRecordReview(t, repo, runID, map[string]any{
		"groupId": "B", "status": "started", "rows": []string{"SC-01"},
	})
	if regroup.Code != "row-already-grouped" {
		t.Errorf("record-review error code = %q, want row-already-grouped over an adopted row", regroup.Code)
	}

	again := runReuse(t, repo, runID, priorRunID, policy)
	if got := reuseDecisionFor(t, again.Review, "A"); got.Reused || got.Reason != "already-recorded" {
		t.Errorf("second reuse of group A = %+v, want it refused already-recorded", got)
	}
	if got := reuseDecisionFor(t, again.Mutation, "SC-10"); got.Reused || got.Reason != "already-recorded" {
		t.Errorf("second reuse of SC-10 = %+v, want it refused already-recorded", got)
	}
	if want := (reuseCounts{Refused: 2}); again.Counts != want {
		t.Errorf("counts = %+v, want %+v", again.Counts, want)
	}
}

// TestReuse_AnIsolatedTestChangeRefusesOnlyItsOwnGroup proves the invalidation
// boundary is read from the recorded manifests: a test file exactly one prior
// group was handed refuses that group and the mutation rows it reviewed, and
// nothing else. The group whose own context did not move is still adopted, and
// its basis names the change it was judged against.
func TestReuse_AnIsolatedTestChangeRefusesOnlyItsOwnGroup(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-isolated-prior"
	const runID = "reuse-isolated-new"
	const isolated = "orders/orders_test.go"
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-02", "SC-10")
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, "SC-01", "SC-02", "SC-10")
	startReviewGroupWith(t, repo, priorRunID, "B", policy, "SC-05", "SC-06")
	priorB := completeReviewGroupWith(t, repo, priorRunID, "B", priorDispatch, &[]string{}, "SC-05", "SC-06")
	unappliableRow(t, repo, priorRunID, "SC-10")

	appendToRepoFile(t, repo, isolated, "\n// an edit to the file, not to any declaration it holds\n")
	commitAll(t, repo, "edit a test file exactly one group was handed")

	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)

	changed := reuseChangedPathFor(t, out, isolated)
	if changed.Class != "isolated" || !slices.Equal(changed.Groups, []string{"A"}) {
		t.Errorf("%s = %+v, want it isolated to group A", isolated, changed)
	}
	want := "isolated-change:" + isolated
	if got := reuseDecisionFor(t, out.Review, "A"); got.Reused || got.Reason != want {
		t.Errorf("group A = %+v, want it refused %q", got, want)
	}
	if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != want {
		t.Errorf("SC-10 = %+v, want the row refused with its group's reason %q", got, want)
	}
	adopted := reuseDecisionFor(t, out.Review, "B")
	if !adopted.Reused || adopted.Reference == nil {
		t.Fatalf("group B = %+v, want a refused group never to block one whose context stands", adopted)
	}
	if adopted.Outcome != priorB.Outcome {
		t.Errorf("group B outcome = %q, want the prior run's %q", adopted.Outcome, priorB.Outcome)
	}
	if !slices.Equal(adopted.Reference.Basis.ChangedPaths, []string{isolated}) {
		t.Errorf("basis changedPaths = %v, want the other group's isolated file", adopted.Reference.Basis.ChangedPaths)
	}
	// Group B's verdicts answer a row the trace found no test for, so its prior
	// outcome is blocked: a failed outcome is adopted like any other and
	// counted, because the run owes the same answer either way.
	if want := (reuseCounts{Reused: 1, ReusedFailed: 1, Refused: 2}); out.Counts != want {
		t.Errorf("counts = %+v, want %+v", out.Counts, want)
	}
}

// TestReuse_AnUnownedChangeWidensToEveryRecord proves widening is total and
// answered per class: a production change, a test file two groups were handed,
// and a test file no manifest names each refuse every prior record — the ones
// whose own context never moved included — with the same reason naming the
// first such path in sorted order, whichever class that path belongs to. Only
// the coverage source's own directory is exempt from the question.
func TestReuse_AnUnownedChangeWidensToEveryRecord(t *testing.T) {
	const publication = "docs/superpowers/coverage/2026-08-30-fixture-report.md"
	const production = "orders/orders.go"
	const productionEdit = "\n// a production comment nobody reviewed\n"
	const unownedTest = "package orders\n\n// a helper file no prior manifest names\n"

	for _, tc := range []struct {
		name   string
		edit   func(t *testing.T, repo string)
		path   string
		class  string
		exempt string
		want   string
	}{
		{
			name: "a production change alone",
			edit: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, production, productionEdit)
			},
			path:  production,
			class: "shared",
			want:  "module-changed:" + production,
		},
		{
			name: "a test file two groups were handed",
			edit: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, "payments/payments_test.go", "\n// an edit to a file two groups share\n")
			},
			path:  "payments/payments_test.go",
			class: "shared",
			want:  "module-changed:payments/payments_test.go",
		},
		{
			name: "an unowned test file sorting before a production change",
			edit: func(t *testing.T, repo string) {
				writeRepoFile(t, repo, "orders/extra_test.go", unownedTest)
				appendToRepoFile(t, repo, production, productionEdit)
				writeRepoFile(t, repo, publication, "# published report\n")
			},
			path:   "orders/extra_test.go",
			class:  "unknown",
			exempt: publication,
			want:   "unknown-ownership:orders/extra_test.go",
		},
		{
			name: "a production change sorting before an unowned test file",
			edit: func(t *testing.T, repo string) {
				appendToRepoFile(t, repo, production, productionEdit)
				writeRepoFile(t, repo, "orders/zz_extra_test.go", unownedTest)
			},
			path:  production,
			class: "shared",
			want:  "module-changed:" + production,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := fixtureRepo(t)
			const priorRunID = "reuse-widen-prior"
			const runID = "reuse-widen-new"
			policy := reusePolicy(t)

			priorDispatch := tracedDispatch(t, repo, priorRunID)
			startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-02", "SC-10")
			completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, "SC-01", "SC-02", "SC-10")
			startReviewGroupWith(t, repo, priorRunID, "B", policy, "SC-05", "SC-06")
			completeReviewGroupWith(t, repo, priorRunID, "B", priorDispatch, &[]string{}, "SC-05", "SC-06")
			unappliableRow(t, repo, priorRunID, "SC-10")

			tc.edit(t, repo)
			commitAll(t, repo, "change the repository under the prior run")

			tracedDispatch(t, repo, runID)
			out := runReuse(t, repo, runID, priorRunID, policy)

			if got := reuseChangedPathFor(t, out, tc.path).Class; got != tc.class {
				t.Errorf("%s class = %q, want %q", tc.path, got, tc.class)
			}
			if tc.exempt != "" {
				if got := reuseChangedPathFor(t, out, tc.exempt).Class; got != "exempt" {
					t.Errorf("%s class = %q, want exempt", tc.exempt, got)
				}
			}
			for _, id := range []string{"A", "B"} {
				if got := reuseDecisionFor(t, out.Review, id); got.Reused || got.Reason != tc.want {
					t.Errorf("group %s = %+v, want it refused %q", id, got, tc.want)
				}
			}
			if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != tc.want {
				t.Errorf("SC-10 = %+v, want it refused %q", got, tc.want)
			}
			if want := (reuseCounts{Refused: 3}); out.Counts != want {
				t.Errorf("counts = %+v, want %+v", out.Counts, want)
			}
		})
	}
}

// TestReuse_RefusesPerRecordWhatItCannotProveUnchanged holds the per-record
// judgement at one commit, where nothing in the repository moved: a group the
// prior run never closed, one whose manifest never recorded what its critic
// read, a request that names no policy, a mutation row no group reviewed, a
// completed row whose outcome rests on no file at all, and a row the new run
// has already grouped are each refused on their own, in one output the op still
// exits zero on.
func TestReuse_RefusesPerRecordWhatItCannotProveUnchanged(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-per-record-prior"
	const runID = "reuse-per-record-new"
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-02")
	startReviewGroupWith(t, repo, priorRunID, "B", policy, "SC-05", "SC-06")
	completeReviewGroupWith(t, repo, priorRunID, "B", priorDispatch, &[]string{}, "SC-05", "SC-06")
	startReviewGroupWith(t, repo, priorRunID, "D", policy, "SC-03")
	completeReviewGroupWith(t, repo, priorRunID, "D", priorDispatch, nil, "SC-03")
	unappliableRow(t, repo, priorRunID, "SC-10")
	completeRowWithoutArtifact(t, repo, priorRunID, "SC-07")

	tracedDispatch(t, repo, runID)

	unspecified := runReuse(t, repo, runID, priorRunID, nil)
	if len(unspecified.ChangedPaths) != 0 {
		t.Errorf("changedPaths = %+v, want none at the commit the prior run was made on", unspecified.ChangedPaths)
	}
	if got := reuseDecisionFor(t, unspecified.Review, "A"); got.Reused || got.Reason != "prior-not-completed" {
		t.Errorf("group A = %+v, want it refused prior-not-completed", got)
	}
	if got := reuseDecisionFor(t, unspecified.Review, "B"); got.Reused || got.Reason != "policy-unspecified" {
		t.Errorf("group B = %+v, want a policy-less request to refuse it policy-unspecified", got)
	}
	if got := reuseDecisionFor(t, unspecified.Review, "D"); got.Reused || got.Reason != "manifest-incomplete:reads-unrecorded" {
		t.Errorf("group D = %+v, want it refused manifest-incomplete:reads-unrecorded", got)
	}
	if got := reuseDecisionFor(t, unspecified.Mutation, "SC-10"); got.Reused || got.Reason != "no-review-context" {
		t.Errorf("SC-10 = %+v, want it refused no-review-context: no prior group reviewed the row", got)
	}
	if got := reuseDecisionFor(t, unspecified.Mutation, "SC-07"); got.Reused || got.Reason != "artifact-missing:evidence" {
		t.Errorf("SC-07 = %+v, want a completed row pointing at nothing refused artifact-missing:evidence", got)
	}
	if want := (reuseCounts{Refused: 5}); unspecified.Counts != want {
		t.Errorf("counts = %+v, want %+v", unspecified.Counts, want)
	}

	startReviewGroup(t, repo, runID, "C", "SC-05")
	grouped := runReuse(t, repo, runID, priorRunID, policy)
	if got := reuseDecisionFor(t, grouped.Review, "B"); got.Reused || got.Reason != "row-already-grouped:SC-05" {
		t.Errorf("group B = %+v, want it refused row-already-grouped:SC-05", got)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want reuse to leave the repository untouched", lines)
	}
}

// pinToolchain fixes the tool versions the trace records, so a test can move a
// run between toolchains without one installed. Nested calls unwind in order.
func pinToolchain(t *testing.T, version string) {
	t.Helper()
	previous := toolVersionsFor
	toolVersionsFor = func(string, string, []string) map[string]string {
		return map[string]string{"go": version}
	}
	t.Cleanup(func() { toolVersionsFor = previous })
}

// completeRowWithoutArtifact leaves a completed mutation-row record whose
// outcome rests on no file at all — the shape a ledger written by a future op
// that forgot its evidence would carry.
func completeRowWithoutArtifact(t *testing.T, repo, runID, rowID string) {
	t.Helper()
	req := fixtureVerifyRequest(repo, runID)
	store, cliErr := openLedger("test", req)
	if cliErr != nil {
		t.Fatalf("openLedger: %v", cliErr.Message)
	}
	src, cliErr := loadVerifiedSource("test", repo, fixtureCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr.Message)
	}
	commit, cliErr := headCommit("test", repo)
	if cliErr != nil {
		t.Fatalf("headCommit: %v", cliErr.Message)
	}
	err := store.Update(ledgerIdentity(req, repo, commit, src), func(l *runstate.Ledger) error {
		if _, err := l.Start(runstate.KindMutationRow, rowID, mutationRowInputs{RunnerKind: src.Runner.Kind}, "", ""); err != nil {
			return err
		}
		_, err := l.Complete(runstate.KindMutationRow, rowID, "caught", nil, "")
		return err
	})
	if err != nil {
		t.Fatalf("complete %s without an artifact: %v", rowID, err)
	}
}

// TestReuse_OwnershipComesFromEveryRecordedManifest proves a change is
// attributed from every manifest the prior run recorded, not only from the ones
// whose records can be adopted. A file a completed group and a group that was
// never closed were both handed belongs to neither alone, so it widens — and
// the group whose own context never moved is refused with it, rather than
// adopted behind a boundary the ledger never proved.
func TestReuse_OwnershipComesFromEveryRecordedManifest(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-ownership-prior"
	const runID = "reuse-ownership-new"
	const shared = "payments/payments_test.go"
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-05")
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, "SC-01", "SC-05")
	// B is handed the same file and is never closed: its bundle lives only in
	// the record it started.
	startReviewGroupWith(t, repo, priorRunID, "B", policy, "SC-06")
	startReviewGroupWith(t, repo, priorRunID, "C", policy, "SC-09")
	completeReviewGroupWith(t, repo, priorRunID, "C", priorDispatch, &[]string{}, "SC-09")

	appendToRepoFile(t, repo, shared, "\n// an edit to a file a closed and an open group share\n")
	commitAll(t, repo, "edit a test file two groups were handed")

	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)

	changed := reuseChangedPathFor(t, out, shared)
	if changed.Class != "shared" {
		t.Errorf("%s class = %q, want shared: two prior groups were handed it", shared, changed.Class)
	}
	if !slices.Contains(changed.Groups, "A") || !slices.Contains(changed.Groups, "B") {
		t.Errorf("%s groups = %v, want both the completed and the started group", shared, changed.Groups)
	}
	want := "module-changed:" + shared
	for _, id := range []string{"A", "C"} {
		if got := reuseDecisionFor(t, out.Review, id); got.Reused || got.Reason != want {
			t.Errorf("group %s = %+v, want it refused %q", id, got, want)
		}
	}
	if got := reuseDecisionFor(t, out.Review, "B"); got.Reused || got.Reason != "prior-not-completed" {
		t.Errorf("group B = %+v, want it refused prior-not-completed", got)
	}
	if want := (reuseCounts{Refused: 3}); out.Counts != want {
		t.Errorf("counts = %+v, want %+v", out.Counts, want)
	}
}

// TestReuse_AMutationRowIsJudgedOnTheToolchainThatRanIt follows a row across
// three runs: one reviewed under a toolchain, one that adopted that review and
// measured the row under another, and one back on the first. The row was
// measured under the second, so the third must not accept it on the strength of
// the review's older manifest — and when the run that measured it can no longer
// say which toolchain it used, the row is refused rather than assumed.
func TestReuse_AMutationRowIsJudgedOnTheToolchainThatRanIt(t *testing.T) {
	repo := fixtureRepo(t)
	const reviewed = "reuse-toolchain-reviewed"
	const measured = "reuse-toolchain-measured"
	const asking = "reuse-toolchain-asking"
	rows := []string{"SC-01", "SC-02", "SC-10"}
	policy := reusePolicy(t)

	pinToolchain(t, "go version go1.0.0")
	priorDispatch := tracedDispatch(t, repo, reviewed)
	startReviewGroupWith(t, repo, reviewed, "A", policy, rows...)
	completeReviewGroupWith(t, repo, reviewed, "A", priorDispatch, &[]string{}, rows...)

	pinToolchain(t, "go version go2.0.0")
	tracedDispatch(t, repo, measured)
	adopted := runReuse(t, repo, measured, reviewed, policy)
	if got := reuseDecisionFor(t, adopted.Review, "A"); !got.Reused {
		t.Fatalf("group A = %+v, want a review adopted across a toolchain change", got)
	}
	unappliableRow(t, repo, measured, "SC-10")

	pinToolchain(t, "go version go1.0.0")
	tracedDispatch(t, repo, asking)
	out := runReuse(t, repo, asking, measured, policy)

	if got := reuseDecisionFor(t, out.Review, "A"); !got.Reused {
		t.Errorf("group A = %+v, want the review still adopted: a critic reads source, it does not run it", got)
	}
	if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != "toolchain-changed" {
		t.Errorf("SC-10 = %+v, want it refused toolchain-changed: it was measured under the other toolchain", got)
	}

	// With the run that measured the row unable to say what it ran under, the
	// row is not judged at all.
	if err := os.Remove(runResume(t, repo, measured).Trace.DispatchPath); err != nil {
		t.Fatalf("remove the measuring run's dispatch document: %v", err)
	}
	unrecorded := runReuse(t, repo, asking, measured, policy)
	if got := reuseDecisionFor(t, unrecorded.Mutation, "SC-10"); got.Reused || got.Reason != "toolchain-unrecorded" {
		t.Errorf("SC-10 = %+v, want it refused toolchain-unrecorded", got)
	}
}

// TestReuse_RefusesAPolicyInputItCannotIdentify keeps a partial policy from
// passing for a matching one: a request naming an instruction file this run
// cannot read describes a critic whose inputs nobody can compare, and the
// digests it did compute would match a prior policy that never named the
// unreadable file at all.
func TestReuse_RefusesAPolicyInputItCannotIdentify(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-policy-prior"
	const runID = "reuse-policy-new"
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-02")
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, "SC-01", "SC-02")
	tracedDispatch(t, repo, runID)

	missing := filepath.Join(t.TempDir(), "second-critic.md")
	widened := &reviewPolicy{Critic: policy.Critic, Instructions: append(slices.Clone(policy.Instructions), missing)}
	out := runReuse(t, repo, runID, priorRunID, widened)
	if want := "policy-unreadable:" + missing; reuseDecisionFor(t, out.Review, "A").Reason != want {
		t.Errorf("group A = %+v, want it refused %q", reuseDecisionFor(t, out.Review, "A"), want)
	}

	// The same request naming only what this run can identify adopts, so the
	// refusal above is the unreadable instruction and nothing else.
	identified := runReuse(t, repo, runID, priorRunID, policy)
	if got := reuseDecisionFor(t, identified.Review, "A"); !got.Reused {
		t.Errorf("group A = %+v, want it adopted under the policy the prior run recorded", got)
	}
}

// pinToolchainMap fixes the whole tool-version map the trace records, so a
// test can present a run whose version probes answered nothing at all. Nested
// calls unwind in order.
func pinToolchainMap(t *testing.T, versions map[string]string) {
	t.Helper()
	previous := toolVersionsFor
	toolVersionsFor = func(string, string, []string) map[string]string {
		return maps.Clone(versions)
	}
	t.Cleanup(func() { toolVersionsFor = previous })
}

// subdirectoryFixtureRepo is the go fixture with its module moved under sub/
// and the coverage document naming sub as the runner root, so every path the
// trace binds is relative to a directory below the repository root.
func subdirectoryFixtureRepo(t *testing.T) string {
	t.Helper()
	return fixtureRepoWith(t, func(repo string) {
		if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
			t.Fatalf("mkdir sub: %v", err)
		}
		for _, name := range []string{"go.mod", "orders", "payments"} {
			if err := os.Rename(filepath.Join(repo, name), filepath.Join(repo, "sub", name)); err != nil {
				t.Fatalf("move %s under sub: %v", name, err)
			}
		}
		rewriteFixtureDoc(t, repo, func(doc string) string {
			return strings.Replace(doc, "- runner-root: .", "- runner-root: sub", 1)
		})
	})
}

// TestReuse_ARunnerRootBelowTheRepositoryKeysTheBundleByRepositoryPath holds
// the bundle to the repository's own paths. The trace binds a test relative to
// the runner root, but a manifest is compared against git's repo-relative
// paths, so a bundle keyed by the runner-relative file would digest nothing
// under a runner root other than "." and classify every change as unowned.
func TestReuse_ARunnerRootBelowTheRepositoryKeysTheBundleByRepositoryPath(t *testing.T) {
	repo := subdirectoryFixtureRepo(t)
	const priorRunID = "reuse-runner-root-prior"
	const runID = "reuse-runner-root-new"
	const ordersTest = "sub/orders/orders_test.go"
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	if got := dispatchRowFor(t, priorDispatch, "SC-01").File; got != "orders/orders_test.go" {
		t.Fatalf("dispatch file = %q, want the trace to bind relative to the runner root", got)
	}
	started := startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-02", "SC-10")
	paths := []string{}
	for _, file := range started.Bundle {
		paths = append(paths, file.Path)
		want, err := evidence.SHA256File(filepath.Join(repo, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Errorf("bundle names %s, which the repository does not hold: %v", file.Path, err)
			continue
		}
		if file.SHA256 != want {
			t.Errorf("bundle[%s] = %q, want the real file's digest %q", file.Path, file.SHA256, want)
		}
	}
	if !slices.Contains(paths, ordersTest) {
		t.Errorf("bundle paths = %v, want %s among them, keyed by its repository path", paths, ordersTest)
	}
	if !slices.Equal(started.ManifestIncomplete, []string{"reads-unrecorded"}) {
		t.Errorf("started manifestIncomplete = %v, want only reads-unrecorded: every bundled file is identifiable", started.ManifestIncomplete)
	}
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, "SC-01", "SC-02", "SC-10")
	startReviewGroupWith(t, repo, priorRunID, "B", policy, "SC-05", "SC-06")
	completeReviewGroupWith(t, repo, priorRunID, "B", priorDispatch, &[]string{}, "SC-05", "SC-06")

	appendToRepoFile(t, repo, ordersTest, "\n// an edit to the file, not to any declaration it holds\n")
	commitAll(t, repo, "edit a test file exactly one group was handed, below the runner root")

	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)

	changed := reuseChangedPathFor(t, out, ordersTest)
	if changed.Class != changeIsolated || !slices.Equal(changed.Groups, []string{"A"}) {
		t.Errorf("%s = %+v, want it isolated to group A", ordersTest, changed)
	}
	want := "isolated-change:" + ordersTest
	if got := reuseDecisionFor(t, out.Review, "A"); got.Reused || got.Reason != want {
		t.Errorf("group A = %+v, want it refused %q", got, want)
	}
	if got := reuseDecisionFor(t, out.Review, "B"); !got.Reused {
		t.Errorf("group B = %+v, want it adopted: nothing it was handed moved", got)
	}
}

// TestReuse_AMutationRowIsJudgedOnTheTraceThatPrecededIt pins which trace
// speaks for a row's toolchain: the one recorded before the row, not the run's
// latest. A row measured under one toolchain, followed by a repeated trace
// under another, was still measured under the first, so a run on the second
// toolchain may not adopt it as matched.
func TestReuse_AMutationRowIsJudgedOnTheTraceThatPrecededIt(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-preceding-trace-prior"
	const runID = "reuse-preceding-trace-new"
	rows := []string{"SC-01", "SC-02", "SC-10"}
	policy := reusePolicy(t)

	pinToolchain(t, "go version go1.0.0")
	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, rows...)
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, rows...)
	unappliableRow(t, repo, priorRunID, "SC-10")

	pinToolchain(t, "go version go2.0.0")
	code, _, stderr := runVerifyOp(t, "trace", requestWith(t, repo, priorRunID, map[string]any{
		"repeat": map[string]any{"reason": "the toolchain was upgraded after the row was measured"},
	}))
	if code != exitOK {
		t.Fatalf("repeat trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)

	if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != "toolchain-changed" {
		t.Errorf("SC-10 = %+v, want it refused toolchain-changed: the trace preceding the row recorded go1.0.0 and still verifies, and this run measures under go2.0.0", got)
	}
}

// TestReuse_AnUnidentifiedToolchainNeverMatchesAnother keeps two failed
// version probes from agreeing with each other: a prior run whose trace
// recorded no tool version at all, or a run asking that recorded none, has
// nothing to compare, and nothing compared is not a match.
func TestReuse_AnUnidentifiedToolchainNeverMatchesAnother(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-empty-toolchain-prior"
	rows := []string{"SC-01", "SC-02", "SC-10"}
	policy := reusePolicy(t)

	pinToolchainMap(t, map[string]string{})
	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, rows...)
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, rows...)
	unappliableRow(t, repo, priorRunID, "SC-10")

	t.Run("both unidentified", func(t *testing.T) {
		const runID = "reuse-empty-toolchain-both"
		tracedDispatch(t, repo, runID)
		out := runReuse(t, repo, runID, priorRunID, policy)
		if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != "toolchain-unrecorded" {
			t.Errorf("SC-10 = %+v, want it refused toolchain-unrecorded: two empty toolchains are not the same toolchain", got)
		}
		if got := reuseDecisionFor(t, out.Review, "A"); !got.Reused {
			t.Errorf("group A = %+v, want the review still adopted: a critic reads source, it does not run it", got)
		}
	})

	t.Run("only the prior unidentified", func(t *testing.T) {
		const runID = "reuse-empty-toolchain-prior-only"
		pinToolchain(t, "go version go1.0.0")
		tracedDispatch(t, repo, runID)
		out := runReuse(t, repo, runID, priorRunID, policy)
		if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != "toolchain-unrecorded" {
			t.Errorf("SC-10 = %+v, want it refused toolchain-unrecorded", got)
		}
	})
}

// TestReuse_RefusesADirtyWorkingTreeBeforeJudgingAnything holds reuse to the
// same gate every other op that reads the repository answers first: an
// uncommitted change is a tree no comparison against a commit describes, so
// nothing is judged and nothing is adopted.
func TestReuse_RefusesADirtyWorkingTreeBeforeJudgingAnything(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-dirty-prior"
	const runID = "reuse-dirty-new"
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, "SC-01", "SC-02")
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, "SC-01", "SC-02")
	tracedDispatch(t, repo, runID)

	writeRepoFile(t, repo, "orders/scratch.txt", "uncommitted\n")
	t.Cleanup(func() { mustGit(t, repo, "clean", "-qfd") })

	got := refuseReuse(t, repo, runID, priorRunID, policy)
	if got.Code != "source-not-reproducible" || !strings.Contains(got.Message, "orders/scratch.txt") {
		t.Errorf("refusal = %+v, want source-not-reproducible naming orders/scratch.txt", got)
	}
	for _, rec := range runResume(t, repo, runID).Records {
		if rec.Kind == runstate.KindReviewGroup {
			t.Errorf("ledger holds %s %s, want nothing adopted over a dirty tree", rec.Kind, rec.ID)
		}
	}
}

// stateDirectories names every directory under the verifier's state root, so
// a test can hold an op to creating none.
func stateDirectories(t *testing.T) []string {
	t.Helper()
	root := os.Getenv(runstate.EnvStateDir)
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk state root %s: %v", root, err)
	}
	return dirs
}

// TestReuse_AMissingPriorLedgerLeavesTheStateRootUntouched keeps a typo from
// leaving a trace: asking to stand on a run that never existed is answered
// from what is on disk, without opening a ledger directory for it.
func TestReuse_AMissingPriorLedgerLeavesTheStateRootUntouched(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "reuse-missing-prior-untouched"
	tracedDispatch(t, repo, runID)
	before := stateDirectories(t)

	if got := refuseReuse(t, repo, runID, "never-ran", nil); got.Code != "prior-ledger-missing" {
		t.Errorf("error code = %q, want prior-ledger-missing", got.Code)
	}
	if after := stateDirectories(t); !slices.Equal(after, before) {
		t.Errorf("state root directories = %v, want the %v it held before: a missing prior ledger is not created", after, before)
	}
}

// TestReuse_ARowRecordedWithoutAnIdentityIsRefusedByName covers a row that
// was answered without ever being prepared: record-attempt opens it with no
// bound test, so nothing about its identity can be compared, which is not the
// same as an identity that compared differently.
func TestReuse_ARowRecordedWithoutAnIdentityIsRefusedByName(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-no-identity-prior"
	const runID = "reuse-no-identity-new"
	rows := []string{"SC-01", "SC-02", "SC-10"}
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, rows...)
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, rows...)
	code, _, stderr := runVerifyOp(t, "record-attempt", requestWith(t, repo, priorRunID, map[string]any{
		"layer": 4, "rowID": "SC-10", "refusal": "the mutation runner could not start in this environment",
	}))
	if code != exitOK {
		t.Fatalf("record-attempt exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	tracedDispatch(t, repo, runID)
	out := runReuse(t, repo, runID, priorRunID, policy)

	if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != "identity-unrecorded" {
		t.Errorf("SC-10 = %+v, want it refused identity-unrecorded: the row was never prepared against a bound test", got)
	}
}

// breakLedgerRecordInputs damages one field of every kind record's captured
// inputs in the ledger at path, leaving the checkpoint valid JSON and every
// other field in place, and returns the original bytes so a test can put them
// back. Each kind is damaged in a field only its own shape declares.
func breakLedgerRecordInputs(t *testing.T, path string, kind runstate.Kind) []byte {
	t.Helper()
	damage := map[runstate.Kind]func(map[string]any){
		runstate.KindReviewGroup: func(inputs map[string]any) { inputs["rows"] = "SC-01" },
		runstate.KindMutationRow: func(inputs map[string]any) { inputs["runnerKind"] = 7 },
	}[kind]
	if damage == nil {
		t.Fatalf("breakLedgerRecordInputs: no damage is defined for kind %s", kind)
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(original, &doc); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	records, _ := doc["records"].([]any)
	broken := 0
	for _, entry := range records {
		rec, _ := entry.(map[string]any)
		if rec["kind"] != string(kind) {
			continue
		}
		inputs, ok := rec["inputs"].(map[string]any)
		if !ok {
			continue
		}
		damage(inputs)
		broken++
	}
	if broken == 0 {
		t.Fatalf("ledger %s holds no %s record with captured inputs", path, kind)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode ledger: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
	return original
}

// fixtureLedgerPath is where a fixture run's checkpoint lives.
func fixtureLedgerPath(t *testing.T, repo, runID string) string {
	t.Helper()
	store, cliErr := openLedger("test", fixtureVerifyRequest(repo, runID))
	if cliErr != nil {
		t.Fatalf("openLedger: %v", cliErr.Message)
	}
	return filepath.Join(store.Dir(), runstate.LedgerFile)
}

// TestReuse_ARecordWhoseInputsWillNotDecodeIsRefusedByName tells a record
// this executable cannot read apart from one written before manifests existed:
// the first is a checkpoint nothing can be told about, the second is history.
func TestReuse_ARecordWhoseInputsWillNotDecodeIsRefusedByName(t *testing.T) {
	repo := fixtureRepo(t)
	const priorRunID = "reuse-unreadable-record-prior"
	rows := []string{"SC-01", "SC-02", "SC-10"}
	policy := reusePolicy(t)

	priorDispatch := tracedDispatch(t, repo, priorRunID)
	startReviewGroupWith(t, repo, priorRunID, "A", policy, rows...)
	completeReviewGroupWith(t, repo, priorRunID, "A", priorDispatch, &[]string{}, rows...)
	unappliableRow(t, repo, priorRunID, "SC-10")
	ledgerPath := fixtureLedgerPath(t, repo, priorRunID)

	t.Run("review group", func(t *testing.T) {
		const runID = "reuse-unreadable-record-review"
		original := breakLedgerRecordInputs(t, ledgerPath, runstate.KindReviewGroup)
		t.Cleanup(func() {
			if err := os.WriteFile(ledgerPath, original, 0o644); err != nil {
				t.Fatalf("restore ledger: %v", err)
			}
		})
		tracedDispatch(t, repo, runID)
		out := runReuse(t, repo, runID, priorRunID, policy)
		if got := reuseDecisionFor(t, out.Review, "A"); got.Reused || got.Reason != "record-unreadable" {
			t.Errorf("group A = %+v, want it refused record-unreadable", got)
		}
	})

	t.Run("mutation row", func(t *testing.T) {
		const runID = "reuse-unreadable-record-row"
		original := breakLedgerRecordInputs(t, ledgerPath, runstate.KindMutationRow)
		t.Cleanup(func() {
			if err := os.WriteFile(ledgerPath, original, 0o644); err != nil {
				t.Fatalf("restore ledger: %v", err)
			}
		})
		tracedDispatch(t, repo, runID)
		out := runReuse(t, repo, runID, priorRunID, policy)
		if got := reuseDecisionFor(t, out.Mutation, "SC-10"); got.Reused || got.Reason != "record-unreadable" {
			t.Errorf("SC-10 = %+v, want it refused record-unreadable", got)
		}
	})
}
