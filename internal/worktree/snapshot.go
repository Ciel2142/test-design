package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"test-design/internal/evidence"
)

var (
	// ErrUnmergedIndex is wrapped into TakeSnapshot's error when the index
	// holds unmerged (conflicted) entries: git write-tree would exit 128
	// against them, so TakeSnapshot detects and reports this before trying.
	ErrUnmergedIndex = errors.New("index has unmerged entries")
	// ErrUnbornHead is wrapped into TakeSnapshot's error when HEAD has no
	// commits yet (a freshly initialized repository).
	ErrUnbornHead = errors.New("HEAD is unborn: no commits yet")
	// ErrSnapshotFieldMissing is wrapped into UnmarshalJSON's error when the
	// document omits one of Snapshot's four captured dimensions: fail
	// closed rather than yield a Snapshot that compares equal to anything.
	ErrSnapshotFieldMissing = errors.New("snapshot document missing required field")
)

// Snapshot captures the state of the original repository at one point in
// time: enough for a later Compare to name what changed without re-running
// git.
type Snapshot struct {
	headCommit      string
	indexTree       string
	porcelainStatus string
	untrackedHashes map[string]string
}

// TakeSnapshot reads the original repository's HEAD, index, and working-tree
// status. It never mutates the repository: the only git write is the loose
// tree object git write-tree produces to fingerprint the index.
func (c *Controller) TakeSnapshot() (*Snapshot, error) {
	head, err := c.runGit("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("take snapshot %s: resolve HEAD: %w: %w", c.repoRoot, ErrUnbornHead, err)
	}
	porcelain, err := c.runGit("--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("take snapshot %s: status: %w", c.repoRoot, err)
	}
	if unmerged := unmergedPaths(porcelain); len(unmerged) > 0 {
		return nil, fmt.Errorf("take snapshot %s: %w: %s", c.repoRoot, ErrUnmergedIndex, strings.Join(unmerged, ", "))
	}
	indexTree, err := c.runGit("write-tree")
	if err != nil {
		return nil, fmt.Errorf("take snapshot %s: write-tree: %w", c.repoRoot, err)
	}

	hashes := make(map[string]string)
	for _, path := range untrackedPaths(porcelain) {
		hashes[path] = untrackedFingerprint(filepath.Join(c.repoRoot, path))
	}

	return &Snapshot{
		headCommit:      strings.TrimSpace(head),
		indexTree:       strings.TrimSpace(indexTree),
		porcelainStatus: porcelain,
		untrackedHashes: hashes,
	}, nil
}

// Clean reports whether the working tree and index match HEAD with no
// untracked files present — equivalent to an empty porcelain status.
func (s *Snapshot) Clean() bool {
	return strings.TrimSpace(s.porcelainStatus) == ""
}

// snapshotDoc is Snapshot's JSON wire form, letting one process take a
// snapshot and a later process load it back for Compare.
type snapshotDoc struct {
	HeadCommit      string            `json:"headCommit"`
	IndexTree       string            `json:"indexTree"`
	PorcelainStatus string            `json:"porcelainStatus"`
	UntrackedHashes map[string]string `json:"untrackedHashes"`
}

// MarshalJSON serializes all four dimensions TakeSnapshot captures.
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(snapshotDoc{
		HeadCommit:      s.headCommit,
		IndexTree:       s.indexTree,
		PorcelainStatus: s.porcelainStatus,
		UntrackedHashes: s.untrackedHashes,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	return data, nil
}

// UnmarshalJSON requires all four dimensions to be present; porcelainStatus
// legitimately serializes as "" for a clean repo, so presence — not a
// non-empty check — is what distinguishes a missing field from a clean one.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}
	for _, key := range []string{"headCommit", "indexTree", "porcelainStatus", "untrackedHashes"} {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("unmarshal snapshot: %w: %q", ErrSnapshotFieldMissing, key)
		}
	}

	var doc snapshotDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}

	s.headCommit = doc.HeadCommit
	s.indexTree = doc.IndexTree
	s.porcelainStatus = doc.PorcelainStatus
	s.untrackedHashes = doc.UntrackedHashes
	return nil
}

func untrackedPaths(porcelain string) []string {
	var paths []string
	for line := range strings.SplitSeq(porcelain, "\n") {
		path, ok := strings.CutPrefix(line, "?? ")
		if !ok {
			continue
		}
		paths = append(paths, unquotePorcelainPath(path))
	}
	return paths
}

// untrackedFingerprint records enough about an untracked path for Compare to
// detect a change, without letting a broken symlink or unreadable file abort
// the whole snapshot.
func untrackedFingerprint(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "unreadable:" + err.Error()
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return "unreadable:" + err.Error()
		}
		return "symlink:" + target
	case info.Mode().IsRegular():
		sum, err := evidence.SHA256File(path)
		if err != nil {
			return "unreadable:" + err.Error()
		}
		return sum
	default:
		return "type:" + info.Mode().Type().String()
	}
}

// unmergedPaths returns the paths of any unmerged (conflicted) index entries
// in porcelain: X/Y == 'U' on either side, or the both-added/both-deleted
// pairs "AA"/"DD" that carry no 'U' but are still unmerged.
func unmergedPaths(porcelain string) []string {
	var paths []string
	for line := range strings.SplitSeq(porcelain, "\n") {
		if len(line) < 3 {
			continue
		}
		x, y := line[0], line[1]
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			paths = append(paths, unquotePorcelainPath(line[3:]))
		}
	}
	return paths
}
