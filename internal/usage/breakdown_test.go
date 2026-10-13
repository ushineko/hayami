package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

// Every number and name in these fixtures is invented, as payload_test.go's
// are.
const claudeFull = `{
  "five_hour": {"utilization": 12.5, "resets_at": "2026-09-27T11:00:00+00:00"},
  "limits": [
    {"kind": "session", "group": "session", "percent": 12, "severity": "normal",
     "resets_at": "2026-09-27T11:00:00+00:00", "scope": null, "is_active": false},
    {"kind": "weekly_all", "group": "weekly", "percent": 40, "severity": "warning",
     "resets_at": "2026-10-01T16:00:00+00:00", "scope": null, "is_active": true},
    {"kind": "weekly_scoped", "group": "weekly", "percent": 7, "severity": "normal",
     "resets_at": "2026-10-01T16:00:00+00:00",
     "scope": {"model": {"id": null, "display_name": "Invented"}, "surface": null}, "is_active": false},
    {"kind": "something_new", "percent": 3, "severity": "normal", "resets_at": ""}
  ],
  "seven_day_breakdown": {
    "as_of": "2026-09-27T10:00:00+00:00",
    "rows": [
      {"key": "claude_code", "display_name": "Claude Code", "percent": 75},
      {"key": "chat", "display_name": "Chats", "percent": 20},
      {"key": "nameless", "percent": 5}
    ]
  },
  "extra_usage": {"is_enabled": true, "monthly_limit": 5000, "used_credits": 1250,
                  "currency": "USD", "decimal_places": 2},
  "spend": {"enabled": false, "percent": 0,
            "used": {"amount_minor": 4200, "currency": "USD", "exponent": 2},
            "limit": {"amount_minor": 100000, "currency": "USD", "exponent": 2}}
}`

// Spec 060 R13. Claude's breakdown is the limits the payload lists, what the
// week went on, extra usage and the spend.
func TestClaudesBreakdownIsItsLimitsItsWeekAndItsExtras(t *testing.T) {
	b, err := usage.ClaudeBreakdown(time.Now(), json.RawMessage(claudeFull))
	require.NoError(t, err)

	require.Len(t, b.Limits, 4)
	assert.Equal(t, "Session", b.Limits[0].Name)
	assert.InDelta(t, 0.12, b.Limits[0].Fraction, 1e-9)
	assert.Equal(t, time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC), b.Limits[0].ResetsAt.UTC())
	assert.Equal(t, "Week", b.Limits[1].Name)
	assert.Equal(t, "warning", b.Limits[1].Severity)
	assert.Equal(t, "Week · Invented", b.Limits[2].Name, "a scoped limit names its scope")
	assert.Equal(t, "something new", b.Limits[3].Name, "a kind this build has no word for appears as written")
	assert.True(t, b.Limits[3].ResetsAt.IsZero())

	assert.Equal(t, "Week by use", b.SharesTitle)
	assert.Equal(t, []usage.Share{
		{Name: "Claude Code", Fraction: 0.75}, {Name: "Chats", Fraction: 0.2}, {Name: "nameless", Fraction: 0.05},
	}, b.Shares)

	assert.Equal(t, []usage.Fact{
		{Name: "Extra usage", Value: "$12.50 of $50.00"},
		{Name: "Spend", Value: "$42.00 of $1000.00"},
	}, b.Facts)
	assert.Empty(t, b.Plan)
}

