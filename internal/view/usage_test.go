package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

func at(h, m int) time.Time {
	return time.Date(2026, time.September, 27, h, m, 0, 0, time.UTC)
}

// A meter is the one shape here that is a bar, and it has to survive all three
// arrangements: a pane shows it as a line, a card shows it as a block.
func TestAMeterRendersInAllThreeArrangements(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "5h", Caption: " 48 % · in   4h 32m", Fraction: 0.48},
	}}

	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		lines := view.Render([]view.Section{s}, a, 60)
		joined := strings.Join(lines, "\n")

		require.NotEmpty(t, lines, "%s drew nothing", a)
		assert.Contains(t, joined, "48 %", "%s lost the caption", a)
		assert.Contains(t, joined, string(view.BarFull), "%s drew no bar", a)
	}
}

// In a pane the bar takes whatever width is left, which is the shape the
// widget being replaced draws and the reason the row arrangement exists.
func TestInARowTheBarTakesTheWidthThatIsLeft(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "5h", Caption: " 50 %", Fraction: 0.5},
	}}

	narrow := view.Render([]view.Section{s}, view.ArrangeRow, 40)[0]
	wide := view.Render([]view.Section{s}, view.ArrangeRow, 100)[0]

	assert.Equal(t, 40, len([]rune(narrow)))
	assert.Equal(t, 100, len([]rune(wide)))
	assert.Greater(t, strings.Count(wide, string(view.BarFull)),
		strings.Count(narrow, string(view.BarFull)),
		"the bar did not grow with the pane")
}

// A pane too narrow for a bar worth drawing keeps the number. The caption is
// the reading and the bar is the impression.
func TestAPaneTooNarrowForABarKeepsTheNumber(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "5h", Caption: " 50 %", Fraction: 0.5},
	}}

	line := view.Render([]view.Section{s}, view.ArrangeRow, 20)[0]

	assert.Contains(t, line, "50 %")
}

// The caption sets the section's width, so it is held to the same rule as any
// other changing value.
func TestACaptionDoesNotChangeWidthWithItsNumbers(t *testing.T) {
	widths := map[int]bool{}
	for _, f := range []float64{0, 0.05, 0.5, 1} {
		widths[len([]rune(view.Percent(f)))] = true
	}
	assert.Len(t, widths, 1, "a percentage changed width between 0 and 100")

	now := at(9, 0)
	countdowns := map[int]bool{}
	for _, then := range []time.Time{at(9, 30), at(11, 0), now.Add(50 * time.Hour), now.Add(-time.Hour)} {
		countdowns[len([]rune(view.Until(now, then)))] = true
	}
	assert.Len(t, countdowns, 1, "a countdown changed width between minutes, hours and days")
	assert.Equal(t, len([]rune(view.Until(now, at(11, 0)))), len([]rune(view.NoUntil())),
		"a countdown that has not arrived is a different width from one that has")
}

// An account is one meter, not one per window. Two accounts and Codex would
// otherwise be six bars and twice the height, in a panel 260 px wide.
func TestAnAccountIsOneMeterHoweverManyWindowsItHas(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "max", Name: "5h", Fraction: 0.10, ResetsAt: at(11, 0)},
		{Account: "max", Name: "7d", Fraction: 0.85, ResetsAt: at(12, 0)},
		{Account: "Codex", Name: "5h", Fraction: 0.20, ResetsAt: at(11, 0)},
	}, at(9, 0))

	require.Len(t, s.Meters, 2)
	assert.Equal(t, "max", s.Meters[0].Label)
	assert.Equal(t, "Codex", s.Meters[1].Label)
}

// The bar shows the shortest window, and it does so even when another window
// is much further along -- which is the whole of the change, because the rule
// it replaces would have picked the other one here.
//
// Every window's figure is still there, so nothing is lost but five bars. The
// caption carries the window the bar is about and the rest go in the stats
// row, because a caption carrying all of them sets the width of the whole
// window. Line() is every figure in reading order, which is what a pane draws
// and what this asserts: the claim is that nothing was lost, not where it
// went.
func TestTheBarShowsTheShortestWindowAndNothingIsLost(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.10, ResetsAt: at(11, 0)},
		{Account: "max", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.85, ResetsAt: at(12, 0)},
	}, at(9, 0))

	require.Len(t, s.Meters, 1)
	assert.InDelta(t, 0.10, s.Meters[0].Fraction, 0.001,
		"the bar is the five-hour window, not the one furthest along")
	assert.Contains(t, s.Meters[0].Line(), "5h: 10 %")
	assert.Contains(t, s.Meters[0].Line(), "7d: 85 %")
	assert.Equal(t, "5h", s.Meters[0].Window, "the name says which window the bar is about")

	// The caption is only the window the bar is about. The other one is
	// below it, which is what keeps the meter narrow.
	assert.Contains(t, s.Meters[0].Caption, "5h: 10 %")
	assert.NotContains(t, s.Meters[0].Caption, "7d: 85 %")
	assert.Contains(t, s.Meters[0].StatsLeft, "7d: 85 %")
}

