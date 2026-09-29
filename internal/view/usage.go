package view

import (
	"strings"
	"time"
)

// UsageWindow is one quota, already measured and not yet formatted. It mirrors
// what the cache decodes to without importing it: the view takes plain values.
type UsageWindow struct {
	// Account is what the caption calls the owner: "max", "Codex".
	Account string

	// Badge is the one letter that says which plan an account is on. Empty
	// where the plan is unknown, which is better than a wrong letter.
	Badge string

	// Name is the window: "5h", "7d", "limit".
	Name string

	Fraction float64
	ResetsAt time.Time

	// Detail is anything the bar cannot carry, such as a limit's used and
	// limit values.
	Detail string

	// Span is how long the window is, and is what decides which window gets
	// the bar. Zero means the provider did not say, which is read as longer
	// than any window that did.
	Span time.Duration
}

// UsageStale is how old a reading may be before its age is worth saying.
//
// Five minutes, which is longer than any pane's poll and shorter than the
// shortest quota window. Below it the age is noise; above it, a reader
// deserves to know the numbers are not live.
const UsageStale = 5 * time.Minute

// Usage turns windows into a section of meters.
//
// **One meter per account, not per window.** An account has two or three
// windows and a panel is 260 px wide; a meter each turns two accounts and
// Codex into six bars and twice the height. The monitor this comes from gives
// an account one line, and putting the two side by side is what settled it.
//
// **The bar is the shortest window**, because that is the one that can stop
// work this afternoon. The caption carries every window's figure in order, so
// nothing is lost — only the five other bars.
//
// It used to be the window furthest along, which sounds like the same thing
// and is not: an account three quarters through its week and a tenth of the
// way through its five hours put the week on the bar, and the week is not what
// runs out at four o'clock. Read down a panel of three accounts and the old
// rule gave three bars about three different windows — seven days, a monthly
// spend, a Business limit — which is not a column anybody can compare.
func Usage(now time.Time, windows []UsageWindow, fetchedAt time.Time) Section {
	s := Section{Key: "usage", Title: "Usage", Icon: IconUsage}

	for _, group := range byAccount(windows) {
		lead := leading(group)
		left, right := spread(now, group, lead)
		s.Meters = append(s.Meters, Meter{
			Label:      group[0].Account,
			Badge:      group[0].Badge,
			Window:     lead.Name,
			Caption:    figure(now, lead, soonest(group)),
			StatsLeft:  left,
			StatsRight: right,
			Reset:      resets(now, soonest(group)),
			Fraction:   lead.Fraction,
			Status:     quota(lead.Fraction),
		})
	}

	if age := now.Sub(fetchedAt); !fetchedAt.IsZero() && age > UsageStale {
		s.Rows = append(s.Rows, Row{
			Label:  "read",
			Value:  ago(age),
			Status: Info,
		})
	}
	return s
}

// byAccount groups windows by the account they belong to, keeping the order
// they arrived in so the panel's rows do not change places.
func byAccount(windows []UsageWindow) [][]UsageWindow {
	var out [][]UsageWindow
	index := map[string]int{}
	for _, w := range windows {
		at, seen := index[w.Account]
		if !seen {
			index[w.Account] = len(out)
			out = append(out, []UsageWindow{w})
			continue
		}
		out[at] = append(out[at], w)
	}
	return out
}

// leading is the window the bar shows: the shortest the account has.
//
// "The shortest there is" rather than "the five-hour one", because an account
// may not have one — a Team account reports a monthly spend and no windows at
// all — and a bar is better about the longest window than about nothing. A
// window with no stated length sorts last for the same reason: an allowance
// with no period is not a window that turns over this afternoon.
func leading(group []UsageWindow) UsageWindow {
	lead := group[0]
	for _, w := range group[1:] {
		if shorter(w, lead) {
			lead = w
		}
	}
	return lead
}

