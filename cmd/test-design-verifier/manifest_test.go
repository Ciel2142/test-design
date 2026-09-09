package main

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// tracedDispatch runs the real trace op over repo and returns the dispatch
// inputs it wrote, so a manifest is captured from a document a run produced
// rather than one a test invented.
func tracedDispatch(t *testing.T, repo, runID string) dispatchInputs {
	t.Helper()
	code, stdout, stderr := runVerifyOp(t, "trace", verifyRequestJSON(t, repo, runID))
	if code != exitOK {
		t.Fatalf("trace exit code = %d, want %d; stderr=%s", code, exitOK, stderr)
	}
	return readDispatchInputs(t, decodeTraceOutput(t, stdout).DispatchPath)
}

// writePolicyFile writes an absolute critic instruction file outside the
// repository, the way a parent hands its critic's own instructions over.
func writePolicyFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeRepoFile(t, filepath.Dir(path), filepath.Base(path), content)
	return path
}

// bundlePathsOf names every file the dispatch document hands the critic for
// rows: each row's primary site and every contributor's, deduplicated.
func bundlePathsOf(t *testing.T, dispatch dispatchInputs, rows ...string) []string {
	t.Helper()
	var paths []string
	for _, id := range rows {
		row := dispatchRowFor(t, dispatch, id)
		for _, file := range append([]string{row.File}, contributorFiles(row)...) {
			if file != "" && !slices.Contains(paths, file) {
				paths = append(paths, file)
			}
		}
	}
	slices.Sort(paths)
	return paths
}

// TestManifest_CaptureRecordsTheInputsTheCriticWasHanded walks what a review
// group's manifest must carry at started time: the approved claim of every
// requested row and no other, the whole file behind every site the critic was
// handed, the runner and toolchain the trace recorded, and the critic policy
// the parent named. Reads are not known yet, so the manifest is incomplete for
// exactly that reason.
func TestManifest_CaptureRecordsTheInputsTheCriticWasHanded(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "manifest-capture"
	dispatch := tracedDispatch(t, repo, runID)
	identity := runstate.Identity{RunID: runID, RepoRoot: repo, RepoCommit: dispatch.RepoCommit}
	rows := []string{"SC-01", "SC-02"}
	instructions := writePolicyFile(t, "critic.md", "judge the assertion, not the name\n")
	policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{instructions}}

	m := captureReviewManifest(repo, identity, &dispatch, rows, policy)

	if m == nil {
		t.Fatal("captureReviewManifest returned nil, want a manifest")
	}
	if m.Version != evidence.ManifestVersion {
		t.Errorf("version = %q, want %q", m.Version, evidence.ManifestVersion)
	}
	if m.RepoCommit != dispatch.RepoCommit {
		t.Errorf("repoCommit = %q, want the commit HEAD carries now (%q)", m.RepoCommit, dispatch.RepoCommit)
	}
	wantClaims := map[string]string{}
	for _, id := range rows {
		wantClaims[id] = dispatchRowFor(t, dispatch, id).ClaimSHA256
	}
	if !reflect.DeepEqual(m.Claims, wantClaims) {
		t.Errorf("claims = %v, want the dispatch document's own claim digests for the requested rows only (%v)", m.Claims, wantClaims)
	}
	wantPaths := bundlePathsOf(t, dispatch, rows...)
	gotPaths := slices.Sorted(maps.Keys(m.Bundle))
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Errorf("bundle paths = %v, want every primary and contributor file of the rows (%v)", gotPaths, wantPaths)
	}
	for _, path := range wantPaths {
		want, err := evidence.SHA256File(filepath.Join(repo, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("SHA256File %s: %v", path, err)
		}
		if m.Bundle[path] != want {
			t.Errorf("bundle[%s] = %q, want the whole file's digest %q", path, m.Bundle[path], want)
		}
	}
	if !reflect.DeepEqual(m.Runner, dispatch.Runner) {
		t.Errorf("runner = %+v, want the dispatch document's %+v", m.Runner, dispatch.Runner)
	}
	if !reflect.DeepEqual(m.ToolVersions, dispatch.ToolVersions) {
		t.Errorf("toolVersions = %v, want the dispatch document's %v", m.ToolVersions, dispatch.ToolVersions)
	}
	if m.Policy == nil || m.Policy.Critic != "coverage-critic" {
		t.Fatalf("policy = %+v, want the named critic", m.Policy)
	}
	wantInstruction, err := evidence.SHA256File(instructions)
	if err != nil {
		t.Fatalf("SHA256File %s: %v", instructions, err)
	}
	if got := m.Policy.Instructions[instructions]; got != wantInstruction {
		t.Errorf("policy.instructions[%s] = %q, want the file's digest %q", instructions, got, wantInstruction)
	}
	if want := []string{"reads-unrecorded"}; !reflect.DeepEqual(m.Incomplete, want) {
		t.Errorf("incomplete = %v, want %v: nothing else about a started group is unknown", m.Incomplete, want)
	}
	if m.Complete {
		t.Error("complete = true, want false while the critic's reads are unrecorded")
	}
}

