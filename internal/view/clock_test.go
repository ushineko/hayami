package view_test

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/view"
)

// Spec 106 R5. The time and the date are one length on every day of the
// week and at every hour, so a tick changes no width in the terminal.
func TestTheClockIsOneLengthEveryDay(t *testing.T) {
	base := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local)
	times, dates := map[int]bool{}, map[int]bool{}
	for d := range 7 {
		for _, h := range []int{0, 9, 13, 23} {
			s := view.Clock(base.AddDate(0, 0, d).Add(time.Duration(h)*time.Hour + 59*time.Minute + 59*time.Second))
			times[utf8.RuneCountInString(s.Title)] = true
			dates[utf8.RuneCountInString(s.TitleEnd)] = true
		}
	}
	assert.Len(t, times, 1, "the time's length changed")
	assert.Len(t, dates, 1, "the date's length changed")
}

// Spec 106 R6. In a stack the clock is its title line, the date against the
// right edge; in row it is one line.
func TestTheClockIsOneLineInTheTerminal(t *testing.T) {
	s := view.Clock(time.Date(2026, 11, 21, 18, 25, 4, 0, time.Local))
	stack := view.Render([]view.Section{s}, view.ArrangeStack, 40)
	assert.Equal(t, []string{"18:25:04" + "                  " + "Sat 2026/11/21"}, stack)
	row := view.Render([]view.Section{s}, view.ArrangeRow, 40)
	assert.Len(t, row, 1)
	assert.Contains(t, row[0], "18:25:04")
	assert.Contains(t, row[0], "Sat 2026/11/21")
}
