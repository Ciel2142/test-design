package live

import (
	"strings"
	"testing"
)

func TestReduceTranscriptPassResult(t *testing.T) {
	step := Step{
		Expectation: "server responds ready",
		Command:     []string{"curl", "http://127.0.0.1:8099/health"},
		Predicates: []Predicate{
			{Form: "exit-code", Value: "0"},
			{Form: "matches", Stream: "stdout", Value: "ready"},
		},
	}
	res := &StepResult{
		ExitStatus:   0,
		StdoutPath:   writeTestCapture(t, "status: ready\n"),
		CombinedPath: writeTestCapture(t, "status: ready\n"),
	}

	rs, err := ReduceTranscript(step, res, nil, nil)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v, want nil", err)
	}
	if rs.Result != "pass" {
		t.Errorf("Result = %q, want %q", rs.Result, "pass")
	}
	if rs.Expectation != step.Expectation {
		t.Errorf("Expectation = %q, want %q", rs.Expectation, step.Expectation)
	}
	if rs.ExitStatus != 0 {
		t.Errorf("ExitStatus = %d, want 0", rs.ExitStatus)
	}
	if !strings.Contains(rs.Decisive, "ready") {
		t.Errorf("Decisive = %q, want it to contain %q", rs.Decisive, "ready")
	}
}

func TestReduceTranscriptFailResult(t *testing.T) {
	step := Step{
		Expectation: "server responds ready",
		Command:     []string{"curl", "http://127.0.0.1:8099/health"},
		Predicates: []Predicate{
			{Form: "matches", Stream: "stdout", Value: "ready"},
		},
	}
	res := &StepResult{
		ExitStatus:   0,
		StdoutPath:   writeTestCapture(t, "status: down\n"),
		CombinedPath: writeTestCapture(t, "status: down\n"),
	}

	rs, err := ReduceTranscript(step, res, nil, nil)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v, want nil", err)
	}
	if rs.Result != "fail" {
		t.Errorf("Result = %q, want %q", rs.Result, "fail")
	}
}

func TestReduceTranscriptRedactsCommand(t *testing.T) {
	step := Step{
		Expectation: "auth request",
		Command:     []string{"curl", "--token=abc123def456", "http://127.0.0.1:8099"},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{ExitStatus: 0, CombinedPath: writeTestCapture(t, "")}

	rs, err := ReduceTranscript(step, res, nil, nil)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v", err)
	}
	for _, arg := range rs.Command {
		if strings.Contains(arg, "abc123def456") {
			t.Errorf("Command = %v, want the token value redacted", rs.Command)
		}
	}
	if rs.Command[1] != "--token=REDACTED" {
		t.Errorf("Command[1] = %q, want %q", rs.Command[1], "--token=REDACTED")
	}
}

func TestReduceTranscriptDecisiveOutputForMatchesIncludesContext(t *testing.T) {
	step := Step{
		Expectation: "log shows startup",
		Command:     []string{"cat", "log.txt"},
		Predicates:  []Predicate{{Form: "matches", Stream: "stdout", Value: "listening on"}},
	}
	res := &StepResult{
		ExitStatus: 0,
		StdoutPath: writeTestCapture(t, "line before\nserver listening on :8099\nline after\n"),
	}

	rs, err := ReduceTranscript(step, res, nil, nil)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v", err)
	}
	if !strings.Contains(rs.Decisive, "listening on") {
		t.Errorf("Decisive = %q, want the matched region", rs.Decisive)
	}
}

func TestReduceTranscriptDecisiveOutputForExitCodeIsTail(t *testing.T) {
	longOutput := strings.Repeat("x", 20000) + "TAIL-MARKER"
	step := Step{
		Expectation: "process exits clean",
		Command:     []string{"run"},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{
		ExitStatus:   0,
		CombinedPath: writeTestCapture(t, longOutput),
	}

	rs, err := ReduceTranscript(step, res, nil, nil)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v", err)
	}
	if len(rs.Decisive) > 8192 {
		t.Errorf("Decisive length = %d, want it capped to a bounded tail", len(rs.Decisive))
	}
	if !strings.Contains(rs.Decisive, "TAIL-MARKER") {
		t.Errorf("Decisive = %q, want it to contain the tail marker", rs.Decisive)
	}
}

