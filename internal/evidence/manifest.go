package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
)

// ManifestVersion is the schema version this package's Manifest shape
// implements. A prior Manifest whose Version differs is
// manifest-version-unsupported: it is never repaired or reinterpreted.
const ManifestVersion = "1"

// ErrDigestMismatch is returned (wrapped) by VerifyFile when the file's
// current digest does not match the expected one.
var ErrDigestMismatch = errors.New("digest mismatch")

// VerifyFile re-hashes path and compares it to want. It returns nil when the
// digests match, an error wrapping ErrDigestMismatch (naming path) when they
// differ, and the underlying read error (wrapped) when path is missing or
// unreadable.
func VerifyFile(path, want string) error {
	got, err := SHA256File(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("verify %s: %w", path, ErrDigestMismatch)
	}
	return nil
}

// FileDigest maps a repo-relative slash path (or an absolute path for policy
// files) to its SHA-256 digest.
type FileDigest map[string]string

// DigestFiles hashes root/<path> for every path. A path that is missing or
// unreadable is reported as "<path>: <error>" in place of a digest, both as
// the map value for that key and in the returned problem list (sorted).
// Result keys are the given paths, unchanged.
func DigestFiles(root string, paths []string) (FileDigest, []string) {
	digests := make(FileDigest, len(paths))
	var problems []string
	for _, p := range paths {
		sum, err := SHA256File(filepath.Join(root, p))
		if err != nil {
			msg := fmt.Sprintf("%s: %s", p, err)
			digests[p] = msg
			problems = append(problems, msg)
			continue
		}
		digests[p] = sum
	}
	sort.Strings(problems)
	return digests, problems
}

// Claim is the approved semantics of one scenario row; ClaimDigest hashes
// its canonical JSON so a change to any field is detectable without storing
// the full row.
type Claim struct {
	ID       string `json:"id"`
	Behavior string `json:"behavior"`
	Expected string `json:"expected"`
	Level    string `json:"level"`
	Mutation string `json:"mutation"`
	Risk     string `json:"risk"`
}

