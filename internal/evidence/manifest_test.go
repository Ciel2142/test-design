package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- VerifyFile ---

func TestVerifyFileNilOnEqualDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	content := []byte("stable content")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want, err := SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}

	if err := VerifyFile(path, want); err != nil {
		t.Errorf("VerifyFile(%q, %q) = %v, want nil", path, want, err)
	}
}

func TestVerifyFileChangedByteReturnsWrappedDigestMismatchNamingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	if err := os.WriteFile(path, []byte("original content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want, err := SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	if err := os.WriteFile(path, []byte("changed content!"), 0o644); err != nil {
		t.Fatalf("WriteFile (mutate): %v", err)
	}

	err = VerifyFile(path, want)
	if err == nil {
		t.Fatalf("VerifyFile(%q, %q) returned nil error, want error", path, want)
	}
	if !errors.Is(err, ErrDigestMismatch) {
		t.Errorf("VerifyFile error %v does not wrap ErrDigestMismatch", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("VerifyFile error %q does not name path %q", err.Error(), path)
	}
}

func TestVerifyFileMissingFileWrapsFsErrNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.txt")

	err := VerifyFile(path, "irrelevant-digest")
	if err == nil {
		t.Fatalf("VerifyFile(%q, ...) returned nil error, want error", path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("VerifyFile error %v does not wrap fs.ErrNotExist", err)
	}
}

// --- DigestFiles ---

func TestDigestFilesReturnsDigestsForPresentAndErrorStringsForMissing(t *testing.T) {
	root := t.TempDir()
	presentRel := filepath.Join("nested", "present.txt")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("digest-files fixture")
	if err := os.WriteFile(filepath.Join(root, presentRel), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sum := sha256.Sum256(content)
	wantDigest := hex.EncodeToString(sum[:])

	missingRel := "missing.txt"
	_, missingErr := SHA256File(filepath.Join(root, missingRel))
	if missingErr == nil {
		t.Fatalf("expected SHA256File on missing fixture path to error")
	}
	wantMissingMsg := fmt.Sprintf("%s: %s", missingRel, missingErr)

	paths := []string{presentRel, missingRel}
	digests, problems := DigestFiles(root, paths)

	if len(digests) != 2 {
		t.Fatalf("DigestFiles returned %d entries, want 2: %+v", len(digests), digests)
	}
	if got := digests[presentRel]; got != wantDigest {
		t.Errorf("DigestFiles digest for %q = %q, want %q", presentRel, got, wantDigest)
	}
	if got := digests[missingRel]; got != wantMissingMsg {
		t.Errorf("DigestFiles digest for %q = %q, want %q", missingRel, got, wantMissingMsg)
	}

	if len(problems) != 1 || problems[0] != wantMissingMsg {
		t.Errorf("DigestFiles problems = %v, want [%q]", problems, wantMissingMsg)
	}
}

// --- ClaimDigest ---

func baselineClaim() Claim {
	return Claim{
		ID:       "row-1",
		Behavior: "returns nil on success",
		Expected: "err is nil",
		Level:    "unit",
		Mutation: "flip return value",
		Risk:     "high",
	}
}

func TestClaimDigestStableAcrossCalls(t *testing.T) {
	c := baselineClaim()
	first := ClaimDigest(c)
	for i := 0; i < 5; i++ {
		if got := ClaimDigest(c); got != first {
			t.Errorf("ClaimDigest call %d = %q, want %q (stable)", i, got, first)
		}
	}
}

func TestClaimDigestEqualsSHA256OfDeclaredCanonicalJSON(t *testing.T) {
	c := baselineClaim()
	literal := fmt.Sprintf(
		`{"id":%q,"behavior":%q,"expected":%q,"level":%q,"mutation":%q,"risk":%q}`,
		c.ID, c.Behavior, c.Expected, c.Level, c.Mutation, c.Risk,
	)

	// The package's own canonical encoder path must reproduce the same bytes,
	// independent of the ClaimDigest hashing step.
	data, err := canonicalJSON(c)
	if err != nil {
		t.Fatalf("canonicalJSON: %v", err)
	}
	if string(data) != literal {
		t.Fatalf("canonicalJSON(c) = %s, want %s", data, literal)
	}

	sum := sha256.Sum256([]byte(literal))
	want := hex.EncodeToString(sum[:])

	if got := ClaimDigest(c); got != want {
		t.Errorf("ClaimDigest(c) = %q, want %q", got, want)
	}
}

func TestClaimDigestDiffersWhenAnyFieldDiffers(t *testing.T) {
	base := baselineClaim()
	baseDigest := ClaimDigest(base)

	mutate := func(name string, apply func(*Claim)) {
		t.Run(name, func(t *testing.T) {
			c := base
			apply(&c)
			if got := ClaimDigest(c); got == baseDigest {
				t.Errorf("ClaimDigest unchanged after mutating %s: %q", name, got)
			}
		})
	}

	mutate("ID", func(c *Claim) { c.ID = "row-2" })
	mutate("Behavior", func(c *Claim) { c.Behavior = "returns an error on failure" })
	mutate("Expected", func(c *Claim) { c.Expected = "err is non-nil" })
	mutate("Level", func(c *Claim) { c.Level = "integration" })
	mutate("Mutation", func(c *Claim) { c.Mutation = "delete the guard clause" })
	mutate("Risk", func(c *Claim) { c.Risk = "low" })
}

// --- Manifest / ReuseReference round-tripping and field order ---

func fullManifest() *Manifest {
	return &Manifest{
		Version:    ManifestVersion,
		Complete:   true,
		Incomplete: nil,
		RepoCommit: "abc123",
		Claims:     map[string]string{"row-1": "digest-1", "row-2": "digest-2"},
		Bundle:     FileDigest{"bundle/one_test.go": "bundle-digest-1"},
		Reads:      FileDigest{"reads/helper.go": "reads-digest-1"},
		Runner: RunnerIdentity{
			Kind:        "go-test",
			Root:        "/repo",
			Command:     []string{"go", "test", "./..."},
			ReportGlobs: []string{"*.json"},
		},
		ToolVersions: map[string]string{"go": "1.26.3"},
		Policy: &PolicyIdentity{
			Critic:       "critic-a",
			Instructions: FileDigest{"/abs/instr1.md": "instr-digest-1"},
		},
	}
}

func writeAndRead(t *testing.T, name string, doc any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := WriteJSON(path, doc); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func assertOrder(t *testing.T, raw string, keysInOrder ...string) {
	t.Helper()
	prevIdx := -1
	prevKey := ""
	for _, key := range keysInOrder {
		needle := `"` + key + `"`
		idx := strings.Index(raw, needle)
		if idx < 0 {
			t.Fatalf("key %q not found in:\n%s", key, raw)
		}
		if idx < prevIdx {
			t.Errorf("key %q (at %d) appears before %q (at %d), want after:\n%s", key, idx, prevKey, prevIdx, raw)
		}
		prevIdx = idx
		prevKey = key
	}
}

func TestManifestRoundTripsAndFieldOrderMatchesSection3(t *testing.T) {
	m := fullManifest()
	raw := writeAndRead(t, "manifest.json", m)

	assertOrder(t, raw,
		"version", "complete", "incomplete", "repoCommit", "claims",
		"bundle", "reads", "runner", "toolVersions", "policy",
	)
	assertOrder(t, raw, "kind", "root", "command", "reportGlobs")
	assertOrder(t, raw, "critic", "instructions")

	var got Manifest
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != m.Version || got.Complete != m.Complete || got.RepoCommit != m.RepoCommit {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, m)
	}
	if !reflect.DeepEqual(got.Claims, m.Claims) {
		t.Errorf("round trip Claims = %+v, want %+v", got.Claims, m.Claims)
	}
	if !reflect.DeepEqual(got.Runner, m.Runner) {
		t.Errorf("round trip Runner = %+v, want %+v", got.Runner, m.Runner)
	}
	if got.Policy == nil || got.Policy.Critic != m.Policy.Critic || !reflect.DeepEqual(got.Policy.Instructions, m.Policy.Instructions) {
		t.Errorf("round trip Policy = %+v, want %+v", got.Policy, m.Policy)
	}
}

func TestManifestOmitsPolicyKeyWhenNil(t *testing.T) {
	m := fullManifest()
	m.Policy = nil
	raw := writeAndRead(t, "manifest.json", m)

	if strings.Contains(raw, `"policy"`) {
		t.Errorf("manifest with nil Policy still emits a policy key:\n%s", raw)
	}
}

func TestManifestIncompleteSerializesAsEmptyArrayNeverNull(t *testing.T) {
	m := fullManifest()
	m.Incomplete = nil
	raw := writeAndRead(t, "manifest.json", m)

	if strings.Contains(raw, `"incomplete": null`) {
		t.Errorf("manifest with nil Incomplete serialized as null:\n%s", raw)
	}
	if !strings.Contains(raw, `"incomplete": []`) {
		t.Errorf("manifest with nil Incomplete did not serialize as []:\n%s", raw)
	}
}

func TestRunnerIdentityCommandAndReportGlobsSerializeAsEmptyArrayNeverNull(t *testing.T) {
	r := RunnerIdentity{Kind: "go-test", Root: "/repo"} // Command and ReportGlobs left nil
	raw := writeAndRead(t, "runner.json", r)

	if strings.Contains(raw, `"command": null`) || strings.Contains(raw, `"reportGlobs": null`) {
		t.Errorf("RunnerIdentity nil slices serialized as null:\n%s", raw)
	}
	if !strings.Contains(raw, `"command": []`) {
		t.Errorf("RunnerIdentity.Command did not serialize as []:\n%s", raw)
	}
	if !strings.Contains(raw, `"reportGlobs": []`) {
		t.Errorf("RunnerIdentity.ReportGlobs did not serialize as []:\n%s", raw)
	}
}

func TestManifestContextPathsSortedUnion(t *testing.T) {
	m := &Manifest{
		Bundle: FileDigest{"b/one.go": "d1", "common.go": "d2"},
		Reads:  FileDigest{"common.go": "d2-from-reads", "r/two.go": "d3"},
	}
	want := []string{"b/one.go", "common.go", "r/two.go"}

	got := m.ContextPaths()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ContextPaths() = %v, want %v", got, want)
	}
}

func fullReuseReference() ReuseReference {
	return ReuseReference{
		PriorRunID:        "run-1",
		PriorRepoCommit:   "abc123",
		PriorSourceSHA256: "source-digest",
		PriorSeq:          3,
		Outcome:           "pass",
		Artifact:          FileRef{Path: "artifact.json", SHA256: "artifact-digest"},
		Basis: ReuseBasis{
			PriorCommit:  "abc123",
			ChangedPaths: nil,
			ClaimRows:    nil,
			ContextFiles: 2,
			Policy:       "matched",
			Toolchain:    "not-applicable",
		},
	}
}

func TestReuseReferenceRoundTripsAndFieldOrderMatchesSection3(t *testing.T) {
	r := fullReuseReference()
	raw := writeAndRead(t, "reuse.json", r)

	assertOrder(t, raw,
		"priorRunId", "priorRepoCommit", "priorSourceSha256", "priorSeq",
		"outcome", "artifact", "basis",
	)
	assertOrder(t, raw, "path", "sha256")
	assertOrder(t, raw, "priorCommit", "changedPaths", "claimRows", "contextFiles", "policy", "toolchain")

	var got ReuseReference
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PriorRunID != r.PriorRunID || got.PriorSeq != r.PriorSeq || got.Artifact != r.Artifact {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, r)
	}
}

