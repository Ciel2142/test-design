package runstate

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockFile is the flock target inside the ledger directory. It is only ever
// locked, never read or written, so it holds no state of its own.
const lockFile = "ledger.lock"

// lockDir takes the flock on <dir>/ledger.lock — exclusive for a write
// transition, shared for a read — and returns the release. Callers already
// holding the lock must not call it again: a second flock from the same process
// on a second descriptor contends with the first.
func lockDir(dir string, exclusive bool) (func(), error) {
	path := filepath.Join(dir, lockFile)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}

	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	for {
		err = syscall.Flock(int(f.Fd()), how)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
