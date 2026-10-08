package window_test

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/aula"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/logitech"
	"github.com/ushineko/sanshoku/razer"
	"github.com/ushineko/sanshoku/steelseries"
)

// peripheralsSettle is how long the panel is given after its window appears
// for the peripherals' first poll, which asks every device and may ask a
// sleeping one twice.
const peripheralsSettle = 6 * time.Second

// answering is the names of the devices on this desk that give a level now,
// through the drivers hayami asks on Windows. It asks what the panel will ask,
// getters only, so the test knows what the picture should hold.
//
// Asked up to three times a second apart: a keyboard idle on its receiver
// misses the first questions while its link wakes, and the asking wakes it.
func answering(t *testing.T) []string {
	t.Helper()
	for range 3 {
		if names := askDesk(t); len(names) > 0 {
			return names
		}
		time.Sleep(time.Second)
	}
	return nil
}

// askDesk is one round of answering's questions.
func askDesk(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	found, _ := sanshoku.Scan(ctx, logitech.Driver{}, razer.Driver{}, steelseries.Driver{}, aula.Driver{})
	var names []string
	for _, c := range found {
		dev, err := c.Open(ctx)
		if err != nil {
			continue
		}
		if src, ok := dev.(battery.Source); ok {
			got, _ := src.Batteries(ctx)
			for _, b := range got {
				if b.HasLevel || b.HasBand {
					names = append(names, b.Name)
				}
			}
		}
		_ = dev.Close()
	}
	return names
}

/*
Spec 035. On Windows the peripherals section draws the devices on the desk.

Read off the picture. Which devices answer depends on the desk at that moment
-- a mouse asleep in its dock reads nothing, a keyboard switched to its cable
says nothing through its receiver -- so the test first asks the devices itself
and skips when none gives a level. Then the panel, started with only that
section, must draw a card: a heading line and, under it, a line with a cell's
name and level on it. Before spec 035 the section found no device on Windows,
drew nothing, and the window held no line of text at all.
*/
func TestThePeripheralsAreDrawnOnWindows(t *testing.T) {
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	live := answering(t)
	if len(live) == 0 {
		t.Skip("no device on this desk gives a level now: wake the mouse or keyboard and run again")
	}
	t.Logf("answering: %v", live)

	hwnd := start(t, panelSettings{Sections: []string{"peripherals"}, Settle: peripheralsSettle})

	// A keyboard idle on its receiver can miss a poll while its link wakes,
	// and the section polls every fifteen seconds; so the picture is taken
	// until it holds a device, for three polls at most.
	var p *picture
	for deadline := time.Now().Add(3 * 15 * time.Second); ; time.Sleep(3 * time.Second) {
		p = shoot(t, hwnd, "peripherals.png")
		if drawsDevices(p.img, p.text) || time.Now().After(deadline) {
			break
		}
	}
	t.Logf("picture: %s", p.path)
	t.Logf("text lines: %v", p.text)

	// A device cell has a bar under its level, whatever its state: coloured
	// by its band on battery, the accent while charging, dim when remembered.
	// A card with no device is the heading, a reason per vendor and
	// placeholder cells, none of which has a bar. The colour of the level was
	// the test before #161, and a charging device (a white level, a blue bar)
	// failed it; the number of text lines before that failed with two devices.
	require.NotEmpty(t, p.text, "no line of text at all")
	n := bars(p.img)
	t.Logf("bars: %d", n)
	require.Positive(t, n,
		"the card is reasons and placeholders, not devices: the section drew none of %v", live)
}

// drawsDevices is whether the card shows a device: a cell with a bar.
func drawsDevices(img *image.NRGBA, _ []span) bool { return bars(img) > 0 }

// barRun is the shortest unbroken run of solid pixels that is a bar. A
// letter is a few pixels wide and a word has gaps between its letters; the
// narrowest bar, a device at a few percent, is still a bar's rounded ends.
const barRun = 40

// bars counts the bars in the picture: runs of rows, each holding barRun or
// more pixels in a row that stand out from the card's background.
func bars(img *image.NRGBA) int {
	bg := background(img)
	n, in := 0, false
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		run, longest := 0, 0
		for x := b.Min.X; x < b.Max.X; x++ {
			if apart(img.NRGBAAt(x, y), bg) {
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
		bar := longest >= barRun
		if bar && !in {
			n++
		}
		in = bar
	}
	return n
}

// background is the card's colour: the commonest pixel in the picture.
func background(img *image.NRGBA) color.NRGBA {
	count := map[color.NRGBA]int{}
	var best color.NRGBA
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			count[c]++
			if count[c] > count[best] {
				best = c
			}
		}
	}
	return best
}

// apart is a pixel clearly not the background: a dim bar still is.
func apart(c, bg color.NRGBA) bool {
	d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
	return d(c.R, bg.R)+d(c.G, bg.G)+d(c.B, bg.B) > 45
}
