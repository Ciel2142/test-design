package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"test-design/internal/live"
)

const liveFinalCommandLine = `- final-command: ["/test-design:coverage-verification", "docs/superpowers/coverage/2026-08-30-fixture-coverage.md"]`

const liveSC10Line = "| SC-10 | reservation quantity guard rejects non-positive input | Reserve rejects qty <= 0 | unit | reservation guard uses `<` instead of `<=` | high |"

func prepareLiveRequestJSON(t *testing.T, repo, runID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"schemaVersion":  "1",
		"repoRoot":       repo,
		"coverageSource": fixtureCoverageSource,
		"runID":          runID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(body)
}

// live-profile header carries a full repo-relative path, not a bare name
// (internal/coverage/testdata/valid-source.md:14).
func liveProfilePath(name string) string {
	return ".test-design/live-profiles/" + name + ".json"
}

// addLiveProfileHeader points the coverage doc's live-profile bullet at profileRelPath.
func addLiveProfileHeader(t *testing.T, repo, profileRelPath string) {
	t.Helper()
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.Replace(s, liveFinalCommandLine, liveFinalCommandLine+"\n- live-profile: "+profileRelPath, 1)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "point coverage doc at a live profile")
}

// addLiveDecision inserts a ## Live decision section carrying body into the
// coverage doc, ahead of ## Retired, and commits it.
func addLiveDecision(t *testing.T, repo, body string) {
	t.Helper()
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.Replace(s, "## Retired", "## Live decision\n\n"+body+"\n## Retired", 1)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "record the coverage doc's live decision")
}

// addNotApplicableLiveDecision records the decision a document of unit-level
// rows only is entitled to.
func addNotApplicableLiveDecision(t *testing.T, repo string) {
	t.Helper()
	addLiveDecision(t, repo, "- decision: not-applicable\n"+
		"- reason: every active row of this document is checkable without a running system\n")
}

// addSmokeRow appends a smoke-level scenario row (SC-20) to the coverage doc
// together with the ## Live decision that row obliges the document to carry: an
// active smoke row in no journey is an incomplete live decision.
func addSmokeRow(t *testing.T, repo string) {
	t.Helper()
	rewriteFixtureDoc(t, repo, func(s string) string {
		return strings.Replace(s, liveSC10Line, liveSC10Line+"\n"+
			"| SC-20 | live smoke check exercises the running app | smoke walkthrough completes | smoke | n/a | low |", 1)
	})
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "add a smoke-level scenario row")
	addLiveDecision(t, repo, "- decision: required\n"+
		"- reason: SC-20 is only observable against the running app\n\n"+
		"| Journey | Scenarios | Expected |\n"+
		"|---|---|---|\n"+
		"| J-1 | SC-20 | the smoke walkthrough completes against the running app |\n")
}

// writeLiveProfileFixture writes and commits repo/.test-design/live-profiles/<name>.json.
func writeLiveProfileFixture(t *testing.T, repo, name string, overrides map[string]any) {
	t.Helper()
	doc := map[string]any{
		"schemaVersion":  "1",
		"classification": "local",
		"start":          [][]string{{"./scripts/start.sh"}},
		"health":         [][]string{{"./scripts/health.sh"}},
		"walkthrough":    [][]string{{"./scripts/walk.sh", "--run={run-id}"}},
		"cleanup":        [][]string{{"./scripts/cleanup.sh"}},
		"cleanupEvidence": []map[string]string{
			{"form": "exit-code", "stream": "combined", "value": "0"},
		},
		"envAllowlist":     []string{"PATH"},
		"resourcePrefixes": []string{"fixture-{run-id}"},
		"endpoints":        []string{"http://127.0.0.1:8099"},
	}
	for k, v := range overrides {
		doc[k] = v
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal live profile fixture: %v", err)
	}

	dir := filepath.Join(repo, ".test-design", "live-profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "add live profile fixture "+name)
}

func TestPrepareLive_DirtyRepositoryRefused(t *testing.T) {
	repo := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "orders", "scratch.txt"), []byte("dirt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-dirty"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "source-not-reproducible" {
		t.Errorf("error code = %q, want source-not-reproducible", got.Code)
	}
	if !strings.Contains(stderr, "scratch.txt") {
		t.Errorf("stderr = %q, want it to name the dirty path", stderr)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want no worktree created before the dirty refusal", left)
	}
}

// TestPrepareLive_MissingLiveDecisionRefused locks the layer-3 gate: a coverage
// source with no ## Live decision is incomplete design, and prepare-live says so
// rather than reading the absence as "nothing to verify live".
func TestPrepareLive_MissingLiveDecisionRefused(t *testing.T) {
	repo := fixtureRepo(t)

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-no-decision"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want live-decision-incomplete", got.Code)
	}
	if !strings.Contains(got.Message, "no ## Live decision section") {
		t.Errorf("message = %q, want the missing code's own detail, not any refusal carrying \"missing\"", got.Message)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want no worktree created before the refusal", left)
	}
}

