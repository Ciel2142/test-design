package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSHA256FileMatchesKnownContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.md")
	content := []byte("# Coverage Source\n\nsc-1 | area | happy path\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])

	got, err := SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	if got != want {
		t.Errorf("SHA256File(%q) = %q, want %q", path, got, want)
	}
}

func TestSHA256FileMissingPathErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.md")

	got, err := SHA256File(path)
	if err == nil {
		t.Fatalf("SHA256File(%q) returned nil error with hash %q, want error", path, got)
	}
	if got != "" {
		t.Errorf("SHA256File(%q) = %q on error, want empty string", path, got)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("SHA256File error %q does not name path %q", err.Error(), path)
	}
}
