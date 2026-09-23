package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"test-design/internal/mutation"
	"test-design/internal/report"
)

// The zero-selection and foreign-diff refusals are already proven end-to-end
// at the op boundary by TestEvaluateMutation_ZeroSelectionOnAlwaysSkippedIdentity
// and TestEvaluateMutation_TestFileDiffRefusedAsForeign in evaluatemutation_test.go;
// no honest e2e construction adds anything those op tests don't already cover.

const (
	reserveGuardDefault      = "\tif qty <= 0 {\n\t\treturn fmt.Errorf(\"reservation quantity must be positive\")\n\t}\n\treturn nil"
	reserveGuardMutated      = "\tif qty < 0 {\n\t\treturn fmt.Errorf(\"reservation quantity must be positive\")\n\t}\n\treturn nil"
	reserveBatchGuardDefault = "\tfor _, qty := range qtys {\n\t\tif qty <= 0 {\n\t\t\treturn fmt.Errorf(\"reservation quantity must be positive\")\n\t\t}\n\t}\n\treturn nil"
	reserveBatchGuardMutated = "\tfor _, qty := range qtys {\n\t\tif qty < 0 {\n\t\t\treturn fmt.Errorf(\"reservation quantity must be positive\")\n\t\t}\n\t}\n\treturn nil"
)

// writeOrdersGoReserveGuards rewrites orders.go with ValidateQuantity fixed
// (untouched by this scenario) and Reserve/ReserveBatch set to the given
// guard bodies, so mutating one function alone produces a single-hunk diff
// against that function only.
func writeOrdersGoReserveGuards(t *testing.T, worktreePath, reserveBody, reserveBatchBody string) {
	t.Helper()
	content := "package orders\n\nimport \"fmt\"\n\nfunc ValidateQuantity(qty int) error {\n" +
		"\tif qty < 0 {\n\t\treturn fmt.Errorf(\"quantity must be greater than zero\")\n\t}\n\treturn nil\n}\n\n" +
		"func Reserve(qty int) error {\n" + reserveBody + "\n}\n\n" +
		"func ReserveBatch(qtys []int) error {\n" + reserveBatchBody + "\n}\n"
	path := filepath.Join(worktreePath, "orders", "orders.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestE2EMutation_GoTwoGuardMultiSite is the wave's core acceptance: one row
// bound to a test that covers only Reserve, evaluated site-at-a-time against
// two independent guards sharing the same mutation shape.
func TestE2EMutation_GoTwoGuardMultiSite(t *testing.T) {
	repo := fixtureRepo(t)
	headBefore := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	prep := preparedMutation(t, repo, "e2emutation-go", "SC-10")
	if prep.RunnerKind != "go" {
		t.Fatalf("runnerKind = %q, want go", prep.RunnerKind)
	}
	if prep.Identity.Scope != "fixturemod/orders" || prep.Identity.Name != "TestReserveGuard" {
		t.Fatalf("identity = %+v, want fixturemod/orders.TestReserveGuard", prep.Identity)
	}
	if info, err := os.Stat(prep.WorktreePath); err != nil || !info.IsDir() {
		t.Fatalf("worktree at %s does not exist: %v", prep.WorktreePath, err)
	}

	// Site A: flip Reserve's guard alone. TestReserveGuard exercises
	// Reserve directly, so this mutant is caught.
	writeOrdersGoReserveGuards(t, prep.WorktreePath, reserveGuardMutated, reserveBatchGuardDefault)
	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "e2emutation-go", prep.RunDir, "reserve", false, "reserve", "reserve-batch"))
	if code != exitOK {
		t.Fatalf("site reserve exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var siteA evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &siteA); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if siteA.Outcome != "caught" {
		t.Errorf("site reserve outcome = %q, want caught", siteA.Outcome)
	}
	if lines := gitStatusLines(t, prep.WorktreePath); len(lines) != 0 {
		t.Errorf("worktree git status after site reserve = %v, want tracked-clean", lines)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want the worktree to survive a non-lastSite call", left)
	}

	// Site B: flip ReserveBatch's guard. TestReserveGuard never exercises
	// ReserveBatch, so this mutant survives under its own site name.
	writeOrdersGoReserveGuards(t, prep.WorktreePath, reserveGuardDefault, reserveBatchGuardMutated)
	code, stdout, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "e2emutation-go", prep.RunDir, "reserve-batch", true))
	if code != exitOK {
		t.Fatalf("site reserve-batch exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var siteB evaluateMutationOutput
	if err := json.Unmarshal([]byte(stdout), &siteB); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	if siteB.Outcome != "survived" {
		t.Errorf("site reserve-batch outcome = %q, want survived", siteB.Outcome)
	}
	if len(siteB.Sites) != 2 {
		t.Fatalf("sites = %+v, want 2 entries", siteB.Sites)
	}
	if siteB.Sites[0].Site != "reserve" || siteB.Sites[0].Outcome != "caught" {
		t.Errorf("sites[0] = %+v, want reserve caught", siteB.Sites[0])
	}
	if siteB.Sites[1].Site != "reserve-batch" || siteB.Sites[1].Outcome != "survived" {
		t.Errorf("sites[1] = %+v, want reserve-batch survived", siteB.Sites[1])
	}
	if siteB.EvidencePath == "" {
		t.Fatal("evidencePath is empty, want the written evidence file's path")
	}
	assertEvidenceFile(t, siteB.EvidencePath, "e2emutation-go", "SC-10", "go", 2, []string{"caught", "survived"})
	loaded, err := report.LoadMutation(siteB.EvidencePath)
	if err != nil {
		t.Fatalf("finalize consumer rejected real mutation evidence: %v", err)
	}
	src, cliErr := loadVerifiedSource("finalize", repo, fixtureCoverageSource)
	if cliErr != nil {
		t.Fatal(cliErr)
	}
	if loaded.RepoCommit != headBefore || loaded.Source != src.Path || loaded.SourceSHA256 != src.SHA256 {
		t.Fatalf("mutation evidence does not describe measured source/commit: %+v", loaded)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the lastSite call to have disposed of it", left)
	}

	// Multi-site rejection: both guards flipped in one diff before evaluating.
	prep2 := preparedMutation(t, repo, "e2emutation-go-multisite", "SC-10")
	writeOrdersGoReserveGuards(t, prep2.WorktreePath, reserveGuardMutated, reserveBatchGuardMutated)
	code, stdout, stderr = runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSON(t, repo, "e2emutation-go-multisite", prep2.RunDir, "both", false))
	if code != exitError {
		t.Fatalf("multi-site exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "multi-site-diff" {
		t.Errorf("error code = %q, want multi-site-diff", got.Code)
	}
	st2, err := mutation.LoadState(prep2.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st2.Sites) != 0 {
		t.Errorf("state.Sites = %v, want none recorded after a multi-site-diff refusal", st2.Sites)
	}

	// Original-repo isolation: nothing in the whole scenario touched repo itself.
	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
	if head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD")); head != headBefore {
		t.Errorf("HEAD = %q, want unchanged %q", head, headBefore)
	}
}

