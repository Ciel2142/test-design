package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

// NewRunDir allocates a run directory keyed by runID under the controller's
// verifier-owned root, outside the original repository, and returns its
// absolute path. A second call with the same runID errors rather than
// clobbering the first call's directory.
func (c *Controller) NewRunDir(runID string) (string, error) {
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

func (c *Controller) runsDir() string {
	return filepath.Join(c.tempRoot, "runs")
}
