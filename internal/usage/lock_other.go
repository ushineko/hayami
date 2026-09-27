//go:build !unix

package usage

// withLock runs fn without a lock.
//
// Where flock is unavailable the Python yields "acquired" and lets the gate
// coordinate on its own. That is the behaviour to copy rather than to improve:
// the two programs share the file, and a hayami that queued where the widget
// did not would be the one that looked broken.
func withLock(_, _ string, fn func(locked bool) error) error { return fn(true) }
