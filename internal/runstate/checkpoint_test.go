package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/evidence"
)

// covers: SC-21
func TestCheckpoint_InterruptedReplacementKeepsPriorCompletion(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "run-1")
	s := openStore(t, dir)
	id := testIdentity()

	art := writeFile(t, filepath.Join(base, "review-g1.json"), `{"verdicts":[{"id":"sc-17","verdict":"HOLDS"}]}`)

	if _, err := s.Start(id, KindReviewGroup, "g1", map[string]any{"rows": []string{"sc-17"}}, "", ""); err != nil {
		t.Fatalf("Start g1: %v", err)
	}
	if _, err := s.Complete(id, KindReviewGroup, "g1", "holds", map[string]string{"evidence": art}, ""); err != nil {
		t.Fatalf("Complete g1: %v", err)
	}

	interrupted := errors.New("write interrupted")
	original := checkpointWrite
	t.Cleanup(func() { checkpointWrite = original })
	checkpointWrite = func(f *os.File, data []byte) error {
		if _, err := f.Write(data[:len(data)/2]); err != nil {
			return err
		}
		return interrupted
	}

	if _, err := s.Start(id, KindReviewGroup, "g2", map[string]any{"rows": []string{"sc-18"}}, "", ""); !errors.Is(err, interrupted) {
		t.Fatalf("Start g2 through the interrupted seam = %v, want %v", err, interrupted)
	}

	checkpointWrite = original

	fresh := openStore(t, dir)
	l, err := fresh.Load()
	if err != nil {
		t.Fatalf("Load after the interrupted replacement: %v", err)
	}
	if l.SchemaVersion != evidence.SchemaVersion || l.Identity != id {
		t.Errorf("ledger header = (%q, %+v), want (%q, %+v)", l.SchemaVersion, l.Identity, evidence.SchemaVersion, id)
	}
	if len(l.Records) != 1 {
		t.Fatalf("records = %+v, want only the completed g1 record", l.Records)
	}
	only := l.Records[0]
	if only.ID != "g1" || only.Status != StatusCompleted || only.Outcome != "holds" {
		t.Errorf("record = %+v, want the completed g1 record with outcome holds", only)
	}
	if got := only.Artifacts["evidence"].Path; got != art {
		t.Errorf("evidence path = %q, want %q", got, art)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read ledger dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{LedgerFile, lockFile}
	if len(names) != len(want) {
		t.Fatalf("ledger dir = %v, want exactly %v", names, want)
	}
	for _, name := range names {
		if name != LedgerFile && name != lockFile {
			t.Errorf("ledger dir holds %q, want only %v", name, want)
		}
		if strings.HasPrefix(name, "."+LedgerFile+".tmp-") {
			t.Errorf("interrupted replacement left the temp file %q behind", name)
		}
	}

	rec, err := fresh.Start(id, KindReviewGroup, "g2", map[string]any{"rows": []string{"sc-18"}}, "", "")
	if err != nil {
		t.Fatalf("Start g2 after restoring the seam: %v", err)
	}
	if rec.Seq != 2 {
		t.Errorf("Seq = %d, want 2", rec.Seq)
	}

	l, err = fresh.Load()
	if err != nil {
		t.Fatalf("Load after the recovered Start: %v", err)
	}
	if len(l.Records) != 2 {
		t.Fatalf("records = %+v, want the completed g1 record and the started g2 one", l.Records)
	}
	if got := Current(l, KindReviewGroup, "g1"); got == nil || got.Status != StatusCompleted {
		t.Errorf("g1 = %+v, want it still completed", got)
	}
	if got := Current(l, KindReviewGroup, "g2"); got == nil || got.Status != StatusStarted {
		t.Errorf("g2 = %+v, want it started", got)
	}
}

func TestSaveLedger_NilRecordsSerializeAsAnEmptyArray(t *testing.T) {
	dir := t.TempDir()

	if err := saveLedger(dir, &Ledger{SchemaVersion: evidence.SchemaVersion, Identity: testIdentity()}); err != nil {
		t.Fatalf("saveLedger: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, LedgerFile))
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if !strings.Contains(string(raw), `"records": []`) {
		t.Errorf("ledger.json = %s, want records serialized as []", raw)
	}

	l, err := loadLedger(dir)
	if err != nil {
		t.Fatalf("loadLedger: %v", err)
	}
	if l.Records == nil {
		t.Error("loadLedger returned nil Records, want an empty slice")
	}
}
