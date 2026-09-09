package runstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"test-design/internal/evidence"
)

func testIdentity() Identity {
	return Identity{
		RunID:        "run-1",
		RepoRoot:     "/repo",
		RepoCommit:   "0123456789abcdef",
		Source:       "docs/coverage.md",
		SourceSHA256: "d0d0d0",
	}
}

func openStore(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(%s): %v", dir, err)
	}
	if s.Dir() != dir {
		t.Fatalf("Dir() = %q, want %q", s.Dir(), dir)
	}
	return s
}

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestDefaultStateDir(t *testing.T) {
	t.Run("uses the environment override when set", func(t *testing.T) {
		t.Setenv(EnvStateDir, "/custom/state")
		got, err := DefaultStateDir()
		if err != nil {
			t.Fatalf("DefaultStateDir: %v", err)
		}
		if got != "/custom/state" {
			t.Errorf("DefaultStateDir = %q, want %q", got, "/custom/state")
		}
	})

	t.Run("falls back to the user cache dir", func(t *testing.T) {
		t.Setenv(EnvStateDir, "")
		cache, err := os.UserCacheDir()
		if err != nil {
			t.Skipf("no user cache dir on this host: %v", err)
		}
		want := filepath.Join(cache, "test-design-verifier")
		got, err := DefaultStateDir()
		if err != nil {
			t.Fatalf("DefaultStateDir: %v", err)
		}
		if got != want {
			t.Errorf("DefaultStateDir = %q, want %q", got, want)
		}
	})
}

