package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evaluateMutationRequestJSON(t *testing.T, repo, runID, runDir, site string, lastSite bool) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          runID,
		"runDir":         runDir,
		"site":           site,
		"lastSite":       lastSite,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func preparedMutation(t *testing.T, repo, runID, rowID string) prepareMutationOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSON(t, repo, runID, rowID))
	if code != exitOK {
		t.Fatalf("prepare-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out prepareMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("prepare-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

// mutateOrdersGuard rewrites orders.go's ValidateQuantity body to bodySrc,
// producing a single-hunk diff against the guard function.
func mutateOrdersGuard(t *testing.T, worktreePath, bodySrc string) {
	t.Helper()
	path := filepath.Join(worktreePath, "orders", "orders.go")
	content := "package orders\n\nimport \"fmt\"\n\nfunc ValidateQuantity(qty int) error {\n" + bodySrc + "\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestEvaluateMutation_CaughtSite(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-caught", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-caught", prep.RunDir, "guard-always-errors", true))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "caught" {
		t.Errorf("outcome = %q, want caught", out.Outcome)
	}
	if out.Diff.File != "orders/orders.go" || out.Diff.Hunks != 1 {
		t.Errorf("diff summary = %+v, want File=orders/orders.go Hunks=1", out.Diff)
	}
	if out.EvidencePath == "" {
		t.Fatal("evidencePath is empty, want the written evidence file's path")
	}
	if len(out.Sites) != 1 || out.Sites[0].Outcome != "caught" {
		t.Errorf("sites = %+v, want one caught site", out.Sites)
	}

	assertEvidenceFile(t, out.EvidencePath, "SC-09", "go", 1, []string{"caught"})

	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the lastSite call to have disposed of it", left)
	}
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
}

func TestEvaluateMutation_SurvivedSite(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-survived", "SC-09")

	// Equivalent mutant: changes the error text on a branch TestSameName
	// (ValidateQuantity(1)) never takes, so the guard test still passes.
	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-survived", prep.RunDir, "guard-message-only", true))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "survived" {
		t.Errorf("outcome = %q, want survived", out.Outcome)
	}

	assertEvidenceFile(t, out.EvidencePath, "SC-09", "go", 1, []string{"survived"})

	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the lastSite call to have disposed of it", left)
	}
}

func TestEvaluateMutation_MultiSiteRowAccumulatesAndDisposesOnLast(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-multisite", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-multisite", prep.RunDir, "site-a", false))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var first evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &first); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if first.Outcome != "survived" {
		t.Errorf("site-a outcome = %q, want survived", first.Outcome)
	}
	if first.Sites != nil || first.EvidencePath != "" {
		t.Errorf("non-lastSite output = %+v, want no sites/evidencePath", first)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the worktree to survive a non-lastSite call", left)
	}

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, stdout, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-multisite", prep.RunDir, "site-b", true))
	if code != exitOK {
		t.Fatalf("site-b exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var second evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &second); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if second.Outcome != "caught" {
		t.Errorf("site-b outcome = %q, want caught", second.Outcome)
	}
	if len(second.Sites) != 2 {
		t.Fatalf("sites = %+v, want 2 entries", second.Sites)
	}
	if second.Sites[0].Site != "site-a" || second.Sites[0].Outcome != "survived" {
		t.Errorf("sites[0] = %+v, want site-a survived", second.Sites[0])
	}
	if second.Sites[1].Site != "site-b" || second.Sites[1].Outcome != "caught" {
		t.Errorf("sites[1] = %+v, want site-b caught", second.Sites[1])
	}
	if !strings.HasSuffix(second.Sites[0].MutatedRunDir, "site-1-mutated") {
		t.Errorf("sites[0].MutatedRunDir = %q, want it to end with site-1-mutated", second.Sites[0].MutatedRunDir)
	}
	if !strings.HasSuffix(second.Sites[1].MutatedRunDir, "site-2-mutated") {
		t.Errorf("sites[1].MutatedRunDir = %q, want it to end with site-2-mutated", second.Sites[1].MutatedRunDir)
	}

	assertEvidenceFile(t, second.EvidencePath, "SC-09", "go", 2, []string{"survived", "caught"})

	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the lastSite call to have disposed of it", left)
	}
}