func TestReuseBasisChangedPathsAndClaimRowsSerializeAsEmptyArrayNeverNull(t *testing.T) {
	r := fullReuseReference() // Basis.ChangedPaths and ClaimRows are nil
	raw := writeAndRead(t, "reuse.json", r)

	if strings.Contains(raw, `"changedPaths": null`) || strings.Contains(raw, `"claimRows": null`) {
		t.Errorf("ReuseBasis nil slices serialized as null:\n%s", raw)
	}
	if !strings.Contains(raw, `"changedPaths": []`) {
		t.Errorf("ReuseBasis.ChangedPaths did not serialize as []:\n%s", raw)
	}
	if !strings.Contains(raw, `"claimRows": []`) {
		t.Errorf("ReuseBasis.ClaimRows did not serialize as []:\n%s", raw)
	}
}

// --- Mismatch ---

func baseMismatchManifest() *Manifest {
	return &Manifest{
		Version:    ManifestVersion,
		Complete:   true,
		Incomplete: nil,
		RepoCommit: "abc123",
		Claims:     map[string]string{"row-1": "digest-1", "row-2": "digest-2"},
		Bundle:     FileDigest{"bundle/one_test.go": "bundle-digest-1"},
		Reads:      FileDigest{"reads/helper.go": "reads-digest-1"},
		Runner: RunnerIdentity{
			Kind:        "go-test",
			Root:        "/repo",
			Command:     []string{"go", "test", "./..."},
			ReportGlobs: []string{"*.json"},
		},
		ToolVersions: map[string]string{"go": "1.26.3"},
		Policy: &PolicyIdentity{
			Critic:       "critic-a",
			Instructions: FileDigest{"/abs/instr1.md": "instr-digest-1"},
		},
	}
}

