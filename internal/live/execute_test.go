package live

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"test-design/internal/subprocess"
)

func writeTestCapture(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.raw")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTestCapture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	return string(data)
}

func TestSanitizedEnv(t *testing.T) {
	tests := []struct {
		name      string
		parent    []string
		allowlist []string
		want      []string
	}{
		{
			name:      "keeps only allowlisted names",
			parent:    []string{"PATH=/usr/bin", "HOME=/home/x", "TOKEN=secretvalue"},
			allowlist: []string{"PATH"},
			want:      []string{"PATH=/usr/bin"},
		},
		{
			name:      "missing allowlisted name is absent",
			parent:    []string{"PATH=/usr/bin"},
			allowlist: []string{"PATH", "MISSING_VAR"},
			want:      []string{"PATH=/usr/bin"},
		},
		{
			name:      "empty allowlist yields empty env",
			parent:    []string{"PATH=/usr/bin", "HOME=/home/x"},
			allowlist: nil,
			want:      []string{},
		},
		{
			name:      "order follows allowlist, not parent order",
			parent:    []string{"HOME=/home/x", "PATH=/usr/bin"},
			allowlist: []string{"PATH", "HOME"},
			want:      []string{"PATH=/usr/bin", "HOME=/home/x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizedEnv(tt.parent, tt.allowlist)
			if len(got) != len(tt.want) {
				t.Fatalf("SanitizedEnv() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SanitizedEnv()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSanitizedEnvOrderStable(t *testing.T) {
	allowlist := []string{"A", "B", "C"}
	parent1 := []string{"C=3", "A=1", "B=2"}
	parent2 := []string{"B=2", "A=1", "C=3"}

	got1 := SanitizedEnv(parent1, allowlist)
	got2 := SanitizedEnv(parent2, allowlist)

	if len(got1) != 3 || len(got2) != 3 {
		t.Fatalf("SanitizedEnv() lengths = %d, %d, want 3, 3", len(got1), len(got2))
	}
	for i := range got1 {
		if got1[i] != got2[i] {
			t.Errorf("SanitizedEnv() not order-stable: got1[%d]=%q got2[%d]=%q", i, got1[i], i, got2[i])
		}
	}
	want := []string{"A=1", "B=2", "C=3"}
	for i := range got1 {
		if got1[i] != want[i] {
			t.Errorf("SanitizedEnv()[%d] = %q, want %q", i, got1[i], want[i])
		}
	}
}

func TestExecuteStepChildNeverSeesNonAllowlistedEnv(t *testing.T) {
	const canaryName = "TD_LIVE_EXECUTE_CANARY"
	const canaryValue = "canary-secret-value-should-not-leak"
	t.Setenv(canaryName, canaryValue)

	parent := os.Environ()
	env := SanitizedEnv(parent, []string{"PATH"})

	step := Step{Expectation: "env is sanitized", Command: []string{"/usr/bin/env"}}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, env, runDir, runDir, Limits{})
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v, want nil", err)
	}
	if !res.Started {
		t.Fatal("ExecuteStep() res.Started = false, want true")
	}
	if res.ExitStatus != 0 {
		t.Fatalf("ExecuteStep() ExitStatus = %d, want 0", res.ExitStatus)
	}
	stdout := readTestCapture(t, res.StdoutPath)
	if strings.Contains(stdout, canaryName) || strings.Contains(stdout, canaryValue) {
		t.Errorf("child env leaked non-allowlisted canary: stdout = %q", stdout)
	}
}

func TestExecuteStepNonzeroExitIsNotAnError(t *testing.T) {
	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Skipf("false binary not found: %v", err)
	}

	step := Step{Expectation: "fails", Command: []string{falsePath}}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir, Limits{})
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v, want nil for a nonzero exit", err)
	}
	if !res.Started {
		t.Error("ExecuteStep() res.Started = false, want true")
	}
	if res.ExitStatus == 0 {
		t.Errorf("ExecuteStep() ExitStatus = 0, want nonzero")
	}
	if pass, _ := EvaluatePredicates([]Predicate{{Form: "exit-code", Value: "1"}}, res); !pass {
		t.Fatal("ordinary nonzero expectation failed")
	}
}

func TestExecuteStepStartFailureIsDistinctError(t *testing.T) {
	step := Step{Expectation: "missing binary", Command: []string{"/no/such/binary-td-live-test"}}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir, Limits{})
	if err == nil {
		t.Fatal("ExecuteStep() error = nil, want a start-failure error")
	}
	if res != nil {
		t.Errorf("ExecuteStep() res = %+v, want nil on start failure", res)
	}
}