func TestEvaluateMutation_EmptyDiffRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-empty", "SC-09")

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-empty", prep.RunDir, "site-a", false))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "empty-diff" {
		t.Errorf("error code = %q, want empty-diff", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the worktree to survive a step-3 failure", left)
	}
}

func TestEvaluateMutation_MultiFileDiffRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-multifile", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	paymentsPath := filepath.Join(prep.WorktreePath, "payments", "payments.go")
	data, err := os.ReadFile(paymentsPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(paymentsPath, append(data, []byte("\n// unrelated edit\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-multifile", prep.RunDir, "site-a", false))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "multi-site-diff" {
		t.Errorf("error code = %q, want multi-site-diff", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the worktree to survive a step-3 failure", left)
	}
	if lines := gitStatusLines(t, prep.WorktreePath); len(lines) != 0 {
		t.Errorf("worktree git status = %v, want the best-effort restore to have reverted it", lines)
	}
}

func TestEvaluateMutation_TestFileDiffRefusedAsForeign(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-foreign", "SC-09")

	testPath := filepath.Join(prep.WorktreePath, "orders", "orders_test.go")
	data, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(testPath, append(data, []byte("\n// mutated test file\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-foreign", prep.RunDir, "site-a", false))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "foreign-diff" {
		t.Errorf("error code = %q, want foreign-diff", got.Code)
	}
}

func TestEvaluateMutation_ZeroSelectionOnAlwaysSkippedIdentity(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-zero", "SC-03")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-zero", prep.RunDir, "site-a", false))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "zero-selection" {
		t.Errorf("error code = %q, want zero-selection", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the worktree to survive a step-4 failure", left)
	}
	if lines := gitStatusLines(t, prep.WorktreePath); len(lines) != 0 {
		t.Errorf("worktree git status = %v, want the best-effort restore to have reverted it", lines)
	}
}

func TestEvaluateMutation_DuplicateSiteRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-dup", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-dup", prep.RunDir, "guard-once", false))
	if code != exitOK {
		t.Fatalf("first call exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-dup", prep.RunDir, "guard-once", false))
	if code != exitError {
		t.Fatalf("second call exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "duplicate-site" {
		t.Errorf("error code = %q, want duplicate-site", got.Code)
	}
	if !strings.Contains(stderr, "guard-once") {
		t.Errorf("stderr = %q, want it to name the duplicate site", stderr)
	}
}

