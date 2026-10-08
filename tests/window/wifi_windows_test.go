package window_test

import (
	"image"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// wifiSettle is how long the panel is given after its window appears: the
// bandwidth section's first poll describes the link, and its second has rates.
const wifiSettle = 5 * time.Second

// interfacesHere are a connected Wi-Fi interface and one this build does not
// describe as Wi-Fi -- wired, or a radio Windows does not list -- on this
// machine, or a skip.
func interfacesHere(t *testing.T) (wifi, wired string) {
	t.Helper()
	radios, err := core.ReadWireless(t.Context())
	if err != nil || len(radios) == 0 {
		t.Skip("no Wi-Fi interface this build can describe")
	}
	for name, w := range radios {
		if w.Connected {
			wifi = name
		}
	}
	if wifi == "" {
		t.Skip("no connected Wi-Fi interface")
	}
	counters, err := core.ReadCounters()
	require.NoError(t, err)
	var names []string
	for name := range counters {
		if _, isRadio := radios[name]; !isRadio && core.ClassifyInterface(name) == core.KindOrdinary {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Skip("no wired interface to line up with")
	}
	// The shortest name: a long one ("Bluetooth Network Connection") fills
	// its label column up to the first arrow, and the picture can no longer
	// tell the label from the rates.
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) < len(names[j])
		}
		return names[i] < names[j]
	})
	return wifi, names[0]
}

/*
Spec 037. A Wi-Fi interface's row leads with its signal bars, and its rates
stay in the column a wired interface's are in.

Read off the picture. The two interfaces' lines are found by their labels
(the harness's card), which also holds the Wi-Fi row's totals and link line
under it: a window that dropped them has fewer lines than the card and fails
there.

A row ends "↓ figure unit ↑ figure unit", so its arrows are the sixth and the
third words from the end, and both rows' arrows are at the same x, to the
pixel: one column of rates. (The units' ink cannot be compared: "B/s" is padded
to the width of "KiB/s" with spaces, which have none.) The four glyphs before
the Wi-Fi row's down arrow are the bars: four runs of one width, filled ones
solid, which no four letters of a label are. The bars sit close enough to a
short label that the two read as one word at the gap that splits words, so
they are told apart by shape rather than by spacing. Bars drawn after the rates
instead would move the Wi-Fi row's arrows; bars not drawn would leave letters
before its ↓.
*/
func TestTheWiFiRowsBarsLeadItsRatesInTheWiredColumn(t *testing.T) {
	wifi, wired := interfacesHere(t)
	s := panelSettings{Sections: []string{"bandwidth"}, Interfaces: []string{wifi, wired}, Settle: wifiSettle}
	hwnd := start(t, s)
	c := cardOf(t, "bandwidth", s)

	p := shoot(t, hwnd, "wifi.png")
	t.Logf("picture: %s", p.path)
	radioLine, cableLine := p.row(t, c, wifi), p.row(t, c, wired)
	radio, cable := p.words(radioLine), p.words(cableLine)
	t.Logf("Wi-Fi %v: %v", radioLine, radio)
	t.Logf("other %v: %v", cableLine, cable)
	require.GreaterOrEqual(t, len(radio), 7, "the Wi-Fi row: a label and two rates")
	require.GreaterOrEqual(t, len(cable), 7, "the wired row: a label and two rates")
	down, up := func(w []span) span { return w[len(w)-6] }, func(w []span) span { return w[len(w)-3] }
	assert.LessOrEqual(t, abs(down(radio).From-down(cable).From), 1,
		"the down arrows are at x=%d and x=%d: the rates are not in one column", down(radio).From, down(cable).From)
	assert.LessOrEqual(t, abs(up(radio).From-up(cable).From), 1,
		"the up arrows are at x=%d and x=%d: the rates are not in one column", up(radio).From, up(cable).From)

	radioBars := barsBefore(p.img, radioLine, down(radio).From)
	cableBars := barsBefore(p.img, cableLine, down(cable).From)
	t.Logf("glyphs before ↓: Wi-Fi %v, other %v", radioBars, cableBars)
	assert.True(t, isBars(p.img, radioLine, radioBars),
		"the four glyphs before the Wi-Fi row's ↓ are not four bars of one width: %v", radioBars)
	assert.False(t, isBars(p.img, cableLine, cableBars),
		"the other row has bars before its ↓ too: %v", cableBars)
}

// barsBefore are the last four glyphs left of x on a line: runs of ink split
// by any column without it, which a bar's gap to the next bar is.
func barsBefore(img *image.NRGBA, line span, x int) []span {
	var runs []span
	for _, w := range words(img, line, 1) {
		if w.To < x {
			runs = append(runs, w)
		}
	}
	if len(runs) > 4 {
		runs = runs[len(runs)-4:]
	}
	return runs
}

// isBars says whether glyphs are the signal's four segments: four runs of the
// same width (to a pixel), at least one of them solid -- a filled segment has
// ink in most of its box, a letter in far less.
func isBars(img *image.NRGBA, line span, glyphs []span) bool {
	if len(glyphs) != 4 {
		return false
	}
	solid := false
	for _, g := range glyphs {
		if abs((g.To-g.From)-(glyphs[0].To-glyphs[0].From)) > 1 {
			return false
		}
		inked, top, bottom := 0, line.To, line.From
		for x := g.From; x <= g.To; x++ {
			for y := line.From; y <= line.To; y++ {
				if glyph(img, x, y) {
					inked++
					top, bottom = min(top, y), max(bottom, y)
				}
			}
		}
		box := (g.To - g.From + 1) * (bottom - top + 1)
		if box > 0 && inked*10 >= box*8 {
			solid = true
		}
	}
	return solid
}
