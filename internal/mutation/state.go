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
	RunID         string          `json:"runID"`
	RepoCommit    string          `json:"repoCommit"`
	Source        string          `json:"source"`
	SourceSHA256  string          `json:"sourceSha256"`
	RowID         string          `json:"rowID"`
	Identity      Identity        `json:"identity"`
	WorktreePath  string          `json:"worktreePath"`
	Snapshot      json.RawMessage `json:"snapshot"`
	Sites         []SiteOutcome   `json:"sites"`
	// Enumerated is the row's registered site list. It reaches disk on the
	// first evaluate-mutation call whose diff clears policing, and is never
	// rewritten by recover-mutation.
	Enumerated []string `json:"enumerated,omitempty"`
	// Started marks the site an evaluation is inside. It is written before
	// the first run that touches the worktree and cleared only by that
	// site's own completion or by recover-mutation, so a state document
	// found carrying it says the workspace is of unknown cleanliness.
	Started *SiteAttempt `json:"started,omitempty"`
	// Discarded is every rejected mutator edit this row's confirmation step
	// has discarded, in the order discard-mutation-edit recorded them.
	// recover-mutation writes it back unchanged along with the rest of st.
	Discarded []DiscardRecord `json:"discarded,omitempty"`
}

// DiscardRecord is one edit discard-mutation-edit rejected: the site it was
// applied at, the caller's reason, and the diff it captured before restoring
// the worktree.
type DiscardRecord struct {
	Site       string `json:"site"`
	Reason     string `json:"reason"`
	DiffPath   string `json:"diffPath"`
	DiffSHA256 string `json:"diffSha256"`
}

// SiteAttempt is the site an evaluation had in flight when the state was last
// written: which site, which of its two runs was about to start, and when.
type SiteAttempt struct {
	Site      string `json:"site"`
	Phase     string `json:"phase"` // mutated | clean
	StartedAt string `json:"startedAt"`
}

type SiteOutcome struct {
	Site              string `json:"site"`
	Outcome           string `json:"outcome"` // caught | survived
	DiffPath          string `json:"diffPath"`
	MutatedRunDir     string `json:"mutatedRunDir"`
	MutatedExitStatus int    `json:"mutatedExitStatus"`
	CleanRunDir       string `json:"cleanRunDir"`
	CleanExitStatus   int    `json:"cleanExitStatus"`
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
	if st.RunID == "" || st.RepoCommit == "" || st.Source == "" || st.SourceSHA256 == "" {
		return nil, fmt.Errorf("load mutation state %s: runID, repoCommit, source and sourceSha256 are required; regenerate prepared state", path)
	}
	if st.WorktreePath == "" {
		return nil, fmt.Errorf("load mutation state %s: worktreePath is empty", path)
	}
	if st.Identity == (Identity{}) {
		return nil, fmt.Errorf("load mutation state %s: identity is empty", path)
	}
	return &st, nil
}