// TestManifest_CaptureNamesEveryInputItCouldNotIdentify: capture never refuses
// a review — a problem with the policy, a moved HEAD or a dirty tree is
// recorded as a reason the manifest is incomplete, which is what makes the
// group ineligible for reuse later without touching this run's verdict.
func TestManifest_CaptureNamesEveryInputItCouldNotIdentify(t *testing.T) {
	repo := fixtureRepo(t)
	const runID = "manifest-incomplete"
	dispatch := tracedDispatch(t, repo, runID)
	identity := runstate.Identity{RunID: runID, RepoRoot: repo, RepoCommit: dispatch.RepoCommit}
	rows := []string{"SC-01"}
	good := writePolicyFile(t, "critic.md", "judge the assertion\n")

	t.Run("no policy at all", func(t *testing.T) {
		m := captureReviewManifest(repo, identity, &dispatch, rows, nil)
		requireIncomplete(t, m, "policy-unrecorded")
		if m.Policy != nil {
			t.Errorf("policy = %+v, want none recorded", m.Policy)
		}
	})

	t.Run("a relative instruction path", func(t *testing.T) {
		policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{"docs/critic.md"}}
		m := captureReviewManifest(repo, identity, &dispatch, rows, policy)
		requireIncomplete(t, m, "policy-path-not-absolute:docs/critic.md")
	})

	t.Run("an unreadable instruction file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "gone.md")
		policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{good, missing}}
		m := captureReviewManifest(repo, identity, &dispatch, rows, policy)
		requireIncomplete(t, m, "policy-unreadable:"+missing)
		if _, listed := m.Policy.Instructions[missing]; listed {
			t.Errorf("policy.instructions = %v, want the unreadable file left out", m.Policy.Instructions)
		}
		if m.Policy.Instructions[good] == "" {
			t.Errorf("policy.instructions = %v, want the readable file still digested", m.Policy.Instructions)
		}
	})

	t.Run("HEAD moved away from the run's identity", func(t *testing.T) {
		moved := runstate.Identity{RunID: runID, RepoRoot: repo, RepoCommit: "deadbeef"}
		policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{good}}
		m := captureReviewManifest(repo, moved, &dispatch, rows, policy)
		requireIncomplete(t, m, "head-moved")
	})

	t.Run("a file the critic would be handed cannot be digested", func(t *testing.T) {
		// trackedAt-style existence is not enough: a path git carries can be a
		// tree, and a file can vanish between the trace and the review. Either
		// way there is no digest to record, and recording an error string in
		// place of one would let a later run match that same string.
		row := dispatchRowFor(t, dispatch, "SC-01")
		path := filepath.Join(repo, filepath.FromSlash(row.File))
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", row.File, err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("replace %s with a directory: %v", row.File, err)
		}
		t.Cleanup(func() {
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove the directory standing in for %s: %v", row.File, err)
			}
			mustGit(t, repo, "checkout", "--", row.File)
		})
		policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{good}}

		m := captureReviewManifest(repo, identity, &dispatch, rows, policy)

		requireIncomplete(t, m, "unidentifiable-read:"+row.File)
		if got, listed := m.Bundle[row.File]; listed {
			t.Errorf("bundle[%s] = %q, want the path left out rather than an error stored where a digest belongs", row.File, got)
		}
	})

	t.Run("a dirty tree", func(t *testing.T) {
		writeRepoFile(t, repo, "orders/scratch.txt", "uncommitted\n")
		t.Cleanup(func() { mustGit(t, repo, "clean", "-qfd") })
		policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{good}}
		m := captureReviewManifest(repo, identity, &dispatch, rows, policy)
		requireIncomplete(t, m, "tree-dirty:orders/scratch.txt")
	})
}

