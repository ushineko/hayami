package gui_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/glance"

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
	if named {
		return machine("Intel(R) Core(TM) i9-14900K", "NVIDIA GeForce RTX 4090", "NZXT Kraken Elite V2", true)
	}
	return machine("", "", "", true)
}

// machine is a processor and a card with those names, and with kraken a
// coolant, a pump and a fan named cooler.
func machine(cpu, gpu, cooler string, kraken bool) view.CoolerReading {
	r := view.CoolerReading{Probes: []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Name: cpu, Load: view.Some(12.0), Temp: view.Some(61.0)},
		{ID: "gpu", Role: view.RoleGPU, Name: gpu, Load: view.Some(7.0), Temp: view.Some(44.0)},
	}}
	if kraken {
		r.Probes = append(r.Probes,
			view.Probe{ID: "coolant", Role: view.RoleCoolant, Name: cooler, Temp: view.Some(31.4)},
			view.Probe{ID: "fan", Role: view.RoleFan, Name: cooler, RPM: view.Some(1210)},
			view.Probe{ID: "pump", Role: view.RolePump, Name: cooler, RPM: view.Some(2650)})
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
has a speeds row that decides the card's width, and the other has no coolant
and no speeds, so there the processor's row does -- and a name used to widen
it from 194 to 230 px. The named rows' label column is pinned now, so the card
is the same size to the pixel.
*/
func TestTheCoolerCardIsTheSameSizeWithNames(t *testing.T) {
	other := func(named bool) view.CoolerReading {
		if named {
			return machine("13th Gen Intel(R) Core(TM) i7-13700K", "NVIDIA GeForce RTX 3080", "", false)
		}
		return machine("", "", "", false)
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
		assert.Equal(t, cardBefore, gui.CardMinSize(p, "cooler"), "%s: the cooler card changed size", name)
	}
}

// The longest names the view makes -- one cut at LabelWidth, one exactly that
// wide, and fifteen capitals -- leave the cooler card the size "CPU" does.
func TestTheLongestNamesDoNotWidenTheCard(t *testing.T) {
	size := func(cpu, gpu string) (fyne.Size, []string) {
		a := test.NewTempApp(t)
		cooler := &fixed{view.Cooler(machine(cpu, gpu, "", false))}
		p := gui.New(a, gui.Options{Sources: []panel.Source{cooler}, Title: "hayami"})
		p.Draw("cooler", cooler.sec, true)
		return gui.CardMinSize(p, "cooler"), gui.CardRows(p, "cooler")
	}
	plain, _ := size("", "")
	for _, names := range [][2]string{
		{"NZXT Kraken Elite V2", "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]"},
		{"WWWWWWWWWWWWWWWWWWWW", "MMMMMMMMMMMMMMMMMMMM"},
	} {
		got, rows := size(names[0], names[1])
		assert.Equal(t, plain, got, "the card changed size with %q", rows)
	}
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
R1.1, R3.1. The window draws each rate as its own part in its own band, the
down and the up independently: the info colour from 1 MiB/s, amber from 10, the
design system's magenta in bold from 100, the row's own text below 1. The
arrows stay plain, and the parts spell the value the row measures.
*/
func TestTheWindowColoursEachRateOnItsOwn(t *testing.T) {
	a := test.NewTempApp(t)
	src := &fixed{view.Bandwidth([]view.BandwidthReading{{Name: "enp5s0", HasRate: true}})}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	parts := func(rx, tx float64) (down, up glance.Part) {
		sec := view.Bandwidth([]view.BandwidthReading{{Name: "enp5s0", RxRate: rx, TxRate: tx, HasRate: true}})
		p.Draw("bandwidth", sec, true)
		ps := gui.RowParts(p, "bandwidth", 0)
		require.Len(t, ps, 4)
		text := ""
		for _, part := range ps {
			text += part.Text
		}
		assert.Equal(t, sec.Rows[0].Value, text, "the parts are not the value")
		assert.Nil(t, ps[0].Colour, "an arrow took a colour")
		assert.Nil(t, ps[2].Colour, "an arrow took a colour")
		return ps[1], ps[3]
	}

	down, up := parts(12<<10, 40<<20)
	assert.Nil(t, down.Colour, "a quiet rate took a colour")
	assert.Equal(t, fd.StatusInfo, down.Status)
	assert.Equal(t, fd.StatusWarn, up.Status, "the up rate's band is its own")

	down, up = parts(2<<20, 400<<20)
	require.NotNil(t, down.Colour, "1 MiB/s and up is the info colour")
	require.NotNil(t, up.Colour)
	assert.False(t, down.Bold)
	assert.True(t, up.Bold, "the strongest band is bold")
	assert.NotEqual(t, down.Colour, up.Colour)

	down, up = parts(400<<20, 1<<10)
	assert.True(t, down.Bold)
	assert.Nil(t, up.Colour, "the down rate's band reached the up rate")
	for _, part := range gui.RowParts(p, "bandwidth", 0) {
		assert.NotEqual(t, fd.StatusBad, part.Status, "a fast rate is not a failure")
	}
}
