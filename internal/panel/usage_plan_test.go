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
