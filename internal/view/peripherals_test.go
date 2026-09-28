package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// reading is one device at a level.
func reading(name string, level int) view.PeripheralReading {
	return view.PeripheralReading{Name: name, Level: level, HasLevel: true}
}

// AC7. The bands are the reference's: red at 20 and below, amber to 50, green
// above.
func TestALevelIsColouredAtTheBands(t *testing.T) {
	for _, c := range []struct {
		level int
		want  view.Status
	}{
		{0, view.Bad},
		{20, view.Bad},
		{21, view.Warn},
		{50, view.Warn},
		{51, view.Good},
		{100, view.Good},
	} {
		s := view.Peripherals(view.PeripheralsReading{
			Devices: []view.PeripheralReading{reading("A Device", c.level)},
		})
		require.Len(t, s.Cells, 1)
		assert.Equal(t, c.want, s.Cells[0].Status, "at %d %%", c.level)
	}
}

// AC7. Charging is said and not coloured.
//
// A device on its cable is not a warning however empty it is — it is being
// dealt with — and a red row for the one battery nobody needs to think about
// is the panel crying wolf.
func TestAChargingDeviceIsSaidAndNotColoured(t *testing.T) {
	d := reading("Arctis", 5)
	d.Charge = view.Filling

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Cells, 1)

	assert.Equal(t, view.Info, s.Cells[0].Status, "a charging device was coloured for being empty")
	assert.Equal(t, "Charging", s.Cells[0].Note)
}

// AC7. Charged is its own word. A device that has finished is not still
// filling, and a panel that said so would be wrong for as long as it stayed on
// the cable.
func TestAChargedDeviceSaysCharged(t *testing.T) {
	d := reading("Arctis", 100)
	d.Charge = view.Charged

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Cells, 1)
	assert.Equal(t, "Charged", s.Cells[0].Note)
}

// AC5. A stale device keeps its number and loses its verdict. A red row for a
// battery nobody has heard from in ten minutes asserts something the panel
// does not know.
func TestAStaleDeviceKeepsItsNumberAndLosesItsVerdict(t *testing.T) {
	d := reading("G502 X PLUS", 15) // low enough to be red if it were current
	d.Stale = true

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Cells, 1)

	assert.Equal(t, view.Dim, s.Cells[0].Status)
	assert.Contains(t, s.Cells[0].Value, "15")
	assert.Equal(t, "Not answering", s.Cells[0].Note)
}

// AC5. A device with no level at all has no verdict either, and says so rather
// than drawing a percentage it does not have.
//
// The column rule the row form was held to does not apply here: a cell centres
// its reading in a column of its own, so there is nothing for a lone percent
// sign to hold a place in and a unit on a blank reading is furniture.
func TestADeviceWithNoLevelSaysSoRatherThanShowingAPercentage(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{
			{Name: "Arctis"},
			reading("G502 X PLUS", 86),
		},
	})
	require.Len(t, s.Cells, 2)

	assert.Equal(t, view.Dim, s.Cells[0].Status)
	assert.Empty(t, s.Cells[0].Unit, "a percent sign with no number in front of it")
	assert.Equal(t, "%", s.Cells[1].Unit)
	assert.NotEmpty(t, s.Cells[0].Note, "a cell with no reading must still say why")
}

// The state is said under every cell, not only the ones doing something
// unusual.
//
// The row form said it only for a battery that was charging, which left the
// ordinary case -- a battery discharging normally -- looking exactly like a
// device nobody had heard from. A blank third of a cell reads as one that has
// not finished loading.
func TestEveryCellSaysWhatItsBatteryIsDoing(t *testing.T) {
	quiet := view.PeripheralReading{Name: "Keychron K4 HE"}
	stale := reading("Headset", 30)
	stale.Stale = true

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		reading("G502 X PLUS", 67), quiet, stale,
	}})

	require.Len(t, s.Cells, 3)
	assert.Equal(t, "Discharging", s.Cells[0].Note, "the ordinary case is still a case")
	assert.NotEmpty(t, s.Cells[1].Note)
	assert.Equal(t, "Not answering", s.Cells[2].Note)
}

// A section with no devices has no cells, so the shells draw no heading.
func TestNoDevicesIsNoCells(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{})
	assert.Empty(t, s.Cells)
	assert.Empty(t, s.Rows)
	assert.Equal(t, "peripherals", s.Key)
}

// The section renders in every arrangement without the shells knowing what a
// battery is, which is the whole point of the view being data.
func TestTheSectionRendersInEveryArrangement(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{
			reading("G502 X PLUS", 86),
			{Name: "Arctis Nova Pro Wireless"},
		},
	})

	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		lines := view.Render([]view.Section{s}, a, 60)
		joined := strings.Join(lines, "\n")
		assert.Contains(t, joined, "G502 X PLUS", "in %s", a)
		assert.Contains(t, joined, "86", "in %s", a)
	}
}

// AC4. A device with several batteries is one cell and a quiet line beneath
// it. A pair of earbuds is one device on the desk and should be one block on
// the panel; which ear is low is exactly what the wearer wants to know, and
// that is what the line is for.
func TestADeviceWithSeveralBatteriesIsOneCell(t *testing.T) {
	d := reading("AirPods Pro", 80)
	d.Cells = []view.PeripheralCell{
		{Name: "L", Level: 80},
		{Name: "R", Level: 90},
		{Name: "case", Level: 50},
	}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})

	require.Len(t, s.Cells, 1, "a pair of earbuds became more than one cell")
	assert.Equal(t, "L 80  R 90  case 50", s.Cells[0].Note)
	assert.Contains(t, s.Cells[0].Value, "80")
}

// AC3. A device with one battery lists no separate batteries; its note is the
// state alone.
func TestADeviceWithOneBatteryListsNoCells(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{reading("G502 X PLUS", 86)},
	})
	require.Len(t, s.Cells, 1)
	assert.Equal(t, "Discharging", s.Cells[0].Note)
}

// The cells and what the battery is doing are both said, not one instead of
// the other.
func TestCellsAndChargingAreBothSaid(t *testing.T) {
	d := reading("AirPods Pro", 40)
	d.Charge = view.Filling
	d.Cells = []view.PeripheralCell{{Name: "L", Level: 40}, {Name: "R", Level: 45}}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Cells, 1)

	assert.Contains(t, s.Cells[0].Note, "L 40")
	assert.Contains(t, s.Cells[0].Note, "Charging")
}

// But the ordinary state is not said beside them. "L 80  R 90  case 50
// Discharging" does not fit a cell, and the last word of it is the one worth
// least: for this device the three numbers are the reading.
func TestAnOrdinaryStateIsNotSaidBesideTheSeparateBatteries(t *testing.T) {
	d := reading("AirPods Pro", 80)
	d.Cells = []view.PeripheralCell{{Name: "L", Level: 80}, {Name: "R", Level: 90}}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})

	require.Len(t, s.Cells, 1)
	assert.Equal(t, "L 80  R 90", s.Cells[0].Note)
}
