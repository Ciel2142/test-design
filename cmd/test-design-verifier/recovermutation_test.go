package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/mutation"
	"test-design/internal/worktree"
)

func recoverMutationRequestJSON(t *testing.T, repo, runID, runDir string) string {
	t.Helper()
	return requestWith(t, repo, runID, map[string]any{"runDir": runDir})
}

func runRecoverMutation(t *testing.T, repo, runID, runDir string) recoverMutationOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "recover-mutation", recoverMutationRequestJSON(t, repo, runID, runDir))
	if code != exitOK {
		t.Fatalf("recover-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out recoverMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("recover-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// refuseRecoverMutation drives the op expecting a refusal and returns it.
func refuseRecoverMutation(t *testing.T, repo, runID, runDir string) cliError {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "recover-mutation", recoverMutationRequestJSON(t, repo, runID, runDir))
	if code != exitError {
		t.Fatalf("recover-mutation exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	return decodeCLIError(t, stderr)
}

// interruptedRow prepares SC-09 and leaves the state a killed evaluation
// leaves: one finished site and the marker of a site that never completed.
func interruptedRow(t *testing.T, repo, runID string) prepareMutationOutput {
	t.Helper()
	prep := preparedMutation(t, repo, runID, "SC-09")
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.Sites = append(st.Sites, mutation.SiteOutcome{
		Site:              "site-a",
		Outcome:           "survived",
		DiffPath:          filepath.Join(prep.RunDir, "site-1.diff"),
		MutatedRunDir:     filepath.Join(prep.RunDir, "site-1-mutated"),
		MutatedExitStatus: 0,
		CleanRunDir:       filepath.Join(prep.RunDir, "site-1-clean"),
		CleanExitStatus:   0,
	})
	st.Started = &mutation.SiteAttempt{Site: "site-b", Phase: "mutated", StartedAt: "2026-09-07T10:00:00Z"}
	if err := mutation.WriteState(prep.RunDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	return prep
}

// assertMarkerSurvives proves a refusal left the workspace's uncertainty on
// the record: the row cannot complete until a recovery actually succeeds.
func assertMarkerSurvives(t *testing.T, prep prepareMutationOutput) {
	t.Helper()
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState after the refusal: %v", err)
	}
	if st.Started == nil || st.Started.Site != "site-b" {
		t.Errorf("started = %+v, want the refusal to have left the marker set", st.Started)
	}
	if st.WorktreePath != prep.WorktreePath {
		t.Errorf("worktreePath = %q, want the refusal to have left it at %q", st.WorktreePath, prep.WorktreePath)
	}
}

func TestRecoverMutation_RunDirRefusals(t *testing.T) {
	tests := []struct {
		name   string
		runDir func(t *testing.T) any
		want   string
	}{
		{"absent from the request", func(t *testing.T) any { return nil }, "missing-run-dir"},
		{"relative", func(t *testing.T) any { return "runs/some-run" }, "run-dir-not-absolute"},
		{"nonexistent", func(t *testing.T) any { return filepath.Join(t.TempDir(), "never-created") }, "run-dir-not-found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := fixtureRepo(t)
			extra := map[string]any{}
			if runDir := tc.runDir(t); runDir != nil {
				extra["runDir"] = runDir
			}
			code, stdout, stderr := runVerifyOp(t, "recover-mutation", requestWith(t, repo, "recover-rundir", extra))
			if code != exitError {
				t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
			}
			if got := decodeCLIError(t, stderr); got.Code != tc.want {
				t.Errorf("error code = %q, want %q", got.Code, tc.want)
			}
		})
	}
}

func TestRecoverMutation_AnUnreadableStateIsRefused(t *testing.T) {
	repo := fixtureRepo(t)

	got := refuseRecoverMutation(t, repo, "recover-nostate", t.TempDir())

	if got.Code != "state-load-failed" {
		t.Errorf("error code = %q, want state-load-failed", got.Code)
	}
}

func TestRecoverMutation_StaleEvidenceIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-stale")
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.RepoCommit = strings.Repeat("a", 40)
	if err := mutation.WriteState(prep.RunDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got := refuseRecoverMutation(t, repo, "recover-stale", prep.RunDir)

	if got.Code != "stale-evidence" {
		t.Errorf("error code = %q, want stale-evidence", got.Code)
	}
}

func TestRecoverMutation_ARowThisRunNeverStartedIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-prepared")
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.RunID = "recover-unstarted"
	if err := mutation.WriteState(prep.RunDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got := refuseRecoverMutation(t, repo, "recover-unstarted", prep.RunDir)

	if got.Code != "mutation-row-not-started" {
		t.Errorf("error code = %q, want mutation-row-not-started", got.Code)
	}
}

func TestRecoverMutation_ARowAlreadyAnsweredIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-answered")
	completeLedgerRow(t, repo, "recover-answered", "SC-09", "survived")

	got := refuseRecoverMutation(t, repo, "recover-answered", prep.RunDir)

	if got.Code != "mutation-row-completed" {
		t.Errorf("error code = %q, want mutation-row-completed", got.Code)
	}
	assertMarkerSurvives(t, prep)
}

func TestRecoverMutation_ACorruptedWorktreeLinkIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-corrupt")
	if err := os.WriteFile(filepath.Join(prep.WorktreePath, ".git"), []byte("not a gitdir link\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := refuseRecoverMutation(t, repo, "recover-corrupt", prep.RunDir)

	if got.Code != "restore-failed" {
		t.Errorf("error code = %q, want restore-failed", got.Code)
	}
	assertMarkerSurvives(t, prep)
}

func TestRecoverMutation_ResidueThatCannotBeRemovedIsRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod-based permission checks don't apply")
	}
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-residue")

	guarded := filepath.Join(prep.WorktreePath, "orders")
	if err := os.WriteFile(filepath.Join(guarded, "residue.txt"), []byte("residue\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(guarded, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(guarded, 0o700) })

	got := refuseRecoverMutation(t, repo, "recover-residue", prep.RunDir)

	if got.Code != "residue-remains" {
		t.Errorf("error code = %q, want residue-remains", got.Code)
	}
	if !strings.Contains(got.Message, "orders/residue.txt") {
		t.Errorf("message = %q, want it to name the path that survived", got.Message)
	}
	assertMarkerSurvives(t, prep)
}

func TestRecoverMutation_AWorktreeAtAnotherRevisionIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-revision")
	mustGit(t, prep.WorktreePath, "checkout", "--detach", "HEAD~1")

	got := refuseRecoverMutation(t, repo, "recover-revision", prep.RunDir)

	if got.Code != "wrong-revision" {
		t.Errorf("error code = %q, want wrong-revision", got.Code)
	}
	if !strings.Contains(got.Message, prep.RepoCommit) {
		t.Errorf("message = %q, want it to name the prepared commit", got.Message)
	}
	assertMarkerSurvives(t, prep)
}

func TestRecoverMutation_AWorktreeThatCannotBeRecreatedIsRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod-based permission checks don't apply")
	}
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-recreate")
	if err := os.RemoveAll(prep.WorktreePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	admin := filepath.Join(repo, ".git", "worktrees")
	if err := os.Chmod(admin, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(admin, 0o700) })

	got := refuseRecoverMutation(t, repo, "recover-recreate", prep.RunDir)

	if got.Code != "recreate-failed" {
		t.Errorf("error code = %q, want recreate-failed", got.Code)
	}
	assertMarkerSurvives(t, prep)
}

