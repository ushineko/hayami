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
// The bar is the window nearest its limit, because that is the one that can
// bite you today. The caption carries every window's figure in order, so
// nothing is lost — only the five other bars.
func Usage(now time.Time, windows []UsageWindow, fetchedAt time.Time) Section {
	s := Section{Key: "usage", Title: "Usage"}

	for _, group := range byAccount(windows) {
		lead := nearest(group)
		s.Meters = append(s.Meters, Meter{
			Label:    group[0].Account,
			Badge:    group[0].Badge,
			Window:   lead.Name,
			Caption:  figures(now, group),
			Reset:    resets(now, soonest(group)),
			Fraction: lead.Fraction,
			Status:   quota(lead.Fraction),
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

// nearest is the window closest to its limit: the one the bar shows.
func nearest(group []UsageWindow) UsageWindow {
	lead := group[0]
	for _, w := range group[1:] {
		if w.Fraction > lead.Fraction {
			lead = w
		}
	}
	return lead
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
func figures(now time.Time, group []UsageWindow) string {
	next := soonest(group)

	out := ""
	for _, w := range group {
		if out != "" {
			out += "  "
		}
		out += w.Name + ": " + strings.TrimSpace(Percent(w.Fraction))
		if w.Detail != "" {
			out += " " + w.Detail
		}
		if left := remaining(now, w, next); left != "" {
			out += " (" + left + ")"
		}
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
