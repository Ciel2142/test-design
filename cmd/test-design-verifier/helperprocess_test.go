package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Every other proof in this package drives an op in process through
// runVerifyOp, which is enough for every question except one: what a run owes
// after the verifier itself stopped mid-op. An in-process call cannot be
// killed without taking the test with it, so the restart proofs run the op in a
// real process — this test binary, re-executed as the verifier — and interrupt
// that. Nothing here is production machinery; it exists so that "the verifier
// died" is something a test can actually do.

// helperEnvVar names the variable that turns a run of this test binary into a
// run of the verifier. It is set on the child alone, so every ordinary test
// run — including the one that spawned the child — leaves TestHelperProcess
// inert.
const helperEnvVar = "TEST_DESIGN_HELPER"

// helperPollInterval is how often the waiters below look again. A live phase
// command takes a few hundred milliseconds, so this is fine enough to catch the
// state being waited for without spending the test's time in pgrep.
const helperPollInterval = 50 * time.Millisecond

// TestHelperProcess is not a test. Under TEST_DESIGN_HELPER=1 it is the
// verifier: it runs run() over the arguments after the "--" that ends the test
// binary's own flags, with the process's real standard streams, and exits with
// the code run returned — so the child's stdout is the op's document and
// nothing else, and its exit status is the op's own. Without the variable it
// returns at once, which is what keeps it out of every other run.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvVar) != "1" {
		return
	}
	os.Exit(run(flag.Args(), os.Stdin, os.Stdout, os.Stderr))
}

// helperProcess is one verifier running beside the test: the command, and the
// two streams it wrote. Both buffers are only safe to read once the process has
// been waited for.
type helperProcess struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer

	waited   bool
	exitCode int
}

// startVerifierProcess starts the test binary as a verifier process for op with
// request on stdin, in its own process group, inheriting the test's
// environment. The environment matters as much as the arguments: isolateEnv
// puts this test's state directory and temporary root in the real environment,
// so the child addresses the same run ledger and the same run directories the
// test's in-process calls do.
func startVerifierProcess(t *testing.T, op, request string) *helperProcess {
	t.Helper()
	return startVerifierArgs(t, []string{op, "--request", "-"}, request)
}

// startVerifierArgs is the general form: args are the verifier's own arguments,
// handed to it after the "--" the test binary's flag parsing stops at, and
// stdin is what the process reads when an argument asks it to.
func startVerifierArgs(t *testing.T, args []string, stdin string) *helperProcess {
	t.Helper()
	h := &helperProcess{}
	full := append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
	h.cmd = exec.Command(os.Args[0], full...)
	h.cmd.Env = append(os.Environ(), helperEnvVar+"=1")
	h.cmd.Stdin = strings.NewReader(stdin)
	h.cmd.Stdout = &h.stdout
	h.cmd.Stderr = &h.stderr
	// Its own process group is what makes a kill reach the verifier and
	// nothing else: not the test runner that started it, and not the live
	// command it is waiting on, which subprocess.Run gave a group of its own.
	h.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A killed process cannot close pipes it no longer holds; the delay bounds
	// the wait rather than trusting every descendant to have let go of them.
	h.cmd.WaitDelay = 5 * time.Second
	if err := h.cmd.Start(); err != nil {
		t.Fatalf("start the verifier process for %v: %v", args, err)
	}
	t.Cleanup(h.stop)
	return h
}

// killAndWait sends SIGKILL to the process group and waits for exit.
func (h *helperProcess) killAndWait(t *testing.T) {
	t.Helper()
	if err := h.signalGroup(syscall.SIGKILL); err != nil {
		t.Fatalf("kill the verifier process group: %v", err)
	}
	h.wait(t)
}

