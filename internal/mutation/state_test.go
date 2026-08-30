package mutation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/evidence"
)

func sampleState() *State {
	return &State{
		RowID:        "SC-02",
		Identity:     Identity{Scope: "fixturemod/orders", Name: "TestValidateQuantityRejectsZero"},
		WorktreePath: "/tmp/wt-example",
		Snapshot:     json.RawMessage(`{"headCommit":"abc123"}`),
		Sites:        []SiteOutcome{{Site: "orders.go:12", Outcome: "caught", DiffPath: "sites/1.diff"}},
	}
}

func TestWriteStateLoadStateRoundTrip(t *testing.T) {
	runDir := t.TempDir()
	want := sampleState()

	if err := WriteState(runDir, want); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	got, err := LoadState(runDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.SchemaVersion != evidence.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, evidence.SchemaVersion)
	}
	if got.RowID != want.RowID {
		t.Errorf("RowID = %q, want %q", got.RowID, want.RowID)
	}
	if got.Identity != want.Identity {
		t.Errorf("Identity = %+v, want %+v", got.Identity, want.Identity)
	}
	if got.WorktreePath != want.WorktreePath {
		t.Errorf("WorktreePath = %q, want %q", got.WorktreePath, want.WorktreePath)
	}
	var gotSnapshot, wantSnapshot map[string]any
	if err := json.Unmarshal(got.Snapshot, &gotSnapshot); err != nil {
		t.Fatalf("unmarshal got.Snapshot: %v", err)
	}
	if err := json.Unmarshal(want.Snapshot, &wantSnapshot); err != nil {
		t.Fatalf("unmarshal want.Snapshot: %v", err)
	}
	if gotSnapshot["headCommit"] != wantSnapshot["headCommit"] {
		t.Errorf("Snapshot = %s, want %s", got.Snapshot, want.Snapshot)
	}
	if len(got.Sites) != 1 || got.Sites[0] != want.Sites[0] {
		t.Errorf("Sites = %+v, want %+v", got.Sites, want.Sites)
	}
}

func TestWriteStateStampsSchemaVersionRegardlessOfCallerInput(t *testing.T) {
	runDir := t.TempDir()
	st := sampleState()
	st.SchemaVersion = "bogus"

	if err := WriteState(runDir, st); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, err := LoadState(runDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.SchemaVersion != evidence.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, evidence.SchemaVersion)
	}
}

func TestWriteStateLeavesNoTempFileBehind(t *testing.T) {
	runDir := t.TempDir()
	if err := WriteState(runDir, sampleState()); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != stateFileName {
		t.Errorf("runDir entries = %v, want exactly %q", entries, stateFileName)
	}
}

func TestLoadStateMissingFileFailsClosed(t *testing.T) {
	runDir := t.TempDir()
	_, err := LoadState(runDir)
	if err == nil {
		t.Fatal("LoadState returned nil error, want a fail-closed error")
	}
	if !strings.Contains(err.Error(), filepath.Join(runDir, stateFileName)) {
		t.Errorf("error = %q, want it to name the state file", err.Error())
	}
}

func TestLoadStateMalformedJSONFailsClosed(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, stateFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadState(runDir)
	if err == nil {
		t.Fatal("LoadState returned nil error, want a fail-closed error")
	}
	if !strings.Contains(err.Error(), stateFileName) {
		t.Errorf("error = %q, want it to name the state file", err.Error())
	}
}

func TestLoadStateWrongSchemaVersionFailsClosed(t *testing.T) {
	runDir := t.TempDir()
	if err := WriteState(runDir, sampleState()); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	path := filepath.Join(runDir, stateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	corrupted := strings.Replace(string(data), `"schemaVersion": "`+evidence.SchemaVersion+`"`, `"schemaVersion": "999"`, 1)
	if corrupted == string(data) {
		t.Fatal("corruption did not apply; the written document's field spacing changed")
	}
	if err := os.WriteFile(path, []byte(corrupted), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = LoadState(runDir)
	if err == nil {
		t.Fatal("LoadState returned nil error, want a fail-closed error")
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error = %q, want it to mention the unsupported version", err.Error())
	}
}

func TestLoadStateEmptyRequiredFieldsFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(*State)
	}{
		{"empty rowID", func(s *State) { s.RowID = "" }},
		{"empty worktreePath", func(s *State) { s.WorktreePath = "" }},
		{"empty identity", func(s *State) { s.Identity = Identity{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runDir := t.TempDir()
			st := sampleState()
			tc.corrupt(st)
			if err := WriteState(runDir, st); err != nil {
				t.Fatalf("WriteState: %v", err)
			}

			_, err := LoadState(runDir)
			if err == nil {
				t.Fatal("LoadState returned nil error, want a fail-closed error")
			}
			if !strings.Contains(err.Error(), stateFileName) {
				t.Errorf("error = %q, want it to name the state file", err.Error())
			}
		})
	}
}