func TestLocate(t *testing.T) {
	const stateDir = "/state"
	const repoRoot = "/repo"
	sum := sha256.Sum256([]byte(repoRoot))
	repoKey := hex.EncodeToString(sum[:])[:16]

	tests := []struct {
		name     string
		stateDir string
		repoRoot string
		runID    string
		want     string
		wantErr  string
	}{
		{
			name:     "derives runs/<repoKey>/<runID>",
			stateDir: stateDir,
			repoRoot: repoRoot,
			runID:    "run-1",
			want:     filepath.Join(stateDir, "runs", repoKey, "run-1"),
		},
		{
			name:     "refuses a run ID with a separator",
			stateDir: stateDir,
			repoRoot: repoRoot,
			runID:    "run-1/nested",
			wantErr:  "path separator",
		},
		{
			name:     "refuses a run ID with ..",
			stateDir: stateDir,
			repoRoot: repoRoot,
			runID:    "..",
			wantErr:  `".."`,
		},
		{
			name:     "refuses an empty run ID",
			stateDir: stateDir,
			repoRoot: repoRoot,
			runID:    "",
			wantErr:  "run ID is empty",
		},
		{
			name:     "refuses an empty state dir",
			stateDir: "",
			repoRoot: repoRoot,
			runID:    "run-1",
			wantErr:  "state dir is empty",
		},
		{
			name:     "refuses an empty repo root",
			stateDir: stateDir,
			repoRoot: "",
			runID:    "run-1",
			wantErr:  "repo root is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Locate(tt.stateDir, tt.repoRoot, tt.runID)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Locate = %q, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Locate error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Locate: %v", err)
			}
			if got != tt.want {
				t.Errorf("Locate = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad_MissingLedgerIsErrNoLedger(t *testing.T) {
	s := openStore(t, t.TempDir())

	l, err := s.Load()
	if !errors.Is(err, ErrNoLedger) {
		t.Fatalf("Load on an empty directory = (%v, %v), want ErrNoLedger", l, err)
	}
	if l != nil {
		t.Errorf("Load returned ledger %+v, want nil", l)
	}
}

func TestStart_FreshStoreCreatesTheLedgerAndFirstRecord(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()

	rec, err := s.Start(id, KindPhase, "trace", map[string]string{"runDir": dir}, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if rec.Seq != 1 {
		t.Errorf("Seq = %d, want 1", rec.Seq)
	}
	if rec.Kind != KindPhase || rec.ID != "trace" {
		t.Errorf("(Kind, ID) = (%q, %q), want (%q, %q)", rec.Kind, rec.ID, KindPhase, "trace")
	}
	if rec.Status != StatusStarted {
		t.Errorf("Status = %q, want %q", rec.Status, StatusStarted)
	}
	if _, err := time.Parse(time.RFC3339, rec.StartedAt); err != nil {
		t.Errorf("StartedAt = %q, want RFC3339: %v", rec.StartedAt, err)
	}
	if !strings.HasSuffix(rec.StartedAt, "Z") {
		t.Errorf("StartedAt = %q, want a UTC timestamp", rec.StartedAt)
	}

	l, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if l.SchemaVersion != evidence.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", l.SchemaVersion, evidence.SchemaVersion)
	}
	if l.Identity != id {
		t.Errorf("Identity = %+v, want %+v", l.Identity, id)
	}
	if len(l.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(l.Records))
	}
	var inputs map[string]string
	if err := json.Unmarshal(l.Records[0].Inputs, &inputs); err != nil {
		t.Fatalf("unmarshal Inputs %s: %v", l.Records[0].Inputs, err)
	}
	if inputs["runDir"] != dir {
		t.Errorf("Inputs[runDir] = %q, want %q", inputs["runDir"], dir)
	}

	raw, err := os.ReadFile(filepath.Join(dir, LedgerFile))
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	for _, want := range []string{`"runID": "run-1"`, `"sourceSha256": "d0d0d0"`, `"records": [`, `"startedAt"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ledger.json missing %s; got:\n%s", want, raw)
		}
	}
}

func TestStart_SupersedesAStartedRecord(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()

	if _, err := s.Start(id, KindLiveAttempt, "live", map[string]string{"runDir": dir}, "first", ""); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	second, err := s.Start(id, KindLiveAttempt, "live", map[string]string{"runDir": dir}, "second", "")
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if second.Seq != 2 {
		t.Fatalf("second Seq = %d, want 2", second.Seq)
	}

	l, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(l.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(l.Records))
	}
	prior := l.Records[0]
	if prior.Status != StatusSuperseded {
		t.Errorf("prior Status = %q, want %q", prior.Status, StatusSuperseded)
	}
	if prior.SupersededBy != 2 {
		t.Errorf("prior SupersededBy = %d, want 2", prior.SupersededBy)
	}
	if prior.Detail != "restarted" {
		t.Errorf("prior Detail = %q, want %q", prior.Detail, "restarted")
	}
	if _, err := time.Parse(time.RFC3339, prior.SupersededAt); err != nil {
		t.Errorf("prior SupersededAt = %q, want RFC3339: %v", prior.SupersededAt, err)
	}

	cur := Current(l, KindLiveAttempt, "live")
	if cur == nil || cur.Seq != 2 || cur.Detail != "second" {
		t.Errorf("Current = %+v, want the seq-2 started record", cur)
	}
	if got := Current(l, KindLiveAttempt, "other"); got != nil {
		t.Errorf("Current for an unknown key = %+v, want nil", got)
	}
}

func TestStart_CompletedRecordNeedsARepeatReason(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()
	art := writeFile(t, filepath.Join(t.TempDir(), "mutation-evidence.json"), `{"outcome":"caught"}`)

	if _, err := s.Start(id, KindMutationRow, "sc-17", map[string]string{"runnerKind": "go"}, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := s.Complete(id, KindMutationRow, "sc-17", "caught", map[string]string{"evidence": art}, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if _, err := s.Start(id, KindMutationRow, "sc-17", map[string]string{"runnerKind": "go"}, "", ""); !errors.Is(err, ErrCompleted) {
		t.Fatalf("Start over a completed record = %v, want ErrCompleted", err)
	}
	l, err := s.Load()
	if err != nil {
		t.Fatalf("Load after the refused Start: %v", err)
	}
	if len(l.Records) != 1 || l.Records[0].Status != StatusCompleted {
		t.Fatalf("records after the refused Start = %+v, want the one completed record untouched", l.Records)
	}

	again, err := s.Start(id, KindMutationRow, "sc-17", map[string]string{"runnerKind": "go"}, "", "row edited")
	if err != nil {
		t.Fatalf("Start with a repeat reason: %v", err)
	}
	if again.Seq != 2 {
		t.Errorf("Seq = %d, want 2", again.Seq)
	}

	l, err = s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	prior := l.Records[0]
	if prior.Status != StatusSuperseded || prior.SupersededBy != 2 {
		t.Errorf("prior = (%q, supersededBy %d), want (superseded, 2)", prior.Status, prior.SupersededBy)
	}
	if prior.Detail != "row edited" {
		t.Errorf("prior Detail = %q, want the repeat reason %q", prior.Detail, "row edited")
	}
	if prior.Outcome != "caught" {
		t.Errorf("prior Outcome = %q, want it preserved as %q", prior.Outcome, "caught")
	}
	if prior.CompletedAt == "" || prior.Artifacts["evidence"].Path != art {
		t.Errorf("prior lost its completion state: %+v", prior)
	}
}

func TestStart_ForeignIdentityIsErrIdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()

	if _, err := s.Start(id, KindPhase, "trace", map[string]string{"runDir": dir}, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	other := id
	other.RepoCommit = "ffffffffffffffff"

	if _, err := s.Start(other, KindPhase, "trace", map[string]string{"runDir": dir}, "", ""); !errors.Is(err, ErrIdentityMismatch) {
		t.Errorf("Start with a foreign identity = %v, want ErrIdentityMismatch", err)
	}
	if _, err := s.Complete(other, KindPhase, "trace", "ok", nil, ""); !errors.Is(err, ErrIdentityMismatch) {
		t.Errorf("Complete with a foreign identity = %v, want ErrIdentityMismatch", err)
	}
}

func TestComplete_DigestsArtifactsFromDisk(t *testing.T) {
	dir := t.TempDir()
	runDir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()

	body := `{"schemaVersion":"1"}`
	art := writeFile(t, filepath.Join(runDir, "trace-evidence.json"), body)
	inputs := writeFile(t, filepath.Join(runDir, "dispatch-inputs.json"), `{"rows":[]}`)
	sum := sha256.Sum256([]byte(body))
	wantDigest := hex.EncodeToString(sum[:])

	if _, err := s.Start(id, KindPhase, "trace", map[string]string{"runDir": runDir}, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	rec, err := s.Complete(id, KindPhase, "trace", "ok", map[string]string{"evidence": art, "dispatchInputs": inputs}, "phase done")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if rec.Status != StatusCompleted || rec.Outcome != "ok" {
		t.Errorf("(Status, Outcome) = (%q, %q), want (%q, %q)", rec.Status, rec.Outcome, StatusCompleted, "ok")
	}
	if _, err := time.Parse(time.RFC3339, rec.CompletedAt); err != nil {
		t.Errorf("CompletedAt = %q, want RFC3339: %v", rec.CompletedAt, err)
	}
	if rec.Detail != "phase done" {
		t.Errorf("Detail = %q, want %q", rec.Detail, "phase done")
	}
	if got := rec.Artifacts["evidence"]; got.Path != art || got.SHA256 != wantDigest {
		t.Errorf("evidence artifact = %+v, want path %s digest %s", got, art, wantDigest)
	}
	if got := rec.Artifacts["dispatchInputs"].Path; got != inputs {
		t.Errorf("dispatchInputs path = %q, want %q", got, inputs)
	}

	l, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := Current(l, KindPhase, "trace").Artifacts["evidence"].SHA256; got != wantDigest {
		t.Errorf("persisted digest = %q, want %q", got, wantDigest)
	}
	if reasons := Verify(Current(l, KindPhase, "trace")); len(reasons) != 0 {
		t.Errorf("Verify on an intact record = %v, want none", reasons)
	}
}

func TestComplete_WithoutAStartedRecordIsErrNotStarted(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()

	if _, err := s.Complete(id, KindPhase, "trace", "ok", nil, ""); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Complete with no ledger = %v, want ErrNotStarted", err)
	}

	if _, err := s.Start(id, KindReviewGroup, "g1", map[string]string{}, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := s.Complete(id, KindReviewGroup, "g2", "holds", nil, ""); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Complete of an unstarted key = %v, want ErrNotStarted", err)
	}
}

func TestComplete_TwiceIsErrDuplicateCompletion(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	id := testIdentity()

	if _, err := s.Start(id, KindReviewGroup, "g1", map[string]string{}, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := s.Complete(id, KindReviewGroup, "g1", "holds", nil, ""); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if _, err := s.Complete(id, KindReviewGroup, "g1", "blocked", nil, ""); !errors.Is(err, ErrDuplicateCompletion) {
		t.Fatalf("second Complete = %v, want ErrDuplicateCompletion", err)
	}

	l, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := Current(l, KindReviewGroup, "g1").Outcome; got != "holds" {
		t.Errorf("Outcome = %q, want the first completion's %q", got, "holds")
	}
}

func TestLoad_FailsClosedOnCorruptLedgers(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "malformed JSON",
			body:    `{"schemaVersion": "1", `,
			wantErr: "malformed JSON",
		},
		{
			name:    "foreign schema version",
			body:    `{"schemaVersion": "2", "identity": {}, "records": []}`,
			wantErr: "schemaVersion",
		},
		{
			name: "two current records for one key",
			body: `{"schemaVersion": "1", "identity": {}, "records": [
				{"seq": 1, "kind": "mutation-row", "id": "sc-17", "status": "started", "inputs": {}, "startedAt": "2026-09-07T00:00:00Z"},
				{"seq": 2, "kind": "mutation-row", "id": "sc-17", "status": "started", "inputs": {}, "startedAt": "2026-09-07T00:00:01Z"}
			]}`,
			wantErr: "two current records",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			s := openStore(t, dir)
			writeFile(t, filepath.Join(dir, LedgerFile), tt.body)

			l, err := s.Load()
			if err == nil {
				t.Fatalf("Load = %+v, want an error containing %q", l, tt.wantErr)
			}
			if l != nil {
				t.Errorf("Load returned ledger %+v alongside %v, want nil", l, err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Load error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestVerify(t *testing.T) {
	dir := t.TempDir()
	present := writeFile(t, filepath.Join(dir, "present.json"), `{"ok":true}`)
	sum := sha256.Sum256([]byte(`{"ok":true}`))

	tests := []struct {
		name string
		rec  *Record
		want []string
	}{
		{
			name: "a record with no artifacts is valid",
			rec:  &Record{Kind: KindPhase, ID: "trace", Status: StatusCompleted},
		},
		{
			name: "an intact artifact is valid",
			rec: &Record{Artifacts: map[string]Artifact{
				"evidence": {Path: present, SHA256: hex.EncodeToString(sum[:])},
			}},
		},
		{
			name: "a missing artifact reports artifact-missing",
			rec: &Record{Artifacts: map[string]Artifact{
				"evidence": {Path: filepath.Join(dir, "gone.json"), SHA256: hex.EncodeToString(sum[:])},
			}},
			want: []string{"artifact-missing:evidence"},
		},
		{
			name: "a rewritten artifact reports artifact-digest-mismatch",
			rec: &Record{Artifacts: map[string]Artifact{
				"evidence": {Path: present, SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
			}},
			want: []string{"artifact-digest-mismatch:evidence"},
		},
		{
			name: "every failing role is reported, ordered by role",
			rec: &Record{Artifacts: map[string]Artifact{
				"evidence": {Path: present, SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
				"attempt":  {Path: filepath.Join(dir, "gone.json"), SHA256: "deadbeef"},
			}},
			want: []string{"artifact-missing:attempt", "artifact-digest-mismatch:evidence"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Verify(tt.rec)
			if len(got) != len(tt.want) {
				t.Fatalf("Verify = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Verify = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestIsFailedOutcome(t *testing.T) {
	tests := []struct {
		kind    Kind
		outcome string
		want    bool
	}{
		{KindPhase, "ok", false},
		{KindReviewGroup, "holds", false},
		{KindReviewGroup, "blocked", true},
		{KindMutationRow, "caught", false},
		{KindMutationRow, "survived", true},
		{KindMutationRow, "unappliable", true},
		{KindMutationRow, "environmental", true},
		{KindLiveAttempt, "pass", false},
		{KindLiveAttempt, "fail", true},
		{KindLiveAttempt, "inconclusive", true},
		{KindLiveAttempt, "unavailable", true},
		{KindPhase, "", false},
		{KindLiveAttempt, "sideways", false},

		// The set is flat: a failing outcome stays failing under any kind, so a
		// ledger whose kind string is unrecognized cannot report a blocked run clean.
		{Kind("unknown"), "fail", true},
		{Kind(""), "survived", true},
		{KindPhase, "blocked", true},
		{KindReviewGroup, "unavailable", true},
		{Kind("unknown"), "ok", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind)+"/"+tt.outcome, func(t *testing.T) {
			if got := IsFailedOutcome(tt.kind, tt.outcome); got != tt.want {
				t.Errorf("IsFailedOutcome(%q, %q) = %v, want %v", tt.kind, tt.outcome, got, tt.want)
			}
		})
	}
}
