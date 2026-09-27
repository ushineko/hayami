package view

import (
	"fmt"
	"strings"
	"time"
)

// Meter is a proportion of something that has a limit: a quota used, a window
// of time elapsed. It is the one shape here that is a bar.
//
// A reading with no limit is a Row. A temperature and a rate have nothing to
// fill up, and drawing one as a bar invents a maximum.
//
// The caption does the work the bar cannot. A bar says "most of it"; the
// caption says which window, how much of it, and when it starts again, which
// is what someone glancing at a pane actually wants. It is not decoration, and
// it is held to the no-jitter rule because it sets the section's width.
type Meter struct {
	Label   string
	Caption string

	// Reset is when the quota starts again, kept apart from the caption
	// because it is a column of its own: in a pane it sits hard against the
	// right edge, where the eye can find it on every line without reading the
	// figures first.
	Reset string

	Fraction float64
	Status   Status
}

// BarFull and BarEmpty are what a bar is drawn with.
//
// A rule, not a block. `█` on `░` is a slab of colour the width of the pane,
// and next to it the numbers — which are the reading — look like a caption.
// The program this replaces draws rich's ProgressBar, which is `━`, and beside
// the two panes the difference is not subtle: the same information, and one of
// them shouts.
//
// The two glyphs differ in weight as well as in colour, which rich's do not:
// its bar is one character in two colours, so in a pipe or under NO_COLOR it
// says nothing at all. Heavy against light survives both.
//
// Eighths would be smoother and are not used. A pane is read at a glance from
// across a desk, and a bar whose last cell is an eighth full reads as the same
// bar as one whose last cell is empty.
const (
	BarFull  = '━'
	BarEmpty = '─'
)

// MeterFraction clamps a fraction into the range a bar can draw. A bar wider
// than its track is a bar that has left the layout.
func MeterFraction(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// Percent formats a percentage at a fixed width, so a caption does not move
// when a number gains a digit.
//
// Three characters and a sign: "  5 %", " 48 %", "100 %". The space before the
// sign is the monitor's own spacing and is what makes a column of them line up.
func Percent(fraction float64) string {
	return fmt.Sprintf("%3.0f %%", MeterFraction(fraction)*100)
}

// NoPercent is a percentage that has not arrived, at the width one takes.
func NoPercent() string { return "  -- %" }

// UntilWidth is the width every countdown is padded to.
//
// Fixed because a countdown sits in a caption, and a caption that shrinks from
// two digits of hours to one drags the rest of the line with it. The widest
// form is "in 999d 23h"; everything shorter is padded to match, including the
// two that are words rather than numbers.
const UntilWidth = 11

// Until is a countdown at a fixed width: "in   4h 32m", "in   4d  8h".
//
// Past its time it says "now". A window whose reset has passed and whose
// numbers have not yet been re-read is ordinary, and it is the truth about
// what is on screen.
func Until(now, then time.Time) string {
	d := then.Sub(now)
	if d <= 0 {
		return padUntil("now")
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h >= 24 {
		return fmt.Sprintf("in %3dd %2dh", h/24, h%24)
	}
	return fmt.Sprintf("in %3dh %2dm", h, m)
}

// NoUntil is a countdown with no time to count to, at the width one takes.
func NoUntil() string { return padUntil("--") }

// LongWindow is where a countdown stops being the useful form.
//
// A day. "in 3d 23h" is arithmetic a reader has to do; "resets Oct 1" is the
// answer. Below a day the countdown is the answer and a date is not: nobody
// plans around "resets today".
const LongWindow = 24 * time.Hour

// Resets is when a window starts again, as a countdown for a short one and a
// date for a long one, both at the same width.
//
// The monitor draws the monthly credit cap as "Resets Oct 1" and the
// five-hour window as "Resets in 3h 4m", and it is right: the two questions
// are different. One is "how long have I got", the other is "when does this
// start over".
func Resets(now, then time.Time) string {
	if then.IsZero() {
		return NoUntil()
	}
	if then.Sub(now) >= LongWindow {
		return padUntil(then.Format("2 Jan"))
	}
	return Until(now, then)
}

// padUntil right-aligns a word in the countdown column, so it lines up under
// the numbers rather than beside them.
func padUntil(s string) string {
	if n := UntilWidth - runeLen(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}
