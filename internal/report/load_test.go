package report

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTraceValidDocument(t *testing.T) {
	ev, err := LoadTrace(filepath.Join("testdata", "trace-valid.json"))
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if ev.RunID != "run-2026-08-31-0001" {
		t.Errorf("RunID = %q", ev.RunID)
	}
	if ev.BaselineStatus != "ok" {
		t.Errorf("BaselineStatus = %q", ev.BaselineStatus)
	}
	if len(ev.RowResults) != 2 || ev.RowResults[0].Status != "passed" {
		t.Errorf("RowResults = %+v", ev.RowResults)
	}
	if len(ev.Findings) != 1 || ev.Findings[0].Kind != "gap" || ev.Findings[0].RowID != "SC-05" {
		t.Errorf("Findings = %+v", ev.Findings)
	}
	if ev.Counts.Passed != 3 || ev.Counts.Skipped != 1 {
		t.Errorf("Counts = %+v", ev.Counts)
	}
	if ev.RowAccounting.ActiveRows != 4 {
		t.Errorf("RowAccounting = %+v", ev.RowAccounting)
	}
}

func TestLoadTraceMissingFileReturnsUnreadable(t *testing.T) {
	_, err := LoadTrace(filepath.Join("testdata", "does-not-exist.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadTrace missing file: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadTraceInvalidJSONReturnsUnreadable(t *testing.T) {
	_, err := LoadTrace(filepath.Join("testdata", "invalid.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadTrace invalid JSON: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadTraceWrongSchemaVersionReturnsSchemaMismatch(t *testing.T) {
	_, err := LoadTrace(filepath.Join("testdata", "wrong-schema.json"))
	if !errors.Is(err, ErrEvidenceSchemaMismatch) {
		t.Fatalf("LoadTrace wrong schema: err = %v, want ErrEvidenceSchemaMismatch", err)
	}
}

func TestLoadTraceMissingRunIDNamesField(t *testing.T) {
	_, err := LoadTrace(filepath.Join("testdata", "trace-missing-runid.json"))
	var fieldErr *FieldMissingError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("LoadTrace missing runId: err = %v, want *FieldMissingError", err)
	}
	if fieldErr.Field != "runId" {
		t.Errorf("Field = %q, want %q", fieldErr.Field, "runId")
	}
}

func TestLoadLiveValidDocument(t *testing.T) {
	ev, err := LoadLive(filepath.Join("testdata", "live-valid.json"))
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if ev.RunID != "run-2026-08-31-0001" {
		t.Errorf("RunID = %q", ev.RunID)
	}
	if len(ev.RowVerdicts) != 2 || ev.RowVerdicts[0].Verdict != "LIVE-PASS" {
		t.Errorf("RowVerdicts = %+v", ev.RowVerdicts)
	}
	if !ev.CleanupOK || ev.CleanupFailed {
		t.Errorf("CleanupOK/CleanupFailed = %v/%v", ev.CleanupOK, ev.CleanupFailed)
	}
	if !ev.SnapshotEqual {
		t.Errorf("SnapshotEqual = %v, want true", ev.SnapshotEqual)
	}
}

func TestLoadLiveMissingFileReturnsUnreadable(t *testing.T) {
	_, err := LoadLive(filepath.Join("testdata", "does-not-exist.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadLive missing file: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadLiveInvalidJSONReturnsUnreadable(t *testing.T) {
	_, err := LoadLive(filepath.Join("testdata", "invalid.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadLive invalid JSON: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadLiveWrongSchemaVersionReturnsSchemaMismatch(t *testing.T) {
	_, err := LoadLive(filepath.Join("testdata", "wrong-schema.json"))
	if !errors.Is(err, ErrEvidenceSchemaMismatch) {
		t.Fatalf("LoadLive wrong schema: err = %v, want ErrEvidenceSchemaMismatch", err)
	}
}

func TestLoadLiveMissingSourceNamesField(t *testing.T) {
	_, err := LoadLive(filepath.Join("testdata", "live-missing-source.json"))
	var fieldErr *FieldMissingError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("LoadLive missing source: err = %v, want *FieldMissingError", err)
	}
	if fieldErr.Field != "source" {
		t.Errorf("Field = %q, want %q", fieldErr.Field, "source")
	}
}

func TestLoadLiveAttemptValidDocument(t *testing.T) {
	ev, err := LoadLiveAttempt(filepath.Join("testdata", "live-attempt-valid.json"))
	if err != nil {
		t.Fatalf("LoadLiveAttempt: %v", err)
	}
	if ev.RunID != "run-2026-08-31-0001" {
		t.Errorf("RunID = %q", ev.RunID)
	}
	if ev.Refusal != "missing-profile" {
		t.Errorf("Refusal = %q", ev.Refusal)
	}
}

func TestLoadLiveAttemptMissingFileReturnsUnreadable(t *testing.T) {
	_, err := LoadLiveAttempt(filepath.Join("testdata", "does-not-exist.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadLiveAttempt missing file: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadLiveAttemptWrongSchemaVersionReturnsSchemaMismatch(t *testing.T) {
	_, err := LoadLiveAttempt(filepath.Join("testdata", "wrong-schema.json"))
	if !errors.Is(err, ErrEvidenceSchemaMismatch) {
		t.Fatalf("LoadLiveAttempt wrong schema: err = %v, want ErrEvidenceSchemaMismatch", err)
	}
}

func TestLoadLiveAttemptMissingRefusalNamesField(t *testing.T) {
	_, err := LoadLiveAttempt(filepath.Join("testdata", "live-attempt-missing-refusal.json"))
	var fieldErr *FieldMissingError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("LoadLiveAttempt missing refusal: err = %v, want *FieldMissingError", err)
	}
	if fieldErr.Field != "refusal" {
		t.Errorf("Field = %q, want %q", fieldErr.Field, "refusal")
	}
}

func TestLoadMutationValidDocument(t *testing.T) {
	ev, err := LoadMutation(filepath.Join("testdata", "mutation-valid.json"))
	if err != nil {
		t.Fatalf("LoadMutation: %v", err)
	}
	if ev.RunID != "run-2026-08-31-0001" {
		t.Errorf("RunID = %q", ev.RunID)
	}
	if ev.RowID != "SC-03" {
		t.Errorf("RowID = %q", ev.RowID)
	}
	if len(ev.Sites) != 1 || ev.Sites[0].Outcome != "caught" {
		t.Errorf("Sites = %+v", ev.Sites)
	}
	if !ev.SnapshotEqual {
		t.Errorf("SnapshotEqual = %v, want true", ev.SnapshotEqual)
	}
}

func TestLoadMutationMissingFileReturnsUnreadable(t *testing.T) {
	_, err := LoadMutation(filepath.Join("testdata", "does-not-exist.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadMutation missing file: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadMutationInvalidJSONReturnsUnreadable(t *testing.T) {
	_, err := LoadMutation(filepath.Join("testdata", "invalid.json"))
	if !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("LoadMutation invalid JSON: err = %v, want ErrEvidenceUnreadable", err)
	}
}

func TestLoadMutationWrongSchemaVersionReturnsSchemaMismatch(t *testing.T) {
	_, err := LoadMutation(filepath.Join("testdata", "wrong-schema.json"))
	if !errors.Is(err, ErrEvidenceSchemaMismatch) {
		t.Fatalf("LoadMutation wrong schema: err = %v, want ErrEvidenceSchemaMismatch", err)
	}
}

func TestLoadMutationMissingRunIDNamesField(t *testing.T) {
	_, err := LoadMutation(filepath.Join("testdata", "mutation-missing-runid.json"))
	var fieldErr *FieldMissingError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("LoadMutation missing runId: err = %v, want *FieldMissingError", err)
	}
	if fieldErr.Field != "runId" {
		t.Errorf("Field = %q, want %q", fieldErr.Field, "runId")
	}
}

func TestLoadEvidenceRequiresProvenance(t *testing.T) {
	for _, kind := range []string{"live", "mutation", "live-attempt", "mutation-attempt"} {
		t.Run(kind, func(t *testing.T) {
			for _, field := range []string{"repoCommit", "source", "sourceSha256"} {
				t.Run(field, func(t *testing.T) {
					doc := map[string]any{
						"schemaVersion": "1", "verifierVersion": "1.1.0",
						"runId": "run-1", "rowID": "SC-01", "refusal": "runner unavailable",
						"repoCommit": "head-1", "source": "spec.md", "sourceSha256": "sha-1",
					}
					delete(doc, field)
					body, err := json.Marshal(doc)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(t.TempDir(), "evidence.json")
					if err := os.WriteFile(path, body, 0o600); err != nil {
						t.Fatal(err)
					}
					switch kind {
					case "live":
						_, err = LoadLive(path)
					case "mutation":
						_, err = LoadMutation(path)
					case "live-attempt":
						_, err = LoadLiveAttempt(path)
					case "mutation-attempt":
						_, err = LoadAttempt(path)
					}
					var missing *FieldMissingError
					if !errors.As(err, &missing) || missing.Field != field {
						t.Fatalf("missing %s accepted or misreported: %v", field, err)
					}
				})
			}
		})
	}
}