// The order the windows arrive in is not the order they are ranked in. The
// provider decides the first, and a reader who saw the bar change meaning
// because a payload put its windows the other way round would be right to
// call it a bug.
func TestTheOrderTheWindowsArriveInDoesNotDecideTheBar(t *testing.T) {
	for _, windows := range [][]view.UsageWindow{
		{
			{Account: "max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.10, ResetsAt: at(11, 0)},
			{Account: "max", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.85, ResetsAt: at(12, 0)},
		},
		{
			{Account: "max", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.85, ResetsAt: at(12, 0)},
			{Account: "max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.10, ResetsAt: at(11, 0)},
		},
	} {
		s := view.Usage(at(9, 0), windows, at(9, 0))

		require.Len(t, s.Meters, 1)
		assert.Equal(t, "5h", s.Meters[0].Window)
	}
}

// A window with no stated length sorts last, because an allowance with no
// period is not something that turns over this afternoon. Codex reports its
// Business limit this way and the monthly spend has no window at all.
func TestAWindowWithNoLengthDoesNotTakeTheBarFromOneThatHasOne(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "Codex", Name: "limit", Fraction: 0.34, ResetsAt: at(11, 0)},
		{Account: "Codex", Name: "5h", Span: 5 * time.Hour, Fraction: 0.02, ResetsAt: at(11, 0)},
	}, at(9, 0))

	require.Len(t, s.Meters, 1)
	assert.Equal(t, "5h", s.Meters[0].Window)
	assert.InDelta(t, 0.02, s.Meters[0].Fraction, 0.001)
}

// An account whose only window has no length still gets a bar about it. A
// Team account reports a monthly spend and no windows at all, and a bar about
// the longest thing there is beats no bar.
func TestAnAccountWithOnlyAnUnboundedWindowStillGetsABar(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "work", Name: "spend", Fraction: 0.85, ResetsAt: at(12, 0)},
	}, at(9, 0))

	require.Len(t, s.Meters, 1)
	assert.Equal(t, "spend", s.Meters[0].Window)
	assert.InDelta(t, 0.85, s.Meters[0].Fraction, 0.001)
}

// A quota is one of the few readings with a true threshold, so its colour is a
// signal rather than decoration. The verdict follows the bar.
func TestAQuotaNearItsLimitIsMarked(t *testing.T) {
	verdict := func(f float64) view.Status {
		s := view.Usage(at(9, 0), []view.UsageWindow{
			{Account: "max", Name: "5h", Fraction: f, ResetsAt: at(11, 0)},
		}, at(9, 0))
		return s.Meters[0].Status
	}

	assert.Equal(t, view.Good, verdict(0.10))
	assert.Equal(t, view.Warn, verdict(0.85))
	assert.Equal(t, view.Bad, verdict(0.99))
}

// The badge comes from the credential file, and an account with none is drawn
// without one rather than with a guess.
func TestAnAccountsBadgeSitsBesideItsName(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "work", Badge: "E", Name: "spend", Fraction: 0.5, ResetsAt: at(11, 0)},
		{Account: "Codex", Name: "5h", Fraction: 0.1, ResetsAt: at(11, 0)},
	}, at(9, 0))

	require.Len(t, s.Meters, 2)
	assert.Equal(t, "work", s.Meters[0].Label)
	assert.Equal(t, "E", s.Meters[0].Badge)
	assert.Equal(t, "spend", s.Meters[0].Window)
	assert.Equal(t, "Codex", s.Meters[1].Label)
	assert.Empty(t, s.Meters[1].Badge, "an account with no plan gets no letter rather than a guess")
	assert.Equal(t, "work E spend", s.Meters[0].Name())
}

