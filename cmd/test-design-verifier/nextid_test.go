package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func decodeNextID(t *testing.T, stdout *bytes.Buffer) nextIDOutput {
	t.Helper()
	var got nextIDOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout did not decode as next-id output: %v\nstdout: %s", err, stdout.String())
	}
	return got
}

func entryFor(entries []nextIDEntry, area string) (nextIDEntry, bool) {
	for _, e := range entries {
		if e.Area == area {
			return e, true
		}
	}
	return nextIDEntry{}, false
}

func TestNextIDHandler_EmptyRepoYieldsEmptyArray(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	code := run([]string{"next-id", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodeNextID(t, &stdout)
	if len(got.NextIDs) != 0 {
		t.Errorf("NextIDs = %+v, want empty", got.NextIDs)
	}
	if got.SchemaVersion == "" || got.VerifierVersion == "" {
		t.Errorf("envelope not populated: %+v", got)
	}
}

func TestNextIDHandler_PerAreaAndPerKindOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "orders-repo")
	code := run([]string{"next-id", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodeNextID(t, &stdout)

	unprefixed, ok := entryFor(got.NextIDs, "")
	if !ok {
		t.Fatalf("no entry for unprefixed area in %+v", got.NextIDs)
	}
	if unprefixed.Kind != "sc" || unprefixed.Next != 2 {
		t.Errorf("unprefixed entry = %+v, want kind sc, next 2", unprefixed)
	}

	orders, ok := entryFor(got.NextIDs, "orders")
	if !ok {
		t.Fatalf("no entry for orders area in %+v", got.NextIDs)
	}
	if orders.Kind != "sc" || orders.Next != 8 {
		t.Errorf("orders entry = %+v, want kind sc, next 8 (orders/SC-07 retired must still occupy its number)", orders)
	}

	if len(got.NextIDs) != 2 {
		t.Errorf("NextIDs = %+v, want exactly 2 entries", got.NextIDs)
	}
	if got.NextIDs[0].Area != "" || got.NextIDs[1].Area != "orders" {
		t.Errorf("NextIDs order = %+v, want [\"\", \"orders\"]", got.NextIDs)
	}
}

func TestNextIDHandler_AreaFilterReturnsSingleEntry(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "orders-repo")
	code := run([]string{"next-id", "--repo", repo, "--area", "orders"}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodeNextID(t, &stdout)
	if len(got.NextIDs) != 1 {
		t.Fatalf("NextIDs = %+v, want exactly 1 entry", got.NextIDs)
	}
	if got.NextIDs[0].Area != "orders" || got.NextIDs[0].Next != 8 {
		t.Errorf("NextIDs[0] = %+v, want area orders, next 8", got.NextIDs[0])
	}
}

func TestNextIDHandler_AreaFilterUnknownAreaYieldsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "orders-repo")
	code := run([]string{"next-id", "--repo", repo, "--area", "billing"}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodeNextID(t, &stdout)
	if len(got.NextIDs) != 1 || got.NextIDs[0].Next != 1 {
		t.Errorf("NextIDs = %+v, want single entry with next 1", got.NextIDs)
	}
}

func TestNextIDHandler_InvalidAreaFlagRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "orders-repo")
	code := run([]string{"next-id", "--repo", repo, "--area", "orders/"}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "invalid-area" {
		t.Errorf("error code = %q, want %q", got.Code, "invalid-area")
	}
}

func TestNextIDHandler_OldHeaderCorpusIDsVisibleToAllocator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "old-header-repo")
	code := run([]string{"next-id", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	got := decodeNextID(t, &stdout)
	unprefixed, ok := entryFor(got.NextIDs, "")
	if !ok || unprefixed.Next != 5 {
		t.Errorf("NextIDs = %+v, want unprefixed area next 5 (old-header SC-01..SC-04 counted)", got.NextIDs)
	}
}

func TestNextIDHandler_CrossDocumentCollisionErrorNamesBothPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := filepath.Join("testdata", "collision-repo")
	code := run([]string{"next-id", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if !strings.Contains(got.Message, "2026-08-26-x-coverage.md") {
		t.Errorf("error message = %q, want it to name 2026-08-26-x-coverage.md", got.Message)
	}
	if !strings.Contains(got.Message, "2026-08-26-y-coverage.md") {
		t.Errorf("error message = %q, want it to name 2026-08-26-y-coverage.md", got.Message)
	}
	if !strings.Contains(got.Message, "SC-03") {
		t.Errorf("error message = %q, want it to name SC-03", got.Message)
	}
}

func TestNextIDHandler_MissingRepoCannotAllocate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing")
	code := run([]string{"next-id", "--repo", missing, "--area", ""}, nil, &stdout, &stderr)
	if code != exitError || stdout.Len() != 0 {
		t.Fatalf("missing repository allocated an ID: exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	if got := decodeError(t, &stderr); got.Code != "registry-error" {
		t.Fatalf("refusal = %q, want registry-error", got.Code)
	}
}
