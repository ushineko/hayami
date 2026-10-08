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

	// A device is drawn with its level in a battery colour (spec 018): green,
	// amber or red. A card with no device is the heading, a reason per vendor
	// and placeholder cells, all in the text colours, so it has no such line.
	// Counting lines instead held only while one device was awake: devices
	// stack, two lines each, and two awake read as a card of reasons.
	require.NotEmpty(t, p.text, "no line of text at all")
	levels := levelLines(p.img, p.text)
	t.Logf("lines in a battery colour: %d", levels)
	require.Positive(t, levels,
		"the card is reasons and placeholders, not devices: the section drew none of %v", live)
}

// drawsDevices is whether the card shows a device's level.
func drawsDevices(img *image.NRGBA, text []span) bool { return levelLines(img, text) > 0 }

// levelLines counts the text lines drawn in a battery colour: saturated, and
// not the plot's blue-led colours. Text, reasons and placeholders are grey
// or white, which a saturation floor leaves out.
func levelLines(img *image.NRGBA, text []span) int {
	n := 0
	for _, line := range text {
		coloured := 0
		for y := line.From; y <= line.To; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				if batteryColour(img.At(x, y)) {
					coloured++
				}
			}
		}
		if coloured >= levelInk {
			n++
		}
	}
	return n
}

// levelInk is how many coloured pixels make a line a level: a two-digit
// percentage at the panel's size is several hundred; anti-aliased edges of
// grey text are a handful.
const levelInk = 60

// batteryColour is a pixel that is clearly coloured and not blue-led.
func batteryColour(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	r, g, b = r>>8, g>>8, b>>8
	hi, lo := max(r, g, b), min(r, g, b)
	return hi-lo > 80 && !plotted(c)
}