// addFlakyMarkerScenario adds a coverage row bound to a test that fails
// whenever an out-of-worktree marker file exists, so a test can simulate an
// unrepeatable clean baseline without violating PoliceDiff's single-file,
// single-hunk rule (the marker never appears in the worktree's diff).
func addFlakyMarkerScenario(t *testing.T, repo string) {
	t.Helper()
	flakyGo := "package orders\n\nfunc Flaky() error {\n\treturn nil\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "orders", "flaky.go"), []byte(flakyGo), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	flakyTest := `package orders

import (
	"os"
	"testing"
)

// covers: SC-100
func TestFlakyGuard(t *testing.T) {
	if p := os.Getenv("EVALUATE_MUTATION_FLAKY_MARKER"); p != "" {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("flaky marker present at %s", p)
		}
	}
	if err := Flaky(); err != nil {
		t.Fatalf("Flaky() = %v, want nil", err)
	}
}
`
	if err := os.WriteFile(filepath.Join(repo, "orders", "flaky_test.go"), []byte(flakyTest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	rewriteFixtureDoc(t, repo, func(s string) string {
		return s + "\n### Behavior: flaky guard\n\n" +
			"| ID | Behavior | Expected | Level | Verification mutation | Risk |\n" +
			"|----|----------|----------|-------|------------------------|------|\n" +
			"| SC-100 | flaky guard behaves | Flaky() returns nil | unit | n/a | low |\n"
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "add flaky guard fixture")
}

func TestEvaluateMutation_CleanRunFailureVoidsSite(t *testing.T) {
	repo := fixtureRepo(t)
	addFlakyMarkerScenario(t, repo)
	prep := preparedMutation(t, repo, "eval-flaky", "SC-100")

	marker := filepath.Join(t.TempDir(), "flaky-marker")
	t.Setenv("EVALUATE_MUTATION_FLAKY_MARKER", marker)
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	flakyPath := filepath.Join(prep.WorktreePath, "orders", "flaky.go")
	mutated := "package orders\n\nimport \"fmt\"\n\nfunc Flaky() error {\n\treturn fmt.Errorf(\"mutated\")\n}\n"
	if err := os.WriteFile(flakyPath, []byte(mutated), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-flaky", prep.RunDir, "site-a", true))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "clean-run-failed" {
		t.Errorf("error code = %q, want clean-run-failed", got.Code)
	}
	if !strings.Contains(got.Message, "clean baseline unrepeatable") {
		t.Errorf("message = %q, want it to name the unrepeatable clean baseline", got.Message)
	}

	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want a clean-run failure to leave the worktree undisposed even on lastSite", left)
	}
	if lines := gitStatusLines(t, prep.WorktreePath); len(lines) != 0 {
		t.Errorf("worktree git status = %v, want RestoreTracked to have reverted the mutation before the failed clean rerun", lines)
	}
}

func TestEvaluateMutation_SnapshotMismatchDetected(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-mismatch", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)

	req := &evaluateMutationRequest{
		verifyRequest: verifyRequest{
			SchemaVersion:  "1",
			RepoRoot:       repo,
			CoverageSource: fixtureCoverageSource,
			RunID:          "eval-mismatch",
		},
		RunDir:   prep.RunDir,
		Site:     "site-a",
		LastSite: true,
	}
	cliErr := evaluateMutationOp(req, discard{}, func() {
		if err := os.WriteFile(filepath.Join(repo, "orders", "sneaked.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	})
	if cliErr == nil {
		t.Fatal("evaluateMutationOp returned no error, want the snapshot-mismatch refusal")
	}
	if cliErr.Code != "snapshot-mismatch" {
		t.Errorf("error code = %q, want snapshot-mismatch", cliErr.Code)
	}
	want := "orders/sneaked.txt; commit or discard the concurrent change, then rerun"
	if !strings.Contains(cliErr.Message, want) {
		t.Errorf("message = %q, want it to contain %q", cliErr.Message, want)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestEvaluateMutation_AttachFailureOnRemovedWorktree(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-attach", "SC-09")

	if err := os.RemoveAll(prep.WorktreePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-attach", prep.RunDir, "site-a", false))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "worktree-attach-failed" {
		t.Errorf("error code = %q, want worktree-attach-failed", got.Code)
	}
}

func assertEvidenceFile(t *testing.T, path, wantRowID, wantRunnerKind string, wantSites int, wantOutcomes []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile evidence: %v", err)
	}
	var ev mutationEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("evidence did not decode: %v\ndata=%s", err, data)
	}
	if ev.RowID != wantRowID {
		t.Errorf("evidence.RowID = %q, want %q", ev.RowID, wantRowID)
	}
	if ev.RunnerKind != wantRunnerKind {
		t.Errorf("evidence.RunnerKind = %q, want %q", ev.RunnerKind, wantRunnerKind)
	}
	if !ev.SnapshotEqual {
		t.Error("evidence.SnapshotEqual = false, want true")
	}
	if len(ev.Sites) != wantSites {
		t.Fatalf("evidence.Sites = %+v, want %d entries", ev.Sites, wantSites)
	}
	for i, wantOutcome := range wantOutcomes {
		if ev.Sites[i].Outcome != wantOutcome {
			t.Errorf("evidence.Sites[%d].Outcome = %q, want %q", i, ev.Sites[i].Outcome, wantOutcome)
		}
		if ev.Sites[i].DiffSHA256 == "" {
			t.Errorf("evidence.Sites[%d].DiffSHA256 is empty, want the diff file's digest", i)
		}
	}
	if len(ev.TargetCommand) == 0 {
		t.Error("evidence.TargetCommand is empty, want the redacted targeted command")
	}
	if len(ev.SuiteCommand) == 0 {
		t.Error("evidence.SuiteCommand is empty, want the redacted suite command")
	}
}
