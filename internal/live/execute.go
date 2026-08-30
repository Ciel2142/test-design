package live

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	stdoutRawFile = "stdout.raw"
	stderrRawFile = "stderr.raw"
)

// StepResult is one ExecuteStep call's outcome: whether the process started,
// its exit status, its captured streams (on disk under runDir, and in memory
// for predicate evaluation), and start/finish timestamps.
type StepResult struct {
	Started    bool
	ExitStatus int
	StartedAt  string
	FinishedAt string
	StdoutPath string
	StderrPath string
	Stdout     []byte
	Stderr     []byte
	Combined   []byte
}

// SanitizedEnv returns only the parent entries whose name is on the
// allowlist, in allowlist order — never the full parent environment. A
// missing allowlisted name is simply absent from the result.
func SanitizedEnv(parent []string, allowlist []string) []string {
	byName := make(map[string]string, len(parent))
	for _, kv := range parent {
		name, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		byName[name] = value
	}

	env := make([]string, 0, len(allowlist))
	for _, name := range allowlist {
		value, ok := byName[name]
		if !ok {
			continue
		}
		env = append(env, name+"="+value)
	}
	return env
}

// ExecuteStep runs step.Command with env in dir, capturing stdout and stderr
// individually and interleaved as "combined" to files under runDir. A start
// failure (e.g. missing binary) is returned as an error; a nonzero exit is a
// normal result carried in StepResult, not an error — same stance as
// cmd/test-design-verifier's runSuite. env is used verbatim: it never falls
// back to os.Environ(), so an unsanitized env is the caller's mistake, not
// this function's.
func ExecuteStep(step Step, env []string, dir, runDir string) (*StepResult, error) {
	if len(step.Command) == 0 {
		return nil, fmt.Errorf("execute step: empty command")
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	// stdout and stderr are copied by separate goroutines inside cmd.Run(),
	// so the buffer they both feed needs its own lock, not a bare bytes.Buffer.
	combinedBuf := &syncBuffer{}
	cmd := exec.Command(step.Command[0], step.Command[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = io.MultiWriter(&stdoutBuf, combinedBuf)
	cmd.Stderr = io.MultiWriter(&stderrBuf, combinedBuf)

	started := time.Now().UTC()
	runErr := cmd.Run()
	finished := time.Now().UTC()

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
	default:
		return nil, fmt.Errorf("execute step: run %s in %s: %w", strings.Join(step.Command, " "), dir, runErr)
	}

	stdoutPath := filepath.Join(runDir, stdoutRawFile)
	if err := os.WriteFile(stdoutPath, stdoutBuf.Bytes(), 0o644); err != nil {
		return nil, fmt.Errorf("execute step: write stdout: %w", err)
	}
	stderrPath := filepath.Join(runDir, stderrRawFile)
	if err := os.WriteFile(stderrPath, stderrBuf.Bytes(), 0o644); err != nil {
		return nil, fmt.Errorf("execute step: write stderr: %w", err)
	}

	return &StepResult{
		Started:    true,
		ExitStatus: cmd.ProcessState.ExitCode(),
		StartedAt:  started.Format(time.RFC3339Nano),
		FinishedAt: finished.Format(time.RFC3339Nano),
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
		Stdout:     stdoutBuf.Bytes(),
		Stderr:     stderrBuf.Bytes(),
		Combined:   combinedBuf.Bytes(),
	}, nil
}

// syncBuffer lets two of cmd.Run()'s internal copy goroutines (one per
// stream) write into the same buffer without racing.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Bytes()
}

// EvaluatePredicates reports whether every predicate holds against res.
// failed lists each violated predicate in order.
func EvaluatePredicates(preds []Predicate, res *StepResult) (pass bool, failed []Predicate) {
	failed = []Predicate{}
	for _, pred := range preds {
		if !predicateHolds(pred, res) {
			failed = append(failed, pred)
		}
	}
	return len(failed) == 0, failed
}

func predicateHolds(pred Predicate, res *StepResult) bool {
	switch pred.Form {
	case "exit-code":
		want, err := strconv.Atoi(pred.Value)
		if err != nil {
			return false
		}
		return res.ExitStatus == want
	case "matches":
		re, err := regexp.Compile(pred.Value)
		if err != nil {
			return false
		}
		return re.Match(streamBytes(pred.Stream, res))
	case "not-matches":
		re, err := regexp.Compile(pred.Value)
		if err != nil {
			return false
		}
		return !re.Match(streamBytes(pred.Stream, res))
	default:
		return false
	}
}

func streamBytes(stream string, res *StepResult) []byte {
	switch stream {
	case "stdout":
		return res.Stdout
	case "stderr":
		return res.Stderr
	default:
		return res.Combined
	}
}