func prepareMutationRequestJSONFor(t *testing.T, repo, source, runID, rowID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": source,
		"runID":          runID,
		"rowID":          rowID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

func preparedMutationFor(t *testing.T, repo, source, runID, rowID string) prepareMutationOutput {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "prepare-mutation", prepareMutationRequestJSONFor(t, repo, source, runID, rowID))
	if code != exitOK {
		t.Fatalf("prepare-mutation exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out prepareMutationOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("prepare-mutation stdout did not decode: %v\nstdout=%s", err, stdout)
	}
	return out
}

func evaluateMutationRequestJSONFor(t *testing.T, repo, source, runID, runDir, site string, lastSite bool, sites ...string) string {
	t.Helper()
	payload := map[string]any{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": source,
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

func readMutationEvidence(t *testing.T, path string) mutationEvidence {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile evidence: %v", err)
	}
	var ev mutationEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("evidence did not decode: %v\ndata=%s", err, data)
	}
	return ev
}

const (
	mavenGuardDefault = "        if (qty <= 0) {\n            return false;\n        }\n        return true;"
	mavenGuardMutated = "        if (qty < 0) {\n            return false;\n        }\n        return true;"
)

func mutateMavenOrdersGuard(t *testing.T, worktreePath, body string) {
	t.Helper()
	content := "package com.example;\n\npublic final class Orders {\n\n    private Orders() {}\n\n    public static boolean isValidQuantity(int qty) {\n" +
		body + "\n    }\n}\n"
	path := filepath.Join(worktreePath, "src", "main", "java", "com", "example", "Orders.java")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestE2EMutation_MavenSingleSite(t *testing.T) {
	repo := mavenFixtureRepo(t)
	runBaselineAndPinSHA(t, repo, mavenCoverageSource)

	prep := preparedMutationFor(t, repo, mavenCoverageSource, "maven-mutation", "SC-07")
	if prep.RunnerKind != "maven" {
		t.Fatalf("runnerKind = %q, want maven", prep.RunnerKind)
	}
	if prep.Identity.Scope != "com.example.OrdersTest" || prep.Identity.Name != "validatesQuantityDeliberately" {
		t.Fatalf("identity = %+v, want com.example.OrdersTest.validatesQuantityDeliberately", prep.Identity)
	}

	mutateMavenOrdersGuard(t, prep.WorktreePath, mavenGuardMutated)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSONFor(t, repo, mavenCoverageSource, "maven-mutation", prep.RunDir, "quantity-guard", true))
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

	ev := readMutationEvidence(t, out.EvidencePath)
	if ev.RunID != "maven-mutation" {
		t.Errorf("evidence.RunID = %q, want maven-mutation", ev.RunID)
	}
	wantFlag := "-Dtest=com.example.OrdersTest#validatesQuantityDeliberately"
	if !slices.Contains(ev.TargetCommand, wantFlag) {
		t.Errorf("targetCommand = %v, want it to contain %q", ev.TargetCommand, wantFlag)
	}

	// A wrongly-scoped report set (failsafe instead of surefire) would have
	// found zero fresh XML and failed normalization instead of reaching a
	// verdict; the caught outcome above already proves surefire's reports
	// were read. Confirm the resolved glob set directly too.
	src, cliErr := loadVerifiedSource("test", repo, mavenCoverageSource)
	if cliErr != nil {
		t.Fatalf("loadVerifiedSource: %v", cliErr)
	}
	target, err := mutation.JVMTarget(src.Runner, mutation.Identity{Scope: prep.Identity.Scope, Name: prep.Identity.Name})
	if err != nil {
		t.Fatalf("JVMTarget: %v", err)
	}
	if len(target.ReportGlobs) != 1 || !strings.Contains(target.ReportGlobs[0], "surefire-reports") {
		t.Errorf("reportGlobs = %v, want exactly the surefire-reports glob", target.ReportGlobs)
	}
}

func TestE2EMutation_MavenTestFileDiffRefusedAsForeign(t *testing.T) {
	repo := mavenFixtureRepo(t)
	runBaselineAndPinSHA(t, repo, mavenCoverageSource)

	prep := preparedMutationFor(t, repo, mavenCoverageSource, "maven-mutation-foreign", "SC-07")

	testPath := filepath.Join(prep.WorktreePath, "src", "test", "java", "com", "example", "OrdersTest.java")
	data, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(testPath, append(data, []byte("\n// mutated test file\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSONFor(t, repo, mavenCoverageSource, "maven-mutation-foreign", prep.RunDir, "site-a", false))
	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "foreign-diff" {
		t.Errorf("error code = %q, want foreign-diff", got.Code)
	}
}

const (
	gradleGuardDefault = "        if (qty <= 0) {\n            return false\n        }\n        return true"
	gradleGuardMutated = "        if (qty < 0) {\n            return false\n        }\n        return true"
)

func mutateGradleOrdersGuard(t *testing.T, worktreePath, body string) {
	t.Helper()
	content := "package com.example\n\nobject Orders {\n    fun isValidQuantity(qty: Int): Boolean {\n" +
		body + "\n    }\n}\n"
	path := filepath.Join(worktreePath, "src", "main", "kotlin", "com", "example", "Orders.kt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestE2EMutation_GradleSingleSite(t *testing.T) {
	repo := gradleFixtureRepo(t)
	runBaselineAndPinSHA(t, repo, gradleCoverageSource)

	prep := preparedMutationFor(t, repo, gradleCoverageSource, "gradle-mutation", "SC-04")
	if prep.RunnerKind != "gradle" {
		t.Fatalf("runnerKind = %q, want gradle", prep.RunnerKind)
	}
	if prep.Identity.Scope != "com.example.OrderKtTest" || prep.Identity.Name != "validatesQuantityDeliberately" {
		t.Fatalf("identity = %+v, want com.example.OrderKtTest.validatesQuantityDeliberately", prep.Identity)
	}

	mutateGradleOrdersGuard(t, prep.WorktreePath, gradleGuardMutated)

	code, stdout, stderr := runVerifyOp(t, "evaluate-mutation",
		evaluateMutationRequestJSONFor(t, repo, gradleCoverageSource, "gradle-mutation", prep.RunDir, "quantity-guard", true))
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

	ev := readMutationEvidence(t, out.EvidencePath)
	if ev.RunID != "gradle-mutation" {
		t.Errorf("evidence.RunID = %q, want gradle-mutation", ev.RunID)
	}
	wantArg := "com.example.OrderKtTest.validatesQuantityDeliberately"
	if !slices.Contains(ev.TargetCommand, "--tests") || !slices.Contains(ev.TargetCommand, wantArg) {
		t.Errorf("targetCommand = %v, want it to contain --tests %q", ev.TargetCommand, wantArg)
	}
	if !slices.Contains(ev.TargetCommand, "--rerun") {
		t.Errorf("targetCommand = %v, want the configured --rerun flag retained", ev.TargetCommand)
	}
}
