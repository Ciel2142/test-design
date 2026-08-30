package worktree

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"test-design/internal/evidence"
)

// PublishFile names one file to publish: RepoRelPath is its destination
// relative to the repository root, SourcePath is where its content is read
// from.
type PublishFile struct {
	RepoRelPath string
	SourcePath  string
}

// PublishRecord is evidence for one published file: its repository-relative
// path and the SHA-256 of the content written there.
type PublishRecord struct {
	RepoRelPath string
	SHA256      string
}

// Publish is the only function in this package permitted to write inside the
// original repository. It first re-verifies the repository still matches
// before (refusing on any concurrent change, writing nothing), then
// validates every RepoRelPath before writing any file, then writes each
// file's content and returns one PublishRecord per file.
func (c *Controller) Publish(before *Snapshot, files []PublishFile) ([]PublishRecord, error) {
	after, err := c.TakeSnapshot()
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if equal, differing := Compare(before, after); !equal {
		return nil, fmt.Errorf("publish: repository changed since snapshot (%s); commit or discard the concurrent change, then rerun", strings.Join(differing, ", "))
	}

	targets := make([]string, len(files))
	for i, f := range files {
		target, err := c.resolvePublishTarget(f.RepoRelPath)
		if err != nil {
			return nil, fmt.Errorf("publish: %w", err)
		}
		targets[i] = target
	}

	var written []string
	records := make([]PublishRecord, 0, len(files))
	for i, f := range files {
		if err := publishFile(targets[i], f.SourcePath); err != nil {
			return nil, fmt.Errorf("publish: wrote %v, did not write %v: %w", written, repoRelPaths(files[i:]), err)
		}
		written = append(written, f.RepoRelPath)

		sum, err := evidence.SHA256File(targets[i])
		if err != nil {
			return nil, fmt.Errorf("publish: wrote %v, did not write %v: %w", written, repoRelPaths(files[i+1:]), err)
		}
		records = append(records, PublishRecord{RepoRelPath: f.RepoRelPath, SHA256: sum})
	}
	return records, nil
}

func (c *Controller) resolvePublishTarget(repoRelPath string) (string, error) {
	if filepath.IsAbs(repoRelPath) {
		return "", fmt.Errorf("publish target %q escapes repository root: absolute path", repoRelPath)
	}
	cleaned := filepath.Clean(repoRelPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("publish target %q escapes repository root: traverses above it", repoRelPath)
	}

	target := filepath.Join(c.repoRoot, cleaned)
	resolvedAncestor, err := nearestExistingAncestor(target)
	if err != nil {
		return "", fmt.Errorf("publish target %q: %w", repoRelPath, err)
	}
	if !isWithin(c.repoRoot, resolvedAncestor) {
		return "", fmt.Errorf("publish target %q escapes repository root: resolves to %s", repoRelPath, resolvedAncestor)
	}
	return target, nil
}

// nearestExistingAncestor walks up from path until it finds a directory that
// exists, then resolves any symlinks in it — the point at which a symlink
// planted inside the repo could redirect a not-yet-created target elsewhere.
func nearestExistingAncestor(path string) (string, error) {
	dir := path
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		dir = parent
	}
}

func publishFile(target, sourcePath string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

func repoRelPaths(files []PublishFile) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.RepoRelPath
	}
	return paths
}