func TestExecuteStepStartFailureErrorIsRedacted(t *testing.T) {
	const secretValue = "secretvalue"
	step := Step{
		Expectation: "missing binary with a secret argv",
		Command:     []string{"/no/such/binary-td-live-test", "--token=" + secretValue},
	}
	runDir := t.TempDir()

	_, err := ExecuteStep(step, []string{}, runDir, runDir, Limits{})
	if err == nil {
		t.Fatal("ExecuteStep() error = nil, want a start-failure error")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("ExecuteStep() error = %q, want the token value redacted", err.Error())
	}
	if !strings.Contains(err.Error(), "--token=REDACTED") {
		t.Errorf("ExecuteStep() error = %q, want it to name the redacted command", err.Error())
	}
}

func TestExecuteStepEmptyCommandErrors(t *testing.T) {
	step := Step{Expectation: "empty", Command: nil}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir, Limits{})
	if err == nil {
		t.Fatal("ExecuteStep() error = nil, want error for empty command")
	}
	if res != nil {
		t.Errorf("ExecuteStep() res = %+v, want nil on error", res)
	}
}

func TestExecuteStepCapturesTimestampsAndRawFiles(t *testing.T) {
	envPath, err := exec.LookPath("env")
	if err != nil {
		t.Skipf("env binary not found: %v", err)
	}
	step := Step{Expectation: "captures output", Command: []string{envPath, "-i", "GREETING=hello-stdout"}}
	runDir := t.TempDir()

	before := time.Now().UTC()
	res, err := ExecuteStep(step, []string{}, runDir, runDir, Limits{})
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}

	started, err := time.Parse(time.RFC3339Nano, res.StartedAt)
	if err != nil {
		t.Fatalf("StartedAt = %q, want RFC3339Nano: %v", res.StartedAt, err)
	}
	finished, err := time.Parse(time.RFC3339Nano, res.FinishedAt)
	if err != nil {
		t.Fatalf("FinishedAt = %q, want RFC3339Nano: %v", res.FinishedAt, err)
	}
	if started.Before(before.Add(-time.Second)) || finished.After(after.Add(time.Second)) {
		t.Errorf("timestamps out of range: started=%v finished=%v before=%v after=%v", started, finished, before, after)
	}

	stdoutOnDisk, err := os.ReadFile(res.StdoutPath)
	if err != nil {
		t.Fatalf("read StdoutPath: %v", err)
	}
	if string(stdoutOnDisk) != "GREETING=hello-stdout\n" {
		t.Errorf("stdout on disk = %q, want emitted environment", stdoutOnDisk)
	}
	if !strings.HasPrefix(res.StdoutPath, runDir) {
		t.Errorf("StdoutPath = %q, want it under runDir %q", res.StdoutPath, runDir)
	}
	if _, err := os.Stat(res.StderrPath); err != nil {
		t.Errorf("stat StderrPath: %v", err)
	}
}

func TestExecuteStepRunsInGivenDir(t *testing.T) {
	pwdPath, err := exec.LookPath("pwd")
	if err != nil {
		t.Skipf("pwd binary not found: %v", err)
	}
	dir := t.TempDir()
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	step := Step{Expectation: "runs in dir", Command: []string{pwdPath}}
	res, err := ExecuteStep(step, []string{}, dir, dir, Limits{})
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}

	got := strings.TrimSpace(readTestCapture(t, res.StdoutPath))
	if got != resolvedDir {
		t.Errorf("pwd output = %q, want %q", got, resolvedDir)
	}
}

func TestExecuteStepCombinedStreamIsRaceSafe(t *testing.T) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh binary not found: %v", err)
	}
	step := Step{
		Expectation: "writes to both streams concurrently",
		Command:     []string{shPath, "-c", "for i in $(seq 1 200); do echo out-$i; echo err-$i 1>&2; done"},
	}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir, Limits{})
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}
	combined := readTestCapture(t, res.CombinedPath)
	if !strings.Contains(combined, "out-1") || !strings.Contains(combined, "err-1") {
		t.Errorf("Combined = %q, want both stdout and stderr lines", combined)
	}
}

