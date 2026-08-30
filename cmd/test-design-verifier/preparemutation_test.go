package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/mutation"
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