// requireIncomplete asserts the manifest is incomplete for reason, and that
// capture answered rather than refused.
func requireIncomplete(t *testing.T, m *evidence.Manifest, reason string) {
	t.Helper()
	if m == nil {
		t.Fatal("manifest = nil, want a manifest: capture never refuses")
	}
	if !slices.Contains(m.Incomplete, reason) {
		t.Errorf("incomplete = %v, want %q among them", m.Incomplete, reason)
	}
	if m.Complete {
		t.Errorf("complete = true, want false while %v is unresolved", m.Incomplete)
	}
}

// TestManifest_CompletionJudgesTheReadsTheCriticClaims: reads are a claim the
// parent relays, so each one must be a file this repository can identify — a
// clean repo-relative path tracked at HEAD. An absent list is unrecorded, an
// empty one says the critic read nothing beyond its bundle, and only a
// completed manifest with nothing unidentified is complete.
func TestManifest_CompletionJudgesTheReadsTheCriticClaims(t *testing.T) {
	repo := fixtureRepo(t)
	head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	identity := runstate.Identity{RunID: "manifest-reads", RepoRoot: repo, RepoCommit: head}
	// The started manifest as capture leaves it: reads unrecorded, everything
	// else identified.
	started := func() *evidence.Manifest {
		return &evidence.Manifest{
			Version: evidence.ManifestVersion, RepoCommit: head,
			Claims:     map[string]string{"SC-01": "claim"},
			Bundle:     evidence.FileDigest{"orders/orders_test.go": "digest"},
			Incomplete: []string{"reads-unrecorded"},
		}
	}

	t.Run("a pre-wave record has no manifest to complete", func(t *testing.T) {
		if m := completeReviewManifest(repo, identity, nil, nil); m != nil {
			t.Errorf("completeReviewManifest(nil) = %+v, want nil", m)
		}
	})

	t.Run("reads absent", func(t *testing.T) {
		m := completeReviewManifest(repo, identity, started(), nil)
		requireIncomplete(t, m, "reads-unrecorded")
	})

	t.Run("reads empty", func(t *testing.T) {
		m := completeReviewManifest(repo, identity, started(), &[]string{})
		if !m.Complete || len(m.Incomplete) != 0 {
			t.Fatalf("manifest = complete %v incomplete %v, want complete: the critic read nothing beyond its bundle", m.Complete, m.Incomplete)
		}
		if m.Reads == nil || len(m.Reads) != 0 {
			t.Errorf("reads = %v, want an empty record of reads rather than none", m.Reads)
		}
	})

	t.Run("reads this repository cannot identify", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "notes.md")
		writeRepoFile(t, filepath.Dir(outside), "notes.md", "outside\n")
		writeRepoFile(t, repo, "orders/untracked.go", "package orders\n")
		t.Cleanup(func() { mustGit(t, repo, "clean", "-qfd") })
		// "orders" is tracked at HEAD — as a tree — so existence alone would
		// accept it and store its read error where a digest belongs.
		reads := []string{outside, "../elsewhere/notes.md", "orders/untracked.go", "orders"}

		m := completeReviewManifest(repo, identity, started(), &reads)

		for _, path := range reads {
			requireIncomplete(t, m, "unidentifiable-read:"+path)
		}
		if len(m.Reads) != 0 {
			t.Errorf("reads = %v, want no digest for a path the repository cannot identify", m.Reads)
		}
	})

	t.Run("a tracked read", func(t *testing.T) {
		reads := []string{"orders/orders.go"}
		m := completeReviewManifest(repo, identity, started(), &reads)
		if !m.Complete || len(m.Incomplete) != 0 {
			t.Fatalf("manifest = complete %v incomplete %v, want complete once every read is identified", m.Complete, m.Incomplete)
		}
		want, err := evidence.SHA256File(filepath.Join(repo, "orders", "orders.go"))
		if err != nil {
			t.Fatalf("SHA256File: %v", err)
		}
		if got := m.Reads["orders/orders.go"]; got != want {
			t.Errorf("reads[orders/orders.go] = %q, want the file's digest %q", got, want)
		}
		if !reflect.DeepEqual(m.Bundle, started().Bundle) || !reflect.DeepEqual(m.Claims, started().Claims) {
			t.Errorf("manifest = %+v, want the started bundle and claims kept", m)
		}
	})

	t.Run("a policy problem recorded at started time survives completion", func(t *testing.T) {
		prior := started()
		prior.Incomplete = []string{"policy-unrecorded", "reads-unrecorded"}
		m := completeReviewManifest(repo, identity, prior, &[]string{})
		if want := []string{"policy-unrecorded"}; !reflect.DeepEqual(m.Incomplete, want) {
			t.Errorf("incomplete = %v, want %v: the policy is still unrecorded, the reads no longer are", m.Incomplete, want)
		}
	})

	t.Run("HEAD moved between started and completed", func(t *testing.T) {
		moved := runstate.Identity{RunID: "manifest-reads", RepoRoot: repo, RepoCommit: "deadbeef"}
		m := completeReviewManifest(repo, moved, started(), &[]string{})
		requireIncomplete(t, m, "head-moved")
	})
}

