package window_test

import (
	"context"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/battery"
)

// chargingWithNoLevel is whether a device on this desk says, now, that it is
// charging and gives no level: an AULA F75 on its receiver with its cable in.
func chargingWithNoLevel(t *testing.T) bool {
	t.Helper()
	var drivers []sanshoku.Driver
	for _, d := range all.Drivers() {
		if desc, ok := sanshoku.Describe(d); ok && desc.Offers("battery") && desc.On("windows") {
			drivers = append(drivers, d)
		}
	}
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
					if !b.HasLevel && !b.HasBand && b.State != battery.Discharging {
						_ = dev.Close()
						cancel()
						return true
					}
				}
			}
			_ = dev.Close()
		}
		cancel()
		time.Sleep(time.Second)
	}
	return false
}

/*
inkBands reads the picture top to bottom as a string of T and B: a T for each
band of rows that holds text, a B for each that holds a bar.

Anything apart from the background counts, so the quiet lines count too -- a
cell's state, a placeholder's dim name -- which textLines leaves out because it
looks for bright ink. The picture's edges are left out: the panel's rounded
border is apart from the card on every row.
*/
func inkBands(img *image.NRGBA) string {
	const edge = 8
	bg := background(img)
	b := img.Bounds()
	var out strings.Builder
	in, bar := false, false
	flush := func() {
		if !in {
			return
		}
		if bar {
			out.WriteByte('B')
		} else {
			out.WriteByte('T')
		}
		in, bar = false, false
	}
	for y := b.Min.Y + edge; y < b.Max.Y-edge; y++ {
		any, run, longest := false, 0, 0
		for x := b.Min.X + edge; x < b.Max.X-edge; x++ {
			if apart(img.NRGBAAt(x, y), bg) {
				any = true
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
		if !any {
			flush()
			continue
		}
		in = true
		bar = bar || longest >= barRun
	}
	flush()
	return out.String()
}

// aCellWithNoBar is whether the bands hold a device's cell that has no bar:
// its three lines of text -- name, reading, state -- not followed by one.
//
// One cell to a line, as the window draws them with only this card in it, a
// device with a level is TTTB, a placeholder is TT (a name and a dash, no
// state) and always last. So after the heading, the text between two bars is
// three lines, or six where a cell with no bar sits between them; and the
// text after the last bar is none, or a placeholder's two, or three or five
// where a cell with no bar is there.
func aCellWithNoBar(bands string) bool {
	body := strings.TrimPrefix(bands, "T") // the card's heading
	runs := strings.Split(body, "B")
	for _, r := range runs[:len(runs)-1] {
		if len(r) >= 6 {
			return true
		}
	}
	last := len(runs[len(runs)-1])
	return last == 3 || last == 5
}

/*
Spec 058. A keyboard charging on its cable, whose receiver gives no level,
keeps its cell: a name, a dash where the number goes, and "Charging" under it,
with no bar.

Read off the picture as bands of text and bars. Before spec 058 the keyboard
had no cell at all while it charged: the card was the other devices and, in
its place, nothing or a placeholder, which has two lines of text and not
three.
*/
func TestADeviceChargingWithNoLevelKeepsItsCell(t *testing.T) {
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	if !chargingWithNoLevel(t) {
		t.Skip("no device says it is charging with no level: put the AULA F75 on its cable, switched to 2.4 GHz, and run again")
	}

	hwnd := start(t, panelSettings{Sections: []string{"peripherals"}, Settle: peripheralsSettle})
	var p *picture
	var got string
	for deadline := time.Now().Add(3 * 15 * time.Second); ; time.Sleep(3 * time.Second) {
		p = shoot(t, hwnd, "charging.png")
		got = inkBands(p.img)
		if aCellWithNoBar(got) || time.Now().After(deadline) {
			break
		}
	}
	t.Logf("picture: %s", p.path)
	t.Logf("bands: %s", got)
	require.True(t, aCellWithNoBar(got),
		"no cell with a state and no bar: the charging device is not on the card")
}

// The reading of the bands, held to the cards it has to tell apart.
func TestACellWithNoBarIsToldFromAPlaceholder(t *testing.T) {
	for bands, want := range map[string]bool{
		"TTTTBTTTB":     false, // two devices with levels
		"TTTTBTT":       false, // a device and a placeholder
		"TTTTBTTT":      true,  // a device, and one charging with no level
		"TTTTTTTTB":     true,  // the charging one first
		"TTTTBTTTTT":    true,  // then a placeholder after it
		"TTTTBTTTBTTTB": false, // three devices with levels
	} {
		require.Equal(t, want, aCellWithNoBar(bands), bands)
	}
}