func TestReduceTranscriptRedactionBarrierBlocksNonAllowlistedEnvValue(t *testing.T) {
	const secretName = "DB_PASSWORD"
	const secretValue = "sup3r-secret-value" // len >= 6

	step := Step{
		Expectation: "leaks a secret",
		Command:     []string{"run"},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{
		ExitStatus:   0,
		CombinedPath: writeTestCapture(t, "connecting with password "+secretValue),
	}
	parentEnv := []string{secretName + "=" + secretValue, "PATH=/usr/bin"}
	allowlist := []string{"PATH"}

	rs, err := ReduceTranscript(step, res, parentEnv, allowlist)
	if err == nil {
		t.Fatal("ReduceTranscript() error = nil, want a redaction barrier error")
	}
	if rs != nil {
		t.Errorf("ReduceTranscript() reduced step = %+v, want nil on redaction barrier error", rs)
	}
	if !strings.Contains(err.Error(), secretName) {
		t.Errorf("error = %q, want it to name the variable %q", err.Error(), secretName)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("error = %q, must never contain the secret value", err.Error())
	}
}

func TestReduceTranscriptRedactionBarrierAllowsAllowlistedEnvValue(t *testing.T) {
	step := Step{
		Expectation: "allowlisted value is fine",
		Command:     []string{"run"},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{
		ExitStatus:   0,
		CombinedPath: writeTestCapture(t, "base url is http://allowed-value-here"),
	}
	parentEnv := []string{"BASE_URL=http://allowed-value-here"}
	allowlist := []string{"BASE_URL"}

	rs, err := ReduceTranscript(step, res, parentEnv, allowlist)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v, want nil for an allowlisted value", err)
	}
	if rs == nil {
		t.Fatal("ReduceTranscript() reduced step = nil, want non-nil")
	}
}

func TestReduceTranscriptRedactionBarrierSkipsShortValues(t *testing.T) {
	step := Step{
		Expectation: "short value is not a false positive",
		Command:     []string{"run"},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{
		ExitStatus:   0,
		CombinedPath: writeTestCapture(t, "port is 8080"),
	}
	parentEnv := []string{"PORT=8080", "PATH=/usr/bin"} // "8080" is 4 chars, below the 6-char floor
	allowlist := []string{"PATH"}

	rs, err := ReduceTranscript(step, res, parentEnv, allowlist)
	if err != nil {
		t.Fatalf("ReduceTranscript() error = %v, want nil: short values must not trigger the barrier", err)
	}
	if rs == nil {
		t.Fatal("ReduceTranscript() reduced step = nil, want non-nil")
	}
}

func TestReduceTranscriptRedactionBarrierCatchesSecretInUnmatchedArgvSlot(t *testing.T) {
	const secretName = "DB_PASSWORD"
	const secretValue = "sup3r-secret-value" // len>=6; "-p" not a name pattern redactCommand catches

	step := Step{
		Expectation: "connects to db",
		Command:     []string{"mysql", "-p", secretValue},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{ExitStatus: 0, CombinedPath: writeTestCapture(t, "connected")}
	parentEnv := []string{secretName + "=" + secretValue}
	allowlist := []string{}

	rs, err := ReduceTranscript(step, res, parentEnv, allowlist)
	if err == nil {
		t.Fatal("ReduceTranscript() error = nil, want a redaction barrier error for a secret in an argv slot redactCommand's name patterns miss")
	}
	if rs != nil {
		t.Errorf("ReduceTranscript() reduced step = %+v, want nil on redaction barrier error", rs)
	}
	if !strings.Contains(err.Error(), secretName) {
		t.Errorf("error = %q, want it to name the variable %q", err.Error(), secretName)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("error = %q, must never contain the secret value", err.Error())
	}
}

func TestReduceTranscriptNeverContainsCanaryValue(t *testing.T) {
	const canaryValue = "planted-canary-leak-value"
	step := Step{
		Expectation: "canary must never surface",
		Command:     []string{"run"},
		Predicates:  []Predicate{{Form: "exit-code", Value: "0"}},
	}
	res := &StepResult{
		ExitStatus:   0,
		CombinedPath: writeTestCapture(t, "output includes "+canaryValue+" inline"),
	}
	parentEnv := []string{"CANARY_VAR=" + canaryValue}
	allowlist := []string{}

	rs, err := ReduceTranscript(step, res, parentEnv, allowlist)
	if err == nil {
		t.Fatal("ReduceTranscript() error = nil, want a redaction barrier error for the canary")
	}
	if rs != nil {
		t.Fatalf("ReduceTranscript() reduced step = %+v, want nil", rs)
	}
}
