package gui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// coolerWith is a cooler section of a processor, the given extra probes, and a
// coolant, each with samples so the card has a plot.
func coolerWith(extra ...view.Probe) view.Section {
	probes := []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Load: view.Some(12.0), Temp: view.Some(61.0), Trail: []float64{60, 61}},
		{ID: "coolant", Role: view.RoleCoolant, Temp: view.Some(31.4), Trail: []float64{31, 31.4}},
	}
	return view.Cooler(view.CoolerReading{Probes: append(probes, extra...)})
}

// ids are the rows' IDs, in order.
func ids(rows []gui.RowPlace) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// top is the top of the row with id.
func top(t *testing.T, rows []gui.RowPlace, id string) float32 {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r.Top
		}
	}
	require.Failf(t, "no row", "no row %q in %v", id, ids(rows))
	return 0
}

/*
Spec 044. A card's rows follow what is read: a graphics card that starts
answering is a row inserted at its place -- after the processor, before the
coolant -- above the plot, and the processor's row does not move. When the card
stops answering its row goes and the coolant goes back where it was. Before
this the card was built with four hidden spare rows, and a row beyond them
went under the plot.
*/
func TestARowArrivingIsInsertedAboveThePlotInItsPlace(t *testing.T) {
	a := test.NewTempApp(t)
	cooler := &fixed{coolerWith()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{cooler}, Title: "hayami"})
	p.Draw("cooler", cooler.sec, true)

	before, plotBefore := gui.LaidOut(p, "cooler")
	require.Equal(t, []string{"cpu", "coolant"}, ids(before))
	require.Positive(t, plotBefore, "the card has a plot")

	gpu := view.Probe{ID: "gpu", Role: view.RoleGPU, Load: view.Some(7.0), Temp: view.Some(44.0), Trail: []float64{44}}
	p.Draw("cooler", coolerWith(gpu), true)
	during, plotDuring := gui.LaidOut(p, "cooler")

	assert.Equal(t, []string{"cpu", "gpu", "coolant"}, ids(during), "the card's row is in its place")
	for _, r := range during {
		assert.Less(t, r.Top, plotDuring, "row %q is under the plot", r.ID)
	}
	assert.Equal(t, top(t, before, "cpu"), top(t, during, "cpu"), "the processor's row moved")
	assert.Greater(t, top(t, during, "coolant"), top(t, before, "coolant"), "the coolant makes room")

	p.Draw("cooler", coolerWith(), true)
	after, plotAfter := gui.LaidOut(p, "cooler")
	assert.Equal(t, ids(before), ids(after), "the card's row is gone")
	assert.Equal(t, top(t, before, "coolant"), top(t, after, "coolant"), "the coolant is back where it was")
	assert.Equal(t, plotBefore, plotAfter, "the plot is back where it was")
}

// A reason that arrives after the card was built -- the cooler stops
// answering an hour in -- is a row above the plot, not through it.
func TestAReasonArrivingLaterIsAboveThePlot(t *testing.T) {
	a := test.NewTempApp(t)
	cooler := &fixed{coolerWith()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{cooler}, Title: "hayami"})
	p.Draw("cooler", cooler.sec, true)

	sec := coolerWith()
	sec.Reasons = []view.Reason{{Text: "the cooler would not answer", Status: view.Warn}}
	p.Draw("cooler", sec, true)
	rows, plot := gui.LaidOut(p, "cooler")

	require.Len(t, rows, 3)
	for _, r := range rows {
		assert.Less(t, r.Top, plot, "row %q (%s) is under the plot", r.ID, r.Label)
	}
}

// A value changing is not a row changing: every row keeps its place and its
// object, poll after poll.
func TestAValueChangingMovesNoRow(t *testing.T) {
	a := test.NewTempApp(t)
	cooler := &fixed{coolerWith()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{cooler}, Title: "hayami"})
	p.Draw("cooler", cooler.sec, true)
	before, plotBefore := gui.LaidOut(p, "cooler")

	hotter := view.Cooler(view.CoolerReading{Probes: []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Name: "AMD Ryzen 5 2600X Six-Core Processor", Load: view.Some(99.0), Temp: view.Some(88.0), Trail: []float64{60, 88}},
		{ID: "coolant", Role: view.RoleCoolant, Temp: view.Some(52.0), Trail: []float64{31, 52}},
	}})
	p.Draw("cooler", hotter, true)
	after, plotAfter := gui.LaidOut(p, "cooler")

	assert.Equal(t, before, withLabels(after, before), "a value or a name arriving moved a row")
	assert.Equal(t, plotBefore, plotAfter)
}

// withLabels is after with before's labels, so a comparison of places is not
// a comparison of a name that arrived.
func withLabels(after, before []gui.RowPlace) []gui.RowPlace {
	out := append([]gui.RowPlace(nil), after...)
	for i := range out {
		if i < len(before) {
			out[i].Label = before[i].Label
		}
	}
	return out
}
