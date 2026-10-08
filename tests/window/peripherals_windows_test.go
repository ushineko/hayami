package window_test

import (
	"context"
	"image"
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

	// A card drawing a device says nothing about what is absent: the reasons
	// are dropped. A heading, the cells' names and their levels is three
	// lines at most; a card with no device is the heading, a reason per
	// vendor and two placeholder cells.
	require.NotEmpty(t, p.text, "no line of text at all")
	require.LessOrEqual(t, len(p.text), 3,
		"the card is reasons and placeholders, not devices: the section drew none of %v", live)
}

// drawsDevices is whether the card is a heading and cells only.
func drawsDevices(img *image.NRGBA, text []span) bool {
	if len(text) == 0 || len(text) > 3 {
		return false
	}
	last := text[len(text)-1]
	return len(words(img, last, (last.To-last.From+1)*3/4)) >= 1
}