// TestPrepareLive_NoApplicableRowsRefused: no-applicable-rows now means the
// document decided live verification does not apply, not that no row happens to
// be smoke or e2e.
func TestPrepareLive_NoApplicableRowsRefused(t *testing.T) {
	repo := fixtureRepo(t)
	addNotApplicableLiveDecision(t, repo)

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-no-rows"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "no-applicable-rows" {
		t.Errorf("error code = %q, want no-applicable-rows", got.Code)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want no worktree created before the refusal", left)
	}
}

// TestPrepareLive_NoLiveProfileHeaderRefused: a required decision with no
// live-profile header names no target, so the gate refuses it before a run dir
// exists to write an attempt document into.
func TestPrepareLive_NoLiveProfileHeaderRefused(t *testing.T) {
	repo := fixtureRepo(t)
	addSmokeRow(t, repo)

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-no-header"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "live-decision-incomplete" {
		t.Errorf("error code = %q, want live-decision-incomplete", got.Code)
	}
	if !strings.Contains(got.Message, "target-missing") {
		t.Errorf("message = %q, want it to name the missing live-profile target", got.Message)
	}
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want no worktree created before the refusal", left)
	}
}

func TestPrepareLive_MissingProfileRefused(t *testing.T) {
	repo := fixtureRepo(t)
	addSmokeRow(t, repo)
	profilePath := liveProfilePath("does-not-exist")
	addLiveProfileHeader(t, repo, profilePath)

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-missing-profile"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "profile-unavailable" {
		t.Errorf("error code = %q, want profile-unavailable", got.Code)
	}
	attemptPath := filepath.Join(verifierRunsDir(t), "live-missing-profile", "live-attempt.json")
	if !strings.Contains(stderr, attemptPath) {
		t.Errorf("stderr = %q, want it to name the attempt-evidence path %s", stderr, attemptPath)
	}
	assertAttemptEvidence(t, attemptPath, "live-missing-profile", profilePath, profilePath)
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareLive_InvalidProfileRefused(t *testing.T) {
	repo := fixtureRepo(t)
	addSmokeRow(t, repo)
	profilePath := liveProfilePath("fixture-production")
	addLiveProfileHeader(t, repo, profilePath)
	writeLiveProfileFixture(t, repo, "fixture-production", map[string]any{"classification": "production"})

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-invalid-profile"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "profile-unavailable" {
		t.Errorf("error code = %q, want profile-unavailable", got.Code)
	}
	if !strings.Contains(stderr, "unsupported-classification") {
		t.Errorf("stderr = %q, want it to embed the unsupported-classification refusal", stderr)
	}
	attemptPath := filepath.Join(verifierRunsDir(t), "live-invalid-profile", "live-attempt.json")
	assertAttemptEvidence(t, attemptPath, "live-invalid-profile", profilePath, "unsupported-classification")
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareLive_PathTraversingProfileNameRefused(t *testing.T) {
	repo := fixtureRepo(t)
	addSmokeRow(t, repo)
	const traversal = "../../etc/evil.json"
	addLiveProfileHeader(t, repo, traversal)

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-traversal"))

	if code != exitError {
		t.Fatalf("exit code = %d, want %d; stdout=%s", code, exitError, stdout)
	}
	got := decodeCLIError(t, stderr)
	if got.Code != "profile-unavailable" {
		t.Errorf("error code = %q, want profile-unavailable", got.Code)
	}
	if !strings.Contains(stderr, "live-profile-outside-repo") || !strings.Contains(stderr, "escapes its root") {
		t.Errorf("stderr = %q, want it to embed confinedRelPath's live-profile-outside-repo refusal", stderr)
	}
	attemptPath := filepath.Join(verifierRunsDir(t), "live-traversal", "live-attempt.json")
	assertAttemptEvidence(t, attemptPath, "live-traversal", traversal, "escapes its root")
	if left := verifierWorktrees(t); len(left) != 0 {
		t.Errorf("worktrees registered = %v, want the failure path to have disposed of it", left)
	}
}

