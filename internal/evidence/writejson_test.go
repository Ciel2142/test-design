package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type writeJSONFixture struct {
	SchemaVersion string         `json:"schemaVersion"`
	Zebra         string         `json:"zebra"`
	Apple         string         `json:"apple"`
	Tags          map[string]int `json:"tags"`
}

func newWriteJSONFixture() writeJSONFixture {
	return writeJSONFixture{
		SchemaVersion: SchemaVersion,
		Zebra:         "z",
		Apple:         "a",
		Tags:          map[string]int{"c": 3, "a": 1, "b": 2},
	}
}

func TestWriteJSONRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.json")
	doc := newWriteJSONFixture()

	if err := WriteJSON(path, doc); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got writeJSONFixture
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SchemaVersion != doc.SchemaVersion || got.Zebra != doc.Zebra || got.Apple != doc.Apple {
		t.Errorf("WriteJSON round trip = %+v, want %+v", got, doc)
	}
}

func TestWriteJSONIsDeterministicAcrossRepeatedCalls(t *testing.T) {
	dir := t.TempDir()
	doc := newWriteJSONFixture()

	const runs = 20
	var first []byte
	for i := range runs {
		path := filepath.Join(dir, "run.json")
		if err := WriteJSON(path, doc); err != nil {
			t.Fatalf("WriteJSON run %d: %v", i, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile run %d: %v", i, err)
		}
		if i == 0 {
			first = data
			continue
		}
		if !bytes.Equal(data, first) {
			t.Fatalf("WriteJSON run %d produced different bytes:\nfirst: %q\nrun %d: %q", i, first, i, data)
		}
	}
}

func TestWriteJSONKeyOrderIsStableRegardlessOfMapInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.json")
	doc := map[string]string{"zebra": "z", "middle": "m", "apple": "a"}

	if err := WriteJSON(path, doc); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	appleIdx := strings.Index(string(data), "apple")
	middleIdx := strings.Index(string(data), "middle")
	zebraIdx := strings.Index(string(data), "zebra")
	if !(appleIdx < middleIdx && middleIdx < zebraIdx) {
		t.Errorf("WriteJSON map keys not in sorted order: %s", data)
	}
}

func TestWriteJSONDoesNotHTMLEscape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.json")
	doc := map[string]string{"note": "a<b && c>d"}

	if err := WriteJSON(path, doc); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, escapeSeq := range []string{"\\u003c", "\\u003e", "\\u0026"} {
		if strings.Contains(string(data), escapeSeq) {
			t.Errorf("WriteJSON HTML-escaped output with %s: %s", escapeSeq, data)
		}
	}
	if !strings.Contains(string(data), "a<b && c>d") {
		t.Errorf("WriteJSON did not preserve literal content: %s", data)
	}
}

func TestWriteJSONMissingParentDirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-subdir", "doc.json")

	err := WriteJSON(path, newWriteJSONFixture())
	if err == nil {
		t.Fatalf("WriteJSON(%q) returned nil error, want error", path)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("WriteJSON error %q does not name path %q", err.Error(), path)
	}
}
