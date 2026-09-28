package panel

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/claude"

	"github.com/ushineko/hayami/internal/usage"
	"github.com/ushineko/hayami/internal/view"
)

// UsageInterval is how often the cache is re-read.
//
// Thirty seconds, and this is a *read*, not a request: the gate in the cache
// is what limits the upstream calls, and re-reading a local file more often
// than a quota can move would be spending a poll on nothing.
const UsageInterval = 30 * time.Second

// Usage draws the accounts' quotas, fetching them when the shared cache's gate
// is open and reading the cache when it is not.
//
// Whether this process or another one goes upstream is the cache's decision,
// not this type's: five panes and a window together make about one request per
// account per window, which is the arrangement the cache exists for.
type Usage struct {
	mu        sync.Mutex
	windows   []view.UsageWindow
	fetchedAt time.Time

	// now is time.Now, replaced by a test so a countdown can be asserted.
	now func() time.Time

	// read gathers the accounts, replaced by a test so neither the real cache
	// nor a real credential store is touched.
	read func(ctx context.Context) ([]view.UsageWindow, time.Time, error)
}

// NewUsage builds the usage source.
func NewUsage() *Usage {
	u := &Usage{now: time.Now}
	u.read = func(ctx context.Context) ([]view.UsageWindow, time.Time, error) {
		return gather(ctx, u.now())
	}
	return u
}

// Key names the section.
func (u *Usage) Key() string { return "usage" }

// Title is what it is called on screen.
func (u *Usage) Title() string { return "Usage" }

// Interval is UsageInterval.
func (u *Usage) Interval() time.Duration { return UsageInterval }

// Poll re-reads the cache. It reports false when the cache holds nothing at
// all, so a machine that has never run any of these programs draws no section
// rather than an empty one.
func (u *Usage) Poll(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err //nolint:wrapcheck // the context's own message is the whole story
	}

	windows, fetchedAt, err := u.read(ctx)
	if err != nil {
		return false, err
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.windows, u.fetchedAt = windows, fetchedAt
	return len(windows) > 0, nil
}

// Section turns the windows into meters.
func (u *Usage) Section() view.Section {
	u.mu.Lock()
	windows, fetchedAt := u.windows, u.fetchedAt
	u.mu.Unlock()
	return view.Usage(u.now(), windows, fetchedAt)
}

// Data is the windows as plain values, for the JSON the command line prints.
func (u *Usage) Data() any {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.windows
}

// gather is every account this machine has, fetched where it can be and read
// where it cannot.
//
// The accounts come from two places and the union is deliberate: a credential
// store hayami can fetch for, and a cache entry some other program wrote.
// Either alone would lose something — a fresh login with no cache yet, or an
// account whose credentials have moved but whose last reading is still worth
// showing.
//
// The oldest reading decides the age shown: a section is as stale as its
// stalest number, and reporting the freshest would claim the panel is more
// current than it is.
func gather(ctx context.Context, now time.Time) ([]view.UsageWindow, time.Time, error) {
	accounts, err := usage.Accounts()
	if err != nil {
		return nil, time.Time{}, err
	}
	stores, badges := claudeStores()
	accounts = merge(accounts, stores)

	var out []view.UsageWindow
	var oldest time.Time
	for _, a := range accounts {
		result := refresh(ctx, now, a, stores)

		windows, err := decode(now, a, result.Data)
		if err != nil {
			// One account's payload not decoding is one account, not the
			// section. The canary in internal/usage reports a format change;
			// a pane should still draw the accounts that do decode.
			continue
		}
		for _, w := range windows {
			out = append(out, view.UsageWindow{
				Account:  a.Label(),
				Badge:    badges[a.Name],
				Name:     w.Name,
				Fraction: w.Fraction,
				ResetsAt: w.ResetsAt,
				Detail:   w.Detail,
				Span:     w.Span,
			})
		}
		if !result.FetchedAt.IsZero() && (oldest.IsZero() || result.FetchedAt.Before(oldest)) {
			oldest = result.FetchedAt
		}
	}
	return out, oldest, nil
}

// refresh asks the cache for one account, fetching when its gate is open and
// there is something to fetch with.
//
// An account with no credential store and no Codex is read and not fetched:
// calling Cached with a fetch that always fails would push the gate out for
// every other program too, which is the opposite of cooperating.
func refresh(ctx context.Context, now time.Time, a usage.Account, stores map[string]claude.Store) usage.Result {
	fetch := fetcher(ctx, a, stores)
	if fetch == nil {
		entry, err := usage.Read(a.Name, a.Provider)
		if err != nil || entry == nil {
			return usage.Result{}
		}
		return usage.Served(*entry)
	}

	result, err := usage.Cached(now, UsageTTL, a.Name, a.Provider, false, fetch)
	if err != nil {
		// A failed fetch still yields the last good reading, which is what
		// Cached returns alongside the error. An outage shows stale numbers
		// rather than an empty panel.
		return result
	}
	return result
}

// fetcher is how this account is fetched, or nil when it cannot be.
func fetcher(ctx context.Context, a usage.Account, stores map[string]claude.Store) usage.Fetch {
	if a.Provider == usage.ProviderCodex {
		if !codexInstalled() {
			return nil
		}
		return fetchCodex(ctx)
	}
	store, ok := stores[a.Name]
	if !ok {
		return nil
	}
	return fetchClaude(ctx, store)
}

// decode reads a payload for the provider it came from.
func decode(now time.Time, a usage.Account, data json.RawMessage) ([]usage.Window, error) {
	if a.Provider == usage.ProviderCodex {
		return usage.Codex(data)
	}
	return usage.Claude(now, data)
}

// claudeStores are the credential stores by profile name, with each one's
// badge. A store that cannot be read contributes neither: it is one account
// absent, and the others still draw.
func claudeStores() (map[string]claude.Store, map[string]string) {
	stores := map[string]claude.Store{}
	badges := map[string]string{}

	found, err := claude.Stores()
	if err != nil {
		return stores, badges
	}
	for _, s := range found {
		stores[s.Name] = s
		if creds, err := claude.Read(s); err == nil {
			badges[s.Name] = creds.Badge()
		}
	}
	return stores, badges
}

// merge adds an account for every credential store the cache does not already
// know, so a profile that has just been logged in appears before anything has
// written a cache file for it.
func merge(accounts []usage.Account, stores map[string]claude.Store) []usage.Account {
	have := map[string]bool{}
	for _, a := range accounts {
		if a.Provider == usage.ProviderClaude {
			have[a.Name] = true
		}
	}
	names := make([]string, 0, len(stores))
	for name := range stores {
		if !have[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		accounts = append(accounts, usage.Account{Provider: usage.ProviderClaude, Name: name})
	}
	return accounts
}

// codexInstalled reports whether there is an app-server to ask. A machine
// without Codex is not a machine with a problem.
var codexInstalled = sync.OnceValue(func() bool {
	_, err := exec.LookPath("codex")
	return err == nil
})
