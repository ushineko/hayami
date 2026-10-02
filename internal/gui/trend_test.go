package gui_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// growing is a counter table that moves on every read, so a second poll has a
// rate to plot.
func growing() func() (map[string]core.Counters, error) {
	var n uint64
	return func() (map[string]core.Counters, error) {
		n++
		return map[string]core.Counters{
			"eno2":  {Rx: n * 1 << 20, Tx: n * 1 << 10},
			"wlan0": {Rx: n * 1 << 10, Tx: n},
		}, nil
	}
}

// The bandwidth card plots its trend, one trace per interface per direction
// (spec 021). The card is built from the first poll, which has no rate yet,
// so the traces must be there to plot into before any sample is.
func TestTheBandwidthCardPlotsFourTraces(t *testing.T) {
	a := test.NewTempApp(t)
	src := panel.NewBandwidth([]string{"eno2", "wlan0"}, growing())
	_, err := src.Poll(t.Context())
	require.NoError(t, err)

	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	require.NotNil(t, p)

	for range 3 {
		// A rate needs time to have passed, and polls this close together
		// can read one instant on a clock as coarse as Windows'.
		time.Sleep(2 * time.Millisecond)
		_, err = src.Poll(t.Context())
		require.NoError(t, err)
	}
	p.Draw("bandwidth", src.Section(), true)

	for _, trace := range []string{"eno2 ↓", "eno2 ↑", "wlan0 ↓", "wlan0 ↑"} {
		assert.Len(t, gui.SparkSamples(p, "bandwidth", trace), 3, "trace %q", trace)
	}

	// All four against one range, so the quieter interface is the flatter
	// line.
	scale, ok := gui.SparkScale(p, "bandwidth")
	require.True(t, ok, "the bandwidth card has no plot")
	assert.Equal(t, glance.ScaleShared, scale)
}

// coolerSource is the cooler's section with a trend, without a cooler.
type coolerSource struct{}

func (coolerSource) Key() string                        { return "cooler" }
func (coolerSource) Interval() time.Duration            { return time.Hour }
func (coolerSource) Poll(context.Context) (bool, error) { return true, nil }
func (coolerSource) Data() any                          { return nil }
func (coolerSource) Section() view.Section {
	return view.Cooler(view.CoolerReading{
		HasLiquid: true, Coolant: 46, Trail: []float64{45.8, 46, 46.2},
		HasCPU: true, CPU: 70, CPUTrail: []float64{65, 80, 98},
		HasGPU: true, GPU: 41, GPUTrail: []float64{40, 41, 43},
	})
}

// The cooler keeps its own kind of plot: each series on its own range, which
// is what makes a coolant moving under a degree readable beside a processor
// swinging thirty-five.
func TestTheCoolerCardKeepsEachSeriesOnItsOwnScale(t *testing.T) {
	a := test.NewTempApp(t)
	p := gui.New(a, gui.Options{Sources: []panel.Source{coolerSource{}}, Title: "hayami"})

	scale, ok := gui.SparkScale(p, "cooler")
	require.True(t, ok, "the cooler card has no plot")
	assert.Equal(t, glance.ScaleEach, scale)
}

// A bandwidth trace is coloured by its interface, the up line the faded form
// of the down; the coolant keeps its status colour and the processors take
// series colours (the rule is the trail's Coloured, not the section's
// scale); a section whose source has gone dims both alike.
func TestTraceColoursFollowTheSeriesUnderASharedScale(t *testing.T) {
	a := test.NewTempApp(t)
	src := panel.NewBandwidth([]string{"eno2"}, growing())
	p := gui.New(a, gui.Options{Sources: []panel.Source{src, coolerSource{}}, Title: "hayami"})
	th := gui.PanelTheme(p)

	bw := view.Section{TrailScale: view.ScaleShared}
	down1 := view.Trail{Name: "wlan0 ↓", Series: 1, Coloured: true}
	up1 := view.Trail{Name: "wlan0 ↑", Series: 1, Secondary: true, Coloured: true}
	down0 := view.Trail{Name: "eno2 ↓", Series: 0, Coloured: true}

	assert.Equal(t, glance.SeriesColour(th, 1), gui.TrailColour(p, down1, bw))
	assert.Equal(t, glance.Faded(glance.SeriesColour(th, 1)), gui.TrailColour(p, up1, bw))
	assert.NotEqual(t, gui.TrailColour(p, down0, bw), gui.TrailColour(p, down1, bw),
		"two interfaces are two colours")

	cooler := coolerSource{}.Section()
	coolant := cooler.Trails[0]
	assert.NotEqual(t, glance.SeriesColour(th, coolant.Series), gui.TrailColour(p, coolant, cooler),
		"the cooler's trace keeps its status colour")

	cpu, gpu := cooler.Trails[1], cooler.Trails[2]
	assert.Equal(t, glance.SeriesColour(th, 0), gui.TrailColour(p, cpu, cooler),
		"the processor's trace stays the link blue it has always been")
	assert.Equal(t, glance.SeriesColour(th, 1), gui.TrailColour(p, gpu, cooler),
		"the graphics card's is violet")
	assert.NotEqual(t, gui.TrailColour(p, cpu, cooler), gui.TrailColour(p, gpu, cooler),
		"three traces on one plot are three colours")

	gone := bw
	gone.Gone = true
	cooler.Gone = true
	assert.Equal(t, gui.TrailColour(p, coolant, cooler), gui.TrailColour(p, down1, gone),
		"a section whose source has gone is dim whatever its scale")
}

// The window's plot holds what the pane's does, for both trends: one constant
// (spec 021 R3.3).
func TestThePlotHoldsBothTrendsWindows(t *testing.T) {
	assert.Equal(t, core.BandwidthTrail, gui.SparkCapacity)
	assert.Equal(t, panel.CoolerTrail, gui.SparkCapacity)
}
