package live

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"test-design/internal/evidence"
)

const stateFileName = "live-state.json"

// State is one prepare call's persisted handoff to the later execute call
// for the same runDir: everything execute needs that only prepare's process
// ever held.
type State struct {
	SchemaVersion string          `json:"schemaVersion"`
	RunID         string          `json:"runID"`
	WorktreePath  string          `json:"worktreePath"`
	Snapshot      json.RawMessage `json:"snapshot"`
	ProfilePath   string          `json:"profilePath"`
	Descriptor    json.RawMessage `json:"descriptor"`
}

// WriteState persists st to <runDir>/live-state.json, stamping
// SchemaVersion itself. It writes to a temp file in runDir and renames it
// into place, so a concurrent LoadState never observes a partial write.
func WriteState(runDir string, st *State) error {
	path := filepath.Join(runDir, stateFileName)

	stamped := *st
	stamped.SchemaVersion = evidence.SchemaVersion

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&stamped); err != nil {
		return fmt.Errorf("write live state %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(runDir, "."+stateFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("write live state %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write live state %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write live state %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write live state %s: %w", path, err)
	}
	return nil
}

// LoadState fails closed: a missing file, malformed JSON, an unsupported
// SchemaVersion, or an empty RunID/WorktreePath/ProfilePath/Descriptor each
// report an error naming the state file rather than returning a State a
// caller might mistake for valid.
func LoadState(runDir string) (*State, error) {
	path := filepath.Join(runDir, stateFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load live state %s: %w", path, err)
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("load live state %s: malformed JSON: %w", path, err)
	}
	if st.SchemaVersion != evidence.SchemaVersion {
		return nil, fmt.Errorf("load live state %s: schemaVersion %q, want %q", path, st.SchemaVersion, evidence.SchemaVersion)
	}
	if st.RunID == "" {
		return nil, fmt.Errorf("load live state %s: runID is empty", path)
	}
	if st.WorktreePath == "" {
		return nil, fmt.Errorf("load live state %s: worktreePath is empty", path)
	}
	if st.ProfilePath == "" {
		return nil, fmt.Errorf("load live state %s: profilePath is empty", path)
	}
	if len(st.Descriptor) == 0 || string(st.Descriptor) == "null" {
		return nil, fmt.Errorf("load live state %s: descriptor is empty", path)
	}
	return &st, nil
}
