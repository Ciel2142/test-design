package subprocess

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRunPreservesOrdinaryExitError(t *testing.T) {
	err := Run(exec.Command("/bin/sh", "-c", "exit 7"), time.Second)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("Run() = %v, want exit status 7", err)
	}
}

func TestRunTimeoutTerminatesDescendants(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	cmd := exec.Command("/bin/sh", "-c", "(trap '' TERM; sleep 1; echo survived > \"$1\") & echo ready; wait", "sh", marker)
	var output bytes.Buffer
	cmd.Stdout = &output
	started := time.Now()
	err := Run(cmd, 200*time.Millisecond)
	if !errors.Is(err, ErrTimeout) || time.Since(started) > 3*time.Second {
		t.Fatalf("Run() = %v after %v, want bounded timeout", err, time.Since(started))
	}
	if output.String() != "ready\n" {
		t.Fatalf("capture = %q, want startup evidence", output.String())
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived timeout: stat = %v", err)
	}
}

func TestRunBoundsInheritedPipeWait(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 20 & exit 0")
	var output bytes.Buffer
	cmd.Stdout = &output
	started := time.Now()
	err := Run(cmd, 2*time.Second)
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("Run() = %v after %v, want bounded inherited-pipe error", err, time.Since(started))
	}
}

func TestRunContextCancellationIsNotOrdinaryExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Run(exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 20"), 3*time.Second)
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) || !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Run() = %v, want cancellation distinct from exit failure", err)
	}
}

func TestRunSignalReturnsControlForCleanup(t *testing.T) {
	if os.Getenv("TD_SUBPROCESS_SIGNAL_CHILD") == "1" {
		err := Run(exec.Command("/bin/sh", "-c", "kill -TERM $PPID; sleep 20"), 3*time.Second)
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("Run() = %v, want interruption", err)
		}
		if err := Run(exec.Command("/bin/sh", "-c", "exit 0"), time.Second); err != nil {
			t.Fatalf("independent cleanup could not run: %v", err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunSignalReturnsControlForCleanup$")
	cmd.Env = append(os.Environ(), "TD_SUBPROCESS_SIGNAL_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("signal child: %v\n%s", err, output)
	}
}