// baseMismatchProbe observes exactly what m declares, but names the policy
// instruction file at a different path with the same digest, proving paths
// are ignored for policy comparison.
func baseMismatchProbe(m *Manifest) Probe {
	return Probe{
		Claims: map[string]string{
			"row-1": m.Claims["row-1"],
			"row-2": m.Claims["row-2"],
		},
		Files: FileDigest{
			"bundle/one_test.go": m.Bundle["bundle/one_test.go"],
			"reads/helper.go":    m.Reads["reads/helper.go"],
		},
		Runner:       m.Runner,
		ToolVersions: map[string]string{"go": "1.26.3"},
		Policy: &PolicyIdentity{
			Critic:       "critic-a",
			Instructions: FileDigest{"/elsewhere/instr1.md": "instr-digest-1"},
		},
	}
}

func TestMismatchEmptyForExactMatch(t *testing.T) {
	m := baseMismatchManifest()
	probe := baseMismatchProbe(m)

	got := Mismatch(m, probe, nil, true, true)
	if len(got) != 0 {
		t.Errorf("Mismatch() = %v, want empty", got)
	}
}

func TestMismatchUnsupportedVersionReportedFirst(t *testing.T) {
	m := baseMismatchManifest()
	m.Version = "2"
	m.Incomplete = []string{"reads-timeout"}
	probe := baseMismatchProbe(m)

	got := Mismatch(m, probe, nil, false, false)
	want := []string{"manifest-version-unsupported", "manifest-incomplete:reads-timeout"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mismatch() = %v, want %v", got, want)
	}
}

