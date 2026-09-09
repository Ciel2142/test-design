package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"test-design/internal/evidence"
	"test-design/internal/runstate"
)

// This file captures the input identity of a review group: everything a later
// run must find unchanged before it may stand on this run's verdict. The rule
// it is built around is that capture never refuses. A verdict is valid for the
// run that produced it whatever its manifest says, so a problem here is
// recorded as a reason the manifest is incomplete — the one consequence being
// that an incomplete manifest is never reusable.

// reviewPolicy is the critic identity the parent declares for a review group:
// which critic judged it and which instruction files that critic was running
// under. The paths are absolute because a critic's instructions live outside
// the repository under review.
type reviewPolicy struct {
	Critic       string   `json:"critic"`
	Instructions []string `json:"instructions"` // absolute paths of the critic's instruction files
}

// captureReviewManifest builds the started-time manifest for rows from the
// current dispatch inputs: each row's approved claim as the dispatch document
// digested it, the whole file behind every site the critic is handed, the
// runner and toolchain that document recorded, and the policy the parent
// named. HEAD and the tree are read now, so a manifest captured against a
// moved HEAD or a dirty tree says so. Reads are not known until the group is
// completed, which is why a started manifest is always incomplete.
func captureReviewManifest(repoRoot string, identity runstate.Identity, dispatch *dispatchInputs, rows []string, policy *reviewPolicy) *evidence.Manifest {
	if dispatch == nil {
		dispatch = &dispatchInputs{}
	}
	byID := dispatchRowsByID(dispatch)
	claims := make(map[string]string, len(rows))
	for _, id := range rows {
		if row, ok := byID[id]; ok {
			claims[id] = row.ClaimSHA256
		}
	}
	paths, _ := bundleFiles(dispatch, rows)
	bundle, undigestable := digestUnder(repoRoot, paths)
	policyID, problems := policyIdentity(policy)
	// A file the critic would be handed but this run cannot digest — a path git
	// carries as a tree, a file that vanished since the trace — is a file
	// nothing later can identify, and it is left out of the bundle rather than
	// recorded as the error that reading it produced.
	for _, path := range undigestable {
		problems = append(problems, "unidentifiable-read:"+path)
	}

	m := &evidence.Manifest{
		Version:      evidence.ManifestVersion,
		Incomplete:   problems,
		Claims:       claims,
		Bundle:       bundle,
		Runner:       dispatch.Runner,
		ToolVersions: dispatch.ToolVersions,
		Policy:       policyID,
	}
	applyRepoState(m, repoRoot, identity, nil)
	return m
}

// completeReviewManifest returns the manifest a started group captured with the
// critic's reads judged — each must be a path this repository can identify —
// and HEAD and the tree read again. The started manifest is left as it was: the
// returned value keeps its claims and bundle and carries every reason it
// already recorded, adding the fresh ones. A record started before manifests
// were captured has none to complete and gets none invented for it.
func completeReviewManifest(repoRoot string, identity runstate.Identity, started *evidence.Manifest, reads *[]string) *evidence.Manifest {
	if started == nil {
		return nil
	}
	m := *started
	applyRepoState(&m, repoRoot, identity, reads)
	return &m
}

// applyRepoState fills in everything a manifest reads from the repository
// rather than from the request: the commit HEAD carries now, the reads the
// parent relayed, and the incompleteness those imply. Incompleteness is
// monotonic — every reason already recorded is carried forward, because a
// manifest captured against a moved HEAD or a dirty tree holds digests of bytes
// no commit carries and cleaning the tree afterwards does not make them
// identifiable again. The one reason completion answers is "reads-unrecorded",
// so that is the only one dropped, and no reason is listed twice.
func applyRepoState(m *evidence.Manifest, repoRoot string, identity runstate.Identity, reads *[]string) {
	reasons := carriedReasons(m.Incomplete)
	add := func(reason string) {
		if !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}

	head := ""
	if out, err := gitOutput(repoRoot, "rev-parse", "HEAD"); err == nil {
		head = strings.TrimSpace(out)
	}
	m.RepoCommit = head
	if head != identity.RepoCommit {
		add("head-moved")
	}
	for _, path := range dirtyPaths(repoRoot) {
		add("tree-dirty:" + path)
	}

	m.Reads = evidence.FileDigest{}
	if reads == nil {
		add("reads-unrecorded")
	}
	judged := map[string]bool{}
	for _, path := range readsOf(reads) {
		if judged[path] {
			continue
		}
		judged[path] = true
		// confinedRelPath is the repository's own answer to "is this a path
		// inside the tree": absolute paths and traversals are refused. Its
		// message is not surfaced here — a read this run cannot identify is
		// one reason whatever made it unidentifiable. Being tracked is not
		// enough either: git carries directories too, and a path with no
		// digest is not a read a later run can compare.
		rel, cliErr := confinedRelPath("record-review", "reads", "unidentifiable-read", path)
		if cliErr != nil || head == "" || !trackedAt(repoRoot, head, rel) {
			add("unidentifiable-read:" + path)
			continue
		}
		digests, undigestable := digestUnder(repoRoot, []string{rel})
		if len(undigestable) > 0 {
			add("unidentifiable-read:" + path)
			continue
		}
		maps.Copy(m.Reads, digests)
	}

	m.Incomplete = reasons
	m.Complete = len(reasons) == 0
}

