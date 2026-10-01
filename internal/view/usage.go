package view

import (
	"fmt"
	"strings"
	"time"
)

// UsageWindow is one quota, already measured and not yet formatted. It mirrors
// what the cache decodes to without importing it: the view takes plain values.
type UsageWindow struct {
	// Account is what the caption calls the owner: "CC max", "CX".
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

	// Used and Limit are Detail's two amounts apart, for the pane, which
	// arranges them the way the widget it replaces does. Severity is a
	// spend's own verdict from the provider; empty for everything else.
	Used     string
	Limit    string
	Severity string

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
			Caption:    caption(now, lead, soonest(group)),
			StatsLeft:  left,
			StatsRight: right,
			Reset:      resets(now, soonest(group)),
			Strip:      strip(now, group, lead),
			Fraction:   lead.Fraction,
			Status:     verdict(lead),
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

// caption is the bar's own window: its name, its percentage, and how long it
// has left when that is not what the countdown column already says.
//
// No amounts, even where the window has them: the lead window's amounts are
// its Detail, at the right of the stats row, and a caption is fixed width.
//
// The parenthetical is spec 004's and stays: a window whose reset is not the
// one in the countdown would otherwise say nothing about when it turns over,
// and the weekly window is the one worth planning around.
func caption(now time.Time, w UsageWindow, next time.Time) string {
	return lasting(now, w, next, w.Name+": "+strings.TrimSpace(Percent(w.Fraction)))
}

// figure is one of an account's other windows in the stats row: the caption's
// form, with the window's amounts where it carries them (spec 023), so a
// Business limit reads "limit: 974.28 / 1200 (81 %)" as the pane's
// "individual 974.28/1200 (81%)" does. The amounts are Used and Limit, the
// widget's compact form the pane prints, not Detail's two decimals.
func figure(now time.Time, w UsageWindow, next time.Time) string {
	if w.Used == "" || w.Limit == "" {
		return caption(now, w, next)
	}
	pct := strings.TrimSpace(Percent(w.Fraction))
	return lasting(now, w, next, w.Name+": "+w.Used+" / "+w.Limit+" ("+pct+")")
}

// lasting is a figure followed by how long its window has left, when there is
// something to say.
func lasting(now time.Time, w UsageWindow, next time.Time, out string) string {
	if left := remaining(now, w, next); left != "" {
		out += " (" + left + ")"
	}
	return out
}

// remaining is how long a window has left, for a window whose reset is not the
// one already in the Reset column.
//
// Days only, rounded up, and "<1d" on the last day, which is the widget's
// arithmetic (issue #102): a window that turns over in thirty-six hours has
// two days left the way a reader counts them, and one that turns over this
// evening has not stopped having a reset. The first version floored and said
// nothing under a day, so the weekly window went silent for its last day and
// read a day short before that. An hour of precision on next Tuesday would be
// false; a day of silence on the day itself was worse.
func remaining(now time.Time, w UsageWindow, shown time.Time) string {
	if w.ResetsAt.IsZero() || w.ResetsAt.Equal(shown) {
		return ""
	}
	return daysLeft(now, w.ResetsAt)
}

// daysLeft is a reset as a count of days, the widget's way: rounded up, "<1d"
// under a day, nothing once it has passed.
func daysLeft(now, at time.Time) string {
	d := at.Sub(now)
	if d <= 0 {
		return ""
	}
	if d < 24*time.Hour {
		return "<1d left"
	}
	days := int(d / (24 * time.Hour))
	if d%(24*time.Hour) != 0 {
		days++
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
// signal rather than decoration. The bands are the usage widget's: amber from
// half, red past four fifths. They were the battery monitor's 80/95 until the
// pane was set beside the widget it replaces (issue #75) and the same 60 %
// was amber in one and green in the other; the widget is the program a reader
// of these numbers has been looking at.
func quota(fraction float64) Status {
	switch {
	case fraction > 0.80:
		return Bad
	case fraction >= 0.50:
		return Warn
	default:
		return Good
	}
}

// verdict is a window's colour: the provider's own severity where it gave one,
// which only a spend does, and the quota bands otherwise.
//
// The severity wins because the API decides what counts as concerning for a
// budget, and the widget honours it. An unrecognised severity falls back to
// the bands rather than to no colour.
func verdict(w UsageWindow) Status {
	switch w.Severity {
	case "normal":
		return Good
	case "warning", "elevated":
		return Warn
	case "critical", "exceeded":
		return Bad
	}
	return quota(w.Fraction)
}

/*
strip is an account as the pane's row draws it, in the words of the widget it
replaces: "12%  ·  7d 40%", "$250.00 / $1000.00 (25%)",
"0%  ·  individual 300.5/1200 (60%)", and "resets 2h 30m" at the right.

A separate form from the caption because the two are read differently. The
window has a card with room under the bar and states which window each figure
is; a pane has one line and a reader who has looked at the widget's for
months, and the bar's own window is already named in the column beside it.

Each figure carries its own verdict and the separators are dim, so a seven-day
window close to its limit is red in a line whose bar is green.

**Not fixed width.** The widget prints "7%", and the pane matches it; a
figure that gains a digit moves the bar's end by a column. That is a choice
made for this form only (issue #75): the window's captions stay fixed width.
*/
func strip(now time.Time, group []UsageWindow, lead UsageWindow) *Strip {
	out := &Strip{Window: lead.Name, Reset: resetsIn(now, lead)}
	if lead.Name == "spend" {
		// A budget is not a window, and the widget leaves the column blank
		// rather than naming it.
		out.Window = ""
		out.Figures = []Figure{{Text: budget(lead), Status: verdict(lead)}}
		return out
	}

	out.Figures = []Figure{{Text: percent(lead.Fraction), Status: verdict(lead)}}
	for _, w := range group {
		if w.Name == lead.Name {
			continue
		}
		switch w.Name {
		case "spend":
			// Beside a plan's windows, a spend is only its amount, and
			// only when something has been spent: a fresh month looks the
			// way it did before there was a budget at all.
			if w.Used == "" || nothing(w.Used) {
				continue
			}
			out.Figures = append(out.Figures, Figure{Text: stripSeparator, Status: Dim},
				Figure{Text: w.Used, Status: verdict(w)})
		case "limit":
			out.Figures = append(out.Figures, Figure{Text: stripSeparator, Status: Dim},
				Figure{Text: individual(w), Status: verdict(w)})
		default:
			// The widget writes "7d 85% (2d left)": the figure, then how
			// long the window has left, which the reset column at the
			// edge does not say because that column is the bar's window.
			// Nothing when the two reset together, as in the card.
			out.Figures = append(out.Figures, Figure{Text: stripSeparator + w.Name + " ", Status: Dim},
				Figure{Text: percent(w.Fraction), Status: verdict(w)})
			if left := remaining(now, w, lead.ResetsAt); left != "" {
				out.Figures = append(out.Figures, Figure{Text: " (" + left + ")", Status: Dim})
			}
		}
	}
	return out
}

// stripSeparator is what goes between two figures in a pane.
const stripSeparator = "  ·  "

// percent is a percentage the widget's way: no padding and no space before
// the sign.
func percent(fraction float64) string {
	return fmt.Sprintf("%.0f%%", fraction*100)
}

// budget is a spend with its cap: "$250.00 / $1000.00 (25%)".
func budget(w UsageWindow) string {
	if w.Used == "" || w.Limit == "" {
		return percent(w.Fraction)
	}
	return w.Used + " / " + w.Limit + " (" + percent(w.Fraction) + ")"
}

// individual is a Codex Business account's own allowance:
// "individual 300.5/1200 (60%)".
func individual(w UsageWindow) string {
	if w.Used == "" || w.Limit == "" {
		return "individual " + percent(w.Fraction)
	}
	return "individual " + w.Used + "/" + w.Limit + " (" + percent(w.Fraction) + ")"
}

// nothing reports whether an amount is zero, whatever its symbol or decimals.
func nothing(amount string) bool { return !strings.ContainsAny(amount, "123456789") }

// resetsIn is the pane's reset column, the widget's way: a countdown for a
// window with a length ("resets 2h 30m", "resets 3d 4h"), a date for an
// allowance without one ("resets Oct 1"), and nothing where the provider did
// not say.
func resetsIn(now time.Time, w UsageWindow) string {
	if w.ResetsAt.IsZero() {
		return ""
	}
	if w.Span == 0 {
		return "resets " + w.ResetsAt.Format("Jan 2")
	}
	d := w.ResetsAt.Sub(now)
	if d <= 0 {
		return "resets now"
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h >= 24:
		return fmt.Sprintf("resets %dd %dh", h/24, h%24)
	case h > 0:
		return fmt.Sprintf("resets %dh %dm", h, m)
	default:
		return fmt.Sprintf("resets %dm", m)
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
