//go:build unix

package readings

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

/*
TryLock takes an advisory lock on a file in this cache, without waiting, for work
only one process should do. It reports whether it got it, and how to let go.

Non-blocking on purpose, as internal/usage's is: a process that loses the race
reads what the winner wrote. flock is released by the kernel when the descriptor
closes, including when the process dies, so a crashed holder cannot wedge the
others.
*/
func TryLock(path string) (release func(), ok bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("making the cache directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // a path this package composed
	if err != nil {
		return nil, false, fmt.Errorf("opening the lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false, nil //nolint:nilerr // someone else has it, which is an answer and not a failure
	}
	return func() { _ = f.Close() }, true, nil
}
