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

// wholeNameShare is the most of the cell's width a name drawn whole takes when
// the name is as short as "DualSense". A name cut with an ellipsis runs to the
// cell's budget: "DualSense Wir…" took four fifths of the cell.
const wholeNameShare = 0.65

// inkBand is one band of rows holding anything apart from the background: a
// line of text or a bar, and how far across its ink runs.
type inkBand struct {
	bar      bool
	from, to int
}

// inkBandsOf reads the picture top to bottom as bands of text and bars,
// leaving out its edges, where the panel's rounded border is apart from the
// card on every row. Quiet text counts: a cell's state, a dim name.
func inkBandsOf(img *image.NRGBA) []inkBand {
	const edge = 8
	bg := background(img)
	b := img.Bounds()
	var out []inkBand
	in := false
	for y := b.Min.Y + edge; y < b.Max.Y-edge; y++ {
		first, last, run, longest := -1, -1, 0, 0
		for x := b.Min.X + edge; x < b.Max.X-edge; x++ {
			if apart(img.NRGBAAt(x, y), bg) {
				if first < 0 {
					first = x
				}
				last = x
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
		if first < 0 {
			in = false
			continue
		}
		if !in {
			out = append(out, inkBand{from: first, to: last})
			in = true
		}
		cur := &out[len(out)-1]
		cur.bar = cur.bar || longest >= barRun
		cur.from, cur.to = min(cur.from, first), max(cur.to, last)
	}
	return out
}

/*
Spec 057. The controller's cell says "DualSense", whole, and not "DualSense
Wir…".

Read off the picture by how wide the name is. With only this card in it the
window is one cell across, and the controller's is the last device with a
level: a name, a level and a state, then its bar. So the name is the third
band of text above the last bar; a placeholder under it has no bar. A name cut to fit runs to the cell's edge; "DualSense" does not
come near it.
*/
func TestTheControllersNameIsDrawnWhole(t *testing.T) {
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	if kinds := kindsAnswering(t); !kinds[battery.KindGamepad] {
		t.Skip("needs a controller giving a level now: wake the DualSense and run again")
	}

	hwnd := start(t, panelSettings{Sections: []string{"peripherals"}, Settle: peripheralsSettle})
	var p *picture
	var bands []inkBand
	for deadline := time.Now().Add(3 * 15 * time.Second); ; time.Sleep(3 * time.Second) {
		p = shoot(t, hwnd, "names.png")
		bands = inkBandsOf(p.img)
		if lastBar(bands) >= 3 || time.Now().After(deadline) {
			break
		}
	}
	t.Logf("picture: %s", p.path)
	bar := lastBar(bands)
	require.GreaterOrEqual(t, bar, 3, "the card has no cell with a bar")
	for _, b := range bands[bar-3 : bar] {
		require.False(t, b.bar, "the last device's cell is not a name, a level and a state over a bar")
	}

	name := bands[bar-3]
	width := float64(p.img.Bounds().Dx())
	share := float64(name.to-name.from) / width
	t.Logf("the controller's name: %d px of a %.0f px cell (%.2f)", name.to-name.from, width, share)
	assert.Less(t, share, wholeNameShare, "the controller's name runs to the cell's edge: it is cut, not whole")
}

// lastBar is the index of the last bar among the bands, or -1.
func lastBar(bands []inkBand) int {
	for i := len(bands) - 1; i >= 0; i-- {
		if bands[i].bar {
			return i
		}
	}
	return -1
}
