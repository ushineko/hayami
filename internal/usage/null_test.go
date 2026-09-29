package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

/*
A fetch that has never succeeded leaves `"data": null`, and that is not a
reading.

json.RawMessage keeps those four bytes rather than nil, and they decode into
zero usage windows and **no error** -- so an account that has never fetched
looked exactly like an account that is fine and has nothing to report. A
machine sat with an empty Usage section for thirty-five minutes with nothing
anywhere saying why (issue #54).
*/
func TestACachedNullIsNotAReading(t *testing.T) {
	dir := tempCache(t)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := `{"next_attempt_at":1790707553.88,"fetched_at":null,"data":null}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"), []byte(body), 0o600))

	e, err := usage.Read("max", usage.ProviderClaude)

	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Empty(t, e.Data, "the literal null survived as a payload")

	_, ok := e.Fetched()
	assert.False(t, ok, "an entry that never fetched must not claim a time")
}

// And it does not survive a round trip either, however it got into the file:
// a caller that writes the literal gets back no payload, not four bytes of
// one.
func TestANullPayloadDoesNotSurviveARoundTrip(t *testing.T) {
	tempCache(t)
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: 1,
		Data:          json.RawMessage("null"),
	}))

	back, err := usage.Read("max", usage.ProviderClaude)

	require.NoError(t, err)
	require.NotNil(t, back)
	assert.Empty(t, back.Data)
}

// A real payload is untouched: the cache file is three programs' file and its
// shape is not this build's to change.
func TestARealPayloadSurvivesUnchanged(t *testing.T) {
	dir := tempCache(t)
	payload := `{"five_hour":{"utilization":5}}`
	require.NoError(t, usage.Write("max", usage.ProviderClaude, usage.Entry{
		NextAttemptAt: 1, Data: json.RawMessage(payload),
	}))

	raw, err := os.ReadFile(filepath.Join(dir, "usage-max.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"five_hour"`)
	assert.Contains(t, string(raw), `"next_attempt_at"`)
	assert.Contains(t, string(raw), `"fetched_at"`)
}

// The gate is readable as a time, so a panel that is waiting can say until
// when rather than drawing a blank for however long the backoff runs.
func TestTheGateIsReadableAsATime(t *testing.T) {
	e := usage.Entry{NextAttemptAt: 1790707553}

	assert.Equal(t, int64(1790707553), e.NextAttempt().Unix())
}