// canonicalJSON encodes v as compact, non-HTML-escaped JSON. It is the one
// encoder path this package uses for content that must hash or compare
// stably regardless of caller-side formatting choices.
func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// ClaimDigest returns the lowercase hex SHA-256 of c's canonical JSON.
func ClaimDigest(c Claim) string {
	data, err := canonicalJSON(c)
	if err != nil {
		// Claim holds only strings; encoding cannot fail.
		panic(fmt.Sprintf("evidence: claim digest: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RunnerIdentity names the runner a manifest's inputs were exercised by.
type RunnerIdentity struct {
	Kind        string   `json:"kind"`
	Root        string   `json:"root"`
	Command     []string `json:"command"`
	ReportGlobs []string `json:"reportGlobs"`
}

// MarshalJSON normalizes nil Command and ReportGlobs to empty arrays before
// encoding so neither field is ever emitted as null.
func (r RunnerIdentity) MarshalJSON() ([]byte, error) {
	type alias RunnerIdentity
	a := alias(r)
	if a.Command == nil {
		a.Command = []string{}
	}
	if a.ReportGlobs == nil {
		a.ReportGlobs = []string{}
	}
	return canonicalJSON(a)
}

// runnerEqual reports whether a and b describe the same runner. Command and
// ReportGlobs are compared as sequences (nil and empty are equal).
func runnerEqual(a, b RunnerIdentity) bool {
	return a.Kind == b.Kind &&
		a.Root == b.Root &&
		slices.Equal(a.Command, b.Command) &&
		slices.Equal(a.ReportGlobs, b.ReportGlobs)
}

// PolicyIdentity names the critic policy a manifest's inputs were reviewed
// under.
type PolicyIdentity struct {
	Critic       string     `json:"critic"`
	Instructions FileDigest `json:"instructions"` // absolute path → sha256
}

// digestValues returns the set of digest values in fd, ignoring keys.
func digestValues(fd FileDigest) map[string]struct{} {
	set := make(map[string]struct{}, len(fd))
	for _, v := range fd {
		set[v] = struct{}{}
	}
	return set
}

// samePolicy reports whether a and b name the same critic and the same set
// of instruction digests; instruction paths are ignored.
func samePolicy(a, b PolicyIdentity) bool {
	if a.Critic != b.Critic {
		return false
	}
	sa, sb := digestValues(a.Instructions), digestValues(b.Instructions)
	if len(sa) != len(sb) {
		return false
	}
	for v := range sa {
		if _, ok := sb[v]; !ok {
			return false
		}
	}
	return true
}

// Manifest is the complete input identity of one review group; a mutation
// row's manifest is the manifest of the group that reviewed the row.
// Complete is false when Incomplete is non-empty.
type Manifest struct {
	Version      string            `json:"version"`
	Complete     bool              `json:"complete"`
	Incomplete   []string          `json:"incomplete"`
	RepoCommit   string            `json:"repoCommit"`
	Claims       map[string]string `json:"claims"` // rowId → ClaimDigest
	Bundle       FileDigest        `json:"bundle"` // whole test files handed to the critic
	Reads        FileDigest        `json:"reads"`  // supporting files the critic reported reading
	Runner       RunnerIdentity    `json:"runner"`
	ToolVersions map[string]string `json:"toolVersions"`
	Policy       *PolicyIdentity   `json:"policy,omitempty"`
}

// MarshalJSON normalizes a nil Incomplete to an empty array before encoding
// so the field is never emitted as null.
func (m Manifest) MarshalJSON() ([]byte, error) {
	type alias Manifest
	a := alias(m)
	if a.Incomplete == nil {
		a.Incomplete = []string{}
	}
	return canonicalJSON(a)
}

// ContextPaths returns the sorted union of Bundle and Reads keys. It takes a
// pointer receiver (unlike MarshalJSON's value receiver) only because it has
// no need to copy the manifest to compute a read-only view.
func (m *Manifest) ContextPaths() []string {
	seen := make(map[string]struct{}, len(m.Bundle)+len(m.Reads))
	for k := range m.Bundle {
		seen[k] = struct{}{}
	}
	for k := range m.Reads {
		seen[k] = struct{}{}
	}
	paths := make([]string, 0, len(seen))
	for k := range seen {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	return paths
}

// Probe is what the consuming run observed for a prior manifest's inputs.
type Probe struct {
	Claims       map[string]string // current ClaimDigest per active row
	Files        FileDigest        // current digests of the prior manifest's ContextPaths; absent = missing
	Runner       RunnerIdentity
	ToolVersions map[string]string
	Policy       *PolicyIdentity
}

// Mismatch lists, in this fixed order and sorted within each kind, why prior
// cannot stand for now: manifest-version-unsupported, each Incomplete entry
// as manifest-incomplete:<reason>, claim-missing:<row>, claim-changed:<row>,
// context-missing:<path>, context-changed:<path>, runner-changed,
// toolchain-changed (only when wantToolchain), policy-unspecified /
// policy-changed (only when wantPolicy). Empty means matched.
//
// Claims are compared for every key of prior.Claims present in rows (nil
// rows means all of them), so a mutation row is judged on its own claim
// only.
func Mismatch(prior *Manifest, now Probe, rows []string, wantPolicy, wantToolchain bool) []string {
	result := []string{}

	if prior.Version != ManifestVersion {
		result = append(result, "manifest-version-unsupported")
	}

	incomplete := slices.Clone(prior.Incomplete)
	sort.Strings(incomplete)
	for _, reason := range incomplete {
		result = append(result, "manifest-incomplete:"+reason)
	}

	var rowSet map[string]struct{}
	if rows != nil {
		rowSet = make(map[string]struct{}, len(rows))
		for _, r := range rows {
			rowSet[r] = struct{}{}
		}
	}

	claimRows := make([]string, 0, len(prior.Claims))
	for r := range prior.Claims {
		claimRows = append(claimRows, r)
	}
	sort.Strings(claimRows)

	var missingClaims, changedClaims []string
	for _, r := range claimRows {
		if rowSet != nil {
			if _, ok := rowSet[r]; !ok {
				continue
			}
		}
		got, ok := now.Claims[r]
		if !ok {
			missingClaims = append(missingClaims, r)
			continue
		}
		if got != prior.Claims[r] {
			changedClaims = append(changedClaims, r)
		}
	}
	for _, r := range missingClaims {
		result = append(result, "claim-missing:"+r)
	}
	for _, r := range changedClaims {
		result = append(result, "claim-changed:"+r)
	}

	var missingPaths, changedPaths []string
	for _, p := range prior.ContextPaths() {
		// A path present in both Bundle and Reads (expected to be rare) is
		// judged against its Bundle digest; the Reads value is not consulted.
		want, ok := prior.Bundle[p]
		if !ok {
			want = prior.Reads[p]
		}
		got, ok := now.Files[p]
		if !ok {
			missingPaths = append(missingPaths, p)
			continue
		}
		if got != want {
			changedPaths = append(changedPaths, p)
		}
	}
	for _, p := range missingPaths {
		result = append(result, "context-missing:"+p)
	}
	for _, p := range changedPaths {
		result = append(result, "context-changed:"+p)
	}

	if !runnerEqual(prior.Runner, now.Runner) {
		result = append(result, "runner-changed")
	}

	if wantToolchain && !maps.Equal(prior.ToolVersions, now.ToolVersions) {
		result = append(result, "toolchain-changed")
	}

	if wantPolicy {
		switch {
		case now.Policy == nil || prior.Policy == nil:
			result = append(result, "policy-unspecified")
		case !samePolicy(*prior.Policy, *now.Policy):
			result = append(result, "policy-changed")
		}
	}

	return result
}

// FileRef names one retained file by path and digest.
type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ReuseBasis records why an adopted record was judged to reuse prior work.
type ReuseBasis struct {
	PriorCommit  string   `json:"priorCommit"`
	ChangedPaths []string `json:"changedPaths"` // tracked paths changed between priorCommit and this run's commit, exempt paths omitted (for an adopted record: other groups' isolated files)
	ClaimRows    []string `json:"claimRows"`
	ContextFiles int      `json:"contextFiles"`
	Policy       string   `json:"policy"`    // matched | not-applicable
	Toolchain    string   `json:"toolchain"` // matched | not-applicable
}

// MarshalJSON normalizes nil ChangedPaths and ClaimRows to empty arrays
// before encoding so neither field is ever emitted as null.
func (b ReuseBasis) MarshalJSON() ([]byte, error) {
	type alias ReuseBasis
	a := alias(b)
	if a.ChangedPaths == nil {
		a.ChangedPaths = []string{}
	}
	if a.ClaimRows == nil {
		a.ClaimRows = []string{}
	}
	return canonicalJSON(a)
}

// ReuseReference is the provenance an adopted record carries: the original
// run, commit, source digest, sequence number, outcome and artifact
// (original path and digest, never a copy).
type ReuseReference struct {
	PriorRunID        string     `json:"priorRunId"`
	PriorRepoCommit   string     `json:"priorRepoCommit"`
	PriorSourceSHA256 string     `json:"priorSourceSha256"`
	PriorSeq          int        `json:"priorSeq"`
	Outcome           string     `json:"outcome"`
	Artifact          FileRef    `json:"artifact"`
	Basis             ReuseBasis `json:"basis"`
}
