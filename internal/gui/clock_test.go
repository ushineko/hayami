package gui_test

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 106 R2, R3. The clock's card is its heading: the time and the date,
// changed in place as the clock ticks, and nothing under them.
func TestTheClockCardIsItsHeadingAndTicks(t *testing.T) {
	a := test.NewTempApp(t)
	at := time.Date(2026, 11, 21, 18, 25, 4, 0, time.Local)
	clock := &fixed{view.Clock(at)}
	p := gui.New(a, gui.Options{Sources: []panel.Source{clock}, Title: "hayami"})

	p.Draw("clock", view.Clock(at), true)
	assert.Equal(t, []string{"18:25:04", "Sat 2026/11/21"}, gui.CardText(p, "clock"))
	tall := gui.CardHeight(p, "clock")

	p.Draw("clock", view.Clock(at.Add(time.Second)), true)
	assert.Equal(t, []string{"18:25:05", "Sat 2026/11/21"}, gui.CardText(p, "clock"))
	assert.InDelta(t, tall, gui.CardHeight(p, "clock"), 0.01)

	p.Draw("clock", view.Clock(at.Add(6*time.Hour)), true)
	assert.Equal(t, []string{"00:25:04", "Sun 2026/11/22"}, gui.CardText(p, "clock"), "the date did not turn")
}

// Spec 106 R3. In row -- one line per reading -- the clock keeps its heading,
// which is its reading; the other cards leave theirs out.
func TestTheClockKeepsItsHeadingInRow(t *testing.T) {
	a := test.NewTempApp(t)
	at := time.Date(2026, 11, 21, 18, 25, 4, 0, time.Local)
	clock := &fixed{view.Clock(at)}
	cooler := &fixed{coolerWith()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{clock, cooler}, Title: "hayami"})
	p.Draw("clock", view.Clock(at), true)
	p.Draw("cooler", cooler.sec, true)

	p.Apply(config.Config{Sections: []string{"clock", "cooler"}, Arrangement: "row"})
	require.Contains(t, gui.CardText(p, "clock"), "18:25:04", "the clock lost its heading in row")
	assert.Contains(t, gui.CardText(p, "clock"), "Sat 2026/11/21")
	assert.NotContains(t, gui.CardText(p, "cooler"), view.CoolerInfo.Title, "a card with rows kept its heading in row")
}
