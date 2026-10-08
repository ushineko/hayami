package panel_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
)

// A token must not reach anything the program prints. --readings is the
// obvious way one would escape, so it is asserted rather than assumed: the
// section's data is built from windows, and a window has no field a token
// could hide in.
func TestNothingThePanelPrintsCouldCarryAToken(t *testing.T) {
	u := panel.NewUsage()

	body, err := json.Marshal(u.Data())

	require.NoError(t, err)
	for _, word := range []string{"token", "Token", "accessToken", "refreshToken", "Bearer"} {
		assert.NotContains(t, string(body), word,
			"the readings carry a field a token could be put in")
	}
}

// Both sections are built from the same keys, by the same function, so neither
// shell can have one the other lacks.
func TestEverySectionKeyBuildsASource(t *testing.T) {
	keys := panel.Keys()
	require.NotEmpty(t, keys)

	sources := panel.Sources(keys, panel.Env{})

	require.Len(t, sources, len(keys))
	for i, s := range sources {
		assert.Equal(t, keys[i], s.Key())
		assert.NotEmpty(t, s.Section().Title, "a section with no title is a card with no heading")
		assert.NotZero(t, s.Interval(), "a source with no interval would never be polled again")
	}
}

// A section with nothing to say is not drawn, rather than drawn empty. On a
// machine with no cache and no credentials that is what usage is.
func TestASectionWithNothingToSayIsNotDrawn(t *testing.T) {
	testenv.Cache(t, t.TempDir())
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", t.TempDir())
	testenv.Home(t, t.TempDir())
	t.Setenv("PATH", "") // no codex to ask

	u := panel.NewUsage()
	drawn, err := u.Poll(t.Context())

	require.NoError(t, err)
	assert.False(t, drawn)
	assert.Empty(t, u.Section().Meters, "a section that is not drawn should have nothing to draw")
}
