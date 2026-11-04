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

// The account record describes whichever account last refreshed it. Once
// another profile has used Claude Code, the record no longer vouches for the
// default store, and the plan it named is remembered (#286): through a poll
// and a restart, until the store itself changes.
func TestARecordedPlanOutlivesTheRecord(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	testenv.Home(t, home)
	testenv.Cache(t, filepath.Join(root, "cache"))
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", filepath.Join(root, "profiles"))
	t.Setenv("PATH", "") // no codex to ask
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o700))

	store := func(subscription, tier string) {
		creds := fmt.Sprintf(`{"claudeAiOauth": {"accessToken": "invented", "refreshToken": "invented",
  "expiresAt": %d, "subscriptionType": %q, "rateLimitTier": %q}}`, time.Now().Add(24*time.Hour).UnixMilli(), subscription, tier)
		require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(creds), 0o600))
	}
	record := func(tier, plan string) {
		body := fmt.Sprintf(`{"oauthAccount": {"organizationRateLimitTier": %q, "planDisplayName": %q}}`, tier, plan)
		require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte(body), 0o600))
	}
	path, err := usage.Path("max", usage.ProviderClaude)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	resets := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"next_attempt_at": %d, "fetched_at": %d, "data": {"five_hour": null,
  "seven_day": {"utilization": 40, "resets_at": %q}}}`,
		time.Now().Add(time.Hour).Unix(), time.Now().Add(-time.Minute).Unix(), resets)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	drawn := func(u *panel.Usage) (label, badge string) {
		t.Helper()
		ok, err := u.Poll(t.Context())
		require.NoError(t, err)
		require.True(t, ok)
		meters := u.Section().Meters
		require.NotEmpty(t, meters)
		return meters[0].Label, meters[0].Badge
	}

	const ultraTier = "default_claude_max_20x"
	store("max", ultraTier)
	record(ultraTier, "Ultra")
	u := panel.NewUsage()
	label, badge := drawn(u)
	require.Equal(t, "CC ultra", label, "the record vouches for the store")
	require.Equal(t, "U", badge)

	// Another account refreshes the record: its tier, and no plan name.
	record("default_enterprise_tier", "")
	label, badge = drawn(u)
	assert.Equal(t, "CC ultra", label, "the plan is remembered")
	assert.Equal(t, "U", badge)
	label, badge = drawn(panel.NewUsage())
	assert.Equal(t, "CC ultra", label, "and remembered across a restart")
	assert.Equal(t, "U", badge)

	// The store changes tier: it probably holds another account now.
	store("max", "default_claude_max_5x")
	label, badge = drawn(panel.NewUsage())
	assert.Equal(t, "CC max", label, "a store that changed forgets its plan")
	assert.Equal(t, "M", badge)

	// Forgotten, not set aside: the old tier back does not bring it back.
	store("max", ultraTier)
	label, _ = drawn(panel.NewUsage())
	assert.Equal(t, "CC max", label, "until the record vouches again")

	record(ultraTier, "Ultra")
	label, _ = drawn(panel.NewUsage())
	assert.Equal(t, "CC ultra", label)
}

// A store that is removed takes its remembered plan with it: one made again
// in its place, with the same type and tier, is not taken for the account
// that was there.
func TestARemovedStoreForgetsItsPlan(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profiles := filepath.Join(root, "profiles")
	testenv.Home(t, home)
	testenv.Cache(t, filepath.Join(root, "cache"))
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", profiles)
	t.Setenv("PATH", "") // no codex to ask
	require.NoError(t, os.MkdirAll(home, 0o700))

	const tier = "default_claude_max_20x"
	dir := filepath.Join(profiles, "work")
	store := func() {
		require.NoError(t, os.MkdirAll(dir, 0o700))
		creds := fmt.Sprintf(`{"claudeAiOauth": {"accessToken": "invented", "refreshToken": "invented",
  "expiresAt": %d, "subscriptionType": "max", "rateLimitTier": %q}}`, time.Now().Add(24*time.Hour).UnixMilli(), tier)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600))
	}
	record := func(tier, plan string) {
		body := fmt.Sprintf(`{"oauthAccount": {"organizationRateLimitTier": %q, "planDisplayName": %q}}`, tier, plan)
		require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte(body), 0o600))
	}
	path, err := usage.Path("work", usage.ProviderClaude)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	resets := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"next_attempt_at": %d, "fetched_at": %d, "data": {"five_hour": null,
  "seven_day": {"utilization": 40, "resets_at": %q}}}`,
		time.Now().Add(time.Hour).Unix(), time.Now().Add(-time.Minute).Unix(), resets)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	badge := func() string {
		t.Helper()
		u := panel.NewUsage()
		ok, err := u.Poll(t.Context())
		require.NoError(t, err)
		require.True(t, ok)
		for _, m := range u.Section().Meters {
			if m.Label == "CC work" {
				return m.Badge
			}
		}
		t.Fatal("no meter for the work profile")
		return ""
	}

	store()
	record(tier, "Ultra")
	require.Equal(t, "U", badge(), "the record vouches for the profile")
	record("default_enterprise_tier", "")
	require.Equal(t, "U", badge(), "and the plan is remembered")

	require.NoError(t, os.RemoveAll(dir))
	badge()
	store()
	assert.Equal(t, "M", badge(), "a store made again is not the one remembered")
}

// A profile directory named like the default store's plan keeps its name, and
// the default store keeps "max": two accounts under one caption would share
// one drawer, which shows whichever comes first.
func TestAPlanNameAProfileHasIsNotTaken(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	profiles := filepath.Join(root, "profiles")
	testenv.Home(t, home)
	testenv.Cache(t, filepath.Join(root, "cache"))
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", profiles)
	t.Setenv("PATH", "") // no codex to ask

	const tier = "default_claude_max_20x"
	creds := fmt.Sprintf(`{"claudeAiOauth": {"accessToken": "invented", "refreshToken": "invented",
  "expiresAt": %d, "subscriptionType": "max", "rateLimitTier": %q}}`, time.Now().Add(24*time.Hour).UnixMilli(), tier)
	for _, dir := range []string{filepath.Join(home, ".claude"), filepath.Join(profiles, "ultra")} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600))
	}
	record := fmt.Sprintf(`{"oauthAccount": {"organizationRateLimitTier": %q, "planDisplayName": "Ultra"}}`, tier)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude.json"), []byte(record), 0o600))

	resets := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	for _, name := range []string{"max", "ultra"} {
		path, err := usage.Path(name, usage.ProviderClaude)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		body := fmt.Sprintf(`{"next_attempt_at": %d, "fetched_at": %d, "data": {"five_hour": null,
  "seven_day": {"utilization": 40, "resets_at": %q}}}`,
			time.Now().Add(time.Hour).Unix(), time.Now().Add(-time.Minute).Unix(), resets)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
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
}