func TestMismatchIncompleteEntriesSortedByReason(t *testing.T) {
	m := baseMismatchManifest()
	m.Incomplete = []string{"z-reason", "a-reason"}
	probe := baseMismatchProbe(m)

	got := Mismatch(m, probe, nil, false, false)
	want := []string{"manifest-incomplete:a-reason", "manifest-incomplete:z-reason"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mismatch() = %v, want %v", got, want)
	}
}

func TestMismatchClaimMissingThenClaimChangedSortedWithinKind(t *testing.T) {
	m := baseMismatchManifest()
	m.Claims = map[string]string{
		"row-d": "digest-d",
		"row-b": "digest-b",
		"row-a": "digest-a",
		"row-c": "digest-c",
		"row-e": "digest-e",
	}
	probe := Probe{
		Claims: map[string]string{
			// row-b, row-d absent entirely -> claim-missing
			"row-a": "digest-a-changed",
			"row-c": "digest-c-changed",
			"row-e": "digest-e", // matches
		},
		Files:        FileDigest{"bundle/one_test.go": m.Bundle["bundle/one_test.go"], "reads/helper.go": m.Reads["reads/helper.go"]},
		Runner:       m.Runner,
		ToolVersions: m.ToolVersions,
		Policy:       m.Policy,
	}

	got := Mismatch(m, probe, nil, false, false)
	want := []string{
		"claim-missing:row-b", "claim-missing:row-d",
		"claim-changed:row-a", "claim-changed:row-c",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mismatch() = %v, want %v", got, want)
	}
}

