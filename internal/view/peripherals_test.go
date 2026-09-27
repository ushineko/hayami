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
		require.Len(t, s.Rows, 1)
		assert.Equal(t, c.want, s.Rows[0].Status, "at %d %%", c.level)
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
	require.Len(t, s.Rows, 1)

	assert.Equal(t, view.Info, s.Rows[0].Status, "a charging device was coloured for being empty")
	assert.Equal(t, "charging", s.Rows[0].Detail)
}

// AC7. Charged is its own word. A device that has finished is not still
// filling, and a panel that said so would be wrong for as long as it stayed on
// the cable.
func TestAChargedDeviceSaysCharged(t *testing.T) {
	d := reading("Arctis", 100)
	d.Charge = view.Charged

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Rows, 1)
	assert.Equal(t, "charged", s.Rows[0].Detail)
}

// AC5. A stale device keeps its number and loses its verdict. A red row for a
// battery nobody has heard from in ten minutes asserts something the panel
// does not know.
func TestAStaleDeviceKeepsItsNumberAndLosesItsVerdict(t *testing.T) {
	d := reading("G502 X PLUS", 15) // low enough to be red if it were current
	d.Stale = true

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Rows, 1)

	assert.Equal(t, view.Dim, s.Rows[0].Status)
	assert.Contains(t, s.Rows[0].Value, "15")
	assert.Equal(t, "not answering", s.Rows[0].Detail)
}

// AC5. A device with no level at all has no verdict either, and keeps its unit
// so the column does not move when one device goes quiet.
func TestADeviceWithNoLevelHasNoVerdictAndKeepsItsColumn(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{
			{Name: "Arctis"},
			reading("G502 X PLUS", 86),
		},
	})
	require.Len(t, s.Rows, 2)

	assert.Equal(t, view.Dim, s.Rows[0].Status)
	assert.Equal(t, s.Rows[1].Unit, s.Rows[0].Unit, "a quiet device moved the unit column")
	assert.Equal(t, len(s.Rows[1].Value), len(s.Rows[0].Value), "a quiet device moved the number column")
}

// A section with no devices has no rows, so the shells draw no heading.
func TestNoDevicesIsNoRows(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{})
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

// AC4. A device with several cells is one row and a quiet line beneath it.
func TestADeviceWithCellsIsOneRowAndADetailLine(t *testing.T) {
	d := reading("AirPods Pro", 80)
	d.Cells = []view.PeripheralCell{
		{Name: "L", Level: 80},
		{Name: "R", Level: 90},
		{Name: "case", Level: 50},
	}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})

	require.Len(t, s.Rows, 1, "a pair of earbuds became more than one row")
	assert.Equal(t, "L 80  R 90  case 50", s.Rows[0].Detail)
	assert.Contains(t, s.Rows[0].Value, "80")
}

// AC3. A device with one battery has no detail line to draw.
func TestADeviceWithOneBatteryHasNoCellLine(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{reading("G502 X PLUS", 86)},
	})
	require.Len(t, s.Rows, 1)
	assert.Empty(t, s.Rows[0].Detail)
}

// The cells and what the battery is doing are both said, not one instead of
// the other.
func TestCellsAndChargingAreBothSaid(t *testing.T) {
	d := reading("AirPods Pro", 40)
	d.Charge = view.Filling
	d.Cells = []view.PeripheralCell{{Name: "L", Level: 40}, {Name: "R", Level: 45}}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Rows, 1)

	assert.Contains(t, s.Rows[0].Detail, "L 40")
	assert.Contains(t, s.Rows[0].Detail, "charging")
}
