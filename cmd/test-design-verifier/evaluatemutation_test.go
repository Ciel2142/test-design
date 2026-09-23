package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/coverage"
	"test-design/internal/mutation"
	"test-design/internal/runstate"
)

// evaluateMutationRequestJSON builds an evaluate-mutation request. sites is
// the row's full enumerated list to register on this call; when the caller
// leaves it empty, evaluateMutationSitesToSend derives it from the row's own
// state, so a single-site test never has to state one.
func evaluateMutationRequestJSON(t *testing.T, repo, runID, runDir, site string, lastSite bool, sites ...string) string {
	t.Helper()
	payload := map[string]any{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          runID,
		"runDir":         runDir,
		"site":           site,
		"lastSite":       lastSite,
	}
	if toSend := evaluateMutationSitesToSend(t, runDir, site, sites); toSend != nil {
		payload["sites"] = toSend
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

// evaluateMutationSitesToSend answers the "sites" value a request should
// carry: sites verbatim when the caller named the row's full registration,
// [site] when the row's state has not registered a list yet, or nil once a
// list is already on disk and the caller left registration to the default —
// resending it would only risk an enumeration-changed refusal.
func evaluateMutationSitesToSend(t *testing.T, runDir, site string, sites []string) []string {
	t.Helper()
	if len(sites) > 0 {
		return sites
	}
	st, err := mutation.LoadState(runDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Enumerated) > 0 {
		return nil
	}
	return []string{site}
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

// ordersGoTail is Reserve and ReserveBatch verbatim as committed in the
// fixture, appended unchanged by every helper that rewrites orders.go so the
// diff stays isolated to the function each helper targets.
const ordersGoTail = `
func Reserve(qty int) error {
	if qty <= 0 {
		return fmt.Errorf("reservation quantity must be positive")
	}
	return nil
}

func ReserveBatch(qtys []int) error {
	for _, qty := range qtys {
		if qty <= 0 {
			return fmt.Errorf("reservation quantity must be positive")
		}
	}
	return nil
}
`

// mutateOrdersGuard rewrites orders.go's ValidateQuantity body to bodySrc,
// producing a single-hunk diff against the guard function.
func mutateOrdersGuard(t *testing.T, worktreePath, bodySrc string) {
	t.Helper()
	path := filepath.Join(worktreePath, "orders", "orders.go")
	content := "package orders\n\nimport \"fmt\"\n\nfunc ValidateQuantity(qty int) error {\n" + bodySrc + "\n}\n" + ordersGoTail
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

	assertEvidenceFile(t, out.EvidencePath, "eval-caught", "SC-09", "go", 1, []string{"caught"})

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

	assertEvidenceFile(t, out.EvidencePath, "eval-survived", "SC-09", "go", 1, []string{"survived"})

	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the lastSite call to have disposed of it", left)
	}
}

func TestEvaluateMutation_MultiSiteRowAccumulatesAndDisposesOnLast(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-multisite", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-multisite", prep.RunDir, "site-a", false, "site-a", "site-b"))
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

	assertEvidenceFile(t, second.EvidencePath, "eval-multisite", "SC-09", "go", 2, []string{"survived", "caught"})

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

func TestEvaluateMutation_NonCompilingMutantRefusedDistinctly(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-buildfail", "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	return fmt.Errorf(\"unterminated")

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-buildfail", prep.RunDir, "site-a", true))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "mutant-build-failed" {
		t.Errorf("error code = %q, want mutant-build-failed", got.Code)
	}
	if !strings.Contains(got.Message, "orders.go") {
		t.Errorf("message = %q, want it to quote the compiler detail naming orders.go", got.Message)
	}

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.Started == nil || st.Started.Phase != "mutated" || st.Started.Site != "site-a" {
		t.Errorf("started = %+v, want site-a still marked mutated", st.Started)
	}

	runRecoverMutation(t, repo, "eval-buildfail", prep.RunDir)

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, stdout, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-buildfail", prep.RunDir, "site-a", true))
	if code != exitOK {
		t.Fatalf("re-apply exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.Outcome != "caught" {
		t.Errorf("outcome = %q, want caught", out.Outcome)
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

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState after the refusal: %v", err)
	}
	if st.Started == nil {
		t.Fatal("state carries no started marker after the clean-run refusal, want the site it was inside")
	}
	if st.Started.Site != "site-a" || st.Started.Phase != "clean" {
		t.Errorf("started = %+v, want site-a in phase clean", st.Started)
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
		Sites:    []string{"site-a"},
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

// fake command, no real JVM toolchain needed: all target records pass but
// exit is nonzero (auxiliary task failure), must not read as survived.
func TestRunTargetedAndVerify_JVMAllPassNonzeroExitYieldsUnexplainedFailure(t *testing.T) {
	src := &coverage.Source{Runner: coverage.RunnerConfig{Kind: "maven", Root: "."}}
	target := mutation.Target{
		Command: []string{"sh", "-c", `mkdir -p reports && cat > reports/a.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.ATest" tests="1">
  <testcase classname="com.example.ATest" name="adds"/>
</testsuite>
EOF
exit 3`},
		ReportGlobs: []string{"reports/*.xml"},
	}
	id := mutation.Identity{Scope: "com.example.ATest", Name: "adds"}

	outcome, exitStatus, cliErr := runTargetedAndVerify("evaluate-mutation", src, "", t.TempDir(), t.TempDir(), target, id)

	if cliErr == nil {
		t.Fatalf("cliErr = nil, outcome = %q, want an error: every record passed but the command exited nonzero", outcome)
	}
	if cliErr.Code != "unexplained-runner-failure" {
		t.Errorf("cliErr.Code = %q, want unexplained-runner-failure", cliErr.Code)
	}
	if outcome != "" {
		t.Errorf("outcome = %q, want empty on error", outcome)
	}
	if exitStatus != 3 {
		t.Errorf("exitStatus = %d, want 3", exitStatus)
	}
}

func TestMapSelectionErr(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{mutation.ErrZeroSelection, "zero-selection"},
		{mutation.ErrForeignIdentity, "foreign-identity"},
		{mutation.ErrUnsupportedIdentity, "unsupported-test-identity"},
		{errors.New("some other failure"), "normalization-failed"},
	}
	for _, c := range cases {
		if got := mapSelectionErr(c.err); got != c.want {
			t.Errorf("mapSelectionErr(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestPoliceDiffCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{mutation.ErrEmptyDiff, "empty-diff"},
		{mutation.ErrMultiSite, "multi-site-diff"},
		{mutation.ErrForeignDiff, "foreign-diff"},
		{errors.New("some other failure"), "diff-police-failed"},
	}
	for _, c := range cases {
		if got := policeDiffCode(c.err); got != c.want {
			t.Errorf("policeDiffCode(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func assertEvidenceFile(t *testing.T, path, wantRunID, wantRowID, wantRunnerKind string, wantSites int, wantOutcomes []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile evidence: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("evidence did not decode: %v\ndata=%s", err, data)
	}
	if decoded["runId"] != wantRunID {
		t.Errorf("evidence[runId] = %v, want %q", decoded["runId"], wantRunID)
	}
	var ev mutationEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("evidence did not decode: %v\ndata=%s", err, data)
	}
	if ev.RunID != wantRunID {
		t.Errorf("evidence.RunID = %q, want %q", ev.RunID, wantRunID)
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

func TestEvaluateMutation_RejectsPreparedIdentityMismatch(t *testing.T) {
	for _, mismatch := range []string{"run", "commit", "source", "sha"} {
		t.Run(mismatch, func(t *testing.T) {
			repo := fixtureRepo(t)
			prep := preparedMutation(t, repo, "identity-check", "SC-09")
			st, err := mutation.LoadState(prep.RunDir)
			if err != nil {
				t.Fatal(err)
			}
			switch mismatch {
			case "run":
				st.RunID = "other-run"
			case "commit":
				st.RepoCommit = strings.Repeat("a", 40)
			case "source":
				st.Source = "other-spec.md"
			case "sha":
				st.SourceSHA256 = "other-sha"
			}
			if err := mutation.WriteState(prep.RunDir, st); err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runVerifyOp(t, "evaluate-mutation",
				evaluateMutationRequestJSON(t, repo, "identity-check", prep.RunDir, "guard", true))
			if code != exitError || !strings.Contains(stderr, "stale-evidence") {
				t.Fatalf("identity mismatch accepted: code=%d stderr=%s", code, stderr)
			}
		})
	}
}

func TestEvaluateMutation_AnInterruptedSiteIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-interrupted", "SC-09")

	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.Started = &mutation.SiteAttempt{Site: "site-a", Phase: "mutated", StartedAt: "2026-09-07T10:00:00Z"}
	if err := mutation.WriteState(prep.RunDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-interrupted", prep.RunDir, "site-b", false))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "site-interrupted" {
		t.Errorf("error code = %q, want site-interrupted", got.Code)
	}
	if !strings.Contains(got.Message, "site-a") {
		t.Errorf("message = %q, want it to name the site the workspace is uncertain about", got.Message)
	}

	after, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState after the refusal: %v", err)
	}
	if len(after.Sites) != 0 {
		t.Errorf("sites = %+v, want nothing measured on an uncertain workspace", after.Sites)
	}
	if after.Started == nil || after.Started.Site != "site-a" {
		t.Errorf("started = %+v, want the marker left for recover-mutation", after.Started)
	}
}

func TestEvaluateMutation_ARowThisRunNeverStartedIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-prepared", "SC-09")

	// The workspace is real; the run ID it now claims has no ledger at all.
	st, err := mutation.LoadState(prep.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.RunID = "eval-unstarted"
	if err := mutation.WriteState(prep.RunDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-unstarted", prep.RunDir, "site-a", false))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "mutation-row-not-started" {
		t.Errorf("error code = %q, want mutation-row-not-started", got.Code)
	}
}

func TestEvaluateMutation_ARowAlreadyAnsweredIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	prep := preparedMutation(t, repo, "eval-answered", "SC-09")
	completeLedgerRow(t, repo, "eval-answered", "SC-09", "caught")
	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "eval-answered", prep.RunDir, "site-a", false))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "mutation-row-completed" {
		t.Errorf("error code = %q, want mutation-row-completed", got.Code)
	}
}

func TestEvaluateMutation_LastSiteCompletesTheLedgerRow(t *testing.T) {
	t.Run("every site caught", func(t *testing.T) {
		repo := fixtureRepo(t)
		prep := preparedMutation(t, repo, "eval-ledger-caught", "SC-09")
		mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)

		code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
			evaluateMutationRequestJSON(t, repo, "eval-ledger-caught", prep.RunDir, "site-a", true))
		if code != exitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		var out evaluateMutationOutput
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
		}

		rec := ledgerRecord(t, runResume(t, repo, "eval-ledger-caught"), runstate.KindMutationRow, "SC-09")
		if rec.Status != runstate.StatusCompleted {
			t.Errorf("status = %q, want %q", rec.Status, runstate.StatusCompleted)
		}
		if rec.Outcome != "caught" {
			t.Errorf("outcome = %q, want caught when every site was caught", rec.Outcome)
		}
		if got := rec.Artifacts[artifactEvidence].Path; got != out.EvidencePath {
			t.Errorf("evidence artifact = %q, want the written evidence %q", got, out.EvidencePath)
		}
		if reasons := runstate.Verify(&rec); len(reasons) != 0 {
			t.Errorf("Verify = %v, want the completed row to rest on a digestible artifact", reasons)
		}

		st, err := mutation.LoadState(prep.RunDir)
		if err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		if st.Started != nil {
			t.Errorf("started = %+v, want the marker cleared by the site it belonged to", st.Started)
		}
	})

	t.Run("a surviving site", func(t *testing.T) {
		repo := fixtureRepo(t)
		prep := preparedMutation(t, repo, "eval-ledger-survived", "SC-09")

		mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
		if code, _, stderr := runVerifyOp(t, "evaluate-mutation",
			evaluateMutationRequestJSON(t, repo, "eval-ledger-survived", prep.RunDir, "site-a", false, "site-a", "site-b")); code != exitOK {
			t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}
		mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
		if code, _, stderr := runVerifyOp(t, "evaluate-mutation",
			evaluateMutationRequestJSON(t, repo, "eval-ledger-survived", prep.RunDir, "site-b", true)); code != exitOK {
			t.Fatalf("site-b exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
		}

		rec := ledgerRecord(t, runResume(t, repo, "eval-ledger-survived"), runstate.KindMutationRow, "SC-09")
		if rec.Status != runstate.StatusCompleted {
			t.Errorf("status = %q, want %q", rec.Status, runstate.StatusCompleted)
		}
		if rec.Outcome != "survived" {
			t.Errorf("outcome = %q, want survived when one site was not caught", rec.Outcome)
		}
	})
}