func TestMismatchClaimsRestrictedToGivenRows(t *testing.T) {
	m := baseMismatchManifest()
	m.Claims = map[string]string{"row-1": "digest-1", "row-2": "digest-2"}
	probe := Probe{
		Claims: map[string]string{
			// row-1 differs but is not in rows, so must be ignored.
			"row-2": "digest-2",
		},
		Files:        FileDigest{"bundle/one_test.go": m.Bundle["bundle/one_test.go"], "reads/helper.go": m.Reads["reads/helper.go"]},
		Runner:       m.Runner,
		ToolVersions: m.ToolVersions,
		Policy:       m.Policy,
	}

	got := Mismatch(m, probe, []string{"row-2"}, false, false)
	if len(got) != 0 {
		t.Errorf("Mismatch() = %v, want empty (row-1 excluded by rows)", got)
	}
}

func TestMismatchContextMissingThenContextChangedSortedWithinKind(t *testing.T) {
	m := baseMismatchManifest()
	m.Bundle = FileDigest{"b/two.go": "d2", "b/one.go": "d1"}
	m.Reads = FileDigest{"r/only.go": "d3"}
	probe := baseMismatchProbe(m)
	probe.Files = FileDigest{
		"b/one.go": "d1-changed",
		// "b/two.go" absent -> context-missing
		// "r/only.go" absent -> context-missing
	}

	got := Mismatch(m, probe, nil, false, false)
	want := []string{
		"context-missing:b/two.go", "context-missing:r/only.go",
		"context-changed:b/one.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mismatch() = %v, want %v", got, want)
	}
}

