package codex_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/codex"
)

// The app-server's own shape, with invented numbers and an account identifier
// of the kind the reply carries.
const reply = `{
  "rateLimitsByLimitId": {
    "codex": {
      "primary":   {"usedPercent": 25.0, "windowDurationMins": 300,   "resetsAt": 1790508605},
      "secondary": {"usedPercent": 60.0, "windowDurationMins": 10080, "resetsAt": 1791095405},
      "planType": "team",
      "individualLimit": {"remainingPercent": 66.0, "resetsAt": 1790812800, "used": "400.5", "limit": "1200"},
      "accountId": "an-account-identifier-that-should-not-be-cached"
    }
  }
}`

// The cache is shared with two Python programs and they write this shape, not
// the app-server's. Caching the raw reply would leave the widget unable to
// read what hayami wrote.
func TestTheReplyIsReshapedIntoWhatTheCacheHolds(t *testing.T) {
	got, err := codex.Normalise(json.RawMessage(reply))

	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(got, &out))

	assert.Equal(t, "codex", out["provider"])
	primary, ok := out["primary"].(map[string]any)
	require.True(t, ok, "the cache's key is primary, in snake case")
	assert.InDelta(t, 25.0, primary["utilization"], 0.001)
	assert.InDelta(t, 300.0, primary["window_minutes"], 0.001)
	assert.InDelta(t, 1790508605.0, primary["resets_at"], 1)
}

// remainingPercent is what the app-server reports and utilization is what the
// cache holds. Getting the subtraction the wrong way round would draw a busy
// account as an idle one.
func TestAnIndividualLimitIsTurnedFromRemainingIntoUsed(t *testing.T) {
	got, err := codex.Normalise(json.RawMessage(reply))
	require.NoError(t, err)

	var out struct {
		Individual struct {
			Utilization      float64 `json:"utilization"`
			RemainingPercent float64 `json:"remaining_percent"`
			Used             string  `json:"used"`
		} `json:"individual_limit"`
	}
	require.NoError(t, json.Unmarshal(got, &out))

	assert.InDelta(t, 34.0, out.Individual.Utilization, 0.001, "sixty-six remaining is thirty-four used")
	assert.InDelta(t, 66.0, out.Individual.RemainingPercent, 0.001)
	assert.Equal(t, "400.5", out.Individual.Used, "the amounts are passed through as the strings they are")
}

// Nothing that identifies an account goes into a file in a cache directory.
func TestNoAccountIdentifierReachesTheCache(t *testing.T) {
	got, err := codex.Normalise(json.RawMessage(reply))

	require.NoError(t, err)
	assert.NotContains(t, string(got), "accountId")
	assert.NotContains(t, string(got), "an-account-identifier-that-should-not-be-cached")
}

// An older app-server reports the limits without the by-id map.
func TestAReplyWithoutTheByIdMapIsStillRead(t *testing.T) {
	got, err := codex.Normalise(json.RawMessage(
		`{"rateLimits": {"primary": {"usedPercent": 10, "windowDurationMins": 300}}}`))

	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(got, &out))
	assert.NotNil(t, out["primary"])
}

// A window the app-server said nothing about is absent rather than nought.
func TestAWindowWithNoFiguresIsAbsent(t *testing.T) {
	got, err := codex.Normalise(json.RawMessage(`{"rateLimits": {"primary": {}}}`))

	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(got, &out))
	assert.Nil(t, out["primary"])
}

func TestAPercentageOutsideItsRangeIsClamped(t *testing.T) {
	got, err := codex.Normalise(json.RawMessage(
		`{"rateLimits": {"primary": {"usedPercent": 140, "windowDurationMins": 300}}}`))

	require.NoError(t, err)
	var out struct {
		Primary struct {
			Utilization float64 `json:"utilization"`
		} `json:"primary"`
	}
	require.NoError(t, json.Unmarshal(got, &out))
	assert.InDelta(t, 100.0, out.Primary.Utilization, 0.001)
}
