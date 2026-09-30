package usage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

// write puts an empty cache file in place, so a test can say which accounts
// exist without saying anything about what they hold.
func touch(t *testing.T, dir, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{"next_attempt_at":1}`), 0o600))
}

// fill is a cache file that has actually fetched something, as distinct from
// one that exists and holds nothing. The difference decides which account
// supersedes which.
func fill(t *testing.T, dir, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := `{"next_attempt_at":1,"fetched_at":1,"data":{"five_hour":{"utilization":1}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}

// Accounts come from the filenames, so nothing here opens a credential store.
// That is the point of discovering them this way: a package that cannot read
// credentials cannot leak or damage them.
func TestAccountsAreFoundByListingTheCacheAndNothingElse(t *testing.T) {
	dir := tempCache(t)
	touch(t, dir, "usage-max.json")
	touch(t, dir, "usage-work.json")
	touch(t, dir, "usage-codex.json")
	touch(t, dir, "usage-max.lock")      // not a reading
	touch(t, dir, "something-else.json") // not ours

	got, err := usage.Accounts()

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "CC max", got[0].Label())
	assert.Equal(t, "CC work", got[1].Label())
	assert.Equal(t, "CX", got[2].Label(), "Claude before Codex, profiles alphabetically")
}

// Every label leads with the provider's shorthand, so a column of accounts
// says which provider each line is before it says whose. Codex's own name is
// replaced by its shorthand rather than said twice.
func TestALabelLeadsWithTheProvider(t *testing.T) {
	for _, c := range []struct {
		account usage.Account
		want    string
	}{
		{usage.Account{Provider: usage.ProviderClaude, Name: "max"}, "CC max"},
		{usage.Account{Provider: usage.ProviderClaude}, "CC"},
		{usage.Account{Provider: usage.ProviderCodex}, "CX"},
		{usage.Account{Provider: usage.ProviderCodex, Name: "team"}, "CX team"},
	} {
		assert.Equal(t, c.want, c.account.Label(), "%+v", c.account)
	}
}

// The widget wrote usage.json before profiles existed and its own docstring
// says the file is left unused afterwards. On the machine this was written on
// it was three weeks older than the named ones; drawing it beside them would
// put a dead reading next to a live one under the same heading.
func TestThePreProfileFileIsDroppedOnceThereAreProfiles(t *testing.T) {
	dir := tempCache(t)
	fill(t, dir, "usage.json")
	fill(t, dir, "usage-max.json")

	got, err := usage.Accounts()

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "CC max", got[0].Label())
}

/*
A named profile that has never fetched supersedes nothing.

The case that cost a machine its whole Usage section: `usage-max.json` held
`"data": null` behind a half-hour backoff, and the nameless `usage.json` --
which the Python widget keeps full and fresh -- was dropped in its favour, so
the section drew nothing at all (issue #54). Superseded means replaced, not
merely outnumbered.
*/
func TestThePreProfileFileIsKeptWhenTheProfilesHaveNothing(t *testing.T) {
	dir := tempCache(t)
	fill(t, dir, "usage.json")
	touch(t, dir, "usage-max.json") // exists, has never fetched

	got, err := usage.Accounts()

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "CC", got[0].Label())
	assert.Equal(t, "CC max", got[1].Label())
}

// And once that profile does fetch, the old file steps aside after all.
func TestThePreProfileFileStepsAsideOnceAProfileFetches(t *testing.T) {
	dir := tempCache(t)
	fill(t, dir, "usage.json")
	touch(t, dir, "usage-max.json")

	before, err := usage.Accounts()
	require.NoError(t, err)
	require.Len(t, before, 2)

	fill(t, dir, "usage-max.json")

	after, err := usage.Accounts()
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, "CC max", after[0].Label())
}

// On a machine that never upgraded, that file is the only reading there is.
func TestThePreProfileFileIsKeptWhenItIsAllThereIs(t *testing.T) {
	dir := tempCache(t)
	touch(t, dir, "usage.json")
	touch(t, dir, "usage-codex.json")

	got, err := usage.Accounts()

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "CC", got[0].Label())
}

// An account name may carry a hyphen, so the provider is read from the front
// rather than by splitting on the last one.
func TestAnAccountNameMayCarryAHyphen(t *testing.T) {
	dir := tempCache(t)
	touch(t, dir, "usage-codex-my-account.json")

	got, err := usage.Accounts()

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, usage.ProviderCodex, got[0].Provider)
	assert.Equal(t, "my-account", got[0].Name)
}

func TestAMachineWithNoCacheHasNoAccounts(t *testing.T) {
	tempCache(t)

	got, err := usage.Accounts()

	require.NoError(t, err)
	assert.Empty(t, got)
}
