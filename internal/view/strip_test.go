package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// The figures of invented accounts, shaped like the three the usage widget's
// --tui draws: a plan with two windows, a budget, and Codex with a Business
// limit. None of them is anybody's usage.
func threeAccounts(now time.Time) []view.UsageWindow {
	october := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	return []view.UsageWindow{
		{Account: "CC max", Badge: "M", Name: "5h", Span: 5 * time.Hour, Fraction: 0.12,
			ResetsAt: now.Add(2*time.Hour + 55*time.Minute)},
		{Account: "CC max", Badge: "M", Name: "7d", Span: 7 * 24 * time.Hour, Fraction: 0.85,
			ResetsAt: now.Add(4 * 24 * time.Hour)},
		{Account: "CC work", Badge: "E", Name: "spend", Fraction: 0.25, ResetsAt: october,
			Used: "$250.00", Limit: "$1000.00", Severity: "normal"},
		{Account: "CX", Name: "5h", Span: 5 * time.Hour, Fraction: 0.03,
			ResetsAt: now.Add(4*time.Hour + 58*time.Minute)},
		{Account: "CX", Name: "limit", Fraction: 0.6, Used: "300.5", Limit: "1200"},
	}
}

// text is a strip's figures as the pane prints them.
func text(s *view.Strip) string {
	var b strings.Builder
	for _, f := range s.Figures {
		b.WriteString(f.Text)
	}
	return b.String()
}

// The pane says what the widget says, in the widget's words (issue #75): the
// bar's own percentage bare, every other window after a dim separator, and
// the reset in its own column.
func TestAStripSaysWhatTheWidgetSays(t *testing.T) {
	now := at(9, 0)
	s := view.Usage(now, threeAccounts(now), now)

	require.Len(t, s.Meters, 3)
	plan, budget, codex := s.Meters[0].Strip, s.Meters[1].Strip, s.Meters[2].Strip
	require.NotNil(t, plan)
	require.NotNil(t, budget)
	require.NotNil(t, codex)

	assert.Equal(t, "12%  ·  7d 85%", text(plan))
	assert.Equal(t, "5h", plan.Window)
	assert.Equal(t, "resets 2h 55m", plan.Reset)

	assert.Equal(t, "$250.00 / $1000.00 (25%)", text(budget))
	assert.Empty(t, budget.Window, "the widget names no window for a budget")
	assert.Equal(t, "resets Oct 1", budget.Reset)

	assert.Equal(t, "3%  ·  individual 300.5/1200 (60%)", text(codex))
	assert.Equal(t, "resets 4h 58m", codex.Reset)
}

// Each figure is coloured for itself, so a week near its limit is red on a line
// whose bar is green. The separators are furniture and are dim.
func TestEachFigureInAStripHasItsOwnColour(t *testing.T) {
	now := at(9, 0)
	s := view.Usage(now, threeAccounts(now), now)
	plan, codex := s.Meters[0].Strip, s.Meters[2].Strip

	require.Len(t, plan.Figures, 3)
	assert.Equal(t, view.Good, plan.Figures[0].Status, "12 % is green")
	assert.Equal(t, view.Dim, plan.Figures[1].Status, "the separator is dim")
	assert.Equal(t, view.Bad, plan.Figures[2].Status, "85 % is red")
	assert.Equal(t, view.Good, s.Meters[0].Status, "the bar is the five-hour window's colour")

	assert.Equal(t, view.Warn, codex.Figures[len(codex.Figures)-1].Status, "60 % is amber")
}

// A budget's colour is the provider's severity, not the bands: the API decides
// what counts as concerning for a spend, and the widget honours it.
func TestABudgetIsColouredByItsSeverity(t *testing.T) {
	now := at(9, 0)
	for severity, want := range map[string]view.Status{
		"normal": view.Good, "warning": view.Warn, "critical": view.Bad, "": view.Bad,
	} {
		s := view.Usage(now, []view.UsageWindow{
			{Account: "CC work", Name: "spend", Fraction: 0.9, Used: "$9.00", Limit: "$10.00",
				Severity: severity},
		}, now)

		assert.Equal(t, want, s.Meters[0].Status, "severity %q", severity)
		assert.Equal(t, want, s.Meters[0].Strip.Figures[0].Status, "severity %q", severity)
	}
}

