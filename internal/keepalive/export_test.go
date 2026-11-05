package keepalive

import (
	"context"
	"testing"
)

// AnswerSystemctl has f answer for the service manager until the test ends.
func AnswerSystemctl(t *testing.T, f func(context.Context, ...string) error) {
	t.Helper()
	wasManager, wasRunning := systemctl, running
	systemctl = f
	running = func(context.Context) error { return nil }
	t.Cleanup(func() { systemctl, running = wasManager, wasRunning })
}

// AnswerRunning has err answer for whether a helper was seen to start.
func AnswerRunning(t *testing.T, err error) {
	t.Helper()
	was := running
	running = func(context.Context) error { return err }
	t.Cleanup(func() { running = was })
}