func TestRecoverMutation_RestoresADirtiedWorkspaceInPlace(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-dirty")
	headBefore := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	mutateOrdersGuard(t, prep.WorktreePath, "	return fmt.Errorf(\"mutant left behind\")")
	if err := os.WriteFile(filepath.Join(prep.WorktreePath, "orders", "residue.txt"), []byte("residue\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := runRecoverMutation(t, repo, "recover-dirty", prep.RunDir)

	if out.Recreated {
		t.Errorf("recreated = true, want the surviving worktree restored in place")
	}
	if out.WorktreePath != prep.WorktreePath {
		t.Errorf("worktreePath = %q, want the prepared %q", out.WorktreePath, prep.WorktreePath)
	}
	if out.RowID != "SC-09" {
		t.Errorf("rowId = %q, want SC-09", out.RowID)
	}
	if len(out.CompletedSites) != 1 || out.CompletedSites[0] != "site-a" {
		t.Errorf("completedSites = %v, want [site-a]", out.CompletedSites)
	}
	if out.InterruptedSite != "site-b" {
		t.Errorf("interruptedSite = %q, want site-b", out.InterruptedSite)
	}

	status := strings.TrimSpace(mustGit(t, prep.WorktreePath, "status", "--porcelain=v1", "--ignored", "--untracked-files=all"))
	if status != "" {
		t.Errorf("worktree status = %q, want empty including ignored paths", status)
	}
	if head := strings.TrimSpace(mustGit(t, prep.WorktreePath, "rev-parse", "HEAD")); head != prep.RepoCommit {
		t.Errorf("worktree HEAD = %q, want the prepared commit %q", head, prep.RepoCommit)
	}

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState after recovery: %v", err)
	}
	if st.Started != nil {
		t.Errorf("started = %+v, want the marker cleared by the recovery", st.Started)
	}
	if len(st.Sites) != 1 || st.Sites[0].Site != "site-a" {
		t.Errorf("sites = %+v, want the finished site kept", st.Sites)
	}
	if st.WorktreePath != prep.WorktreePath {
		t.Errorf("state worktreePath = %q, want it unchanged at %q", st.WorktreePath, prep.WorktreePath)
	}

	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
	if head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD")); head != headBefore {
		t.Errorf("original repository HEAD = %q, want unchanged %q", head, headBefore)
	}
}