func TestMismatchRunnerChangedOnAnyFieldDifference(t *testing.T) {
	cases := []struct {
		name    string
		mutator func(*RunnerIdentity)
	}{
		{"kind", func(r *RunnerIdentity) { r.Kind = "other-kind" }},
		{"root", func(r *RunnerIdentity) { r.Root = "/other-root" }},
		{"command", func(r *RunnerIdentity) { r.Command = []string{"go", "test", "./other"} }},
		{"reportGlobs", func(r *RunnerIdentity) { r.ReportGlobs = []string{"*.xml"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := baseMismatchManifest()
			probe := baseMismatchProbe(m)
			tc.mutator(&probe.Runner)

			got := Mismatch(m, probe, nil, false, false)
			want := []string{"runner-changed"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Mismatch() = %v, want %v", got, want)
			}
		})
	}
}

func TestMismatchToolchainChangedOnlyWhenWanted(t *testing.T) {
	m := baseMismatchManifest()
	probe := baseMismatchProbe(m)
	probe.ToolVersions = map[string]string{"go": "1.99.0"}

	if got := Mismatch(m, probe, nil, false, false); len(got) != 0 {
		t.Errorf("Mismatch() with wantToolchain=false = %v, want empty", got)
	}
	if got := Mismatch(m, probe, nil, false, true); !reflect.DeepEqual(got, []string{"toolchain-changed"}) {
		t.Errorf("Mismatch() with wantToolchain=true = %v, want [toolchain-changed]", got)
	}
}

func TestMismatchPolicyUnspecifiedWhenEitherSideNil(t *testing.T) {
	t.Run("probe policy nil", func(t *testing.T) {
		m := baseMismatchManifest()
		probe := baseMismatchProbe(m)
		probe.Policy = nil

		got := Mismatch(m, probe, nil, true, false)
		if !reflect.DeepEqual(got, []string{"policy-unspecified"}) {
			t.Errorf("Mismatch() = %v, want [policy-unspecified]", got)
		}
	})
	t.Run("prior policy nil", func(t *testing.T) {
		m := baseMismatchManifest()
		m.Policy = nil
		probe := baseMismatchProbe(m)

		got := Mismatch(m, probe, nil, true, false)
		if !reflect.DeepEqual(got, []string{"policy-unspecified"}) {
			t.Errorf("Mismatch() = %v, want [policy-unspecified]", got)
		}
	})
	t.Run("both nil ignored when not wanted", func(t *testing.T) {
		m := baseMismatchManifest()
		m.Policy = nil
		probe := baseMismatchProbe(m)
		probe.Policy = nil

		got := Mismatch(m, probe, nil, false, false)
		if len(got) != 0 {
			t.Errorf("Mismatch() with wantPolicy=false = %v, want empty", got)
		}
	})
}

func TestMismatchPolicyChangedOnCriticOrDigestSetDifferencePathsIgnored(t *testing.T) {
	t.Run("critic differs", func(t *testing.T) {
		m := baseMismatchManifest()
		probe := baseMismatchProbe(m)
		probe.Policy.Critic = "critic-b"

		got := Mismatch(m, probe, nil, true, false)
		if !reflect.DeepEqual(got, []string{"policy-changed"}) {
			t.Errorf("Mismatch() = %v, want [policy-changed]", got)
		}
	})
	t.Run("digest set differs", func(t *testing.T) {
		m := baseMismatchManifest()
		probe := baseMismatchProbe(m)
		probe.Policy.Instructions = FileDigest{"/elsewhere/instr1.md": "instr-digest-DIFFERENT"}

		got := Mismatch(m, probe, nil, true, false)
		if !reflect.DeepEqual(got, []string{"policy-changed"}) {
			t.Errorf("Mismatch() = %v, want [policy-changed]", got)
		}
	})
	t.Run("same digest set different paths matches", func(t *testing.T) {
		m := baseMismatchManifest()
		probe := baseMismatchProbe(m) // already uses a different instruction path, same digest

		got := Mismatch(m, probe, nil, true, false)
		if len(got) != 0 {
			t.Errorf("Mismatch() = %v, want empty (paths ignored)", got)
		}
	})
}

func TestMismatchFixedOrderAcrossAllKinds(t *testing.T) {
	m := baseMismatchManifest()
	m.Version = "2"
	m.Incomplete = []string{"reads-timeout"}
	m.Claims = map[string]string{"row-1": "digest-1", "row-2": "digest-2"}

	probe := Probe{
		Claims: map[string]string{
			// row-1 missing
			"row-2": "digest-2-changed",
		},
		Files: FileDigest{
			// bundle/one_test.go missing
			"reads/helper.go": "reads-digest-1-changed",
		},
		Runner:       RunnerIdentity{Kind: "different-kind", Root: m.Runner.Root, Command: m.Runner.Command, ReportGlobs: m.Runner.ReportGlobs},
		ToolVersions: map[string]string{"go": "1.99.0"},
		Policy:       &PolicyIdentity{Critic: "critic-b", Instructions: m.Policy.Instructions},
	}

	got := Mismatch(m, probe, nil, true, true)
	want := []string{
		"manifest-version-unsupported",
		"manifest-incomplete:reads-timeout",
		"claim-missing:row-1",
		"claim-changed:row-2",
		"context-missing:bundle/one_test.go",
		"context-changed:reads/helper.go",
		"runner-changed",
		"toolchain-changed",
		"policy-changed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mismatch() = %v, want %v", got, want)
	}
}
