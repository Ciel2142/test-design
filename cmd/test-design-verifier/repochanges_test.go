package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeRepoFile writes content at repo/rel, creating the directories it needs.
func writeRepoFile(t *testing.T, repo, rel, content string) string {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

// commitAll stages and commits everything in repo and returns the new HEAD.
func commitAll(t *testing.T, repo, message string) string {
	t.Helper()
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", message)
	return strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))
}

// TestRepoChanges_ChangedPathsNamesEveryChangedPathSorted holds changedPaths to
// what a later reuse decision needs: the tracked paths that differ between two
// commits, as repo-relative slash paths in sorted order, and — because renames
// are not followed — both ends of a rename, so a file that moved is never
// mistaken for a file nothing touched.
func TestRepoChanges_ChangedPathsNamesEveryChangedPathSorted(t *testing.T) {
	repo := fixtureRepo(t)
	base := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	orders := filepath.Join(repo, "orders", "orders.go")
	data, err := os.ReadFile(orders)
	if err != nil {
		t.Fatalf("read the fixture's orders source: %v", err)
	}
	if err := os.WriteFile(orders, append(data, []byte("\n// touched\n")...), 0o644); err != nil {
		t.Fatalf("write the fixture's orders source: %v", err)
	}
	writeRepoFile(t, repo, "shipping/shipping.go", "package shipping\n")
	edited := commitAll(t, repo, "edit one file and add another")

	got, err := changedPaths(repo, base, edited)
	if err != nil {
		t.Fatalf("changedPaths: %v", err)
	}
	if want := []string{"orders/orders.go", "shipping/shipping.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("changedPaths(%s, %s) = %v, want %v", base, edited, got, want)
	}

	mustGit(t, repo, "mv", "orders/orders.go", "orders/moved.go")
	renamed := commitAll(t, repo, "rename the orders source")

	got, err = changedPaths(repo, edited, renamed)
	if err != nil {
		t.Fatalf("changedPaths over the rename: %v", err)
	}
	if want := []string{"orders/moved.go", "orders/orders.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("changedPaths over the rename = %v, want both ends %v", got, want)
	}
}

// TestRepoChanges_CommitExistsAndTrackedAtAskTheObjectStore: both answers come
// from the repository's own objects, never from the working tree. A commit the
// repository never had is absent, and a file the working tree holds but the
// commit does not is untracked there.
func TestRepoChanges_CommitExistsAndTrackedAtAskTheObjectStore(t *testing.T) {
	repo := fixtureRepo(t)
	head := strings.TrimSpace(mustGit(t, repo, "rev-parse", "HEAD"))

	if !commitExists(repo, head) {
		t.Errorf("commitExists(%s) = false, want true for the repository's own HEAD", head)
	}
	if commitExists(repo, "deadbeef") {
		t.Error("commitExists(deadbeef) = true, want false for a commit this repository never had")
	}
	if !trackedAt(repo, head, "orders/orders.go") {
		t.Error("trackedAt(HEAD, orders/orders.go) = false, want true for a committed file")
	}
	writeRepoFile(t, repo, "orders/untracked.go", "package orders\n")
	if trackedAt(repo, head, "orders/untracked.go") {
		t.Error("trackedAt(HEAD, orders/untracked.go) = true, want false for a file HEAD does not carry")
	}
}

// TestRepoChanges_IsTestFileClassifiesByRunnerKind: a change to a test file is
// read differently from a change to production source, so the classification is
// per runner kind and per path segment — a directory merely ending in "src" is
// not a Maven test root, and an unsupported kind claims nothing.
func TestRepoChanges_IsTestFileClassifiesByRunnerKind(t *testing.T) {
	cases := []struct {
		kind string
		path string
		want bool
	}{
		{"go", "orders/orders_test.go", true},
		{"go", "orders/orders.go", false},
		{"go", "orders/testdata/orders_test.golden", false},
		{"maven", "src/test/java/OrdersTest.java", true},
		{"maven", "src/main/java/Orders.java", false},
		{"maven", "mysrc/test/java/OrdersTest.java", false},
		{"gradle", "app/src/test/kotlin/OrdersTest.kt", true},
		{"gradle", "app/src/main/kotlin/Orders.kt", false},
		{"pytest", "tests/test_orders.py", false},
	}
	for _, tc := range cases {
		if got := isTestFile(tc.kind, tc.path); got != tc.want {
			t.Errorf("isTestFile(%q, %q) = %v, want %v", tc.kind, tc.path, got, tc.want)
		}
	}
}
