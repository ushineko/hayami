package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/claude"
)

// accountRecord writes Claude Code's account record in the shape it writes
// one, names and all, every value invented.
func accountRecord(t *testing.T, plan, tier string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".claude.json")
	body := `{
      "numStartups": 1,
      "oauthAccount": {
        "accountUuid": "00000000-0000-0000-0000-000000000000",
        "emailAddress": "someone@example.com",
        "organizationType": "claude_max",
        "organizationRateLimitTier": "` + tier + `",
        "planDisplayName": "` + plan + `"
      }
    }`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	t.Setenv(claude.AccountFileEnv, path)
}

// An Ultra store's credential file still says "max"; the account record is
// where the plan is named, and the badge follows it.
func TestTheAccountRecordNamesAPlanTheStoreDoesNot(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "main"), "max")
	accountRecord(t, "Ultra", "default_claude_ai")

	got, err := claude.Read(claude.Store{Name: "main", Path: path})

	require.NoError(t, err)
	assert.Equal(t, "max", got.Plan, "the store's own word is kept")
	assert.Equal(t, "ultra", got.RecordedPlan)
	assert.Equal(t, "U", got.Badge())
}

// The record is one file for every profile, so it describes whichever account
// last wrote it. A record for another tier is another account's.
func TestARecordForAnotherAccountIsNotThisStoresPlan(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "enterprise")
	accountRecord(t, "Ultra", "default_claude_max_20x")

	got, err := claude.Read(claude.Store{Name: "work", Path: path})

	require.NoError(t, err)
	assert.Empty(t, got.RecordedPlan)
	assert.Equal(t, "E", got.Badge())
}
