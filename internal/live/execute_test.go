package live

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

	res, err := ExecuteStep(step, env, runDir, runDir)
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v, want nil", err)
	}
	if !res.Started {
		t.Fatal("ExecuteStep() res.Started = false, want true")
	}
	if res.ExitStatus != 0 {
		t.Fatalf("ExecuteStep() ExitStatus = %d, want 0", res.ExitStatus)
	}
	if strings.Contains(string(res.Stdout), canaryName) {
		t.Errorf("child env leaked non-allowlisted var %s: stdout = %q", canaryName, res.Stdout)
	}
	if strings.Contains(string(res.Stdout), canaryValue) {
		t.Errorf("child env leaked non-allowlisted value: stdout = %q", res.Stdout)
	}
}

func TestExecuteStepNonzeroExitIsNotAnError(t *testing.T) {
	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Skipf("false binary not found: %v", err)
	}

	step := Step{Expectation: "fails", Command: []string{falsePath}}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir)
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v, want nil for a nonzero exit", err)
	}
	if !res.Started {
		t.Error("ExecuteStep() res.Started = false, want true")
	}
	if res.ExitStatus == 0 {
		t.Errorf("ExecuteStep() ExitStatus = 0, want nonzero")
	}
}

func TestExecuteStepStartFailureIsDistinctError(t *testing.T) {
	step := Step{Expectation: "missing binary", Command: []string{"/no/such/binary-td-live-test"}}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir)
	if err == nil {
		t.Fatal("ExecuteStep() error = nil, want a start-failure error")
	}
	if res != nil {
		t.Errorf("ExecuteStep() res = %+v, want nil on start failure", res)
	}
}

func TestExecuteStepEmptyCommandErrors(t *testing.T) {
	step := Step{Expectation: "empty", Command: nil}
	runDir := t.TempDir()

	res, err := ExecuteStep(step, []string{}, runDir, runDir)
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
	res, err := ExecuteStep(step, []string{}, runDir, runDir)
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

	if !strings.Contains(string(res.Stdout), "GREETING=hello-stdout") {
		t.Errorf("res.Stdout = %q, want it to contain GREETING=hello-stdout", res.Stdout)
	}

	stdoutOnDisk, err := os.ReadFile(res.StdoutPath)
	if err != nil {
		t.Fatalf("read StdoutPath: %v", err)
	}
	if string(stdoutOnDisk) != string(res.Stdout) {
		t.Errorf("stdout on disk = %q, want it to match res.Stdout = %q", stdoutOnDisk, res.Stdout)
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
	res, err := ExecuteStep(step, []string{}, dir, dir)
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}

	got := strings.TrimSpace(string(res.Stdout))
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

	res, err := ExecuteStep(step, []string{}, runDir, runDir)
	if err != nil {
		t.Fatalf("ExecuteStep() error = %v", err)
	}
	if !strings.Contains(string(res.Combined), "out-1") || !strings.Contains(string(res.Combined), "err-1") {
		t.Errorf("Combined = %q, want both stdout and stderr lines", res.Combined)
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
			res:      &StepResult{Stdout: []byte("hello world")},
			wantPass: true,
		},
		{
			name:       "matches stderr fails when absent",
			preds:      []Predicate{{Form: "matches", Stream: "stderr", Value: "hello"}},
			res:        &StepResult{Stderr: []byte("goodbye")},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "matches", Stream: "stderr", Value: "hello"}},
		},
		{
			name:     "not-matches combined holds",
			preds:    []Predicate{{Form: "not-matches", Stream: "combined", Value: "ERROR"}},
			res:      &StepResult{Combined: []byte("all good")},
			wantPass: true,
		},
		{
			name:       "not-matches combined fails",
			preds:      []Predicate{{Form: "not-matches", Stream: "combined", Value: "ERROR"}},
			res:        &StepResult{Combined: []byte("an ERROR occurred")},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "not-matches", Stream: "combined", Value: "ERROR"}},
		},
		{
			name: "all must hold: one fails among several",
			preds: []Predicate{
				{Form: "exit-code", Value: "0"},
				{Form: "matches", Stream: "stdout", Value: "ready"},
			},
			res:        &StepResult{ExitStatus: 0, Stdout: []byte("not there")},
			wantPass:   false,
			wantFailed: []Predicate{{Form: "matches", Stream: "stdout", Value: "ready"}},
		},
		{
			name:     "no predicates always pass",
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
