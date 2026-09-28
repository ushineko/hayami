package usage_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

// Every number in these fixtures is invented. A usage figure is a fact about
// a person's account and does not belong in a repository, which
// .claude/CLAUDE.md says in as many words.
const claudeWindows = `{
  "five_hour": {"utilization": 12.5, "resets_at": "2026-09-27T11:00:00.029840+00:00"},
  "seven_day": {"utilization": 40.0, "resets_at": "2026-10-01T16:00:00+00:00"},
  "spend": {"enabled": false, "percent": 0, "used": {"amount_minor": 0, "currency": "USD", "exponent": 2}},
  "never_heard_of": {"deep": [1, 2, 3]},
  "iguana_necktie": null
}`

const claudeSpend = `{
  "five_hour": null,
  "seven_day": null,
  "spend": {
    "enabled": true, "percent": 42, "severity": "normal",
    "used":  {"amount_minor": 4200, "currency": "USD", "exponent": 2},
    "limit": {"amount_minor": 10000, "currency": "USD", "exponent": 2}
  }
}`

const codexPayload = `{
  "provider": "codex",
  "primary":   {"utilization": 25.0, "window_minutes": 300,   "resets_at": 1790508605},
  "secondary": {"utilization": 60.0, "window_minutes": 10080, "resets_at": 1791095405},
  "individual_limit": {"utilization": 33.0, "resets_at": 1790812800, "used": "400.5", "limit": "1200"}
}`

func TestAClaudePayloadDecodesToItsTwoWindows(t *testing.T) {
	got, err := usage.Claude(time.Now(), json.RawMessage(claudeWindows))

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "5h", got[0].Name)
	assert.InDelta(t, 0.125, got[0].Fraction, 0.0001, "a percentage became a fraction")
	assert.Equal(t, 2026, got[0].ResetsAt.Year())
	assert.Equal(t, "7d", got[1].Name)
}

// The payload carries thirty keys this program has never heard of, several
// with plainly internal names. Decoding two of them must not be disturbed by
// the rest.
func TestAnUnknownKeyDoesNotBreakTheClaudePayload(t *testing.T) {
	got, err := usage.Claude(time.Now(), json.RawMessage(claudeWindows))

	require.NoError(t, err)
	assert.Len(t, got, 2)
}

// A Team account reports no windows at all and a budget instead. Both shapes
// arrive in the same field, and whichever is present is what is drawn.
func TestAnAccountOnABudgetReportsItsSpendInstead(t *testing.T) {
	now := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)

	got, err := usage.Claude(now, json.RawMessage(claudeSpend))

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "spend", got[0].Name)
	assert.InDelta(t, 0.42, got[0].Fraction, 0.0001)
	assert.Equal(t, "$42.00 / $100.00", got[0].Detail,
		"this payload declares its currency, so the symbol reports what the source said")
	assert.Equal(t, time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC), got[0].ResetsAt,
		"the credit cap has no reset in the payload; the official screen derives first-of-next-month")
}

// A spend that is switched off is not a spend of nought. An account with
// credits disabled has no budget, and drawing an empty bar would invent one.
func TestASpendThatIsDisabledIsNotDrawn(t *testing.T) {
	got, err := usage.Claude(time.Now(), json.RawMessage(claudeWindows))

	require.NoError(t, err)
	for _, w := range got {
		assert.NotEqual(t, "spend", w.Name)
	}
}

func TestACodexPayloadDecodesItsWindowsAndItsLimit(t *testing.T) {
	got, err := usage.Codex(json.RawMessage(codexPayload))

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "5h", got[0].Name, "three hundred minutes is five hours")
	assert.Equal(t, "7d", got[1].Name, "ten thousand and eighty minutes is seven days")
	assert.False(t, got[0].ResetsAt.IsZero(), "an epoch reset should read as a time")
}

// The app-server does not declare these units as currency. A panel that
// printed a dollar sign would be asserting something the source never said,
// and the program this one replaces makes the same point in its README.
func TestTheCodexIndividualLimitCarriesNoCurrencySymbol(t *testing.T) {
	got, err := usage.Codex(json.RawMessage(codexPayload))

	require.NoError(t, err)
	limit := got[len(got)-1]
	assert.Equal(t, "limit", limit.Name)
	assert.Equal(t, "400.50 / 1200.00", limit.Detail)
	assert.NotContains(t, limit.Detail, "$")
}

// The window the bar is about is picked by length, so a length that does not
// arrive is a bar about the wrong window. A name is not a length: "5h" and
// "weekly" only sort if something knows what they mean, and this is where
// that knowledge is put on the value.
func TestAWindowCarriesItsLengthAndAnAllowanceDoesNot(t *testing.T) {
	claude, err := usage.Claude(time.Now(), json.RawMessage(claudeWindows))
	require.NoError(t, err)
	require.Len(t, claude, 2)
	assert.Equal(t, 5*time.Hour, claude[0].Span)
	assert.Equal(t, 7*24*time.Hour, claude[1].Span)

	codex, err := usage.Codex(json.RawMessage(codexPayload))
	require.NoError(t, err)
	require.Len(t, codex, 3)
	assert.Equal(t, 5*time.Hour, codex[0].Span, "three hundred minutes is five hours")
	assert.Equal(t, 7*24*time.Hour, codex[1].Span)
	assert.Zero(t, codex[2].Span,
		"a Business limit states no period, and a guessed one would put it on the bar")
}

func TestASpendStatesNoPeriodEither(t *testing.T) {
	got, err := usage.Claude(time.Now(), json.RawMessage(claudeSpend))

	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Zero(t, got[len(got)-1].Span)
}

func TestDecemberRollsOverIntoTheNextYear(t *testing.T) {
	now := time.Date(2026, time.December, 14, 9, 0, 0, 0, time.UTC)

	assert.Equal(t, time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC), usage.NextMonth(now))
}

func TestAnEmptyPayloadIsNoWindowsAndNoError(t *testing.T) {
	got, err := usage.Claude(time.Now(), nil)

	require.NoError(t, err)
	assert.Empty(t, got)
}
