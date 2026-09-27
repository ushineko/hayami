//go:build unix

package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// withLock runs fn holding the account's lock, telling it whether the lock was
// taken.
//
// Non-blocking on purpose. A caller that loses the race is not made to wait
// for someone else's request: it reads what is in the cache and draws that,
// which is the whole point of a cache several panes share. The lock exists to
// stop a stampede at cold start; the gate is what stops the second request.
//
// flock is released by the kernel when the descriptor closes, including when
// the process dies, so a crashed holder cannot wedge the cache for everyone
// else.
func withLock(account, provider string, fn func(locked bool) error) error {
	path, err := LockPath(account, provider)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // a cache directory others read
		return fmt.Errorf("making the cache directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644) //nolint:gosec // the lock the other programs open the same way
	if err != nil {
		// No lock file means no coordination, not no answer. The gate still
		// works, which is what the Python falls back to as well.
		return fn(true)
	}
	defer func() { _ = f.Close() }()

	locked := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
	if locked {
		defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	}
	return fn(locked)
}
