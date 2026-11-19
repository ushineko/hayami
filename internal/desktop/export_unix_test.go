//go:build !windows

package desktop

import "time"

// SetKeepEvery shortens how often the watch is checked, for a test, and
// returns what puts it back.
func SetKeepEvery(d time.Duration) (restore func()) {
	was := keepEvery
	keepEvery = d
	return func() { keepEvery = was }
}