// shorter reports whether a is the better candidate for the bar than b.
func shorter(a, b UsageWindow) bool {
	switch {
	case a.Span == b.Span:
		return false
	case a.Span == 0:
		return false
	case b.Span == 0:
		return true
	default:
		return a.Span < b.Span
	}
}

// figures is every window in the group: its name, its figure, and — for a
// window whose own reset is not the one in the Reset column — how long that
// window has left.
//
// One reset goes in the column and it is the soonest, so a seven-day window
// sitting beside a five-hour one would otherwise say nothing about when it
// turns over. The monitor writes that as "(5d left)" and it is the answer to a
// question a reader does actually ask: the five-hour window resets today
// whatever happens, and the weekly one is the one worth planning around.
/*
spread splits an account's other figures across the two ends of the stats row.

The caption keeps the window the bar is about — the one nearest its limit, the
one that can bite you today. Everything else goes below it: the other windows'
percentages at the left, and the lead window's own amounts at the right, which
is where the archetype puts them.

This is the whole of the width fix. The same figures in one caption made the
window 655 px wide; across a row with a stretch in the middle they cost the
width of the longest pair.
*/
func spread(now time.Time, group []UsageWindow, lead UsageWindow) (left, right string) {
	next := soonest(group)
	for _, w := range group {
		if w.Name == lead.Name {
			continue
		}
		if left != "" {
			left += "  "
		}
		left += figure(now, w, next)
	}
	return left, strings.TrimSpace(lead.Detail)
}

// figure is one window's name, its percentage, and how long it has left when
// that is not what the countdown column already says.
//
// The parenthetical is spec 004's and stays: a window whose reset is not the
// one in the countdown would otherwise say nothing about when it turns over,
// and the weekly window is the one worth planning around.
func figure(now time.Time, w UsageWindow, next time.Time) string {
	out := w.Name + ": " + strings.TrimSpace(Percent(w.Fraction))
	if left := remaining(now, w, next); left != "" {
		out += " (" + left + ")"
	}
	return out
}

// remaining is how long a window has left, for a window whose reset is not the
// one already in the Reset column and is far enough away to be worth saying.
//
// Days only. A window that turns over this afternoon is covered by the
// countdown in the column; one that turns over next Tuesday is what this is
// for, and an hour of precision on it would be false.
func remaining(now time.Time, w UsageWindow, shown time.Time) string {
	if w.ResetsAt.IsZero() || w.ResetsAt.Equal(shown) {
		return ""
	}
	days := int(w.ResetsAt.Sub(now).Hours()) / 24
	if days < 1 {
		return ""
	}
	return itoa(days) + "d left"
}

// soonest is the next reset among the group's windows, ignoring the ones the
// provider said nothing about.
func soonest(group []UsageWindow) time.Time {
	var out time.Time
	for _, w := range group {
		if w.ResetsAt.IsZero() {
			continue
		}
		if out.IsZero() || w.ResetsAt.Before(out) {
			out = w.ResetsAt
		}
	}
	return out
}

// resets is when the window starts again: a countdown for a short one, a date
// for a long one, and a fixed-width blank when the provider did not say.
func resets(now, at time.Time) string { return Resets(now, at) }

// quota is the verdict on a proportion of something with a limit.
//
// A quota is one of the few readings with a true threshold, so its colour is a
// signal rather than decoration. The bands are the monitor's: amber at four
// fifths, red at nineteen twentieths.
func quota(fraction float64) Status {
	switch {
	case fraction >= 0.95:
		return Bad
	case fraction >= 0.80:
		return Warn
	default:
		return Good
	}
}

// ago is how old a reading is, at a fixed width.
func ago(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return pad(itoa(int(d.Hours())/24) + "d ago")
	case d >= time.Hour:
		return pad(itoa(int(d.Hours())) + "h ago")
	default:
		return pad(itoa(int(d.Minutes())) + "m ago")
	}
}

// itoa is strconv.Itoa, named here to keep the import list of this file to the
// one thing it is about.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
