package gui_test

import (
	"context"
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// fixed is a source whose section the test sets.
type fixed struct{ sec view.Section }

func (f *fixed) Key() string                        { return f.sec.Key }
func (f *fixed) Interval() time.Duration            { return time.Hour }
func (f *fixed) Poll(context.Context) (bool, error) { return true, nil }
func (f *fixed) Section() view.Section              { return f.sec }
func (f *fixed) Data() any                          { return nil }

// desk is a reading from the desk with the Kraken: the processor, the card,
// the coolant and the pump, named or not.
func desk(named bool) view.CoolerReading {
	r := view.CoolerReading{
		CPU: 61, HasCPU: true, CPULoad: 12, HasCPULoad: true,
		GPU: 44, HasGPU: true, GPULoad: 7, HasGPULoad: true,
		Coolant: 31.4, HasLiquid: true, PumpRPM: 2650, HasPump: true, FanRPM: 1210, HasFan: true,
	}
	if named {
		r.CPUName = "Intel(R) Core(TM) i9-14900K"
		r.GPUName = "NVIDIA GeForce RTX 4090"
		r.CoolerName = "NZXT Kraken Elite V2"
	}
	return r
}

// interfaces is the bandwidth card both desks draw: a wired interface and the
// tailnet, at rates that put the line at its full width.
func interfaces() view.Section {
	return view.Bandwidth([]view.BandwidthReading{
		{Name: "enp5s0", RxRate: 40 << 20, TxRate: 900 << 10, HasRate: true, RxTotal: 1 << 36, TxTotal: 1 << 33, HasTotal: true},
		{Name: "tailscale0", RxRate: 2 << 10, TxRate: 1 << 10, HasRate: true, RxTotal: 1 << 28, TxTotal: 1 << 27, HasTotal: true},
	})
}

/*
Spec 031, R2.5. The cooler card is the same size with names as without: the
panel was the size it was, and it stays it when the names arrive.

On both desks, which differ in the way that matters: the one with the Kraken
has a speeds row, and the other has no coolant and no speeds, so its cooler card
is three rows narrower in what decides its width. The panel's width is its
widest card, the bandwidth card, and a name is held to LabelWidth so that it
cannot make the cooler card the widest.
*/
func TestTheCoolerCardIsTheSameSizeWithNames(t *testing.T) {
	other := func(named bool) view.CoolerReading {
		r := desk(named)
		r.HasLiquid, r.HasPump, r.HasFan, r.CoolerName = false, false, false, ""
		if named {
			r.CPUName, r.GPUName = "13th Gen Intel(R) Core(TM) i7-13700K", "NVIDIA GeForce RTX 3080"
		}
		return r
	}
	for name, reading := range map[string]func(bool) view.CoolerReading{"kraken": desk, "other": other} {
		a := test.NewTempApp(t)
		cooler := &fixed{view.Cooler(reading(false))}
		bandwidth := &fixed{interfaces()}
		p := gui.New(a, gui.Options{Sources: []panel.Source{bandwidth, cooler}, Title: "hayami"})
		p.Draw("bandwidth", bandwidth.sec, true)
		p.Draw("cooler", cooler.sec, true)
		panelBefore := gui.PanelSize(p)
		cardBefore := gui.CardMinSize(p, "cooler")

		named := view.Cooler(reading(true))
		p.Draw("cooler", named, true)
		require.Contains(t, gui.CardRows(p, "cooler"), named.Rows[0].Label, "%s: the names did not arrive", name)

		t.Logf("%s: cooler card %v unnamed, %v named; bandwidth card %v; panel %v",
			name, cardBefore, gui.CardMinSize(p, "cooler"), gui.CardMinSize(p, "bandwidth"), panelBefore)
		assert.Equal(t, panelBefore, gui.PanelSize(p), "%s: the panel changed size when the names arrived", name)
		assert.InDelta(t, cardBefore.Height, gui.CardMinSize(p, "cooler").Height, 0.01,
			"%s: the cooler card changed height", name)
		assert.LessOrEqual(t, gui.CardMinSize(p, "cooler").Width, gui.CardMinSize(p, "bandwidth").Width,
			"%s: a name made the cooler card the widest, and the panel follows the widest", name)
	}
}

/*
The longest names the view makes, on the desk whose cooler card is narrowest:
a name cut at LabelWidth ("RX 7900 XT/790…") and one exactly that wide ("Kraken
Elite V2"), on every row. They still do not make the cooler card the widest.

**This is a measurement, not a guarantee.** The label is drawn in a
proportional face, so LabelWidth characters of capitals would be wider than
these, and a lone cooler card -- the bandwidth card hidden -- has nothing wider
to hide behind. A row with a label column of fixed pixel width needs the design
system; it is in spec 031's gaps.
*/
func TestTheLongestNamesStillFit(t *testing.T) {
	a := test.NewTempApp(t)
	r := desk(true)
	r.HasLiquid, r.HasPump, r.HasFan = false, false, false
	r.CPUName = "NZXT Kraken Elite V2"
	r.GPUName = "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]"
	cooler := &fixed{view.Cooler(r)}
	bandwidth := &fixed{interfaces()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{bandwidth, cooler}, Title: "hayami"})
	p.Draw("bandwidth", bandwidth.sec, true)
	p.Draw("cooler", cooler.sec, true)

	t.Logf("cooler card %v with %q and %q; bandwidth card %v", gui.CardMinSize(p, "cooler"),
		cooler.sec.Rows[0].Label, cooler.sec.Rows[1].Label, gui.CardMinSize(p, "bandwidth"))
	assert.LessOrEqual(t, gui.CardMinSize(p, "cooler").Width, gui.CardMinSize(p, "bandwidth").Width)
}

