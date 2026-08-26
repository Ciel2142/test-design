package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func writeRequestFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRun_RequestUnsupportedSchemaVersion(t *testing.T) {
	for _, op := range []string{"next-id", "preflight"} {
		t.Run(op, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			stdin := strings.NewReader(`{"schemaVersion":"2"}`)
			args := []string{op, "--request", "-"}
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

func TestRun_RequestFileUnsupportedSchemaVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	path := writeRequestFile(t, `{"schemaVersion":"2"}`)
	code := run([]string{"next-id", "--request", path}, nil, &stdout, &stderr)

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

func TestRun_RequestMissingSchemaVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader(`{"foo":"bar"}`)
	code := run([]string{"next-id", "--request", "-"}, stdin, &stdout, &stderr)

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

func TestRun_RequestMalformedJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("not json")
	code := run([]string{"next-id", "--request", "-"}, stdin, &stdout, &stderr)

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

func TestRun_RequestEmptyDocumentIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("")
	code := run([]string{"next-id", "--request", "-"}, stdin, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d (an explicitly supplied empty request is not a valid document)", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "malformed-request" {
		t.Errorf("error code = %q, want %q", got.Code, "malformed-request")
	}
}

func TestRun_RequestFileUnreadableNamesPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	path := filepath.Join(t.TempDir(), "no-such-request.json")
	code := run([]string{"next-id", "--request", path}, nil, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := decodeError(t, &stderr)
	if got.Code != "request-read-failed" {
		t.Errorf("error code = %q, want %q", got.Code, "request-read-failed")
	}
	if !strings.Contains(got.Message, path) {
		t.Errorf("error message = %q, want it to name %q", got.Message, path)
	}
}

func TestRun_RequestFlagWithEmptyPathRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"next-id", "--request", ""}, nil, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	got := decodeError(t, &stderr)
	if got.Code != "missing-request-path" {
		t.Errorf("error code = %q, want %q", got.Code, "missing-request-path")
	}
}

func TestRun_RequestValidSchemaVersionProceedsToOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	stdin := strings.NewReader(`{"schemaVersion":"1"}`)
	code := run([]string{"next-id", "--repo", repo, "--request", "-"}, stdin, &stdout, &stderr)

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

func TestRun_NoRequestFlagSkipsVersionCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	repo := t.TempDir()
	code := run([]string{"next-id", "--repo", repo}, strings.NewReader(`{"schemaVersion":"2"}`), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (no --request means no request document); stderr=%s", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Errorf("stdout is empty, want a JSON document")
	}
}

func TestRun_UnknownFlagIsReportedBeforeAnyRequestRead(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"next-id", "--bogus"}, neverClosingReader{}, &stdout, &stderr)

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	got := decodeError(t, &stderr)
	if got.Code != "unknown-flag" {
		t.Errorf("error code = %q, want %q", got.Code, "unknown-flag")
	}
}

// neverClosingReader stands in for an inherited pipe whose writer never
// closes: the exact stdin a CI or agent harness hands a child process.
type neverClosingReader struct{}

func (neverClosingReader) Read([]byte) (int, error) {
	select {}
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

func TestRun_InheritedOpenStdinPipeDoesNotBlockTheOperation(t *testing.T) {
	repo := filepath.Join("testdata", "orders-repo")

	done := make(chan runResult, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		code := run([]string{"next-id", "--repo", repo}, neverClosingReader{}, &stdout, &stderr)
		done <- runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}()

	select {
	case got := <-done:
		if got.code != exitOK {
			t.Fatalf("exit code = %d, want %d; stderr=%s", got.code, exitOK, got.stderr)
		}
		if got.stdout == "" {
			t.Errorf("stdout is empty, want a JSON document")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return within 15s with an open stdin pipe: the CLI still reads stdin unbidden")
	}
}
