package runstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"test-design/internal/evidence"
)

// checkpointWrite copies data into the temp file that will replace the
// checkpoint. Tests replace it to fail a replacement after writing began;
// production never reassigns it.
var checkpointWrite = func(f *os.File, data []byte) error {
	_, err := f.Write(data)
	return err
}

// saveLedger replaces <dir>/ledger.json with l: the bytes go to a temp file in
// the same directory, which is synced, closed and renamed over the checkpoint.
// A failure at any point removes the temp file and leaves the checkpoint as it
// was, so a reader either sees the whole previous ledger or the whole new one.
func saveLedger(dir string, l *Ledger) error {
	path := filepath.Join(dir, LedgerFile)

	out := *l
	if out.Records == nil {
		out.Records = []Record{}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&out); err != nil {
		return fmt.Errorf("save ledger %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, "."+LedgerFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("save ledger %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if err := checkpointWrite(tmp, buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("save ledger %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("save ledger %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("save ledger %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("save ledger %s: %w", path, err)
	}
	return nil
}

// loadLedger reads <dir>/ledger.json and nothing else — a temp file left by an
// interrupted replacement is never read. It fails closed: a missing checkpoint
// is ErrNoLedger, and malformed JSON, a foreign schemaVersion or two current
// records for one (Kind, ID) each return an error naming the checkpoint rather
// than a ledger a caller might mistake for valid.
func loadLedger(dir string) (*Ledger, error) {
	path := filepath.Join(dir, LedgerFile)

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("load ledger %s: %w", path, ErrNoLedger)
	}
	if err != nil {
		return nil, fmt.Errorf("load ledger %s: %w", path, err)
	}

	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("load ledger %s: malformed JSON: %w", path, err)
	}
	if l.SchemaVersion != evidence.SchemaVersion {
		return nil, fmt.Errorf("load ledger %s: schemaVersion %q, want %q", path, l.SchemaVersion, evidence.SchemaVersion)
	}
	if l.Records == nil {
		l.Records = []Record{}
	}

	current := make(map[Kind]map[string]int, len(l.Records))
	for _, rec := range l.Records {
		if rec.Status == StatusSuperseded {
			continue
		}
		byID := current[rec.Kind]
		if byID == nil {
			byID = make(map[string]int)
			current[rec.Kind] = byID
		}
		if prior, ok := byID[rec.ID]; ok {
			return nil, fmt.Errorf("load ledger %s: two current records for %s/%s (seq %d and %d)", path, rec.Kind, rec.ID, prior, rec.Seq)
		}
		byID[rec.ID] = rec.Seq
	}

	return &l, nil
}