// wait collects the process's exit status. A signalled or nonzero exit is the
// answer a caller asked for rather than a failure of the harness; only an error
// that left no exit status at all fails the test.
func (h *helperProcess) wait(t *testing.T) int {
	t.Helper()
	if h.waited {
		return h.exitCode
	}
	err := h.cmd.Wait()
	h.waited = true
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) && !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("wait for the verifier process: %v", err)
	}
	if h.cmd.ProcessState != nil {
		h.exitCode = h.cmd.ProcessState.ExitCode()
	}
	return h.exitCode
}

// stop ends a verifier that outlived the test that started it. It is registered
// in t.Cleanup, so a proof that fails before its own kill still leaves no
// process behind.
func (h *helperProcess) stop() {
	if h.waited {
		return
	}
	_ = h.signalGroup(syscall.SIGKILL)
	_ = h.cmd.Wait()
	h.waited = true
}

// signalGroup signals the whole process group rather than the pid: a verifier
// that had started a shell of its own would otherwise leave it running. A group
// that is already gone is the state the caller wanted, not an error.
func (h *helperProcess) signalGroup(sig syscall.Signal) error {
	if h.cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-h.cmd.Process.Pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// waitForPath polls until path exists or the deadline passes.
func waitForPath(t *testing.T, path string, deadline time.Duration) {
	t.Helper()
	if !pollUntil(deadline, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}) {
		t.Fatalf("%s did not appear within %s", path, deadline)
	}
}

// waitForNoProcessMatching polls pgrep -f pattern until it reports nothing. A
// live command the verifier started is in its own process group, so killing the
// verifier never reached it; it finishes on its own, and a test that inspected
// the run before it did would be reading a half-finished world.
func waitForNoProcessMatching(t *testing.T, pattern string, deadline time.Duration) {
	t.Helper()
	var last string
	gone := pollUntil(deadline, func() bool {
		matches, err := pgrepMatches(pattern)
		if err != nil {
			t.Fatalf("pgrep -f %q: %v", pattern, err)
		}
		last = matches
		return matches == ""
	})
	if !gone {
		t.Fatalf("a process matching %q was still running after %s: %s", pattern, deadline, last)
	}
}

// pgrepMatches returns the pids whose full command line matches pattern. pgrep
// exits 1 when nothing matched, which is the state the waiter is after and
// never an error; any other failure is one, so a missing pgrep cannot pass for
// an empty machine.
func pgrepMatches(pattern string) (string, error) {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// pollUntil is the poll both waiters share. It tests the condition before it
// sleeps, so a state that already holds costs nothing.
func pollUntil(deadline time.Duration, done func() bool) bool {
	stop := time.Now().Add(deadline)
	for {
		if done() {
			return true
		}
		if time.Now().After(stop) {
			return false
		}
		time.Sleep(helperPollInterval)
	}
}

// TestHelperProcessRunsTheVerifier proves the machinery the restart proofs
// stand on, before any of them depends on it: the child this package spawns
// really is this repository's verifier, its exit status is the op's, and its
// stdout carries the op's document alone rather than anything the testing
// framework wrote around it. preflight is the cheapest op that answers all
// three — it reads a repository, decides nothing about state, and writes a
// document only when it succeeded.
func TestHelperProcessRunsTheVerifier(t *testing.T) {
	source := filepath.Join("docs", "superpowers", "coverage", "2026-08-26-widgets-coverage.md")
	h := startVerifierArgs(t, []string{
		"preflight",
		"--repo", filepath.Join("testdata", "valid-repo"),
		"--source", source,
	}, "")

	if code := h.wait(t); code != exitOK {
		t.Fatalf("preflight through the helper exited %d, want %d; stderr=%s", code, exitOK, h.stderr.String())
	}
	var out preflightOutput
	if err := json.Unmarshal(h.stdout.Bytes(), &out); err != nil {
		t.Fatalf("helper stdout did not decode as preflight output: %v\nstdout=%s", err, h.stdout.String())
	}
	if got := out.Source; got != filepath.ToSlash(source) {
		t.Errorf("preflight source = %q, want %q: the helper must have run the op the arguments named", got, filepath.ToSlash(source))
	}
}