// Beside a plan's windows a spend is its amount, and only once something has
// been spent: a fresh month looks the way it did before there was a budget.
func TestASpendBesideAPlanIsItsAmountOnceThereIsOne(t *testing.T) {
	now := at(9, 0)
	windows := func(used string) []view.UsageWindow {
		return []view.UsageWindow{
			{Account: "CC max", Name: "5h", Span: 5 * time.Hour, Fraction: 0.1, ResetsAt: at(11, 0)},
			{Account: "CC max", Name: "spend", Fraction: 0, Used: used, Limit: "$50.00"},
		}
	}

	spent := view.Usage(now, windows("$12.50"), now).Meters[0].Strip
	idle := view.Usage(now, windows("$0.00"), now).Meters[0].Strip

	assert.Equal(t, "10%  ·  $12.50", text(spent))
	assert.Equal(t, "10%", text(idle))
}

// renderPane is the three accounts at a width, as the pane draws them.
func renderPane(width int) []string {
	now := at(9, 0)
	return view.Render([]view.Section{view.Usage(now, threeAccounts(now), now)},
		view.ArrangeRow, width)
}

// The name and window columns are the widget's: names padded to each other
// with the badge after them, Codex's shorthand bare, two spaces, then the
// window.
func TestAStripLineBeginsTheWayTheWidgetsDoes(t *testing.T) {
	lines := renderPane(200)

	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[0], "CC max  M  5h "), "%q", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "CC work E     "), "%q", lines[1])
	assert.True(t, strings.HasPrefix(lines[2], "CX         5h "), "%q", lines[2])
}

// Every line fills the pane, the bars start and end at one column, the figures
// begin at one column, and the reset ends at the right edge.
func TestAStripLineIsLaidOutInColumns(t *testing.T) {
	const width = 200
	lines := renderPane(width)

	for _, l := range lines {
		assert.Equal(t, width, len([]rune(l)), "%q", l)
	}
	barEnd := func(l string) int {
		return strings.LastIndexAny(string([]rune(l)), string(view.BarFull)+string(view.BarEmpty))
	}
	assert.Equal(t, barEnd(lines[0]), barEnd(lines[1]))
	assert.Equal(t, barEnd(lines[1]), barEnd(lines[2]))
	assert.True(t, strings.HasSuffix(lines[0], "resets 2h 55m"))
	assert.True(t, strings.HasSuffix(lines[1], "resets Oct 1"))
	assert.True(t, strings.HasSuffix(lines[2], "resets 4h 58m"))
}

// The spare width goes three to one between the bar and the gap before the
// reset, rounded up as rich rounds it. The numbers are the widget's own at 200
// columns with these widths of figures, measured from its pane side by side
// with this one.
func TestTheBarTakesThreeQuartersOfTheSpareWidth(t *testing.T) {
	// Four widths in a row, so every remainder of the split is tried: at
	// most of them rounding up and rounding to nearest agree.
	for width := 200; width < 204; width++ {
		line := renderPane(width)[0]

		bar := strings.Count(line, string(view.BarFull)) + strings.Count(line, string(view.BarEmpty))
		// The width, less "CC max  M  " and "5h ", the space before the
		// figures, the widest figures, and " " plus the widest reset.
		figures := len([]rune("3%  ·  individual 300.5/1200 (60%)"))
		slack := width - 11 - 3 - 1 - figures - len(" resets 2h 55m")
		assert.Equal(t, (3*slack+3)/4, bar, "at %d columns", width)
	}
}

// Too narrow for a bar, the figures stay and the bar goes: the figures are
// the reading.
func TestANarrowPaneKeepsAStripsFigures(t *testing.T) {
	lines := renderPane(40)

	for _, l := range lines {
		assert.Equal(t, 40, len([]rune(l)), "%q", l)
		assert.NotContains(t, l, string(view.BarFull))
	}
	assert.Contains(t, lines[0], "12%")
	assert.Contains(t, lines[1], "$250.00")
}