func TestEvaluateMutation_FirstCallWithoutSitesRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "eval-missing-sites"
	prep := preparedMutation(t, repo, runID, "SC-09")

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-a", "lastSite": false,
	}))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "missing-sites" {
		t.Errorf("error code = %q, want missing-sites", got.Code)
	}
}

func TestEvaluateMutation_InvalidSitesRefused(t *testing.T) {
	cases := []struct {
		name  string
		sites []string
		site  string
	}{
		{"empty-label", []string{"", "site-b"}, "site-b"},
		{"separator", []string{"a/b", "site-b"}, "site-b"},
		{"duplicate", []string{"site-a", "site-a"}, "site-a"},
		{"site-not-in-list", []string{"site-a", "site-b"}, "site-c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := fixtureRepo(t)
			runID := "eval-invalid-" + c.name
			prep := preparedMutation(t, repo, runID, "SC-09")

			code, stdout, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
				"runDir": prep.RunDir, "site": c.site, "lastSite": false, "sites": c.sites,
			}))
			if code != exitError {
				t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
			}
			got := decodeCLIError(t, stderr)
			if got.Code != "invalid-sites" {
				t.Errorf("error code = %q, want invalid-sites", got.Code)
			}
		})
	}
}

func TestEvaluateMutation_EnumerationChangedRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "eval-enum-changed"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-a", "lastSite": false, "sites": []string{"site-a", "site-b"},
	}))
	if code != exitOK {
		t.Fatalf("first call exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-b", "lastSite": true, "sites": []string{"site-b", "site-c"},
	}))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "enumeration-changed" {
		t.Errorf("error code = %q, want enumeration-changed", got.Code)
	}
}

