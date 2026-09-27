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
	// Provider is ProviderClaude or ProviderCodex.
	Provider string

	// Name is the profile, as the cache filename spells it. Empty for the
	// single-account files the widget wrote before profiles existed, and for
	// Codex, which has one.
	Name string
}

// Label is what a caption calls the account.
//
// A profile's own name, not "Claude max": the label shares a line with a
// window name, a percentage, a countdown and sometimes an amount, in a panel
// that is 260 px wide. "Claude" is only worth the room when the account has no
// name of its own, and Codex says which it is because it is the other
// provider.
func (a Account) Label() string {
	if a.Provider == ProviderCodex {
		if a.Name == "" {
			return "Codex"
		}
		return "Codex " + a.Name
	}
	if a.Name == "" {
		return "Claude"
	}
	return a.Name
}

// Accounts are the readings the cache holds, found by listing it.
//
// The filenames name the accounts, so nothing here opens a credential store.
// That is worth stating plainly: a package that discovers accounts without
// reading credentials is one that cannot leak or damage them, and discovery
// from the credential store arrives only where fetching needs it.
//
// The order is stable: Claude before Codex, and profiles alphabetically, so a
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

	out = withoutSupersededDefault(out)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider == ProviderClaude
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// withoutSupersededDefault drops the nameless Claude account when a named one
// exists.
//
// `usage.json` is what the widget wrote before profiles existed. Its own
// docstring says the file is "simply left unused" after the upgrade, and on
// this machine it was last written three weeks before the named ones. Drawing
// it beside them would put a dead reading next to a live one under the same
// heading.
func withoutSupersededDefault(accounts []Account) []Account {
	named := false
	for _, a := range accounts {
		if a.Provider == ProviderClaude && a.Name != "" {
			named = true
		}
	}
	if !named {
		return accounts
	}
	out := accounts[:0]
	for _, a := range accounts {
		if a.Provider == ProviderClaude && a.Name == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

// account reads a filename's suffix back into a provider and a profile.
//
// The suffix is what Slug writes: nothing, "-codex", "-max" or "-codex-max".
// An account name may itself contain a hyphen, so the provider is taken from
// the front and everything after it is the name.
func account(suffix string) Account {
	rest, ok := strings.CutPrefix(suffix, "-"+ProviderCodex)
	if ok {
		return Account{Provider: ProviderCodex, Name: strings.TrimPrefix(rest, "-")}
	}
	return Account{Provider: ProviderClaude, Name: strings.TrimPrefix(suffix, "-")}
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

	var windows []Window
	if a.Provider == ProviderCodex {
		windows, err = Codex(entry.Data)
	} else {
		windows, err = Claude(now, entry.Data)
	}
	return windows, r, err
}