func TestRecoverMutation_RecreatesAVanishedWorktree(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-vanished")
	if err := os.RemoveAll(prep.WorktreePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	out := runRecoverMutation(t, repo, "recover-vanished", prep.RunDir)

	if !out.Recreated {
		t.Errorf("recreated = false, want a vanished worktree recreated")
	}
	if out.WorktreePath == prep.WorktreePath {
		t.Fatalf("worktreePath = %q, want a fresh worktree, not the vanished one", out.WorktreePath)
	}
	if info, err := os.Stat(out.WorktreePath); err != nil || !info.IsDir() {
		t.Fatalf("recreated worktree %s does not exist: %v", out.WorktreePath, err)
	}
	listed := mustGit(t, repo, "worktree", "list", "--porcelain")
	if !strings.Contains(listed, out.WorktreePath) {
		t.Errorf("worktree list = %q, want the recreated worktree registered", listed)
	}
	if strings.Contains(listed, prep.WorktreePath) {
		t.Errorf("worktree list = %q, want the vanished worktree's entry pruned", listed)
	}
	if head := strings.TrimSpace(mustGit(t, out.WorktreePath, "rev-parse", "HEAD")); head != prep.RepoCommit {
		t.Errorf("recreated worktree HEAD = %q, want the prepared commit %q", head, prep.RepoCommit)
	}
	if status := strings.TrimSpace(mustGit(t, out.WorktreePath, "status", "--porcelain=v1", "--ignored", "--untracked-files=all")); status != "" {
		t.Errorf("recreated worktree status = %q, want it clean", status)
	}

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState after recovery: %v", err)
	}
	if st.WorktreePath != out.WorktreePath {
		t.Errorf("state worktreePath = %q, want the recreated %q", st.WorktreePath, out.WorktreePath)
	}
	if st.Started != nil {
		t.Errorf("started = %+v, want the marker cleared by the recovery", st.Started)
	}
	if len(st.Sites) != 1 || st.Sites[0].Site != "site-a" {
		t.Errorf("sites = %+v, want the finished site kept", st.Sites)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
}

// TestRecoverMutation_RecreationForgetsOnlyTheRowsOwnWorktree: the registry
// recover-mutation clears an entry from is the repository's own, shared with
// every other worktree of it. Replacing the row's vanished workspace drops that
// one entry and no other — another worktree whose directory is absent at the
// same moment stays registered.
func TestRecoverMutation_RecreationForgetsOnlyTheRowsOwnWorktree(t *testing.T) {
	repo := fixtureRepo(t)
	prep := interruptedRow(t, repo, "recover-forget-one")
	if err := os.RemoveAll(prep.WorktreePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	ctrl, err := worktree.NewController(repo)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	bystander, err := ctrl.Create(prep.RepoCommit)
	if err != nil {
		t.Fatalf("Create the bystander worktree: %v", err)
	}
	if err := os.RemoveAll(bystander.Path); err != nil {
		t.Fatalf("RemoveAll the bystander: %v", err)
	}

	out := runRecoverMutation(t, repo, "recover-forget-one", prep.RunDir)

	if !out.Recreated {
		t.Errorf("recreated = false, want the vanished workspace recreated")
	}
	listed := mustGit(t, repo, "worktree", "list", "--porcelain")
	if strings.Contains(listed, prep.WorktreePath) {
		t.Errorf("worktree list = %q, want the row's vanished worktree forgotten", listed)
	}
	if !strings.Contains(listed, bystander.Path) {
		t.Errorf("worktree list = %q, want the bystander's entry %s to survive the recreation", listed, bystander.Path)
	}
	if !strings.Contains(listed, out.WorktreePath) {
		t.Errorf("worktree list = %q, want the recreated worktree registered", listed)
	}
}

// TestRecoverMutation_AWorktreePathItCannotProveGoneIsRefused: only a path
// that does not exist is a vanished workspace. One the verifier cannot stat,
// or one that exists as something other than a directory, is refused rather
// than replaced — nothing proved the workspace gone, and the marker stays.
func TestRecoverMutation_AWorktreePathItCannotProveGoneIsRefused(t *testing.T) {
	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{"a path beneath a regular file", func(t *testing.T) string {
			file := filepath.Join(t.TempDir(), "not-a-directory")
			if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			return filepath.Join(file, "worktree")
		}},
		{"a regular file", func(t *testing.T) string {
			file := filepath.Join(t.TempDir(), "worktree")
			if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			return file
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := fixtureRepo(t)
			prep := interruptedRow(t, repo, "recover-unprovable")
			st, err := mutation.LoadState(prep.RunDir)
			if err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			st.WorktreePath = tc.path(t)
			if err := mutation.WriteState(prep.RunDir, st); err != nil {
				t.Fatalf("WriteState: %v", err)
			}
			// The prepared workspace itself is still standing; the refusal must
			// not add a replacement beside it.
			before := verifierWorktrees(t)

			got := refuseRecoverMutation(t, repo, "recover-unprovable", prep.RunDir)

			if got.Code != "restore-failed" {
				t.Errorf("error code = %q, want restore-failed (message %q)", got.Code, got.Message)
			}
			after, err := mutation.LoadState(prep.RunDir)
			if err != nil {
				t.Fatalf("LoadState after the refusal: %v", err)
			}
			if after.Started == nil || after.Started.Site != "site-b" || after.WorktreePath != st.WorktreePath {
				t.Errorf("state after the refusal = %+v, want the marker and the path left as they were", after)
			}
			if after := verifierWorktrees(t); len(after) != len(before) {
				t.Errorf("worktrees = %v, want the refusal to have created none beyond %v", after, before)
			}
		})
	}
}

func TestRecoverMutation_PreservesRegisteredSites(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "recover-preserves-sites"
	prep := interruptedRow(t, repo, runID)

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.Enumerated = []string{"site-a", "site-b"}
	st.Discarded = []mutation.DiscardRecord{
		{Site: "site-b", Reason: "not the named mutation", DiffPath: filepath.Join(prep.RunDir, "discard-1.diff"), DiffSHA256: "deadbeef"},
	}
	if err := mutation.WriteState(prep.RunDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	runRecoverMutation(t, repo, runID, prep.RunDir)

	after, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState after recovery: %v", err)
	}
	if len(after.Enumerated) != 2 || after.Enumerated[0] != "site-a" || after.Enumerated[1] != "site-b" {
		t.Errorf("Enumerated = %v, want [site-a site-b] preserved through recovery", after.Enumerated)
	}
	if len(after.Discarded) != 1 || after.Discarded[0].Site != "site-b" {
		t.Errorf("Discarded = %+v, want the discard record preserved through recovery", after.Discarded)
	}
}