// digestUnder hashes every path under repoRoot, keeping only what it could
// read: a path it could not is returned by name rather than recorded with the
// error in place of its digest. An error string is deterministic, so a manifest
// that stored one would match a later probe that failed the same way and let a
// run stand on a file neither of them ever read.
func digestUnder(repoRoot string, paths []string) (evidence.FileDigest, []string) {
	digests := evidence.FileDigest{}
	var undigestable []string
	for _, path := range paths {
		sum, err := evidence.SHA256File(filepath.Join(repoRoot, filepath.FromSlash(path)))
		if err != nil {
			undigestable = append(undigestable, path)
			continue
		}
		digests[path] = sum
	}
	return digests, undigestable
}

// readsOf is the relayed read list, or none when the parent recorded none.
func readsOf(reads *[]string) []string {
	if reads == nil {
		return nil
	}
	return *reads
}

// carriedReasons keeps every reason a manifest already recorded except the one
// completion exists to answer: whether the critic's reads were recorded at all.
// A reason is never withdrawn — the run that observed it saw inputs it could
// not identify, and a later run reading this manifest has no way to see that
// state again.
func carriedReasons(incomplete []string) []string {
	var kept []string
	for _, reason := range incomplete {
		if reason != "reads-unrecorded" {
			kept = append(kept, reason)
		}
	}
	return kept
}

// policyIdentity digests the critic's instruction files. A policy the parent
// did not declare, a path it gave relative to nothing this run knows, or a
// file that cannot be read are each a reason the manifest is incomplete; the
// instructions that were readable are still recorded, so a later run compares
// what it can.
func policyIdentity(policy *reviewPolicy) (*evidence.PolicyIdentity, []string) {
	if policy == nil {
		return nil, []string{"policy-unrecorded"}
	}
	id := &evidence.PolicyIdentity{Critic: policy.Critic, Instructions: evidence.FileDigest{}}
	var problems []string
	for _, path := range policy.Instructions {
		if !filepath.IsAbs(path) {
			problems = append(problems, "policy-path-not-absolute:"+path)
			continue
		}
		sum, err := evidence.SHA256File(path)
		if err != nil {
			problems = append(problems, "policy-unreadable:"+path)
			continue
		}
		id.Instructions[path] = sum
	}
	return id, problems
}

// dispatchRowsByID indexes the dispatch document's rows.
func dispatchRowsByID(dispatch *dispatchInputs) map[string]dispatchRow {
	byID := make(map[string]dispatchRow, len(dispatch.Rows))
	for _, row := range dispatch.Rows {
		byID[row.RowID] = row
	}
	return byID
}

// contributorFiles names the file of every bound site crediting a row, in the
// order the dispatch document lists them.
func contributorFiles(row dispatchRow) []string {
	files := make([]string, 0, len(row.Contributors))
	for _, contributor := range row.Contributors {
		files = append(files, contributor.File)
	}
	return files
}

// bundleFiles names the whole files the critic is handed for rows — each row's
// primary site and every contributor's — with the rows each file serves, in the
// order the group named them. The manifest digests whole files, not the bound
// ranges the dispatch document carries: a critic reads the file it is given,
// and an edit anywhere in it is an edit to what was reviewed.
func bundleFiles(dispatch *dispatchInputs, rows []string) ([]string, map[string][]string) {
	byID := dispatchRowsByID(dispatch)
	served := map[string][]string{}
	for _, id := range rows {
		row, ok := byID[id]
		if !ok {
			continue
		}
		for _, file := range append([]string{row.File}, contributorFiles(row)...) {
			if file == "" || slices.Contains(served[file], id) {
				continue
			}
			served[file] = append(served[file], id)
		}
	}
	return slices.Sorted(maps.Keys(served)), served
}

// reviewBundleFor is what the parent is told its critic will be handed: every
// file, its digest as the manifest recorded it, and the rows it serves.
func reviewBundleFor(dispatch *dispatchInputs, rows []string, digests evidence.FileDigest) []reviewBundleFile {
	paths, served := bundleFiles(dispatch, rows)
	bundle := make([]reviewBundleFile, 0, len(paths))
	for _, path := range paths {
		bundle = append(bundle, reviewBundleFile{Path: path, SHA256: digests[path], Rows: served[path]})
	}
	return bundle
}

// manifestComplete and manifestIncomplete report a manifest in an op's output.
// A nil manifest — a group started before this run's executable captured one —
// is not complete and has nothing to say about why.
func manifestComplete(m *evidence.Manifest) bool {
	return m != nil && m.Complete
}

func manifestIncomplete(m *evidence.Manifest) []string {
	if m == nil || m.Incomplete == nil {
		return []string{}
	}
	return m.Incomplete
}