// R14. Each key absent or null leaves its lines out and nothing else: every
// one of the four, taken away in turn, costs only its own part.
func TestEachMissingClaudeKeyLeavesOnlyItsOwnLinesOut(t *testing.T) {
	full, err := usage.ClaudeBreakdown(time.Now(), json.RawMessage(claudeFull))
	require.NoError(t, err)

	without := func(key string, null bool) usage.Breakdown {
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(claudeFull), &m))
		if null {
			m[key] = json.RawMessage("null")
		} else {
			delete(m, key)
		}
		raw, err := json.Marshal(m)
		require.NoError(t, err)
		b, err := usage.ClaudeBreakdown(time.Now(), raw)
		require.NoError(t, err, key)
		return b
	}

	for _, null := range []bool{false, true} {
		b := without("limits", null)
		assert.Empty(t, b.Limits)
		assert.Equal(t, full.Shares, b.Shares)
		assert.Equal(t, full.Facts, b.Facts)

		b = without("seven_day_breakdown", null)
		assert.Empty(t, b.Shares)
		assert.Empty(t, b.SharesTitle, "no heading over no lines")
		assert.Equal(t, full.Limits, b.Limits)

		b = without("extra_usage", null)
		assert.Equal(t, []usage.Fact{{Name: "Spend", Value: "$42.00 of $1000.00"}}, b.Facts)

		b = without("spend", null)
		assert.Equal(t, []usage.Fact{{Name: "Extra usage", Value: "$12.50 of $50.00"}}, b.Facts)
	}
}

// R14. A key whose shape has changed costs its own lines and not the rest,
// and a null inside a part is a line left out.
func TestAKeyThatChangedShapeCostsOnlyItself(t *testing.T) {
	b, err := usage.ClaudeBreakdown(time.Now(), json.RawMessage(`{
	  "limits": "not a list any more",
	  "seven_day_breakdown": {"rows": [{"key": "chat", "display_name": "Chats", "percent": null},
	                                   {"key": "other", "display_name": "Other", "percent": 100}]},
	  "extra_usage": {"is_enabled": false, "monthly_limit": {"now": "an object"}},
	  "spend": {"used": null}
	}`))

	require.NoError(t, err)
	assert.Empty(t, b.Limits)
	assert.Equal(t, []usage.Share{{Name: "Other", Fraction: 1}}, b.Shares)
	assert.Equal(t, []usage.Fact{{Name: "Extra usage", Value: "off"}}, b.Facts)
}

// Extra usage is off, on with nothing spent to report, or what was spent,
// with the limit where there is one.
func TestExtraUsageSaysWhatItKnows(t *testing.T) {
	extra := func(body string) string {
		b, err := usage.ClaudeBreakdown(time.Now(), json.RawMessage(`{"extra_usage": `+body+`}`))
		require.NoError(t, err)
		require.Len(t, b.Facts, 1)
		return b.Facts[0].Value
	}

	assert.Equal(t, "off", extra(`{"is_enabled": false, "used_credits": 900}`))
	assert.Equal(t, "on", extra(`{"is_enabled": true, "used_credits": null}`))
	assert.Equal(t, "$9.00", extra(`{"is_enabled": true, "used_credits": 900, "currency": "USD", "decimal_places": 2}`))
	assert.Equal(t, "9.00 EUR of 20.00 EUR",
		extra(`{"is_enabled": true, "used_credits": "900", "monthly_limit": 2000, "currency": "EUR", "decimal_places": 2}`))
}

const codexFull = `{
  "provider": "codex",
  "plan_type": "business",
  "primary":   {"utilization": 30.0, "window_minutes": 300, "resets_at": 1790000000},
  "secondary": {"utilization": 55.5, "window_minutes": 10080, "resets_at": 1790500000},
  "individual_limit": {"utilization": 25.0, "remaining_percent": 75.0, "resets_at": 1791000000,
                       "used": "300.50", "limit": "1200.00"},
  "credits": {"hasCredits": true, "unlimited": false, "balance": "42.00"},
  "rate_limit_reached_type": null
}`

// R13. Codex's breakdown is the plan, both windows, the individual limit
// with what remains, and the credits.
func TestCodexsBreakdownIsItsPlanItsWindowsItsLimitAndItsCredits(t *testing.T) {
	b, err := usage.CodexBreakdown(time.Now(), json.RawMessage(codexFull))
	require.NoError(t, err)

	assert.Equal(t, "business", b.Plan)
	require.Len(t, b.Limits, 3)
	assert.Equal(t, "5h", b.Limits[0].Name)
	assert.InDelta(t, 0.30, b.Limits[0].Fraction, 1e-9)
	assert.Equal(t, time.Unix(1790000000, 0), b.Limits[0].ResetsAt)
	assert.Equal(t, "7d", b.Limits[1].Name)
	assert.Equal(t, usage.Limit{Name: "Individual", Fraction: 0.25, ResetsAt: time.Unix(1791000000, 0), Amount: "300.5 / 1200"}, b.Limits[2])
	assert.Equal(t, []usage.Fact{{Name: "Remaining", Value: "75 %"}, {Name: "Credits", Value: "42.00"}}, b.Facts)
	assert.Empty(t, b.Shares)
}

