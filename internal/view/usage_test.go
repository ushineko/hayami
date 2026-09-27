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

// A quota is one of the few readings with a true threshold, so its colour is a
// signal rather than decoration.
func TestAQuotaNearItsLimitIsMarked(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "Claude max", Name: "5h", Fraction: 0.10, ResetsAt: at(11, 0)},
		{Account: "Claude max", Name: "7d", Fraction: 0.85, ResetsAt: at(11, 0)},
		{Account: "Claude max", Name: "spend", Fraction: 0.99, ResetsAt: at(11, 0)},
	}, at(9, 0))

	require.Len(t, s.Meters, 3)
	assert.Equal(t, view.Good, s.Meters[0].Status)
	assert.Equal(t, view.Warn, s.Meters[1].Status)
	assert.Equal(t, view.Bad, s.Meters[2].Status)
}

// A reading older than any pane's poll should say so. A stale panel that looks
// live is worse than one that admits it.
func TestAStaleReadingSaysItsAge(t *testing.T) {
	now := at(12, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "Claude max", Name: "5h", Fraction: 0.1, ResetsAt: at(13, 0)},
	}, now.Add(-3*time.Hour))

	require.Len(t, s.Rows, 1)
	assert.Contains(t, s.Rows[0].Value, "3h ago")
}

func TestAFreshReadingSaysNothingAboutItsAge(t *testing.T) {
	now := at(12, 0)
	s := view.Usage(now, []view.UsageWindow{
		{Account: "Claude max", Name: "5h", Fraction: 0.1, ResetsAt: at(13, 0)},
	}, now.Add(-time.Minute))

	assert.Empty(t, s.Rows)
}

// A limit's own numbers belong in the caption: the bar says "most of it" and
// the caption says how much of what.
func TestAWindowsDetailReachesItsCaption(t *testing.T) {
	s := view.Usage(at(9, 0), []view.UsageWindow{
		{Account: "Codex", Name: "limit", Fraction: 0.34, ResetsAt: at(11, 0), Detail: "403.51 / 1200.00"},
	}, at(9, 0))

	require.Len(t, s.Meters, 1)
	assert.Contains(t, s.Meters[0].Caption, "403.51 / 1200.00")
}
