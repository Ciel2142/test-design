package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NewRunDir allocates a run directory keyed by runID under the controller's
// verifier-owned root, outside the original repository, and returns its
// absolute path. A second call with the same runID errors rather than
// clobbering the first call's directory. runID must be a single plain path
// component: an absolute, separator-containing, or ".."-containing runID is
// refused before any filesystem touch, since joining it unchecked would let
// it place the run directory outside the verifier-owned root.
func (c *Controller) NewRunDir(runID string) (string, error) {
	if err := validateRunID(runID); err != nil {
		return "", fmt.Errorf("new run dir: %w", err)
	}

	dir := c.runsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("new run dir %s: %w", runID, err)
	}

	path := filepath.Join(dir, runID)
	if err := os.Mkdir(path, 0o755); err != nil {
		return "", fmt.Errorf("new run dir %s: %w", runID, err)
	}
	return path, nil
}

func validateRunID(runID string) error {
	if runID == "" {
		return errors.New("run ID is empty")
	}
	if filepath.IsAbs(runID) {
		return fmt.Errorf("run ID %q is not a single path component: absolute path", runID)
	}
	if strings.ContainsRune(runID, filepath.Separator) || strings.ContainsRune(runID, '/') {
		return fmt.Errorf("run ID %q is not a single path component: contains a path separator", runID)
	}
	if strings.Contains(runID, "..") {
		return fmt.Errorf("run ID %q is not a single path component: contains \"..\"", runID)
	}
	return nil
}

func (c *Controller) runsDir() string {
	return filepath.Join(c.tempRoot, "runs")
}
