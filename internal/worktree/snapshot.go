package worktree

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"test-design/internal/evidence"
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
	head, err := runGit(c.repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("take snapshot %s: resolve HEAD: %w", c.repoRoot, err)
	}
	indexTree, err := runGit(c.repoRoot, "write-tree")
	if err != nil {
		return nil, fmt.Errorf("take snapshot %s: write-tree: %w", c.repoRoot, err)
	}
	porcelain, err := runGit(c.repoRoot, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("take snapshot %s: status: %w", c.repoRoot, err)
	}

	hashes := make(map[string]string)
	for _, path := range untrackedPaths(porcelain) {
		sum, err := evidence.SHA256File(filepath.Join(c.repoRoot, path))
		if err != nil {
			return nil, fmt.Errorf("take snapshot %s: %w", c.repoRoot, err)
		}
		hashes[path] = sum
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
	for _, line := range strings.Split(porcelain, "\n") {
		path, ok := strings.CutPrefix(line, "?? ")
		if !ok {
			continue
		}
		if strings.HasPrefix(path, `"`) {
			if unquoted, err := strconv.Unquote(path); err == nil {
				path = unquoted
			}
		}
		paths = append(paths, path)
	}
	return paths
}
