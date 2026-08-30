package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"test-design/internal/evidence"
)

func writePublishSource(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}

func TestPublishRefusesOnConcurrentRepoChange(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("concurrent edit\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	src := writePublishSource(t, t.TempDir(), "published.txt", "new content\n")
	records, err := ctrl.Publish(before, []PublishFile{{RepoRelPath: "published.txt", SourcePath: src}})
	if err == nil {
		t.Fatalf("Publish = %+v, nil, want an error after a concurrent repo change", records)
	}
	if !strings.Contains(err.Error(), "tracked.txt") {
		t.Errorf("Publish error = %v, want it to name %q", err, "tracked.txt")
	}
	if !strings.Contains(err.Error(), "commit or discard the concurrent change, then rerun") {
		t.Errorf("Publish error = %v, want the commit-or-discard instruction", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "corrupt") {
		t.Errorf("Publish error = %v, must not claim corruption", err)
	}

	if _, statErr := os.Stat(filepath.Join(dir, "published.txt")); !os.IsNotExist(statErr) {
		t.Errorf("published.txt exists after refused Publish, stat err = %v", statErr)
	}
}

func TestPublishWritesFilesWithMatchingHashes(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	srcDir := t.TempDir()
	srcA := writePublishSource(t, srcDir, "a.txt", "content A\n")
	srcB := writePublishSource(t, srcDir, "b.txt", "content B\n")

	records, err := ctrl.Publish(before, []PublishFile{
		{RepoRelPath: "a.txt", SourcePath: srcA},
		{RepoRelPath: "b.txt", SourcePath: srcB},
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Publish returned %d records, want 2", len(records))
	}

	for i, rel := range []string{"a.txt", "b.txt"} {
		published := filepath.Join(dir, rel)
		got, err := os.ReadFile(published)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", published, err)
		}
		src := srcA
		if rel == "b.txt" {
			src = srcB
		}
		want, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", src, err)
		}
		if string(got) != string(want) {
			t.Errorf("published %q content = %q, want %q", rel, got, want)
		}

		wantSum, err := evidence.SHA256File(published)
		if err != nil {
			t.Fatalf("SHA256File(%q): %v", published, err)
		}
		if records[i].RepoRelPath != rel {
			t.Errorf("records[%d].RepoRelPath = %q, want %q", i, records[i].RepoRelPath, rel)
		}
		if records[i].SHA256 != wantSum {
			t.Errorf("records[%d].SHA256 = %q, want %q", i, records[i].SHA256, wantSum)
		}
	}
}

func TestPublishRefusesPathEscapingRepoRoot(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	src := writePublishSource(t, t.TempDir(), "escape.txt", "escape\n")

	tests := []struct {
		name string
		rel  string
	}{
		{"relative traversal", "../escape.txt"},
		{"absolute path", filepath.Join(t.TempDir(), "escape.txt")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := ctrl.Publish(before, []PublishFile{{RepoRelPath: tt.rel, SourcePath: src}})
			if err == nil {
				t.Fatalf("Publish(%q) = %+v, nil, want an error", tt.rel, records)
			}

			outsidePath := filepath.Join(filepath.Dir(dir), "escape.txt")
			if _, statErr := os.Stat(outsidePath); !os.IsNotExist(statErr) {
				t.Errorf("file created outside repo at %q, stat err = %v", outsidePath, statErr)
			}
		})
	}
}

func TestPublishCreatesNestedDirectories(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	src := writePublishSource(t, t.TempDir(), "nested.txt", "nested content\n")
	records, err := ctrl.Publish(before, []PublishFile{{RepoRelPath: filepath.Join("a", "b", "nested.txt"), SourcePath: src}})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Publish returned %d records, want 1", len(records))
	}

	published := filepath.Join(dir, "a", "b", "nested.txt")
	got, err := os.ReadFile(published)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", published, err)
	}
	if string(got) != "nested content\n" {
		t.Errorf("published content = %q, want %q", got, "nested content\n")
	}
}

func TestPublishPartialFailureNamesWrittenAndNotWritten(t *testing.T) {
	dir := newTestRepo(t)
	ctrl, err := NewController(dir)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	before, err := ctrl.TakeSnapshot()
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	srcDir := t.TempDir()
	ok := writePublishSource(t, srcDir, "ok.txt", "ok\n")
	missing := filepath.Join(srcDir, "does-not-exist.txt")

	records, err := ctrl.Publish(before, []PublishFile{
		{RepoRelPath: "ok.txt", SourcePath: ok},
		{RepoRelPath: "broken.txt", SourcePath: missing},
	})
	if err == nil {
		t.Fatalf("Publish = %+v, nil, want an error for a missing source file", records)
	}
	if !strings.Contains(err.Error(), "ok.txt") {
		t.Errorf("Publish error = %v, want it to name written file %q", err, "ok.txt")
	}
	if !strings.Contains(err.Error(), "broken.txt") {
		t.Errorf("Publish error = %v, want it to name unwritten file %q", err, "broken.txt")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "ok.txt")); statErr != nil {
		t.Errorf("ok.txt not found after partial publish failure: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "broken.txt")); !os.IsNotExist(statErr) {
		t.Errorf("broken.txt exists after failed write, stat err = %v", statErr)
	}
}
