package worktree

import (
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
