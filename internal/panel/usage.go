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

	// reasons are the accounts that had nothing to say and why, rebuilt every
	// poll.
	reasons []view.Reason

	// read gathers the accounts, replaced by a test so neither the real cache
	// nor a real credential store is touched.
	read func(ctx context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error)
}

// NewUsage builds the usage source.
func NewUsage() *Usage {
	u := &Usage{now: time.Now}
	u.read = func(ctx context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return gather(ctx, u.now())
	}
	return u
}

// Key names the section.
func (u *Usage) Key() string { return view.UsageInfo.Key }

// Interval is UsageInterval.
func (u *Usage) Interval() time.Duration { return UsageInterval }

// Poll re-reads the cache. It reports false when the cache holds nothing at
// all, so a machine that has never run any of these programs draws no section
// rather than an empty one.
func (u *Usage) Poll(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err //nolint:wrapcheck // the context's own message is the whole story
	}

	windows, fetchedAt, reasons, err := u.read(ctx)
	if err != nil {
		u.mu.Lock()
		u.reasons = []view.Reason{reason(view.Reason{
			Text: "the usage cache could not be read", Status: view.Warn,
		}, err)}
		u.mu.Unlock()
		return false, err
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.windows, u.fetchedAt, u.reasons = windows, fetchedAt, reasons
	return len(windows) > 0, nil
}

// Section turns the windows into meters.
func (u *Usage) Section() view.Section {
	u.mu.Lock()
	windows, fetchedAt, reasons := u.windows, u.fetchedAt, u.reasons
	u.mu.Unlock()
	sec := view.Usage(u.now(), windows, fetchedAt)
	sec.Reasons = reasons
	return sec
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
func gather(ctx context.Context, now time.Time) ([]view.UsageWindow, time.Time, []view.Reason, error) {
	accounts, err := usage.Accounts()
	if err != nil {
		return nil, time.Time{}, nil, err
	}
	stores, badges := claudeStores()
	accounts = merge(accounts, stores)

	if len(accounts) == 0 {
		return nil, time.Time{}, []view.Reason{{
			Text: "no Claude or Codex account", Status: view.Info,
			Detail: "no credential store and nothing in the usage cache",
		}}, nil
	}

	var out []view.UsageWindow
	var reasons []view.Reason
	var oldest time.Time
	for _, a := range accounts {
		result, ferr := refresh(ctx, now, a, stores)

		windows, err := decode(now, a, result.Data)
		if err != nil {
			// One account's payload not decoding is one account, not the
			// section. The canary in internal/usage reports a format change;
			// a pane should still draw the accounts that do decode.
			reasons = append(reasons, reason(view.Reason{
				Label: a.Label(), Text: "unreadable reading", Status: view.Warn,
			}, err))
			continue
		}
		if len(windows) == 0 {
			reasons = append(reasons, silence(now, a, ferr))
		}
		for _, w := range windows {
			out = append(out, view.UsageWindow{
				Account:  a.Label(),
				Badge:    badges[a.Name],
				Name:     w.Name,
				Fraction: w.Fraction,
				ResetsAt: w.ResetsAt,
				Detail:   w.Detail,
				Used:     w.Used,
				Limit:    w.Limit,
				Severity: w.Severity,
				Span:     w.Span,
			})
		}
		if !result.FetchedAt.IsZero() && (oldest.IsZero() || result.FetchedAt.Before(oldest)) {
			oldest = result.FetchedAt
		}
	}
	// The reasons stand even when other accounts drew: a quota missing from a
	// list of quotas is exactly the silence this is here to break.
	return out, oldest, reasons, nil
}

/*
silence is why one account contributed no windows.

Three cases, and telling them apart is the point. A fetch that failed is a
Warn with the error under it. A fetch that has not been *allowed* yet -- the
gate closed by an earlier failure, with nothing cached behind it -- says so and
says until when, because thirty-five minutes of blank panel with no explanation
is the fault that prompted this (issue #54). Anything else is an account that
has simply never fetched.

The gate is not shortened here. It is written into a file three programs read,
and a build that set its own would be deciding for the other two; this says
what is happening instead.
*/
func silence(now time.Time, a usage.Account, err error) view.Reason {
	if err != nil {
		return reason(view.Reason{Label: a.Label(), Text: "could not be read", Status: view.Warn}, err)
	}

	entry, rerr := usage.Read(a.Name, a.Provider)
	if rerr == nil && entry != nil && !entry.Open(now) {
		return view.Reason{
			Label: a.Label(), Text: "waiting to retry", Status: view.Warn,
			Detail: "an earlier fetch failed; the shared cache's gate opens at " +
				entry.NextAttempt().Format("15:04"),
		}
	}
	return view.Reason{
		Label: a.Label(), Text: "nothing fetched yet", Status: view.Info,
	}
}

// refresh asks the cache for one account, fetching when its gate is open and
// there is something to fetch with.
//
// An account with no credential store and no Codex is read and not fetched:
// calling Cached with a fetch that always fails would push the gate out for
// every other program too, which is the opposite of cooperating.
//
// The error is returned as well as the result, because the two are not the
// same thing: a failed fetch still yields the last good reading, and a section
// that draws stale numbers should still be able to say why they are stale.
func refresh(ctx context.Context, now time.Time, a usage.Account, stores map[string]claude.Store) (usage.Result, error) {
	fetch := fetcher(ctx, a, stores)
	if fetch == nil {
		entry, err := usage.Read(a.Name, a.Provider)
		if err != nil || entry == nil {
			return usage.Result{}, err
		}
		return usage.Served(*entry), nil
	}

	// A failed fetch still yields the last good reading, which is what Cached
	// returns alongside the error. An outage shows stale numbers rather than
	// an empty panel.
	result, err := usage.Cached(now, UsageTTL, a.Name, a.Provider, false, fetch)
	return result, err
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