// TestManifest_CompletionNeverWithdrawsAReasonCaptureRecorded: incompleteness
// is monotonic. A group captured against a dirty tree holds digests of bytes no
// commit carries, and cleaning the tree before the group is completed does not
// turn those digests back into something a later run could identify. A reason
// observed once is carried forward, and never twice.
func TestManifest_CompletionNeverWithdrawsAReasonCaptureRecorded(t *testing.T) {
	repo := fixtureRepo(t)
	head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
	identity := runstate.Identity{RunID: "manifest-monotonic", RepoRoot: repo, RepoCommit: head}
	dispatch := dispatchInputs{Rows: []dispatchRow{{
		RowID: "SC-01", File: "orders/orders_test.go", ClaimSHA256: "claim",
	}}}
	policy := &reviewPolicy{Critic: "coverage-critic", Instructions: []string{
		writePolicyFile(t, "critic.md", "judge the assertion\n"),
	}}
	writeRepoFile(t, repo, "orders/scratch.txt", "uncommitted\n")
	t.Cleanup(func() { mustGit(t, repo, "clean", "-qfd") })

	started := captureReviewManifest(repo, identity, &dispatch, []string{"SC-01"}, policy)
	requireIncomplete(t, started, "tree-dirty:orders/scratch.txt")

	stillDirty := completeReviewManifest(repo, identity, started, &[]string{})
	if got := slices.Compact(slices.Clone(stillDirty.Incomplete)); len(got) != len(stillDirty.Incomplete) {
		t.Errorf("incomplete = %v, want each reason once", stillDirty.Incomplete)
	}

	mustGit(t, repo, "clean", "-qfd")

	cleaned := completeReviewManifest(repo, identity, started, &[]string{})
	requireIncomplete(t, cleaned, "tree-dirty:orders/scratch.txt")
	if !slices.Contains(started.Incomplete, "tree-dirty:orders/scratch.txt") {
		t.Errorf("started manifest = %v, want completion to have left it alone", started.Incomplete)
	}
}
