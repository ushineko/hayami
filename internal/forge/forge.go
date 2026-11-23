/*
Package forge reads a person's own activity on the code forges they use,
GitHub and GitLab, over the last fortnight (spec 102): how much happened each
day, and what.

It is headless, as every reader in hayami is: a reading is plain values, and
what a day looks like on a card is the view's. It reaches only the forges'
APIs, over HTTPS, through NewClient, with a credential found by the functions
in creds.go and held for the request and no longer.
*/
package forge

import (
	"errors"
	"fmt"
	"time"
)

// Days is how many days a reading covers, today the last of them.
const Days = 14

// The providers, as the settings and the detail IDs name them.
const (
	GitHub = "github"
	GitLab = "gitlab"
)

// Activity is one reading of one forge.
type Activity struct {
	Provider string `json:"provider"`

	// Days are the fortnight's counts, oldest first, today last.
	Days []Day `json:"days"`

	// Items are what happened, each on its day. A day's count is its
	// items' (spec 109): what a forge counts and will not name -- GitHub's
	// contributions in an organisation's repositories this sign-in cannot
	// see -- is not the person's to read here.
	Items []Item `json:"items,omitempty"`

	// Unit names what a day's count counts: "contributions", "events".
	Unit string `json:"unit"`

	ReadAt time.Time `json:"read_at"`
}

// Day is one day's count.
type Day struct {
	Date  string `json:"date"` // YYYY-MM-DD, the desk's day
	Count int    `json:"count"`
}

// Kind is what an item is.
type Kind string

// The kinds of item.
const (
	KindCommits Kind = "commits" // a day's commits to a repository, counted
	KindPushed  Kind = "pushed"  // a push to a branch
	KindPR      Kind = "pr"      // a GitHub pull request
	KindMR      Kind = "mr"      // a GitLab merge request
	KindIssue   Kind = "issue"
	KindReview  Kind = "review"
	KindComment Kind = "comment"
	KindOther   Kind = "other"
)

// Item is one thing done on a day.
type Item struct {
	Date string `json:"date"`
	// Repo is the repository's path, owner included; the view drops the
	// owner, which is a user name (rule 9).
	Repo string `json:"repo"`
	Kind Kind   `json:"kind"`
	// Ref is "#12", "!34", or a branch's name for a push.
	Ref   string `json:"ref,omitempty"`
	Title string `json:"title,omitempty"`
	// Action is what was done ("opened", "merged", "approved", "pushed"),
	// or for a GitHub contribution the thing's state now.
	Action string `json:"action,omitempty"`
	Count  int    `json:"count"`

	// From, To and Commits are a GitLab push's: the commits before and
	// after it and how many it carried, which its page reads (spec 111).
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Commits int    `json:"commits,omitempty"`
}

// DateOf is a moment's day on the desk.
func DateOf(t time.Time) string { return t.Local().Format(time.DateOnly) }

// Window is the fortnight ending today: its first day's start and the
// dates, oldest first.
func Window(now time.Time) (time.Time, []string) {
	y, m, d := now.Local().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	start := today.AddDate(0, 0, -(Days - 1))
	dates := make([]string, Days)
	for i := range dates {
		dates[i] = start.AddDate(0, 0, i).Format(time.DateOnly)
	}
	return start, dates
}

// countItems is the window's days counted from the items, for a forge whose
// count is its events.
func countItems(dates []string, items []Item) []Day {
	at := map[string]int{}
	for _, it := range items {
		at[it.Date] += it.Count
	}
	out := make([]Day, len(dates))
	for i, d := range dates {
		out[i] = Day{Date: d, Count: at[d]}
	}
	return out
}

// ErrNoCredential is a forge with no credential to read it with.
var ErrNoCredential = errors.New("no credential")

// ErrUnauthorized is a forge that refused the credential.
var ErrUnauthorized = errors.New("the forge refused the credential")

// RateLimited is a forge that will take no more requests until Until.
type RateLimited struct{ Until time.Time }

func (e *RateLimited) Error() string {
	if e.Until.IsZero() {
		return "rate limited"
	}
	return fmt.Sprintf("rate limited until %s", e.Until.Local().Format("15:04"))
}
