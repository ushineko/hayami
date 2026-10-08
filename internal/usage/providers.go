package usage

import (
	"encoding/json"
	"strings"
	"time"
)

/*
Spec is one provider the cache holds, as data (spec 045): what its files are
called, what its accounts are labelled, and how its payload decodes.

The table is the one place a provider is named. Adding one is an entry here,
its decoder, and its fetcher in the panel's table; nothing else spells it.
Credentials and fetching are not here: this package reads and writes the
cache and opens no credential store, which its doc promises and keeps.
*/
type Spec struct {
	// Name is the provider as the cache filename spells it: the suffix after
	// "usage-", empty for DefaultProvider.
	Name string

	// Display is the provider's name in a sentence: "Claude", "Codex".
	Display string

	// Short is the shorthand every meter label leads with: "CC", "CX".
	Short string

	// Decode reads the provider's payload into windows.
	Decode func(now time.Time, data json.RawMessage) ([]Window, error)
}

// DefaultProvider is the provider whose files carry no suffix: the widget
// named its files that way before there was a second provider, and Slug keeps
// every filename that already exists on a machine.
const DefaultProvider = ProviderClaude

// providers is every provider, in the order a pane lists them: the default
// first.
var providers = []Spec{
	{Name: ProviderClaude, Display: "Claude", Short: ShortClaude, Decode: Claude},
	{Name: ProviderCodex, Display: "Codex", Short: ShortCodex,
		Decode: func(_ time.Time, data json.RawMessage) ([]Window, error) { return Codex(data) }},
}

// Providers are every provider the cache holds, default first.
func Providers() []Spec { return append([]Spec(nil), providers...) }

// Lookup is the provider named name. One the table does not name is read as
// the default, which is what a file with no recognised suffix has always been.
func Lookup(name string) Spec {
	for _, p := range providers {
		if p.Name == name {
			return p
		}
	}
	return providers[0]
}

// rank is a provider's place in the table, for a stable order of accounts.
func rank(name string) int {
	for i, p := range providers {
		if p.Name == name {
			return i
		}
	}
	return len(providers)
}

// Decode reads an account's payload by its provider.
func Decode(now time.Time, a Account, data json.RawMessage) ([]Window, error) {
	return Lookup(a.Provider).Decode(now, data)
}

// Displays are the providers' names joined for a sentence: "Claude or Codex".
func Displays() string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Display
	}
	return strings.Join(names, " or ")
}
