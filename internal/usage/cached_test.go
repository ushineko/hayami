package usage_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/usage"
)

// tempCache points the package at a directory of this test's own. Nothing
// here writes to the real cache: it belongs to programs that are running.
func tempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testenv.Cache(t, dir)
	return filepath.Join(dir, usage.DirName)
}

func payload(s string) json.RawMessage { return json.RawMessage(`{"v":"` + s + `"}`) }

// A field this build has never heard of belongs to a program that is still
// running on the same machine. Dropping it on a write would take away a
// setting nobody could see going.
func TestAPayloadThisBuildCannotDecodeSurvivesAWrite(t *testing.T) {
	dir := tempCache(t)
	at := 1700.0
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: 2000,
		FetchedAt:     &at,
		Data:          json.RawMessage(`{"known":1,"never_heard_of":{"deep":[1,2]}}`),
	}))

	body, err := os.ReadFile(filepath.Join(dir, "usage-max.json"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "never_heard_of")
	assert.Contains(t, string(body), `"deep"`)
}

// Null is not zero. A pane shows the age of a reading, and an age of
// fifty-six years is worse than no age at all.
func TestAnEntryThatHasNeverSucceededHasNoFetchedAtRatherThanZero(t *testing.T) {
	tempCache(t)
	require.NoError(t, usage.Write("max", usage.ProviderClaude,
		usage.Entry{NextAttemptAt: 2000}))

	got, err := usage.Read("max", usage.ProviderClaude)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.FetchedAt)
	_, ok := got.Fetched()
	assert.False(t, ok)
}

// A rename, not a write in place, so no reader ever sees half a file.
func TestAWriteLeavesNoTemporaryFileBehind(t *testing.T) {
	dir := tempCache(t)
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{Data: payload("a")}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "a temporary file was left in the cache")
	}
}

// The gate is what makes five panes cost one API call. Inside it, the fetch
// function must not be called at all.
func TestInsideTheGateNothingGoesUpstream(t *testing.T) {
	tempCache(t)
	now := time.Now()
	at := float64(now.Add(-time.Minute).Unix())
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: float64(now.Add(time.Minute).Unix()),
		FetchedAt:     &at,
		Data:          payload("cached"),
	}))

	called := false
	got, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, false,
		func() (json.RawMessage, time.Duration, error) {
			called = true
			return payload("fresh"), 0, nil
		})

	require.NoError(t, err)
	assert.False(t, called, "the gate was open when it should have been shut")
	assert.JSONEq(t, string(payload("cached")), string(got.Data))
	assert.False(t, got.Fresh)
}

func TestPastTheGateOneCallerFetchesAndTheResultIsWritten(t *testing.T) {
	tempCache(t)
	now := time.Now()

	got, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, false,
		func() (json.RawMessage, time.Duration, error) { return payload("fresh"), 0, nil })

	require.NoError(t, err)
	assert.True(t, got.Fresh)
	assert.JSONEq(t, string(payload("fresh")), string(got.Data))

	entry, err := usage.Read("max", usage.ProviderClaude)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, entry.Open(now), "the gate was not set ahead after a fetch")
}

// force is for a person pressing refresh. It skips the gate and still writes,
// so the refresh never stampedes and every other pane gets the benefit.
func TestForceSkipsTheGate(t *testing.T) {
	tempCache(t)
	now := time.Now()
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: float64(now.Add(time.Hour).Unix()),
		Data:          payload("cached"),
	}))

	got, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, true,
		func() (json.RawMessage, time.Duration, error) { return payload("fresh"), 0, nil })

	require.NoError(t, err)
	assert.True(t, got.Fresh)
}

// An outage must not make every pane flap to an error. The last good reading
// stays, its age stays, and only the gate moves.
func TestAFailedFetchKeepsTheLastGoodReadingAndMovesTheGate(t *testing.T) {
	tempCache(t)
	now := time.Now()
	at := float64(now.Add(-time.Hour).Unix())
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: float64(now.Add(-time.Minute).Unix()),
		FetchedAt:     &at,
		Data:          payload("good"),
	}))

	got, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, false,
		func() (json.RawMessage, time.Duration, error) {
			return nil, 0, errors.New("the server said no")
		})

	require.Error(t, err)
	assert.JSONEq(t, string(payload("good")), string(got.Data), "an outage clobbered good data")

	entry, err := usage.Read("max", usage.ProviderClaude)
	require.NoError(t, err)
	assert.JSONEq(t, string(payload("good")), string(entry.Data))
	require.NotNil(t, entry.FetchedAt)
	assert.InDelta(t, at, *entry.FetchedAt, 0.001, "the age of the reading changed although nothing was read")
	assert.False(t, entry.Open(now), "the gate did not move after a failure")
}

// A server that says how long to wait is obeyed, even when it asks for longer
// than the ttl.
func TestARetryAfterLongerThanTheTtlWins(t *testing.T) {
	tempCache(t)
	now := time.Now()

	_, err := usage.Cached(now, time.Minute, "max", usage.ProviderClaude, false,
		func() (json.RawMessage, time.Duration, error) {
			return nil, time.Hour, errors.New("too many requests")
		})
	require.Error(t, err)

	entry, err := usage.Read("max", usage.ProviderClaude)
	require.NoError(t, err)
	assert.True(t, entry.Open(now.Add(61*time.Minute)))
	assert.False(t, entry.Open(now.Add(59*time.Minute)), "the retry-after was ignored")
}

// A cache is a cache. A corrupt one costs one extra request; a panel that
// refused to draw because of it would cost the whole reading.
func TestACorruptCacheFileIsTreatedAsNoCache(t *testing.T) {
	dir := tempCache(t)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"),
		[]byte("{not json"), 0o600))

	got, err := usage.Read("max", usage.ProviderClaude)

	require.NoError(t, err)
	assert.Nil(t, got)
}
