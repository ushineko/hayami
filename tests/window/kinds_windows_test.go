package window_test

import (
	"context"
	"image"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/battery"
)

// kindsAnswering is the kinds of device on this desk that give a level now,
// through every battery driver hayami asks on Windows.
func kindsAnswering(t *testing.T) map[battery.Kind]bool {
	t.Helper()
	var drivers []sanshoku.Driver
	for _, d := range all.Drivers() {
		if desc, ok := sanshoku.Describe(d); ok && desc.Offers("battery") && desc.On("windows") {
			drivers = append(drivers, d)
		}
	}
	kinds := map[battery.Kind]bool{}
	for range 3 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		found, _ := sanshoku.Scan(ctx, drivers...)
		for _, c := range found {
			dev, err := c.Open(ctx)
			if err != nil {
				continue
			}
			if src, ok := dev.(battery.Source); ok {
				got, _ := src.Batteries(ctx)
				for _, b := range got {
					if b.HasLevel || b.HasBand {
						kinds[b.Kind] = true
					}
				}
			}
			_ = dev.Close()
		}
		cancel()
		if len(kinds) >= 3 {
			break
		}
		time.Sleep(time.Second)
	}
	return kinds
}

// barLines are the bars in the picture, line by line: for each band of rows
// that holds a bar, how many bars sit side by side in it.
func barLines(img *image.NRGBA) []int {
	bg := background(img)
	b := img.Bounds()
	var lines []int
	in := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		n, run := 0, 0
		for x := b.Min.X; x <= b.Max.X; x++ {
			if x < b.Max.X && apart(img.NRGBAAt(x, y), bg) {
				run++
				continue
			}
			if run >= barRun {
				n++
			}
			run = 0
		}
		switch {
		case n > 0 && !in:
			lines = append(lines, n)
			in = true
		case n > 0:
			lines[len(lines)-1] = max(lines[len(lines)-1], n)
		default:
			in = false
		}
	}
	return lines
}

/*
Spec 056. With three or four kinds of device answering -- on the desk this was
written at, a mouse, a keyboard, headphones and a controller -- the
peripherals card draws a cell per kind, two across and two lines down, read
off the picture by the bars under the levels. A cell padding the card to four
has no bar, so three kinds are a line of two bars and a line of one.

And the card is never wider for it: the window with the peripherals card above
the cooler card is as wide as the window with the cooler card alone. Before
spec 056 the card had two cells; the window's width is its widest card's, so a
grid that laid four cells across would have widened it.
*/
func TestAKindOfDeviceIsACellTwoAcross(t *testing.T) {
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	kinds := kindsAnswering(t)
	t.Logf("kinds answering: %v", kinds)
	if len(kinds) < 3 {
		t.Skip("fewer than three kinds of device give a level now: wake the mouse, keyboard, headphones or controller and run again")
	}

	alone := start(t, panelSettings{Sections: []string{"cooler"}, Settle: peripheralsSettle})
	base := shoot(t, alone, "cooler.png")

	hwnd := start(t, panelSettings{Sections: []string{"peripherals", "cooler"}, Settle: peripheralsSettle})
	var p *picture
	var lines []int
	for deadline := time.Now().Add(3 * 15 * time.Second); ; time.Sleep(3 * time.Second) {
		p = shoot(t, hwnd, "kinds.png")
		lines = firstLines(barLines(p.img), 2)
		if sum(lines) >= len(kinds) || time.Now().After(deadline) {
			break
		}
	}
	t.Logf("picture: %s", p.path)
	t.Logf("bars by line: %v", lines)

	want := []int{2, 2}
	if len(kinds) == 3 {
		want = []int{2, 1}
	}
	require.Equal(t, want, lines, "the cells are not two across, a line per pair of kinds")
	assert.Equal(t, base.img.Bounds().Dx(), p.img.Bounds().Dx(),
		"the window with the peripherals card is wider than without it")
}

// firstLines are the first n bar lines: the peripherals card's, drawn first,
// before anything the cooler card draws.
func firstLines(lines []int, n int) []int { return lines[:min(n, len(lines))] }

// sum is the bars in all the lines.
func sum(lines []int) int {
	n := 0
	for _, l := range lines {
		n += l
	}
	return n
}