func TestEvaluateMutation_UnregisteredSiteIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "eval-unknown-site"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, _, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-a", "lastSite": false, "sites": []string{"site-a", "site-b"},
	}))
	if code != exitOK {
		t.Fatalf("first call exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-c", "lastSite": false,
	}))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "unknown-site" {
		t.Errorf("error code = %q, want unknown-site", got.Code)
	}
}

func TestEvaluateMutation_LastSiteBeforeEveryRegisteredSiteIsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "eval-sites-remaining"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-a", "lastSite": true, "sites": []string{"site-a", "site-b"},
	}))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "sites-remaining" {
		t.Errorf("error code = %q, want sites-remaining", got.Code)
	}

	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the worktree to survive a premature lastSite", left)
	}
	rec := ledgerRecord(t, runResume(t, repo, runID), runstate.KindMutationRow, "SC-09")
	if rec.Status != runstate.StatusStarted {
		t.Errorf("status = %q, want %q", rec.Status, runstate.StatusStarted)
	}
}

func TestEvaluateMutation_CompletesRowOnlyAfterEveryRegisteredSite(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "eval-completes-without-lastsite"
	prep := preparedMutation(t, repo, runID, "SC-09")

	mutateOrdersGuard(t, prep.WorktreePath, "	if qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be positive\")\n\t}\n\treturn nil")
	code, _, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-a", "lastSite": false, "sites": []string{"site-a", "site-b"},
	}))
	if code != exitOK {
		t.Fatalf("site-a exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the row still open after one of two sites", left)
	}

	mutateOrdersGuard(t, prep.WorktreePath, `	return fmt.Errorf("always rejected")`)
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation", requestWith(t, repo, runID, map[string]any{
		"runDir": prep.RunDir, "site": "site-b", "lastSite": false,
	}))
	if code != exitOK {
		t.Fatalf("site-b exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if out.EvidencePath == "" {
		t.Fatal("evidencePath is empty, want the final registered site to complete the row without lastSite")
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want completion to dispose the worktree", left)
	}

	ev := readMutationEvidence(t, out.EvidencePath)
	if len(ev.EnumeratedSites) != 2 || ev.EnumeratedSites[0] != "site-a" || ev.EnumeratedSites[1] != "site-b" {
		t.Errorf("evidence.EnumeratedSites = %v, want [site-a site-b]", ev.EnumeratedSites)
	}
}
