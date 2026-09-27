package panel

import (
	"context"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/usage"
	"github.com/ushineko/hayami/internal/view"
)

// UsageInterval is how often the cache is re-read.
//
// Thirty seconds, and this is a *read*, not a request: the gate in the cache
// is what limits the upstream calls, and re-reading a local file more often
// than a quota can move would be spending a poll on nothing.
const UsageInterval = 30 * time.Second

// Usage draws what the shared cache holds.
//
// It does not fetch. The Python widget and monitor keep the cache fresh, and
// on a machine without them this section stays empty until hayami can fetch
// for itself — which is a separate spec because it means writing to a
// credential store.
type Usage struct {
	mu        sync.Mutex
	windows   []view.UsageWindow
	fetchedAt time.Time

	// now is time.Now, replaced by a test so a countdown can be asserted.
	now func() time.Time

	// read is usage.Accounts and usage.Windows, replaced by a test so the
	// real cache is never touched.
	read func() ([]view.UsageWindow, time.Time, error)
}

// NewUsage builds the usage source.
func NewUsage() *Usage {
	u := &Usage{now: time.Now}
	u.read = func() ([]view.UsageWindow, time.Time, error) { return readCache(u.now()) }
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

	windows, fetchedAt, err := u.read()
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

// readCache is every account the cache holds, flattened into windows.
//
// The oldest reading decides the age shown: a section is as stale as its
// stalest number, and reporting the freshest would say the panel is more
// current than it is.
func readCache(now time.Time) ([]view.UsageWindow, time.Time, error) {
	accounts, err := usage.Accounts()
	if err != nil {
		return nil, time.Time{}, err
	}

	var out []view.UsageWindow
	var oldest time.Time
	for _, a := range accounts {
		windows, result, err := usage.Windows(now, a)
		if err != nil {
			// One account's payload not decoding is one account, not the
			// section. The canary in internal/usage is what reports a format
			// change; a pane should still draw the accounts that do decode.
			continue
		}
		for _, w := range windows {
			out = append(out, view.UsageWindow{
				Account:  a.Label(),
				Name:     w.Name,
				Fraction: w.Fraction,
				ResetsAt: w.ResetsAt,
				Detail:   w.Detail,
			})
		}
		if !result.FetchedAt.IsZero() && (oldest.IsZero() || result.FetchedAt.Before(oldest)) {
			oldest = result.FetchedAt
		}
	}
	return out, oldest, nil
}