// R2.5. The full names are the card's tip, the window's hover.
func TestTheFullNamesAreTheTip(t *testing.T) {
	a := test.NewTempApp(t)
	cooler := &fixed{view.Cooler(desk(true))}
	p := gui.New(a, gui.Options{Sources: []panel.Source{cooler}, Title: "hayami"})
	p.Draw("cooler", cooler.sec, true)

	tip := gui.CardTip(p, "cooler")
	assert.Contains(t, tip, "CPU: Intel(R) Core(TM) i9-14900K")
	assert.Contains(t, tip, "GPU: NVIDIA GeForce RTX 4090")
	assert.Contains(t, tip, "Coolant: NZXT Kraken Elite V2")
}

/*
R1.2, R3.1. The window draws a bandwidth row in the colour of its strongest
rate: the info colour from 1 MiB/s, amber from 10, the design system's magenta
from 100, and the row's own text below 1. One colour for the row, because the
library's row has one; the pane colours each rate.
*/
func TestTheWindowColoursARowByItsStrongestRate(t *testing.T) {
	a := test.NewTempApp(t)
	src := &fixed{view.Bandwidth([]view.BandwidthReading{{Name: "enp5s0", HasRate: true}})}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	colour := func(rx, tx float64) color.Color {
		sec := view.Bandwidth([]view.BandwidthReading{{Name: "enp5s0", RxRate: rx, TxRate: tx, HasRate: true}})
		p.Draw("bandwidth", sec, true)
		return gui.RowColour(p, "bandwidth", 0)
	}

	assert.Nil(t, colour(12<<10, 1<<10), "a quiet interface took a colour")
	assert.Equal(t, "info", gui.Status(p, "bandwidth", 0))

	assert.Nil(t, colour(1<<10, 40<<20), "amber is the Warn status, which the library colours itself")
	assert.Equal(t, "warn", gui.Status(p, "bandwidth", 0), "the up rate's band did not reach the row")

	accent := colour(2<<20, 1<<10)
	strong := colour(400<<20, 2<<20)
	require.NotNil(t, accent)
	require.NotNil(t, strong)
	assert.NotEqual(t, accent, strong)
	assert.NotEqual(t, "bad", gui.Status(p, "bandwidth", 0), "a fast rate is not a failure")
}
