package mutation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"test-design/internal/evidence"
)

const stateFileName = "mutation-state.json"

// State is one prepare-mutation call's persisted handoff to the later
// evaluate-mutation call for the same runDir: everything evaluate needs that
// only prepare's process ever held.
type State struct {
	SchemaVersion string          `json:"schemaVersion"`
	RowID         string          `json:"rowID"`
	Identity      Identity        `json:"identity"`
	WorktreePath  string          `json:"worktreePath"`
	Snapshot      json.RawMessage `json:"snapshot"`
	Sites         []SiteOutcome   `json:"sites"`
}

type SiteOutcome struct {
	Site     string `json:"site"`
	Outcome  string `json:"outcome"` // caught | survived
	DiffPath string `json:"diffPath"`
}

// WriteState persists st to <runDir>/mutation-state.json, stamping
// SchemaVersion itself. It writes to a temp file in runDir and renames it
// into place, so a concurrent LoadState never observes a partial write.
func WriteState(runDir string, st *State) error {
	path := filepath.Join(runDir, stateFileName)

	stamped := *st
	stamped.SchemaVersion = evidence.SchemaVersion
	if stamped.Sites == nil {
		stamped.Sites = []SiteOutcome{}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&stamped); err != nil {
		return fmt.Errorf("write mutation state %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(runDir, "."+stateFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("write mutation state %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write mutation state %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write mutation state %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write mutation state %s: %w", path, err)
	}
	return nil
}

// LoadState fails closed: a missing file, malformed JSON, an unsupported
// SchemaVersion, or an empty RowID/WorktreePath/Identity each report an
// error naming the state file rather than returning a State a caller might
// mistake for valid.
func LoadState(runDir string) (*State, error) {
	path := filepath.Join(runDir, stateFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load mutation state %s: %w", path, err)
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("load mutation state %s: malformed JSON: %w", path, err)
	}
	if st.SchemaVersion != evidence.SchemaVersion {
		return nil, fmt.Errorf("load mutation state %s: schemaVersion %q, want %q", path, st.SchemaVersion, evidence.SchemaVersion)
	}
	if st.RowID == "" {
		return nil, fmt.Errorf("load mutation state %s: rowID is empty", path)
	}
	if st.WorktreePath == "" {
		return nil, fmt.Errorf("load mutation state %s: worktreePath is empty", path)
	}
	if st.Identity == (Identity{}) {
		return nil, fmt.Errorf("load mutation state %s: identity is empty", path)
	}
	return &st, nil
}