func TestPrepareLive_GoFixtureHappyPath(t *testing.T) {
	repo := fixtureRepo(t)
	addSmokeRow(t, repo)
	addLiveProfileHeader(t, repo, liveProfilePath("fixture-local"))
	writeLiveProfileFixture(t, repo, "fixture-local", nil)

	code, stdout, stderr := runVerifyOp(t, "prepare-live", prepareLiveRequestJSON(t, repo, "live-happy"))

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	var out prepareLiveOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout did not decode: %v\nstdout=%s", err, stdout)
	}

	if out.RunID != "live-happy" {
		t.Errorf("runID = %q, want live-happy", out.RunID)
	}
	if out.WorktreePath == "" {
		t.Error("worktreePath is empty, want the created worktree's path")
	}
	if out.RunDir == "" {
		t.Error("runDir is empty, want the allocated run directory")
	}
	wantProfilePath := ".test-design/live-profiles/fixture-local.json"
	if out.ProfilePath != wantProfilePath {
		t.Errorf("profilePath = %q, want %q", out.ProfilePath, wantProfilePath)
	}

	var descriptor map[string]any
	if err := json.Unmarshal(out.Descriptor, &descriptor); err != nil {
		t.Fatalf("descriptor did not decode: %v\ndescriptor=%s", err, out.Descriptor)
	}
	walkthrough, ok := descriptor["walkthrough"].([]any)
	if !ok || len(walkthrough) != 1 {
		t.Fatalf("descriptor.walkthrough = %+v, want one step", descriptor["walkthrough"])
	}
	step, ok := walkthrough[0].([]any)
	if !ok || len(step) != 2 || step[1] != "--run=live-happy" {
		t.Errorf("descriptor.walkthrough[0] = %+v, want [\"./scripts/walk.sh\" \"--run=live-happy\"]", step)
	}
	resourcePrefixes, ok := descriptor["resourcePrefixes"].([]any)
	if !ok || len(resourcePrefixes) != 1 || resourcePrefixes[0] != "fixture-live-happy" {
		t.Errorf("descriptor.resourcePrefixes = %+v, want [fixture-live-happy]", descriptor["resourcePrefixes"])
	}
	if strings.Contains(string(out.Descriptor), "{run-id}") {
		t.Errorf("descriptor = %s, want no remaining {run-id} placeholder", out.Descriptor)
	}

	if info, err := os.Stat(out.WorktreePath); err != nil || !info.IsDir() {
		t.Errorf("worktree at %s does not exist after success, want it to survive: %v", out.WorktreePath, err)
	}
	if left := verifierWorktrees(t); len(left) != 1 {
		t.Errorf("worktrees registered = %v, want exactly one surviving worktree", left)
	}

	st, err := live.LoadState(out.RunDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.RunID != out.RunID {
		t.Errorf("state.RunID = %q, want %q", st.RunID, out.RunID)
	}
	if st.WorktreePath != out.WorktreePath {
		t.Errorf("state.WorktreePath = %q, want %q", st.WorktreePath, out.WorktreePath)
	}
	if st.ProfilePath != out.ProfilePath {
		t.Errorf("state.ProfilePath = %q, want %q", st.ProfilePath, out.ProfilePath)
	}
	if len(st.Snapshot) == 0 {
		t.Error("state.Snapshot is empty, want the before snapshot recorded")
	}
	var stDescriptor map[string]any
	if err := json.Unmarshal(st.Descriptor, &stDescriptor); err != nil {
		t.Fatalf("unmarshal state.Descriptor: %v", err)
	}
	if !equalJSON(t, stDescriptor, descriptor) {
		t.Errorf("state.Descriptor = %s, want it to match the response descriptor %s", st.Descriptor, out.Descriptor)
	}

	if lines := gitStatusLines(t, repo); len(lines) != 0 {
		t.Errorf("git status = %v, want the original repository untouched", lines)
	}
}

// verifierRunsDir locates the runs dir so a test can name an exact expected run-dir path.
func verifierRunsDir(t *testing.T) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "test-design-verifier-*", "runs"))
	if err != nil {
		t.Fatalf("glob runs dir: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("runs dirs = %v, want exactly one", matches)
	}
	return matches[0]
}

func equalJSON(t *testing.T, a, b map[string]any) bool {
	t.Helper()
	return reflect.DeepEqual(a, b)
}

func assertAttemptEvidence(t *testing.T, path, wantRunID, wantProfile, wantRefusalSubstring string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read attempt evidence %s: %v", path, err)
	}
	var doc liveAttemptOutput
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("attempt evidence did not decode: %v\ndata=%s", err, data)
	}
	if doc.SchemaVersion == "" {
		t.Error("attempt evidence schemaVersion is empty")
	}
	if doc.RunID != wantRunID {
		t.Errorf("attempt evidence runID = %q, want %q", doc.RunID, wantRunID)
	}
	if doc.Profile != wantProfile {
		t.Errorf("attempt evidence profile = %q, want %q", doc.Profile, wantProfile)
	}
	if !strings.Contains(doc.Refusal, wantRefusalSubstring) {
		t.Errorf("attempt evidence refusal = %q, want it to contain %q", doc.Refusal, wantRefusalSubstring)
	}
}
