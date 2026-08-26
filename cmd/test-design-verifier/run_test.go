package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func decodeError(t *testing.T, stderr *bytes.Buffer) cliError {
	t.Helper()
	var got cliError
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("stderr did not decode as JSON error: %v\nstderr: %s", err, stderr.String())
	}
	return got
}

func TestRun_NoOperation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, nil, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "unknown-operation" {
		t.Errorf("error code = %q, want %q", got.Code, "unknown-operation")
	}
}

func TestRun_UnknownOperation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus-op"}, nil, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "unknown-operation" {
		t.Errorf("error code = %q, want %q", got.Code, "unknown-operation")
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	for _, op := range []string{"next-id", "preflight"} {
		t.Run(op, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{op, "--bogus", "x"}, nil, &stdout, &stderr)

			if code != exitError {
				t.Errorf("exit code = %d, want %d", code, exitError)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			got := decodeError(t, &stderr)
			if got.Code != "unknown-flag" {
				t.Errorf("error code = %q, want %q", got.Code, "unknown-flag")
			}
		})
	}
}

func TestRun_NextIDDispatchesToRealHandler(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	code := run([]string{"next-id", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Errorf("stdout is empty, want a JSON document")
	}
}

func TestRun_PreflightDispatchesToRealHandler(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	source := filepath.Join(repo, "missing-coverage.md")
	code := run([]string{"preflight", "--repo", repo, "--source", source}, nil, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code == "unknown-operation" || got.Code == "unknown-flag" || got.Code == "not-implemented" {
		t.Errorf("error code = %q, want a real preflight error, not a dispatch/stub error", got.Code)
	}
}

func TestRun_StdinUnsupportedSchemaVersion(t *testing.T) {
	for _, op := range []string{"next-id", "preflight"} {
		t.Run(op, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			stdin := strings.NewReader(`{"schemaVersion":"2"}`)
			args := []string{op}
			if op == "preflight" {
				args = append(args, "--source", "coverage.md")
			}
			code := run(args, stdin, &stdout, &stderr)

			if code != exitError {
				t.Errorf("exit code = %d, want %d", code, exitError)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			got := decodeError(t, &stderr)
			if got.Code != "unsupported-schema-version" {
				t.Errorf("error code = %q, want %q", got.Code, "unsupported-schema-version")
			}
		})
	}
}

func TestRun_StdinMissingSchemaVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader(`{"foo":"bar"}`)
	code := run([]string{"next-id"}, stdin, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "unsupported-schema-version" {
		t.Errorf("error code = %q, want %q", got.Code, "unsupported-schema-version")
	}
}

func TestRun_StdinMalformedJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("not json")
	code := run([]string{"next-id"}, stdin, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "malformed-request" {
		t.Errorf("error code = %q, want %q", got.Code, "malformed-request")
	}
}

func TestRun_StdinValidSchemaVersionProceedsToOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	stdin := strings.NewReader(`{"schemaVersion":"1"}`)
	code := run([]string{"next-id", "--repo", repo}, stdin, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (version gate should have passed); stderr=%s", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Errorf("stdout is empty, want a JSON document")
	}
}

func TestRun_StdinEmptyReaderSkipsVersionCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	stdin := strings.NewReader("")
	code := run([]string{"next-id", "--repo", repo}, stdin, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (empty stdin should skip the version check); stderr=%s", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Errorf("stdout is empty, want a JSON document")
	}
}

func TestRun_NilStdinSkipsVersionCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	code := run([]string{"next-id", "--repo", repo}, nil, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (nil stdin should skip the version check); stderr=%s", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Errorf("stdout is empty, want a JSON document")
	}
}
