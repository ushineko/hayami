package gui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// fourDevices is a desk with one device of each kind the card has a slot for.
func fourDevices() view.PeripheralsReading {
	return view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: "Mouse", Level: 76, Kind: view.KindMouse},
		{Name: "Keyboard", Level: 50, Kind: view.KindKeyboard},
		{Name: "Headset", Level: 40, Kind: view.KindHeadset},
		{Name: "Controller", Level: 90, Kind: view.KindGamepad},
	}}
}

// oneLine is the default settings with every device on one line or not, in
// the arrangement given.
func oneLine(arrangement string, on bool) config.Config {
	c := config.Default()
	c.Arrangement = arrangement
	return config.Peripherals.Set(c, config.PeripheralsSettings{OneLine: on})
}

/*
Spec 071 R1, R3. Four devices wrap by default, as many across as the window's
width fits. With the setting on they are one line and the window is wider;
off again, they wrap as they did at the old width, with no restart.
*/
func TestEveryDeviceGoesOnOneLineAndBack(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{reading: fourDevices()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	p.Apply(oneLine("stack", false))
	p.Draw("peripherals", src.Section(), true)
	require.Equal(t, 4, gui.ShownCells(p, "peripherals"))
	lines := gui.CellLines(p, "peripherals")
	require.Greater(t, lines, 1, "four devices should wrap by default")
	wrapped := p.Window().Panel().Size()

	p.Apply(oneLine("stack", true))
	assert.Equal(t, 1, gui.CellLines(p, "peripherals"))
	assert.Greater(t, p.Window().Panel().Size().Width, wrapped.Width, "the window did not grow wider")

	p.Apply(oneLine("stack", false))
	assert.Equal(t, lines, gui.CellLines(p, "peripherals"))
	assert.InDelta(t, wrapped.Width, p.Window().Panel().Size().Width, 1, "the window did not return to its width")
}

// Spec 071 R4. In row the library's Lines wins: one device per line.
func TestRowIsOneDevicePerLineWhateverTheSetting(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{reading: fourDevices()}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	p.Draw("peripherals", src.Section(), true)

	p.Apply(oneLine("row", true))

	assert.Equal(t, 4, gui.CellLines(p, "peripherals"))
}
