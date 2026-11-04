package panel_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/usage"
)

// An Ultra account's credential store still says "max", which is also the
// default store's name and its cache file's. Claude Code's account record
// names the plan; the panel calls the account by it, and the cache file keeps
// the name the other programs share.
func TestTheDefaultStoreIsCalledByItsRecordedPlan(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	testenv.Home(t, home)
	testenv.Cache(t, filepath.Join(root, "cache"))
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", filepath.Join(root, "profiles"))
	t.Setenv("PATH", "") // no codex to ask

	const tier = "default_claude_max_20x"
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))
	creds := fmt.Sprintf(`{"claudeAiOauth": {"accessToken": "invented", "refreshToken": "invented",
  "expiresAt": %d, "subscriptionType": "max", "rateLimitTier": %q}}`, time.Now().Add(24*time.Hour).UnixMilli(), tier)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(creds), 0o600))
	record := fmt.Sprintf(`{"oauthAccount": {"organizationRateLimitTier": %q, "planDisplayName": "Ultra"}}`, tier)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte(record), 0o600))

	// No five-hour window, which is what an Ultra account reports. The gate is
	// closed, so nothing is fetched.
	path, err := usage.Path("max", usage.ProviderClaude)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	resets := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"next_attempt_at": %d, "fetched_at": %d, "data": {"five_hour": null,
  "seven_day": {"utilization": 40, "resets_at": %q}}}`,
		time.Now().Add(time.Hour).Unix(), time.Now().Add(-time.Minute).Unix(), resets)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	u := panel.NewUsage()
	drawn, err := u.Poll(t.Context())
	require.NoError(t, err)
	require.True(t, drawn)

	meters := u.Section().Meters
	require.NotEmpty(t, meters)
	for _, m := range meters {
		assert.Equal(t, "CC ultra", m.Label)
		assert.Equal(t, "U", m.Badge)
	}

	// The drawer finds the account by the label its meter carries.
	_, err = u.PollDetail(t.Context(), "CC ultra")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "ultra", "the cache keeps the name the other programs read")
	}
}

// planDesk is a machine with credential stores, an account record and usage
// readings, all invented, whose gates are closed so nothing is fetched.
type planDesk struct {
	t        *testing.T
	home     string
	profiles string
}

func newPlanDesk(t *testing.T) planDesk {
	t.Helper()
	root := t.TempDir()
	d := planDesk{t: t, home: filepath.Join(root, "home"), profiles: filepath.Join(root, "profiles")}
	testenv.Home(t, d.home)
	testenv.Cache(t, filepath.Join(root, "cache"))
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", d.profiles)
	t.Setenv("PATH", "") // no codex to ask
	require.NoError(t, os.MkdirAll(d.home, 0o700))
	return d
}

// store writes a credential store: the default one for "max", a profile
// otherwise.
func (d planDesk) store(name, subscription string) {
	d.t.Helper()
	dir := filepath.Join(d.profiles, name)
	if name == "max" {
		dir = filepath.Join(d.home, ".claude")
	}
	require.NoError(d.t, os.MkdirAll(dir, 0o700))
	creds := fmt.Sprintf(`{"claudeAiOauth": {"accessToken": "invented", "refreshToken": "invented",
  "expiresAt": %d, "subscriptionType": %q, "rateLimitTier": %q}}`,
		time.Now().Add(24*time.Hour).UnixMilli(), subscription, planTier)
	require.NoError(d.t, os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600))
}

// record writes the account record, which one account at a time is described
// by.
func (d planDesk) record(tier, plan string) {
	d.t.Helper()
	body := fmt.Sprintf(`{"oauthAccount": {"organizationRateLimitTier": %q, "planDisplayName": %q}}`, tier, plan)
	require.NoError(d.t, os.WriteFile(filepath.Join(d.home, ".claude.json"), []byte(body), 0o600))
}

// reading writes an account's cached payload behind a closed gate.
func (d planDesk) reading(name, data string) {
	d.t.Helper()
	path, err := usage.Path(name, usage.ProviderClaude)
	require.NoError(d.t, err)
	require.NoError(d.t, os.MkdirAll(filepath.Dir(path), 0o700))
	body := fmt.Sprintf(`{"next_attempt_at": %d, "fetched_at": %d, "data": %s}`,
		time.Now().Add(time.Hour).Unix(), time.Now().Add(-time.Minute).Unix(), data)
	require.NoError(d.t, os.WriteFile(path, []byte(body), 0o600))
}

// drawn is the caption and badge a new panel gives its first meter.
func (d planDesk) drawn() (label, badge string) {
	d.t.Helper()
	u := panel.NewUsage()
	ok, err := u.Poll(d.t.Context())
	require.NoError(d.t, err)
	require.True(d.t, ok)
	meters := u.Section().Meters
	require.NotEmpty(d.t, meters)
	return meters[0].Label, meters[0].Badge
}

const (
	planTier    = "default_claude_max_20x"
	otherTier   = "default_enterprise_tier"
	weekOnly    = `{"five_hour": null, "seven_day": {"utilization": 40, "resets_at": "2026-12-01T00:00:00Z"}}`
	bothWindows = `{"five_hour": {"utilization": 10, "resets_at": "2026-12-01T00:00:00Z"},
  "seven_day": {"utilization": 40, "resets_at": "2026-12-01T00:00:00Z"}}`
	spendOnly = `{"five_hour": null, "seven_day": null, "spend": {"enabled": true, "percent": 42,
  "severity": "normal", "used": {"amount_minor": 4200, "currency": "USD", "exponent": 2}}}`
)

// What a store is called when the record does not settle it comes from its own
// reading, and what the record names, when it vouches, wins either way (#288).
func TestAStoresPlanComesFromTheRecordThenItsReading(t *testing.T) {
	for _, tc := range []struct {
		name         string
		recordTier   string
		recordPlan   string
		reading      string
		label, badge string
	}{
		{"another account's record, no five-hour window", otherTier, "", weekOnly, "CC ultra", "U"},
		{"another account's record, both windows", otherTier, "", bothWindows, "CC max", "M"},
		{"another account's record naming its plan", otherTier, "Ultra", bothWindows, "CC max", "M"},
		{"the record names Ultra, both windows", planTier, "Ultra", bothWindows, "CC ultra", "U"},
		{"the record names Max, no five-hour window", planTier, "Max", weekOnly, "CC max", "M"},
		{"the record vouches and names nothing", planTier, "", weekOnly, "CC ultra", "U"},
		{"a budget and no windows is no sign of a plan", otherTier, "", spendOnly, "CC max", "M"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newPlanDesk(t)
			d.store("max", "max")
			d.record(tc.recordTier, tc.recordPlan)
			d.reading("max", tc.reading)

			label, badge := d.drawn()
			assert.Equal(t, tc.label, label)
			assert.Equal(t, tc.badge, badge)
		})
	}
}

// Nothing is kept: the label is the reading's, so it follows the account back.
func TestThePlanFollowsTheReadingBothWays(t *testing.T) {
	d := newPlanDesk(t)
	d.store("max", "max")
	d.record(otherTier, "")

	d.reading("max", weekOnly)
	label, _ := d.drawn()
	require.Equal(t, "CC ultra", label)

	d.reading("max", bothWindows)
	label, badge := d.drawn()
	assert.Equal(t, "CC max", label, "the five-hour window is back")
	assert.Equal(t, "M", badge)
}

// A store with nothing fetched yet says nothing about its plan: no meters, and
// the reason for that carries the store's own name.
func TestAnEmptyReadingIsNoSignOfAPlan(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"a file with no data", "null"},
		{"a payload with no windows", `{"five_hour": null, "seven_day": null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newPlanDesk(t)
			d.store("max", "max")
			d.record(otherTier, "")
			d.reading("max", tc.data)

			u := panel.NewUsage()
			_, err := u.Poll(t.Context())
			require.NoError(t, err)
			sec := u.Section()
			require.Empty(t, sec.Meters)
			var labels []string
			for _, r := range sec.Reasons {
				labels = append(labels, r.Label)
			}
			assert.Contains(t, labels, "CC max")
		})
	}
}