// R14. Each Codex key absent or null leaves its lines out.
func TestEachMissingCodexKeyLeavesItsLinesOut(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		limits        int
		plan          string
		facts         []usage.Fact
	}{
		{"nothing", `{}`, 0, "", nil},
		{"nulls", `{"plan_type": null, "primary": null, "secondary": null, "individual_limit": null, "credits": null}`, 0, "", nil},
		{"one window", `{"primary": {"utilization": 5, "window_minutes": 300, "resets_at": 0}}`, 1, "", nil},
		{"a limit with no figure", `{"individual_limit": {"utilization": null, "used": "1", "limit": "2"}}`, 0, "", nil},
		{"unlimited", `{"credits": {"unlimited": true}, "plan_type": "pro"}`, 0, "pro", []usage.Fact{{Name: "Credits", Value: "unlimited"}}},
		{"no credits", `{"credits": {"hasCredits": false, "balance": null}}`, 0, "", []usage.Fact{{Name: "Credits", Value: "none"}}},
		{"credits of a shape unknown", `{"credits": [1, 2]}`, 0, "", nil},
	} {
		b, err := usage.CodexBreakdown(time.Now(), json.RawMessage(tc.payload))
		require.NoError(t, err, tc.name)
		assert.Len(t, b.Limits, tc.limits, tc.name)
		assert.Equal(t, tc.plan, b.Plan, tc.name)
		assert.Equal(t, tc.facts, b.Facts, tc.name)
	}
}

// An empty payload is an empty breakdown; one that is not an object at all
// is an error, as it is for the windows.
func TestAPayloadThatIsNotAnObjectIsAnError(t *testing.T) {
	for _, decode := range []func(time.Time, json.RawMessage) (usage.Breakdown, error){usage.ClaudeBreakdown, usage.CodexBreakdown} {
		b, err := decode(time.Now(), nil)
		require.NoError(t, err)
		assert.Empty(t, b.Limits)

		_, err = decode(time.Now(), json.RawMessage(`[1, 2, 3]`))
		assert.Error(t, err)
	}
}

// Every provider in the table has a breakdown decoder.
func TestEveryProviderHasABreakdown(t *testing.T) {
	for _, p := range usage.Providers() {
		assert.NotNil(t, p.Breakdown, p.Name)
	}
}

// The canary for the breakdown's keys, beside the one for the entry's
// format: every real cache file on this machine decodes to a breakdown
// without an error. It reads and never writes, and prints no figure.
func TestTheRealCacheFilesDecodeToABreakdown(t *testing.T) {
	dir, err := usage.Dir()
	require.NoError(t, err)
	accounts, err := usage.Accounts()
	if err != nil || len(accounts) == 0 {
		t.Skip("no shared cache on this machine; nothing to check the keys against")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no shared cache on this machine; nothing to check the keys against")
	}

	checked := 0
	for _, a := range accounts {
		entry, err := usage.Read(a.Name, a.Provider)
		require.NoError(t, err)
		if entry == nil || len(entry.Data) == 0 {
			continue
		}
		b, err := usage.Lookup(a.Provider).Breakdown(time.Now(), usage.Served(*entry).Data)
		require.NoError(t, err, "the %s payload no longer decodes to a breakdown", a.Provider)
		for _, l := range b.Limits {
			assert.NotEmpty(t, strings.TrimSpace(l.Name))
			assert.GreaterOrEqual(t, l.Fraction, 0.0)
		}
		checked++
	}
	if checked == 0 {
		t.Skip("the shared cache holds no payload yet")
	}
	t.Logf("decoded %d payloads under %s", checked, filepath.Base(dir))
}
