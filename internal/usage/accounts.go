package usage

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Account is one cached reading's owner: a provider and, for Claude, a profile
// name.
type Account struct {
	// Provider is a Spec's Name: ProviderClaude or ProviderCodex.
	Provider string

	// Name is the profile, as the cache filename spells it. Empty for the
	// single-account files the widget wrote before profiles existed, and for
	// Codex, which has one.
	Name string
}

// ShortClaude and ShortCodex are the provider shorthands every meter label
// leads with.
//
// Two letters each, so a column of accounts reads as a column of providers
// before it reads as names: "CC max", "CC work", "CX" line up where "max",
// "work" and "Codex" were three different shapes. CC is Claude Code's own
// abbreviation; CX is the one Codex goes by.
const (
	ShortClaude = "CC"
	ShortCodex  = "CX"
)

// Label is what a caption calls the account: the provider's shorthand, then
// the profile's name when it has one.
//
// Short on purpose: the label shares a line with a badge, a window name, a
// percentage, a countdown and sometimes an amount, in a panel that is 260 px
// wide. The shorthand says which provider, so Codex is "CX" rather than
// "Codex", and a nameless Claude account is "CC" alone.
func (a Account) Label() string {
	short := Lookup(a.Provider).Short
	if a.Name == "" {
		return short
	}
	return short + " " + a.Name
}

// Accounts are the readings the cache holds, found by listing it.
//
// The filenames name the accounts, so nothing here opens a credential store.
// That is worth stating plainly: a package that discovers accounts without
// reading credentials is one that cannot leak or damage them, and discovery
// from the credential store arrives only where fetching needs it.
//
// The order is stable: providers in the table's order (Claude before Codex),
// and profiles alphabetically, so a
// pane's lines do not change places between two runs.
func Accounts() ([]Account, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listing the usage cache: %w", err)
	}

	var out []Account
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		rest, ok := strings.CutPrefix(name, "usage")
		if !ok {
			continue
		}
		out = append(out, account(rest))
	}

	out = withoutSupersededDefault(out, hasData)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return rank(out[i].Provider) < rank(out[j].Provider)
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

/*
withoutSupersededDefault drops the nameless Claude account when a named one has
actually fetched something.

`usage.json` is what the widget wrote before profiles existed. Its own
docstring says the file is "simply left unused" after the upgrade, and on the
machine this was written for it was last written three weeks before the named
ones. Drawing it beside them would put a dead reading next to a live one under
the same heading.

**Superseded means replaced, not merely outnumbered.** A named account that
exists and has never fetched supersedes nothing: on a second machine the named
file held `"data": null` behind a half-hour backoff while the nameless one --
which the Python widget keeps full and fresh -- was dropped in its favour, and
the section drew nothing at all (issue #54). So the named accounts have to have
a reading between them before the old file is set aside.

has reports whether an account has a payload cached. It is injected so the
rule can be tested without a cache directory; the real one reads the files this
function is already deciding about and opens no credential store, which is the
property the package docstring claims and keeps.
*/
func withoutSupersededDefault(accounts []Account, has func(Account) bool) []Account {
	named := false
	for _, a := range accounts {
		if a.Provider == DefaultProvider && a.Name != "" && has(a) {
			named = true
		}
	}
	if !named {
		return accounts
	}
	out := accounts[:0]
	for _, a := range accounts {
		if a.Provider == DefaultProvider && a.Name == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// hasData reports whether an account's cache file holds a payload. An entry
// that cannot be read holds none, which is the same answer a missing file
// gives and the safe one: it keeps the older account rather than dropping it
// for a newer one that has nothing.
func hasData(a Account) bool {
	e, err := Read(a.Name, a.Provider)
	return err == nil && e != nil && len(e.Data) > 0
}

// account reads a filename's suffix back into a provider and a profile.
//
// The suffix is what Slug writes: nothing, "-codex", "-max" or "-codex-max".
// An account name may itself contain a hyphen, so the provider is taken from
// the front and everything after it is the name.
func account(suffix string) Account {
	for _, p := range providers {
		if p.Name == DefaultProvider {
			continue
		}
		if rest, ok := strings.CutPrefix(suffix, "-"+p.Name); ok {
			return Account{Provider: p.Name, Name: strings.TrimPrefix(rest, "-")}
		}
	}
	return Account{Provider: DefaultProvider, Name: strings.TrimPrefix(suffix, "-")}
}

// Windows reads one account's cached windows, with the age of the reading.
//
// It never fetches. This is the read-only half of the cache: the Python
// widget and monitor keep it fresh, and a machine without them shows nothing
// until hayami can fetch for itself.
func Windows(now time.Time, a Account) ([]Window, Result, error) {
	entry, err := Read(a.Name, a.Provider)
	if err != nil || entry == nil {
		return nil, Result{}, err
	}
	r := served(*entry)

	windows, err := Decode(now, a, entry.Data)
	return windows, r, err
}
