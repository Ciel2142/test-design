package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/mutation"
	"test-design/internal/runstate"
)

func decodeCLIError(t *testing.T, stderr string) cliError {
	t.Helper()
	var got cliError
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		t.Fatalf("stderr did not decode as JSON error: %v\nstderr: %s", err, stderr)
	}
	return got
}

func prepareMutationRequestJSON(t *testing.T, repo, runID, rowID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          runID,
		"rowID":          rowID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func TestPrepareMutation_GoFixtureHappyPath(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-happy", "SC-02"))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out prepareMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	if out.RowID != "SC-02" {
		t.Errorf("rowID = %q, want SC-02", out.RowID)
	}
	if out.RunnerKind != "go" {
		t.Errorf("runnerKind = %q, want go", out.RunnerKind)
	}
	if out.Identity.Scope != "fixturemod/orders" || out.Identity.Name != "TestValidateQuantityRejectsZero" {
		t.Errorf("identity = %+v, want fixturemod/orders.TestValidateQuantityRejectsZero", out.Identity)
	}
	if out.WorktreePath == "" {
		t.Error("worktreePath is empty, want the created worktree's path")
	}
	if out.RunDir == "" {
		t.Error("runDir is empty, want the allocated run directory")
	}

	if info, err := os.Stat(out.WorktreePath); err != nil || !info.IsDir() {
		t.Errorf("worktree at %s does not exist after success, want it to survive: %v", out.WorktreePath, err)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want exactly one surviving worktree", left)
	}

	st, err := mutation.LoadState(out.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.RowID != out.RowID {
		t.Errorf("state.RowID = %q, want %q", st.RowID, out.RowID)
	}
	if st.Identity.Scope != out.Identity.Scope || st.Identity.Name != out.Identity.Name {
		t.Errorf("state.Identity = %+v, want %+v", st.Identity, out.Identity)
	}
	if st.WorktreePath != out.WorktreePath {
		t.Errorf("state.WorktreePath = %q, want %q", st.WorktreePath, out.WorktreePath)
	}
	if len(st.Snapshot) == 0 {
		t.Error("state.Snapshot is empty, want the before snapshot recorded")
	}
	if len(st.Sites) != 0 {
		t.Errorf("state.Sites = %v, want empty on prepare", st.Sites)
	}

	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
}

func TestPrepareMutation_RowNotFoundUnknownDisposesWorktree(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-unknown", "SC-999"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	if !strings.Contains(stderr, "SC-999") || !strings.Contains(stderr, fixtureCoverageSource) {
		t.Errorf("stderr = %q, want it to name the unknown row and the source path", stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "row-not-found" {
		t.Errorf("error code = %q, want row-not-found", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareMutation_RowNotFoundRetired(t *testing.T) {
	repo := fixtureRepo(t)

	code, _, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-retired", "SC-90"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "SC-90") || !strings.Contains(stderr, "retired") {
		t.Errorf("stderr = %q, want it to name the retired row", stderr)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "row-not-found" {
		t.Errorf("error code = %q, want row-not-found", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareMutation_RowNotBoundAmbiguousCitation(t *testing.T) {
	repo := fixtureRepo(t)

	code, _, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-ambiguous", "SC-01"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "row-not-bound" {
		t.Errorf("error code = %q, want row-not-bound", got.Code)
	}
	if !strings.Contains(stderr, "SC-01") {
		t.Errorf("stderr = %q, want it to name the ambiguous row", stderr)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareMutation_RowNotBoundNoCitation(t *testing.T) {
	repo := fixtureRepo(t)
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.Replace(s,
			"| SC-01 | quantity at the valid lower bound is accepted | ValidateQuantity(5) returns nil | unit | guard rejects any qty > 0 | low |",
			"| SC-01 | quantity at the valid lower bound is accepted | ValidateQuantity(5) returns nil | unit | guard rejects any qty > 0 | low |\n"+
				"| SC-15 | scenario added with no citing test | n/a | unit | n/a | low |",
			1)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "add an uncited scenario row")

	code, _, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-uncited", "SC-15"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "row-not-bound" {
		t.Errorf("error code = %q, want row-not-bound", got.Code)
	}
	if !strings.Contains(stderr, "SC-15") {
		t.Errorf("stderr = %q, want it to name the uncited row", stderr)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareMutation_DirtyRepositoryRefused(t *testing.T) {
	repo := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "orders", "scratch.txt"), []byte("dirt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-dirty", "SC-02"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "dirty-repository" {
		t.Errorf("error code = %q, want dirty-repository", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want no worktree created before the dirty refusal", left)
	}
}

func TestPrepareMutation_MissingRowIDRefused(t *testing.T) {
	repo := fixtureRepo(t)
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          "prep-no-rowid",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	code, _, stderr := runVerifyOp(t, "prepare-mutation", string(body))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "missing-row-id" {
		t.Errorf("error code = %q, want missing-row-id", got.Code)
	}
}

// completeLedgerRow closes an already-started mutation row in the run's real
// ledger the way a finished evaluation does, so a test can drive an op that
// meets a row this run has already answered.
func completeLedgerRow(t *testing.T, repo, runID, rowID, outcome string) {
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
	if _, err := store.Complete(ledgerIdentity(req, repo, commit, src), runstate.KindMutationRow, rowID, outcome, nil, ""); err != nil {
		t.Fatalf("complete mutation-row/%s: %v", rowID, err)
	}
}

// answeredMutationRow opens and closes rowID's obligation without preparing a
// workspace: the ledger standing of a row this run already measured.
func answeredMutationRow(t *testing.T, repo, runID, rowID, outcome string) {
	t.Helper()
	startLedgerRecord(t, repo, runID, runstate.KindMutationRow, rowID, mutationRowInputs{RunnerKind: "go"})
	completeLedgerRow(t, repo, runID, rowID, outcome)
}

func TestPrepareMutation_OpensTheRowWithItsPreparedWorkspace(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "prep-opens-row", "SC-09")

	rec := ledgerRecord(t, runResume(t, repo, "prep-opens-row"), runstate.KindMutationRow, "SC-09")
	if rec.Status != runstate.StatusStarted {
		t.Errorf("mutation-row/SC-09 status = %q, want %q", rec.Status, runstate.StatusStarted)
	}
	var inputs mutationRowInputs
	if err := recordInputs(&rec, &inputs); err != nil {
		t.Fatalf("decode record inputs: %v", err)
	}
	want := mutationRowInputs{
		Identity:     mutationRowIdentity{Scope: prep.Identity.Scope, Name: prep.Identity.Name},
		RunnerKind:   prep.RunnerKind,
		RunDir:       prep.RunDir,
		WorktreePath: prep.WorktreePath,
	}
	if inputs != want {
		t.Errorf("mutation-row/SC-09 inputs = %+v, want %+v", inputs, want)
	}
}

func TestPrepareMutation_ACompletedRowIsRefusedBeforeAnyWorktree(t *testing.T) {
	repo := fixtureRepo(t)
	answeredMutationRow(t, repo, "prep-completed", "SC-09", "caught")

	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, "prep-completed", "SC-09"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "mutation-row-completed" {
		t.Errorf("error code = %q, want mutation-row-completed", got.Code)
	}
	if !strings.Contains(got.Message, "SC-09") {
		t.Errorf("message = %q, want it to name the answered row", got.Message)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want nothing created for a row this run already answered", left)
	}
}

func TestPrepareMutation_ARepeatReopensACompletedRow(t *testing.T) {
	repo := fixtureRepo(t)
	answeredMutationRow(t, repo, "prep-repeat", "SC-09", "survived")

	const reason = "the guard was rewritten; measure it again"
	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", requestWith(t, repo, "prep-repeat", map[string]any{
		"rowID":  "SC-09",
		"repeat": map[string]any{"reason": reason},
	}))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out prepareMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if info, err := os.Stat(out.WorktreePath); err != nil || !info.IsDir() {
		t.Errorf("worktree at %s does not exist after the repeat: %v", out.WorktreePath, err)
	}

	resumed := runResume(t, repo, "prep-repeat")
	rec := ledgerRecord(t, resumed, runstate.KindMutationRow, "SC-09")
	if rec.Status != runstate.StatusStarted {
		t.Errorf("current mutation-row/SC-09 status = %q, want %q", rec.Status, runstate.StatusStarted)
	}
	var superseded *runstate.Record
	for i := range resumed.Records {
		if resumed.Records[i].Status == runstate.StatusSuperseded {
			superseded = &resumed.Records[i]
		}
	}
	if superseded == nil {
		t.Fatalf("records = %+v, want the answered row superseded by the repeat", resumed.Records)
	}
	if superseded.Detail != reason {
		t.Errorf("superseded detail = %q, want the repeat reason %q", superseded.Detail, reason)
	}
	if superseded.Outcome != "survived" {
		t.Errorf("superseded outcome = %q, want the answer it already carried", superseded.Outcome)
	}
}