func TestEvaluatePredicates(t *testing.T) {
	tests := []struct {
		name       string
		preds      []Predicate
		res        *StepResult
		wantPass   bool
		wantFailed []Predicate
	}{
		{
			name:     "exit-code holds",
			preds:    []Predicate{{Form: "exit-code", Value: "0"}},
			res:      &StepResult{ExitStatus: 0},
			wantPass: true,
		},
		{
			name:       "exit-code fails",
			preds:      []Predicate{{Form: "exit-code", Value: "0"}},
			res:        &StepResult{ExitStatus: 1},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "exit-code", Value: "0"}},
		},
		{
			name:     "matches stdout holds",
			preds:    []Predicate{{Form: "matches", Stream: "stdout", Value: "hel+o"}},
			res:      &StepResult{StdoutPath: writeTestCapture(t, "hello world")},
			wantPass: true,
		},
		{
			name:       "matches stderr fails when absent",
			preds:      []Predicate{{Form: "matches", Stream: "stderr", Value: "hello"}},
			res:        &StepResult{StderrPath: writeTestCapture(t, "goodbye")},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "matches", Stream: "stderr", Value: "hello"}},
		},
		{
			name:     "not-matches combined holds",
			preds:    []Predicate{{Form: "not-matches", Stream: "combined", Value: "ERROR"}},
			res:      &StepResult{CombinedPath: writeTestCapture(t, "all good")},
			wantPass: true,
		},
		{
			name:       "not-matches combined fails",
			preds:      []Predicate{{Form: "not-matches", Stream: "combined", Value: "ERROR"}},
			res:        &StepResult{CombinedPath: writeTestCapture(t, "an ERROR occurred")},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "not-matches", Stream: "combined", Value: "ERROR"}},
		},
		{
			name: "all must hold: one fails among several",
			preds: []Predicate{
				{Form: "exit-code", Value: "0"},
				{Form: "matches", Stream: "stdout", Value: "ready"},
			},
			res:        &StepResult{ExitStatus: 0, StdoutPath: writeTestCapture(t, "not there")},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "matches", Stream: "stdout", Value: "ready"}},
		},
		{
			name:     "unbound predicate-free result",
			preds:    nil,
			res:      &StepResult{ExitStatus: 1},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPass, gotFailed := EvaluatePredicates(tt.preds, tt.res)
			if gotPass != tt.wantPass {
				t.Errorf("EvaluatePredicates() pass = %v, want %v", gotPass, tt.wantPass)
			}
			if len(gotFailed) != len(tt.wantFailed) {
				t.Fatalf("EvaluatePredicates() failed = %v, want %v", gotFailed, tt.wantFailed)
			}
			for i := range gotFailed {
				if gotFailed[i] != tt.wantFailed[i] {
					t.Errorf("EvaluatePredicates() failed[%d] = %+v, want %+v", i, gotFailed[i], tt.wantFailed[i])
				}
			}
		})
	}
}

func TestExecuteStepTimeoutPreservesRawAndCannotPass(t *testing.T) {
	dir := t.TempDir()
	step := Step{Command: []string{"/bin/sh", "-c", "echo ready; sleep 20"},
		Predicates: []Predicate{{Form: "matches", Stream: "stdout", Value: "ready"}}}
	res, err := ExecuteStep(step, []string{}, dir, dir, Limits{Timeout: 100 * time.Millisecond, OutputBytes: 1024})
	if !errors.Is(err, subprocess.ErrTimeout) || res == nil {
		t.Fatalf("ExecuteStep() = %+v, %v, want timeout with raw evidence", res, err)
	}
	data, readErr := os.ReadFile(res.StdoutPath)
	if readErr != nil || string(data) != "ready\n" {
		t.Fatalf("raw capture = %q, %v", data, readErr)
	}
	if pass, _ := EvaluatePredicates(step.Predicates, res); pass {
		t.Fatal("timed-out output satisfied predicates")
	}
}

func TestExecuteStepOutputLimitCountsBothStreamsOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		output string
		wantError bool
	}{
		{"exact combined budget", "printf 1234; printf 5678 >&2", false},
		{"combined overflow", "printf 1234; printf 56789 >&2; sleep 20", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			step := Step{Command: []string{"/bin/sh", "-c", tc.output}}
			res, err := ExecuteStep(step, []string{}, dir, dir, Limits{Timeout: time.Second, OutputBytes: 8})
			if (err != nil) != tc.wantError || res == nil {
				t.Fatalf("ExecuteStep() = %+v, %v", res, err)
			}
			if tc.wantError && !errors.Is(err, ErrOutputLimit) {
				t.Fatalf("error = %v, want output limit", err)
			}
			stdout, err := os.ReadFile(res.StdoutPath)
			if err != nil { t.Fatal(err) }
			stderr, err := os.ReadFile(res.StderrPath)
			if err != nil { t.Fatal(err) }
			combined, err := os.ReadFile(res.CombinedPath)
			if err != nil { t.Fatal(err) }
			if len(stdout)+len(stderr) != 8 || len(combined) != 8 {
				t.Fatalf("capture byte counts = %d+%d, combined %d", len(stdout), len(stderr), len(combined))
			}
			if pass, _ := EvaluatePredicates(nil, res); pass == tc.wantError {
				t.Fatalf("pass = %v with output error = %v", pass, tc.wantError)
			}
		})
	}
}

func TestEvaluatePredicatesMissingCaptureCannotSatisfyAbsence(t *testing.T) {
	res := &StepResult{StdoutPath: filepath.Join(t.TempDir(), "missing")}
	preds := []Predicate{{Form: "not-matches", Stream: "stdout", Value: "ERROR"}}
	if pass, _ := EvaluatePredicates(preds, res); pass {
		t.Fatal("missing capture satisfied a negative predicate")
	}
}
