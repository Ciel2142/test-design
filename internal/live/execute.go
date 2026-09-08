package live

import (
	"context"
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

	"test-design/internal/subprocess"
)

const (
	stdoutRawFile   = "stdout.raw"
	stderrRawFile   = "stderr.raw"
	combinedRawFile = "combined.raw"
)

var ErrOutputLimit = errors.New("live output limit exceeded")

// StepResult retains file-backed raw evidence, not duplicate in-memory streams.
// Err distinguishes incomplete capture/execution from ordinary nonzero exits.
type StepResult struct {
	Started      bool
	ExitStatus   int
	StartedAt    string
	FinishedAt   string
	StdoutPath   string
	StderrPath   string
	CombinedPath string
	Err          error
	outputBytes  int64
}

// SanitizedEnv returns only allowlisted parent entries, in allowlist order.
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

// ExecuteStep captures stdout, stderr, and their interleaving directly to files.
// The byte budget counts emitted bytes once across both streams. On overflow it
// retains the captured prefix, terminates the process group, and returns an error
// alongside raw evidence. Nonzero exits remain normal results for predicates.
func ExecuteStep(step Step, env []string, dir, runDir string, limits Limits) (*StepResult, error) {
	if len(step.Command) == 0 {
		return nil, fmt.Errorf("execute step: empty command")
	}
	if limits.Timeout < 0 || limits.OutputBytes < 0 {
		return nil, fmt.Errorf("execute step: limits must not be negative")
	}
	if limits.Timeout == 0 {
		limits.Timeout = defaultTimeout
	}
	if limits.OutputBytes == 0 {
		limits.OutputBytes = defaultMaxOutputBytes
	}
	res := &StepResult{
		ExitStatus:   -1,
		StdoutPath:   filepath.Join(runDir, stdoutRawFile),
		StderrPath:   filepath.Join(runDir, stderrRawFile),
		CombinedPath: filepath.Join(runDir, combinedRawFile),
		outputBytes:  limits.OutputBytes,
	}
	var files []*os.File
	for _, path := range []string{res.StdoutPath, res.StderrPath, res.CombinedPath} {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			for _, open := range files {
				_ = open.Close()
			}
			return nil, fmt.Errorf("execute step: create raw capture: %w", err)
		}
		files = append(files, file)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	capture := &fileCapture{combined: files[2], remaining: limits.OutputBytes, cancel: cancel}
	cmd := exec.CommandContext(ctx, step.Command[0], step.Command[1:]...)
	cmd.Dir = dir
	// A nil Env inherits the parent; even a nil input must mean no variables.
	cmd.Env = env
	if cmd.Env == nil {
		cmd.Env = []string{}
	}
	cmd.Stdout = &captureWriter{capture: capture, stream: files[0]}
	cmd.Stderr = &captureWriter{capture: capture, stream: files[1]}
	res.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	runErr := subprocess.Run(cmd, limits.Timeout)
	res.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	res.Started = cmd.Process != nil
	if cmd.ProcessState != nil {
		res.ExitStatus = cmd.ProcessState.ExitCode()
	}
	capture.mu.Lock()
	for _, file := range files {
		if err := file.Close(); err != nil && capture.err == nil {
			capture.err = err
		}
	}
	captureErr := capture.err
	capture.mu.Unlock()
	if captureErr != nil {
		runErr = captureErr
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		res.Err = fmt.Errorf("execute step: run %s in %s: %w", strings.Join(redactCommand(step.Command), " "), dir, runErr)
		if !res.Started {
			return nil, res.Err
		}
		return res, res.Err
	}
	return res, nil
}

// One lock serializes both streams, their shared budget, and combined ordering.
// Only the small os/exec copy buffers are resident during execution.
type fileCapture struct {
	mu        sync.Mutex
	combined  *os.File
	remaining int64
	cancel    context.CancelFunc
	err       error
}

type captureWriter struct {
	capture *fileCapture
	stream  *os.File
}

func (w *captureWriter) Write(p []byte) (int, error) {
	c := w.capture
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return 0, c.err
	}
	allowed := len(p)
	if int64(allowed) > c.remaining {
		allowed = int(c.remaining)
	}
	n, err := w.stream.Write(p[:allowed])
	c.remaining -= int64(n)
	if n > 0 {
		_, combinedErr := c.combined.Write(p[:n])
		if err == nil {
			err = combinedErr
		}
	}
	if err == nil && allowed < len(p) {
		err = ErrOutputLimit
	}
	if err != nil {
		c.err = err
		c.cancel()
	}
	return n, err
}

// EvaluatePredicates fails closed for incomplete execution or unreadable capture.
func EvaluatePredicates(preds []Predicate, res *StepResult) (pass bool, failed []Predicate) {
	failed = []Predicate{}
	if res == nil || res.Err != nil {
		return false, append(failed, preds...)
	}
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
		return err == nil && res.ExitStatus == want
	case "matches", "not-matches":
		re, err := regexp.Compile(pred.Value)
		if err != nil {
			return false
		}
		data, err := streamBytes(pred.Stream, res)
		if err != nil {
			return false
		}
		matched := re.Match(data)
		return matched == (pred.Form == "matches")
	default:
		return false
	}
}

func streamBytes(stream string, res *StepResult) ([]byte, error) {
	path := res.CombinedPath
	switch stream {
	case "stdout":
		path = res.StdoutPath
	case "stderr":
		path = res.StderrPath
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read live capture: %w", err)
	}
	defer file.Close()
	limit := res.outputBytes
	if limit == 0 {
		limit = defaultMaxOutputBytes
	}
	// Stat first avoids allocating an oversized capture even if a file changed
	// since execution. LimitReader also bounds growth during the read.
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, ErrOutputLimit
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	if n, err := file.Read(extra[:]); n != 0 || err != io.EOF {
		if err != nil && err != io.EOF {
			return nil, err
		}
		return nil, ErrOutputLimit
	}
	return data, nil
}