// Two questions, two forms. "How long have I got" for a window that ends
// today; "when does this start over" for one that ends next month.
func TestAShortWindowCountsDownAndALongOneNamesItsDate(t *testing.T) {
	now := at(9, 0)

	assert.Contains(t, view.Resets(now, now.Add(3*time.Hour)), "in")
	assert.Contains(t, view.Resets(now, now.Add(3*time.Hour)), "3h")

	october := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	assert.Contains(t, view.Resets(now, october), "1 Oct")
	assert.NotContains(t, view.Resets(now, october), "in ")

	assert.Equal(t, len([]rune(view.Resets(now, october))),
		len([]rune(view.Resets(now, now.Add(3*time.Hour)))),
		"a date and a countdown must be the same width")
}

// A reading older than any pane's poll should say so. A stale panel that looks
// live is worse than one that admits it.
func TestAStaleReadingSaysItsAge(t *testing.T) {
	now := at(12, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "max", Name: "5h", Fraction: 0.1, ResetsAt: at(13, 0)},
	}, now.Add(-3*time.Hour))

	require.Len(t, s.Rows, 1)
	assert.Contains(t, s.Rows[0].Value, "3h ago")
}

func TestAFreshReadingSaysNothingAboutItsAge(t *testing.T) {
	now := at(12, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "max", Name: "5h", Fraction: 0.1, ResetsAt: at(13, 0)},
	}, now.Add(-time.Minute))

	assert.Empty(t, s.Rows)
}

// A limit's own numbers are still drawn: the bar says "most of it" and the
// figures say how much of what. They sit at the right-hand end of the stats
// row, which is where the archetype puts them.
func TestAWindowsDetailIsDrawnBesideItsFigure(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "Codex", Name: "limit", Fraction: 0.34, ResetsAt: at(11, 0), Detail: "403.51 / 1200.00"},
	}, at(9, 0))

	require.Len(t, s.Meters, 1)
	assert.Equal(t, "403.51 / 1200.00", s.Meters[0].StatsRight)
	assert.Contains(t, s.Meters[0].Line(), "403.51 / 1200.00")
}

// The bar and the countdown answer different questions. The bar shows the
// shortest window; the countdown answers "when does anything here change",
// which is the next window to turn over whether or not it is the one the bar
// is about.
//
// They coincide most of the time -- the shortest window is usually the next
// to reset -- so the case worth a test is the one where they do not: a
// five-hour window that has just turned over and a weekly one about to.
func TestTheCountdownIsTheSoonestResetNotTheLeadingOnes(t *testing.T) {
	now := at(9, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.04, ResetsAt: now.Add(4 * time.Hour)},
		{Account: "max", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.20, ResetsAt: now.Add(2 * time.Hour)},
	}, now)

	require.Len(t, s.Meters, 1)
	assert.InDelta(t, 0.04, s.Meters[0].Fraction, 0.001, "the bar is the shortest window")
	assert.Contains(t, s.Meters[0].Reset, "2h", "the countdown is the next reset")
}

// One reset goes in the column and it is the soonest, so a seven-day window
// beside a five-hour one would otherwise say nothing about when it turns over.
// The five-hour one resets today whatever happens; the weekly one is the one
// worth planning around.
func TestALongerWindowSaysHowLongItHasLeft(t *testing.T) {
	now := at(9, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.04, ResetsAt: now.Add(3 * time.Hour)},
		{Account: "max", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.21,
			ResetsAt: now.Add(5*24*time.Hour + time.Hour)},
	}, now)

	require.Len(t, s.Meters, 1)
	assert.Contains(t, s.Meters[0].StatsLeft, "7d: 21 % (5d left)")
	assert.NotContains(t, s.Meters[0].Caption, "5h: 4 % (",
		"the window whose reset is already in the column repeats nothing")
	assert.Contains(t, s.Meters[0].Reset, "3h")
}

// A window that turns over this afternoon is covered by the countdown in the
// column; days are the only unit worth spending room on here.
func TestAWindowLessThanADayAwaySaysNothingExtra(t *testing.T) {
	now := at(9, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.04, ResetsAt: now.Add(2 * time.Hour)},
		{Account: "max", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.21, ResetsAt: now.Add(6 * time.Hour)},
	}, now)

	assert.NotContains(t, s.Meters[0].Line(), "left")
}
