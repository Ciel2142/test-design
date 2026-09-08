// Package subprocess runs finite commands without leaving process groups behind
// when a deadline, cancellation, or termination signal interrupts verification.
package subprocess

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	SuiteTimeout   = 30 * time.Minute
	UtilityTimeout = 30 * time.Second
	pipeWait       = 250 * time.Millisecond
)

var (
	ErrTimeout     = errors.New("subprocess timeout")
	ErrInterrupted = errors.New("subprocess interrupted")
)

// Run preserves ordinary *exec.ExitError results. Timeout, context cancellation,
// inherited-pipe leaks, and SIGINT/SIGTERM instead return infrastructure errors
// that cannot be mistaken for an expected nonzero test or live-step result.
// Each invocation owns its signal subscription, so cleanup can run independently
// after an interrupted command has returned.
func Run(cmd *exec.Cmd, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("subprocess: timeout must be positive")
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pgid = 0
	cmd.WaitDelay = pipeWait
	var cancelled atomic.Bool
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			cancelled.Store(true)
			return killGroup(cmd)
		}
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var interrupted error
	select {
	case err := <-done:
		if cancelled.Load() {
			return ErrInterrupted
		}
		if errors.Is(err, exec.ErrWaitDelay) {
			_ = killGroup(cmd)
			return fmt.Errorf("%w: inherited output pipes remained open", ErrInterrupted)
		}
		return err
	case <-timer.C:
		interrupted = ErrTimeout
	case sig := <-signals:
		interrupted = fmt.Errorf("%w: %s", ErrInterrupted, sig)
	}
	killErr := killGroup(cmd)
	// SIGKILL normally makes Wait return immediately. Bound even an abnormal
	// kernel wait; WaitDelay separately bounds inherited output-pipe drains.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		return fmt.Errorf("%w: process wait did not finish after termination", interrupted)
	}
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return fmt.Errorf("%w: terminate process group: %v", interrupted, killErr)
	}
	return interrupted
}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