// A store with no cache file at all is looked up too, and is simply not Ultra.
// The drawer is what reaches it without a fetch.
func TestAStoreWithNoCacheFileIsLookedUp(t *testing.T) {
	d := newPlanDesk(t)
	d.store("max", "max")
	d.store("work", "enterprise")
	d.record(otherTier, "")
	d.reading("work", weekOnly)

	_, err := panel.NewUsage().PollDetail(t.Context(), "CC work")
	assert.NoError(t, err)
}

// A cache file with no store behind it is drawn as an account too, so its name
// is as taken as a profile's.
func TestAPlanNameACacheFileHasIsNotTaken(t *testing.T) {
	d := newPlanDesk(t)
	d.store("max", "max")
	d.record(otherTier, "")
	d.reading("max", weekOnly)
	d.reading("ultra", bothWindows)

	u := panel.NewUsage()
	ok, err := u.Poll(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	labels := map[string]bool{}
	for _, m := range u.Section().Meters {
		labels[m.Label] = true
	}
	assert.Equal(t, map[string]bool{"CC max": true, "CC ultra": true}, labels)
}

// Only a store saying "max" can be Ultra by its reading. A profile keeps its
// own name either way, and is badged by what it shows.
func TestOnlyAMaxStoreIsReadAsUltra(t *testing.T) {
	d := newPlanDesk(t)
	d.store("work", "enterprise")
	d.record(otherTier, "")
	d.reading("work", weekOnly)
	label, badge := d.drawn()
	assert.Equal(t, "CC work", label)
	assert.Equal(t, "E", badge)

	d.store("work", "max")
	label, badge = d.drawn()
	assert.Equal(t, "CC work", label, "a profile's name is its directory's")
	assert.Equal(t, "U", badge)
}

// A profile directory named like the default store's plan keeps its name, and
// the default store keeps "max": two accounts under one caption would share
// one drawer, which shows whichever comes first. It holds wherever the plan
// came from.
func TestAPlanNameAProfileHasIsNotTaken(t *testing.T) {
	for _, tc := range []struct{ name, recordTier, recordPlan string }{
		{"named by the record", planTier, "Ultra"},
		{"shown by the reading", otherTier, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newPlanDesk(t)
			d.record(tc.recordTier, tc.recordPlan)
			for _, name := range []string{"max", "ultra"} {
				d.store(name, "max")
				d.reading(name, weekOnly)
			}

			u := panel.NewUsage()
			ok, err := u.Poll(t.Context())
			require.NoError(t, err)
			require.True(t, ok)
			labels := map[string]int{}
			for _, m := range u.Section().Meters {
				labels[m.Label]++
				assert.Equal(t, "U", m.Badge, "both are Ultra accounts")
			}
			assert.Equal(t, map[string]int{"CC max": 1, "CC ultra": 1}, labels)
		})
	}
}
