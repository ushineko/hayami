package panel

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
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

	// breakdown reads one account's breakdown by its label, replaced by a
	// test for the reason read is.
	breakdown func(label string) (view.UsageBreakdown, error)
}

// NewUsage builds the usage source.
func NewUsage() *Usage {
	u := &Usage{now: time.Now}
	u.breakdown = cachedBreakdown(func() time.Time { return u.now() })
	last := &fetches{last: map[string]failure{}}
	u.read = func(ctx context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return gather(ctx, u.now(), last)
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
	// The view's own reasons (an old reading's age) come after the source's.
	sec.Reasons = append(append([]view.Reason(nil), reasons...), sec.Reasons...)
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
func gather(ctx context.Context, now time.Time, last *fetches) ([]view.UsageWindow, time.Time, []view.Reason, error) {
	accounts, err := usage.Accounts()
	if err != nil {
		return nil, time.Time{}, nil, err
	}
	states := loadProviders()
	accounts = merge(accounts, states)

	if len(accounts) == 0 {
		return nil, time.Time{}, []view.Reason{{
			Text: "no " + usage.Displays() + " account", Status: view.Info,
			Detail: "no credential store and nothing in the usage cache",
		}}, nil
	}

	var out []view.UsageWindow
	var reasons []view.Reason
	var oldest time.Time
	for _, a := range accounts {
		label := accountLabel(a, states)
		result, ferr := refresh(ctx, now, a, states)
		ferr = last.settle(a.Provider+"/"+a.Name, now, result, ferr)

		windows, err := usage.Decode(now, a, result.Data)
		if err != nil {
			// One account's payload not decoding is one account, not the
			// section. The canary in internal/usage reports a format change;
			// a pane should still draw the accounts that do decode.
			reasons = append(reasons, reason(view.Reason{
				Label: label, Text: "unreadable reading", Status: view.Warn,
			}, err))
			continue
		}
		windows, turned := current(now, result.FetchedAt, windows)

		// Why this account's figures are missing or old (spec 089). A refused
		// login is said whether or not old figures stand, because nothing
		// changes until a person signs in; any other failure behind figures
		// is an aside, because a minute without a network must not add a line.
		switch {
		case errors.Is(ferr, claude.ErrNeedsLogin) || result.LoginRefused:
			reasons = append(reasons, needsLogin(label))
		case len(windows) > 0:
			if ferr != nil {
				reasons = append(reasons, reason(view.Reason{
					Label: label, Text: "not refreshed", Status: view.Warn, Aside: true,
				}, ferr))
			}
		case turned && ferr == nil:
			reasons = append(reasons, view.Reason{
				Label: label, Text: "out of date", Status: view.Info,
				Detail: "read " + result.FetchedAt.Local().Format("2006-01-02 15:04") +
					"; every window has started over since",
			})
		default:
			reasons = append(reasons, silence(now, a, label, ferr))
		}
		for _, w := range windows {
			out = append(out, view.UsageWindow{
				Account:  label,
				Badge:    states[a.Provider].badges[a.Name],
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
func silence(now time.Time, a usage.Account, label string, err error) view.Reason {
	if err != nil {
		return reason(view.Reason{Label: label, Text: "could not be read", Status: view.Warn}, err)
	}

	entry, rerr := usage.Read(a.Name, a.Provider)
	if rerr == nil && entry != nil && !entry.Open(now) {
		return view.Reason{
			Label: label, Text: "waiting to retry", Status: view.Warn,
			Detail: "an earlier fetch failed; the shared cache's gate opens at " +
				entry.NextAttempt().Format("15:04"),
		}
	}
	return view.Reason{
		Label: label, Text: "nothing fetched yet", Status: view.Info,
	}
}

// needsLogin is an account whose sign-in the server refused: a line, because
// it holds until a person acts (spec 089 R2).
func needsLogin(label string) view.Reason {
	return view.Reason{
		Label: label, Text: "needs a login", Status: view.Warn, Actionable: true,
		Detail: "the server refused this account's sign-in; sign in with Claude Code again",
	}
}

/*
fetches are what this process's last fetch for each account came to, so the
polls between fetches can say it too (spec 089 R1).

A fetch is made at most once a gate, two minutes, and the polls are every
thirty seconds; between fetches the cache is served without an error. A
reason built from the fetch's own error alone was drawn for one poll in four,
and a line that comes and goes moves the panel each time.
*/
type fetches struct {
	mu   sync.Mutex
	last map[string]failure
}

// failure is a fetch that failed and when it was asked for.
type failure struct {
	err error
	at  time.Time
}

// settle records a fetch's outcome and returns the error that stands for the
// account: this fetch's, or the last one's while nothing newer has been read.
// A reading obtained after the failure, by this process or another, clears it.
func (f *fetches) settle(key string, now time.Time, r usage.Result, err error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.last[key] = failure{err: err, at: now}
		return err
	}
	prev, ok := f.last[key]
	if !ok {
		return nil
	}
	// A reading from the future is a forged or broken file, not a newer one:
	// taking it as newer cleared the failure on every poll but the fetch's.
	if r.Fresh || (r.FetchedAt.After(prev.at) && !r.FetchedAt.After(now)) {
		delete(f.last, key)
		return nil
	}
	return prev.err
}

/*
turnedOver reports whether a window resetting at resets has started over
since a reading fetched at fetched, by more than the stale margin (spec 089
R5).

The fraction then describes a window that is gone: the machine this came
from drew a five-hour window at 1 % with its reset a month past. The margin
is the panel's stale threshold, so a window does not vanish for the minutes
between its reset and the next fetch.
*/
func turnedOver(now, fetched, resets time.Time) bool {
	return !resets.IsZero() && resets.Before(now.Add(-view.UsageStale)) && fetched.Before(resets)
}

// current is the windows that have not turned over, and whether any had.
func current(now, fetched time.Time, windows []usage.Window) ([]usage.Window, bool) {
	out := make([]usage.Window, 0, len(windows))
	for _, w := range windows {
		if !turnedOver(now, fetched, w.ResetsAt) {
			out = append(out, w)
		}
	}
	return out, len(out) < len(windows)
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
func refresh(ctx context.Context, now time.Time, a usage.Account, states map[string]providerState) (usage.Result, error) {
	var fetch usage.Fetch
	if st, ok := states[a.Provider]; ok && st.fetcher != nil {
		fetch = st.fetcher(ctx, a.Name)
	}
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

// claudeStores are the credential stores by profile name, with each one's
// badge and, where it is called something else on screen, its name. A store
// that cannot be read contributes neither: it is one account absent, and the
// others still draw.
//
// The default store is the one renamed. "max" is a name this program gave it,
// after the wrapper that reaches it, and the cache files keep it; on screen it
// is called by its plan once Claude Code's account record names that plan, so
// an Ultra account is "CC ultra" and not "CC max". The record describes the
// account that last refreshed it, so when it is another account's the store's
// own usage reading is asked (spec 083). A profile's name is its directory's,
// which somebody chose, and stays.
func claudeStores() (stores map[string]claude.Store, badges, names map[string]string) {
	stores = map[string]claude.Store{}
	badges = map[string]string{}
	names = map[string]string{}

	found, err := claude.Stores()
	if err != nil {
		return stores, badges, names
	}
	read := map[string]*claude.Credentials{}
	for _, s := range found {
		stores[s.Name] = s
		if creds, err := claude.Read(s); err == nil {
			creds.ObservedPlan = observedPlan(s.Name, creds)
			read[s.Name] = creds
		}
	}
	for _, s := range found {
		creds, ok := read[s.Name]
		if !ok {
			continue
		}
		badges[s.Name] = creds.Badge()
		// A profile already called by the plan's name keeps it: two accounts
		// under one caption would share a drawer.
		plan := creds.ShownPlan()
		if s.Name == claude.DefaultProfileName && plan != "" && !claudeNameTaken(plan, stores) {
			names[s.Name] = plan
		}
	}
	return stores, badges, names
}

// claudeNameTaken reports whether an account is already called name: a store,
// or a cache file with no store behind it, which is drawn too.
func claudeNameTaken(name string, stores map[string]claude.Store) bool {
	if _, ok := stores[name]; ok {
		return true
	}
	accounts, err := usage.Accounts()
	if err != nil {
		return false
	}
	return slices.Contains(accounts, usage.Account{Provider: usage.ProviderClaude, Name: name})
}

/*
observedPlan is the plan a store's own usage reading shows, or empty. An Ultra
store's credential file says "max", and what tells it from a Max account is
that it has no five-hour window (#288). Worked out from the reading every time,
so it cannot outlast the plan, and a failed fetch leaves the last good reading
in the cache and so the same answer.

It is an inference from a shape: should Max lose its five-hour window, this is
the line that changes. A record that vouches for the store comes first, which
is ShownPlan's order.
*/
func observedPlan(name string, creds *claude.Credentials) string {
	if creds.Plan != "max" {
		return ""
	}
	entry, err := usage.Read(name, usage.ProviderClaude)
	if err != nil || entry == nil || !usage.ClaudeWeekOnly(entry.Data) {
		return ""
	}
	return "ultra"
}

// accountLabel is what a caption calls an account: its own label, or the
// label of the name its provider gives it on screen.
func accountLabel(a usage.Account, states map[string]providerState) string {
	if name, ok := states[a.Provider].names[a.Name]; ok {
		return usage.Account{Provider: a.Provider, Name: name}.Label()
	}
	return a.Label()
}

// merge adds an account for every one a provider announces that the cache does
// not already know, so a profile that has just been logged in appears before
// anything has written a cache file for it: providers in the table's order,
// each one's accounts by name.
func merge(accounts []usage.Account, states map[string]providerState) []usage.Account {
	have := map[usage.Account]bool{}
	for _, a := range accounts {
		have[a] = true
	}
	for _, p := range usageProviders {
		names := append([]string(nil), states[p.name].announced...)
		sort.Strings(names)
		for _, name := range names {
			if a := (usage.Account{Provider: p.name, Name: name}); !have[a] {
				accounts = append(accounts, a)
			}
		}
	}
	return accounts
}

/*
usageProvider is how this program fetches one provider's accounts, as data
(spec 045). What the cache calls a provider, its label and its decoder are
usage.Spec's; this is the part that needs credentials and a network, which the
cache package keeps out of.

Adding a provider is an entry here and one in usage's table, beside its fetch
and decode code; nothing else names it.
*/
type usageProvider struct {
	name string

	// load reads what this machine holds for the provider, once per poll.
	load func() providerState
}

// providerState is one provider's credentials as one poll found them.
type providerState struct {
	// announced are accounts drawn before the cache has a file for them: a
	// profile that has just been logged in.
	announced []string

	// badges are the plan letters by account name.
	badges map[string]string

	// names are what accounts are called on screen, by the name the cache
	// gives them, where the two differ.
	names map[string]string

	// fetcher is how one account is fetched; nil, or a nil result, is an
	// account read and not fetched, which is what one with no credentials is.
	fetcher func(ctx context.Context, name string) usage.Fetch
}

// usageProviders are the providers this program fetches for, in usage's table
// order.
var usageProviders = []usageProvider{
	{name: usage.ProviderClaude, load: loadClaude},
	{name: usage.ProviderCodex, load: loadCodex},
}

// loadProviders is every provider's state for one poll.
func loadProviders() map[string]providerState {
	out := make(map[string]providerState, len(usageProviders))
	for _, p := range usageProviders {
		out[p.name] = p.load()
	}
	return out
}

// loadClaude finds the credential stores: each is an account, announced,
// badged with its plan, and fetched with its own token.
func loadClaude() providerState {
	stores, badges, names := claudeStores()
	announced := make([]string, 0, len(stores))
	for name := range stores {
		announced = append(announced, name)
	}
	return providerState{
		announced: announced,
		badges:    badges,
		names:     names,
		fetcher: func(ctx context.Context, name string) usage.Fetch {
			store, ok := stores[name]
			if !ok {
				return nil
			}
			return fetchClaude(ctx, store)
		},
	}
}

// loadCodex announces nothing: Codex has one account, and the cache names it.
// It is fetched through the app-server when there is one to ask.
func loadCodex() providerState {
	return providerState{
		fetcher: func(ctx context.Context, _ string) usage.Fetch {
			if !codexInstalled() {
				return nil
			}
			return fetchCodex(ctx)
		},
	}
}

// codexInstalled reports whether there is an app-server to ask. A machine
// without Codex is not a machine with a problem.
var codexInstalled = sync.OnceValue(func() bool {
	_, err := exec.LookPath("codex")
	return err == nil
})

// Details are every account's breakdown as one detail, which is what the
// card opens (spec 062 R9), and then each account's own, by the label its
// meter carries.
func (u *Usage) Details() []view.DetailInfo {
	accounts := u.accounts()
	if len(accounts) == 0 {
		return nil
	}
	out := []view.DetailInfo{{ID: view.DetailAccounts, Title: view.AccountsTitle}}
	for _, account := range accounts {
		out = append(out, view.DetailInfo{ID: account, Title: account})
	}
	return out
}

// accounts are the labels of the accounts the last reading had, in the
// meters' order.
func (u *Usage) accounts() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	for _, w := range u.windows {
		if !seen[w.Account] {
			seen[w.Account] = true
			out = append(out, w.Account)
		}
	}
	return out
}

// DetailInterval is UsageInterval: a breakdown is the same cache entry the
// meters are read from.
func (u *Usage) DetailInterval(string) time.Duration { return UsageInterval }

// CloseDetail has nothing to let go of.
func (u *Usage) CloseDetail(string) {}

/*
PollDetail reads one account's breakdown out of the cache, or every
account's. An account that cannot be read is a part with its reason in it,
and the others are whole; the error is every account's that failed.

**It reads and never fetches.** The meters' poll is what goes upstream, behind
the cache's gate; this decodes more of the entry that poll left, so opening a
drawer costs no request and writes nothing (spec 060 R14).
*/
func (u *Usage) PollDetail(ctx context.Context, id string) (view.Detail, error) {
	if id == view.DetailAccounts {
		var parts []view.Detail
		var failed []error
		for _, account := range u.accounts() {
			part, err := u.PollDetail(ctx, account)
			if err != nil {
				failed = append(failed, err)
			}
			parts = append(parts, part)
		}
		return view.UsageAccounts(parts), errors.Join(failed...)
	}
	b, err := u.breakdown(id)
	if err != nil {
		return unread(u.Key(), id, id, err), err
	}
	return view.UsageDetail(u.now(), b), nil
}

// cachedBreakdown is the breakdown of the account labelled label, from its
// cache entry.
func cachedBreakdown(now func() time.Time) func(label string) (view.UsageBreakdown, error) {
	return func(label string) (view.UsageBreakdown, error) {
		accounts, err := usage.Accounts()
		if err != nil {
			return view.UsageBreakdown{}, err
		}
		_, _, names := claudeStores()
		states := map[string]providerState{usage.ProviderClaude: {names: names}}
		for _, a := range accounts {
			if accountLabel(a, states) != label {
				continue
			}
			entry, err := usage.Read(a.Name, a.Provider)
			if err != nil {
				return view.UsageBreakdown{}, err
			}
			if entry == nil {
				break
			}
			served := usage.Served(*entry)
			b, err := usage.Lookup(a.Provider).Breakdown(now(), served.Data)
			if err != nil {
				return view.UsageBreakdown{}, err
			}
			// The drawer drops what the card drops (spec 089 R5).
			b.Limits = slices.DeleteFunc(b.Limits, func(l usage.Limit) bool {
				return turnedOver(now(), served.FetchedAt, l.ResetsAt)
			})
			return breakdownView(label, b), nil
		}
		return view.UsageBreakdown{}, fmt.Errorf("nothing in the usage cache for %s", label)
	}
}

// breakdownView is a breakdown as the view takes it: the same parts, in the
// plain values of the view.
func breakdownView(label string, b usage.Breakdown) view.UsageBreakdown {
	out := view.UsageBreakdown{Account: label, Plan: b.Plan, SharesTitle: b.SharesTitle}
	for _, l := range b.Limits {
		out.Limits = append(out.Limits, view.UsageLimit{
			Name: l.Name, Fraction: l.Fraction, ResetsAt: l.ResetsAt, Severity: l.Severity, Amount: l.Amount,
		})
	}
	for _, s := range b.Shares {
		out.Shares = append(out.Shares, view.UsageShare{Name: s.Name, Fraction: s.Fraction})
	}
	for _, f := range b.Facts {
		out.Facts = append(out.Facts, view.UsageFact{Name: f.Name, Value: f.Value})
	}
	return out
}
