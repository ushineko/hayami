package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/claude"
)

// Every token in these tests is invented and looks it. A real one does not
// belong in a repository, and one that looked real would invite somebody to
// wonder.
const (
	fakeAccess  = "not-a-real-access-token"
	fakeRefresh = "not-a-real-refresh-token"
)

// storeFile writes a credential file in the shape Claude Code writes one,
// including a key this build has never heard of.
func storeFile(t *testing.T, dir, plan string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, claude.CredentialsFile)
	body := `{
      "claudeAiOauth": {
        "accessToken": "` + fakeAccess + `",
        "refreshToken": "` + fakeRefresh + `",
        "expiresAt": 4102444800000,
        "scopes": ["user:inference"],
        "subscriptionType": "` + plan + `",
        "rateLimitTier": "default_claude_ai",
        "something_this_build_has_never_heard_of": {"deep": [1, 2]}
      },
      "anotherTopLevelKey": {"kept": true}
    }`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// profiles points the package at a root of this test's own, so nothing here
// goes near a real credential store.
func profiles(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv(claude.ProfileRootEnv, root)
	return root
}

func TestStoresAreFoundByTheDirectoryConvention(t *testing.T) {
	root := profiles(t)
	storeFile(t, filepath.Join(root, "work"), "enterprise")
	storeFile(t, filepath.Join(root, "another"), "pro")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "logged-out"), 0o700)) // no file yet

	got, err := claude.Stores()

	require.NoError(t, err)
	names := []string{}
	for _, s := range got {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "another")
	assert.Contains(t, names, "work")
	assert.NotContains(t, names, "logged-out",
		"a directory the wrapper made before the login that fills it is not an account")
}

func TestAStoreYieldsItsTokensAndItsPlan(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "enterprise")

	got, err := claude.Read(claude.Store{Name: "work", Path: path})

	require.NoError(t, err)
	assert.Equal(t, fakeAccess, got.AccessToken)
	assert.Equal(t, fakeRefresh, got.RefreshToken)
	assert.Equal(t, "enterprise", got.Plan)
	assert.Equal(t, "E", got.Badge())
}

// The plan comes from the file rather than the API, so an account whose token
// has expired is still labelled correctly.
func TestTheBadgeComesFromTheFile(t *testing.T) {
	root := profiles(t)
	for plan, badge := range map[string]string{
		"max": "M", "enterprise": "E", "pro": "P", "team": "T", "something_new": "",
	} {
		path := storeFile(t, filepath.Join(root, plan), plan)
		got, err := claude.Read(claude.Store{Name: plan, Path: path})
		require.NoError(t, err)
		assert.Equal(t, badge, got.Badge(), "plan %q", plan)
	}
}

// One unreadable store is one account. A panel that lost its whole usage
// section because one profile had a truncated file would be worse than one
// that lost that profile.
func TestAStoreThatDoesNotParseIsRefused(t *testing.T) {
	root := profiles(t)
	dir := filepath.Join(root, "broken")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, claude.CredentialsFile)
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	_, err := claude.Read(claude.Store{Name: "broken", Path: path})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "token", "an error must not carry what it was reading")
}

func TestAStoreWithNoOauthObjectIsRefused(t *testing.T) {
	root := profiles(t)
	dir := filepath.Join(root, "empty")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, claude.CredentialsFile)
	require.NoError(t, os.WriteFile(path, []byte(`{"somethingElse": {}}`), 0o600))

	_, err := claude.Read(claude.Store{Name: "empty", Path: path})

	require.Error(t, err)
}
