package window_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
An interface's totals are drawn in the columns of the rates above them, so the
Network card reads as a table.

Read off the picture. A row ends "↓ figure unit ↑ figure unit" and the line
under it "Σ figure unit figure unit". A figure is right-aligned, so its left
edge moves with its value; a unit is left-aligned, so the column is where the
unit starts, and that is what is compared, with the sum sign against the down
arrow. Within a third of the word gap, because K, G and B do not start at the
same pixel of their cells; a column out by one character is well past it.
(The arrows are not read as words: a label as long as "Ethernet" runs into one.)

Totals padded to their own unit's width start two characters to the right of
the rates' units, which is how this card used to look.
*/
func TestTheTotalsAreInTheColumnsOfTheRates(t *testing.T) {
	name := shortestOrdinary(t, func(string) bool { return false })
	s := panelSettings{Sections: []string{"bandwidth"}, Interfaces: []string{name}, Settle: wifiSettle}
	hwnd := start(t, s)
	c := cardOf(t, "bandwidth", s)

	p := shoot(t, hwnd, "grid.png")
	t.Logf("picture: %s", p.path)
	rateLine := p.row(t, c, name)
	at := slices.Index(p.text, rateLine)
	require.Less(t, at+1, len(p.text), "no line under the rates")
	rate, total := p.words(rateLine), p.words(p.text[at+1])
	t.Logf("rates %v: %v", rateLine, rate)
	t.Logf("totals %v: %v", p.text[at+1], total)
	require.GreaterOrEqual(t, len(rate), 6, "two rates")
	require.GreaterOrEqual(t, len(total), 5, "the sum sign and two totals")

	slack := p.gap() / 3
	for _, col := range []struct {
		name         string
		rates, total int
	}{
		{"the down units", rate[len(rate)-4].From, total[len(total)-3].From},
		{"the up units", rate[len(rate)-1].From, total[len(total)-1].From},
	} {
		assert.LessOrEqual(t, abs(col.rates-col.total), slack,
			"%s start at x=%d and x=%d: not one column", col.name, col.rates, col.total)
	}

	// The arrow is found as a glyph, because a label can run into it as a word,
	// and by its middle, because it has less ink in its cell than the sum sign.
	before := barsBefore(p.img, rateLine, rate[len(rate)-5].From)
	require.NotEmpty(t, before, "nothing before the down rate's figure")
	arrow, sum := before[len(before)-1], total[len(total)-5]
	assert.LessOrEqual(t, abs((arrow.From+arrow.To)/2-(sum.From+sum.To)/2), slack,
		"the down arrow is at %v and the sum sign at %v: not one column", arrow, sum)
}
