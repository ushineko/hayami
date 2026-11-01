package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

/*
Spec 076. Another program shares the cache and honours a server's
Retry-After without a limit. A gate it wrote years ahead holds for at most
MaxWait past the file's writing: within it nothing goes upstream, after it
hayami fetches again.
*/
func TestAGateFromAnotherProgramHoldsForAnHourAtMost(t *testing.T) {
	dir := tempCache(t)
	now := time.Now()
	write := func(age time.Duration) {
		at := float64(now.Add(-2 * time.Minute).Unix())
		body, err := json.Marshal(usage.Entry{
			NextAttemptAt: float64(now.Add(100 * 365 * 24 * time.Hour).Unix()),
			FetchedAt:     &at, Data: payload("old"),
		})
		require.NoError(t, err)
		path := filepath.Join(dir, "usage-max.json")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(path, body, 0o600))
		require.NoError(t, os.Chtimes(path, now.Add(-age), now.Add(-age)))
	}
	calls := 0
	fetch := func() (json.RawMessage, time.Duration, error) { calls++; return payload("new"), 0, nil }

	write(10 * time.Minute)
	_, err := usage.Cached(now, 5*time.Minute, "max", usage.ProviderClaude, false, fetch)
	require.NoError(t, err)
	assert.Zero(t, calls, "inside the hour the gate holds")

	write(usage.MaxWait + time.Minute)
	got, err := usage.Cached(now, 5*time.Minute, "max", usage.ProviderClaude, false, fetch)
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "past the hour hayami fetches again")
	assert.JSONEq(t, string(payload("new")), string(got.Data))
}
