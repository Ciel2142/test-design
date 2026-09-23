package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"test-design/internal/mutation"
)

func discardMutationEditRequestJSON(t *testing.T, repo, runID, runDir, site, reason string) string {
	t.Helper()
	return requestWith(t, repo, runID, map[string]any{"runDir": runDir, "site": site, "reason": reason})
}

func runDiscardMutationEdit(t *testing.T, repo, runID, runDir, site, reason string) discardMutationEditOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "discard-mutation-edit", discardMutationEditRequestJSON(t, repo, runID, runDir, site, reason))
	if code != exitOK {
		t.Fatalf("discard-mutation-edit exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out discardMutationEditOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("discard-mutation-edit stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// refuseDiscardMutationEdit drives the op expecting a refusal and returns it.
func refuseDiscardMutationEdit(t *testing.T, repo, runID, runDir, site, reason string) cliError {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "discard-mutation-edit", discardMutationEditRequestJSON(t, repo, runID, runDir, site, reason))
	if code != exitError {
		t.Fatalf("discard-mutation-edit exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	return decodeCLIError(t, stderr)
}

func TestDiscardMutationEdit_RestoresUnevaluatedEditKeepsCompletedSites(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-restores"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, _, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", false, "site-a", "site-b"))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"unrelated\")\n\t}\n\treturn nil")

	out := runDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-b", "not the named mutation")

	if out.Site != "site-b" || out.Reason != "not the named mutation" {
		t.Errorf("output = %+v, want site-b with the given reason", out)
	}
	if out.DiffPath == "" {
		t.Fatal("diffPath is empty, want the discarded diff's path")
	}
	if data, err := os.ReadFile(out.DiffPath); err != nil || len(data) == 0 {
		t.Errorf("discard diff %s unreadable or empty: %v", out.DiffPath, err)
	}

	if lines := gitStatusLines(t, prep.WorktreePath); len(lines) != 0 {
		t.Errorf("worktree git status = %v, want the discard to have restored it", lines)
	}

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Sites) != 1 || st.Sites[0].Site != "site-a" {
		t.Errorf("sites = %+v, want site-a kept", st.Sites)
	}
	if len(st.Discarded) != 1 || st.Discarded[0].Site != "site-b" {
		t.Errorf("discarded = %+v, want one entry for site-b", st.Discarded)
	}
	if st.Discarded[0].Reason != "not the named mutation" {
		t.Errorf("discarded[0].Reason = %q, want the given reason", st.Discarded[0].Reason)
	}
	if st.Discarded[0].DiffSHA256 == "" {
		t.Error("discarded[0].DiffSHA256 is empty, want the diff file's digest")
	}
}

func TestDiscardMutationEdit_ThenSameSiteEvaluates(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-then-evaluates"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", false, "site-a", "site-b"))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("unrelated edit")`)
	discardOut := runDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-b", "touched an unrelated branch")
	if discardOut.Site != "site-b" {
		t.Fatalf("discard site = %q, want site-b", discardOut.Site)
	}

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-b", true))
	if code != exitOK {
		t.Fatalf("site-b exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.EvidencePath == "" {
		t.Fatal("evidencePath is empty, want the row to complete")
	}

	ev := readMutationEvidence(t, out.EvidencePath)
	if len(ev.Sites) != 2 || ev.Sites[0].Site != "site-a" || ev.Sites[1].Site != "site-b" {
		t.Errorf("evidence.Sites = %+v, want [site-a site-b]", ev.Sites)
	}
	if len(ev.Discarded) != 1 || ev.Discarded[0].Site != "site-b" {
		t.Errorf("evidence.Discarded = %+v, want one entry for site-b", ev.Discarded)
	}
	if ev.Discarded[0].Reason != "touched an unrelated branch" {
		t.Errorf("evidence.Discarded[0].Reason = %q, want the recorded reason", ev.Discarded[0].Reason)
	}
}

func TestDiscardMutationEdit_EvaluatedSiteRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-evaluated-site"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", false, "site-a", "site-b"))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "reconsidered")

	if got.Code != "duplicate-site" {
		t.Errorf("error code = %q, want duplicate-site", got.Code)
	}
	if lines := gitStatusLines(t, prep.WorktreePath); len(lines) != 0 {
		t.Errorf("worktree git status = %v, want it untouched by the refusal", lines)
	}
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Discarded) != 0 {
		t.Errorf("discarded = %+v, want nothing recorded on a refusal", st.Discarded)
	}
}

func TestDiscardMutationEdit_UnregisteredSiteRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-unregistered-site"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, runID, prep.RunDir, "site-a", false, "site-a", "site-b"))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-c", "not part of the row")

	if got.Code != "unknown-site" {
		t.Errorf("error code = %q, want unknown-site", got.Code)
	}
}

func TestDiscardMutationEdit_InterruptedRowRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-interrupted"
	prep := interruptedRow(t, repo, runID)

	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-c", "irrelevant")

	if got.Code != "site-interrupted" {
		t.Errorf("error code = %q, want site-interrupted", got.Code)
	}
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.Started == nil || st.Started.Site != "site-b" {
		t.Errorf("started = %+v, want the marker left in place", st.Started)
	}
}

func TestDiscardMutationEdit_BlankReasonRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-blank-reason"
	prep := preparedMutation(t, repo, runID, "SC-09")

	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "   ")

	if got.Code != "missing-reason" {
		t.Errorf("error code = %q, want missing-reason", got.Code)
	}
}

func TestDiscardMutationEdit_EmptyDiffRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-empty-diff"
	prep := preparedMutation(t, repo, runID, "SC-09")

	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "no edit was ever applied")

	if got.Code != "empty-diff" {
		t.Errorf("error code = %q, want empty-diff", got.Code)
	}
}

func TestDiscardMutationEdit_RefusedRestoreWritesNoState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod-based permission checks don't apply")
	}
	repo := fixtureRepo(t)
	const runID = "discard-refused-restore"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	guarded := filepath.Join(prep.WorktreePath, "payments")
	if err := os.WriteFile(filepath.Join(guarded, "residue.txt"), []byte("residue\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(guarded, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(guarded, 0o700) })

	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "not the named mutation")

	if got.Code != "residue-remains" {
		t.Errorf("error code = %q, want residue-remains", got.Code)
	}
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Discarded) != 0 {
		t.Errorf("discarded = %+v, want nothing recorded when the restore is refused", st.Discarded)
	}
}

func TestDiscardMutationEdit_LimitRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "discard-limit"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("first rejected edit")`)
	runDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "first rejected edit")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("second rejected edit")`)
	runDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "second rejected edit")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("third rejected edit")`)
	got := refuseDiscardMutationEdit(t, repo, runID, prep.RunDir, "site-a", "third rejected edit")

	if got.Code != "discard-limit" {
		t.Errorf("error code = %q, want discard-limit", got.Code)
	}

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Discarded) != 2 {
		t.Errorf("discarded = %+v, want exactly 2 entries kept", st.Discarded)
	}
}
