//go:build unix

package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

// A caller that loses the race must not queue behind someone else's request.
// It reads what is in the cache and draws that, which is the whole reason
// several panes share one file.
//
// The rival lock is taken on the real file descriptor rather than simulated:
// flock is the mechanism, and a test that faked it would pass against code
// that could not coordinate at all.
func TestACallerThatLosesTheLockReadsTheCacheRatherThanFetching(t *testing.T) {
	dir := tempCache(t)
	now := time.Now()
	at := float64(now.Add(-time.Minute).Unix())
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: float64(now.Add(-time.Second).Unix()), // the gate is open
		FetchedAt:     &at,
		Data:          payload("cached"),
	}))

	rival, err := os.OpenFile(filepath.Join(dir, "usage-max.lock"),
		os.O_CREATE|os.O_RDWR, 0o644)
	require.NoError(t, err)
	require.NoError(t, syscall.Flock(int(rival.Fd()), syscall.LOCK_EX|syscall.LOCK_NB))
	defer func() {
		_ = syscall.Flock(int(rival.Fd()), syscall.LOCK_UN)
		_ = rival.Close()
	}()

	called := false
	got, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, false,
		func() (json.RawMessage, time.Duration, error) {
			called = true
			return payload("fresh"), 0, nil
		})

	require.NoError(t, err)
	assert.False(t, called,
		"a second caller fetched while another process held the lock")
	assert.JSONEq(t, string(payload("cached")), string(got.Data))
}

// The lock is released when the call returns, or the next caller would find it
// held by a process that has finished.
func TestTheLockIsReleasedWhenTheCallReturns(t *testing.T) {
	dir := tempCache(t)
	now := time.Now()

	_, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, false,
		func() (json.RawMessage, time.Duration, error) { return payload("a"), 0, nil })
	require.NoError(t, err)

	f, err := os.OpenFile(filepath.Join(dir, "usage-max.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	assert.NoError(t, syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB),
		"the lock was still held after the call returned")
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
