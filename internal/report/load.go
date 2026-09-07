package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"test-design/internal/evidence"
)

var (
	ErrEvidenceUnreadable     = errors.New("evidence unreadable")
	ErrEvidenceSchemaMismatch = errors.New("evidence schema mismatch")
)

type FieldMissingError struct {
	Path  string
	Field string
}

func (e *FieldMissingError) Error() string {
	return fmt.Sprintf("%s: missing required field %q", e.Path, e.Field)
}

func decodeEvidence(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w: %v", path, ErrEvidenceUnreadable, err)
	}
	if !json.Valid(data) {
		return fmt.Errorf("%s: %w: invalid JSON", path, ErrEvidenceUnreadable)
	}
	if _, err := evidence.DecodeEnvelope(data); err != nil {
		return fmt.Errorf("%s: %w: %v", path, ErrEvidenceSchemaMismatch, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w: %v", path, ErrEvidenceUnreadable, err)
	}
	return nil
}

func LoadTrace(path string) (*TraceEvidence, error) {
	var ev TraceEvidence
	if err := decodeEvidence(path, &ev); err != nil {
		return nil, err
	}
	switch {
	case ev.RunID == "":
		return nil, &FieldMissingError{Path: path, Field: "runId"}
	case ev.Source == "":
		return nil, &FieldMissingError{Path: path, Field: "source"}
	case ev.SourceSHA256 == "":
		return nil, &FieldMissingError{Path: path, Field: "sourceSha256"}
	case ev.RepoCommit == "":
		return nil, &FieldMissingError{Path: path, Field: "repoCommit"}
	case ev.BaselineStatus == "":
		return nil, &FieldMissingError{Path: path, Field: "baselineStatus"}
	}
	return &ev, nil
}

func LoadLive(path string) (*LiveEvidence, error) {
	var ev LiveEvidence
	if err := decodeEvidence(path, &ev); err != nil {
		return nil, err
	}
	switch {
	case ev.RunID == "":
		return nil, &FieldMissingError{Path: path, Field: "runId"}
	case ev.Source == "":
		return nil, &FieldMissingError{Path: path, Field: "source"}
	case ev.SourceSHA256 == "":
		return nil, &FieldMissingError{Path: path, Field: "sourceSha256"}
	}
	if err := requireIdentity(path, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
		return nil, err
	}
	return &ev, nil
}

func LoadLiveAttempt(path string) (*LiveAttempt, error) {
	var ev LiveAttempt
	if err := decodeEvidence(path, &ev); err != nil {
		return nil, err
	}
	switch {
	case ev.RunID == "":
		return nil, &FieldMissingError{Path: path, Field: "runID"}
	case ev.Refusal == "":
		return nil, &FieldMissingError{Path: path, Field: "refusal"}
	}
	if err := requireIdentity(path, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
		return nil, err
	}
	return &ev, nil
}

func LoadMutation(path string) (*MutationEvidence, error) {
	var ev MutationEvidence
	if err := decodeEvidence(path, &ev); err != nil {
		return nil, err
	}
	switch {
	case ev.RunID == "":
		return nil, &FieldMissingError{Path: path, Field: "runId"}
	case ev.RowID == "":
		return nil, &FieldMissingError{Path: path, Field: "rowID"}
	}
	if err := requireIdentity(path, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
		return nil, err
	}
	return &ev, nil
}

func LoadAttempt(path string) (*Attempt, error) {
	var ev Attempt
	if err := decodeEvidence(path, &ev); err != nil {
		return nil, err
	}
	switch {
	case ev.RunID == "":
		return nil, &FieldMissingError{Path: path, Field: "runID"}
	case ev.RowID == "":
		return nil, &FieldMissingError{Path: path, Field: "rowID"}
	case ev.Refusal == "":
		return nil, &FieldMissingError{Path: path, Field: "refusal"}
	}
	if err := requireIdentity(path, ev.RepoCommit, ev.Source, ev.SourceSHA256); err != nil {
		return nil, err
	}
	return &ev, nil
}

func requireIdentity(path, commit, source, sha string) error {
	switch {
	case commit == "":
		return &FieldMissingError{Path: path, Field: "repoCommit"}
	case source == "":
		return &FieldMissingError{Path: path, Field: "source"}
	case sha == "":
		return &FieldMissingError{Path: path, Field: "sourceSha256"}
	}
	return nil
}
