package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// reading is one device at a level.
func reading(name string, level int) view.PeripheralReading {
	return view.PeripheralReading{Name: name, Level: level}
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
		require.Len(t, s.Cells, view.PeripheralSlots)
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
	require.Len(t, s.Cells, view.PeripheralSlots)

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
	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "Charged", s.Cells[0].Note)
}

// AC2. A stale cell keeps its number and its verdict, and is marked for the
// shells to dim.
//
// Dropping the verdict as well would take a low battery's colour away at the
// moment it is least likely to be getting charged. The design system dims a
// stale reading over whatever colour it had, which says the one thing that
// needs saying: this is the last number heard.
func TestAStaleCellKeepsItsNumberAndItsVerdict(t *testing.T) {
	d := reading("G502 X PLUS", 15) // low enough to be red
	d.Stale = true

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Cells, view.PeripheralSlots)

	assert.True(t, s.Cells[0].Stale)
	assert.Equal(t, view.Bad, s.Cells[0].Status, "the verdict was dropped as well as dimmed")
	assert.Contains(t, s.Cells[0].Value, "15")
	assert.Equal(t, "Offline", s.Cells[0].Note)
}

// AC2. A stale cell is painted dim in the pane, whatever its verdict. Dimming
// wins over the verdict, as it does for a row in the design system.
func TestAStaleCellIsPaintedDim(t *testing.T) {
	d := reading("G502 X PLUS", 15)
	d.Stale = true
	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})

	seen := map[view.Status][]string{}
	painter := func(text string, st view.Status) string {
		seen[st] = append(seen[st], text)
		return text
	}
	view.RenderWith([]view.Section{s}, view.ArrangeStack, 40, painter)

	assert.Contains(t, strings.Join(seen[view.Dim], ""), "15")
	assert.NotContains(t, strings.Join(seen[view.Bad], ""), "15",
		"a stale reading kept its status colour in the pane")
}

// AC15. The mouse's cell is first, whatever it is called.
//
// A desk has one mouse, it is there whenever the machine is, and its battery is
// the one a glance is usually after. Ordering by name alone put the headset in
// the first cell on the machine this was written on, which is the device its
// owner thinks about least.
func TestTheMouseComesFirst(t *testing.T) {
	mouse := reading("G502 X PLUS", 78)
	mouse.Kind = view.KindMouse
	headset := reading("Arctis Nova Pro Wireless", 47)
	headset.Kind = view.KindHeadset
	keyboard := reading("Keychron K4 HE", 90)
	keyboard.Kind = view.KindKeyboard
	other := reading("A Gamepad", 60)

	// Given in the order a name sort would produce, which is the order this
	// must not be.
	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{other, headset, mouse, keyboard})})

	// Two slots: the mouse, then the best of the rest, which with everything
	// detected at once is the keyboard. The other two are the note.
	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, []string{"G502 X PLUS", "Keychron K4 HE"},
		[]string{s.Cells[0].Label, s.Cells[1].Label})
	assert.Contains(t, s.Note, "Arctis Nova Pro Wireless")
	assert.Contains(t, s.Note, "A Gamepad")
}

// AC15. Within a kind it is still by name, so a cell moves only when the
// hardware does.
func TestTwoDevicesOfOneKindAreOrderedByName(t *testing.T) {
	first := reading("MX Master 3S", 40)
	first.Kind = view.KindMouse
	second := reading("G502 X PLUS", 78)
	second.Kind = view.KindMouse

	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{first, second})})

	require.Len(t, s.Cells, 2)
	assert.Equal(t, "G502 X PLUS", s.Cells[0].Label)
	assert.Equal(t, "MX Master 3S", s.Cells[1].Label)
}

// The state is said under every cell, not only the ones doing something
// unusual.
//
// The row form said it only for a battery that was charging, which left the
// ordinary case -- a battery discharging normally -- looking exactly like a
// device nobody had heard from. A blank third of a cell reads as one that has
// not finished loading.
func TestEveryCellSaysWhatItsBatteryIsDoing(t *testing.T) {
	filling := reading("Keychron K4 HE", 12)
	filling.Charge = view.Filling

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		reading("G502 X PLUS", 67), filling,
	}})

	require.Len(t, s.Cells, 2)
	assert.Equal(t, "Discharging", s.Cells[0].Note, "the ordinary case is still a case")
	assert.Equal(t, "Charging", s.Cells[1].Note)
}

// AC (spec 022). No devices at all is both placeholders, so the card is the
// same shape as with two. Whether the card is drawn at all is the panel's
// decision (Poll), not the section's.
func TestNoDevicesIsTwoPlaceholders(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, view.NoMouse, s.Cells[0].Label)
	assert.Equal(t, view.NoDevice, s.Cells[1].Label)
	for _, c := range s.Cells {
		assert.True(t, c.Placeholder)
		assert.True(t, c.Stale, "a placeholder is drawn dim, like a quiet device")
		assert.Equal(t, view.NoQuantity(), c.Value)
	}
	assert.Empty(t, s.Rows)
	assert.Empty(t, s.Reasons, "a placeholder is not a reason")
	assert.Empty(t, s.Note)
	assert.Equal(t, "peripherals", s.Key)
}

// AC (spec 022). A mouse alone is the mouse and the "no device" placeholder,
// so the card does not collapse to one centred cell.
func TestAMouseAloneHasAPlaceholderBesideIt(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		detected("G502 X PLUS", 78, view.KindMouse, t0),
	}})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "G502 X PLUS", s.Cells[0].Label)
	assert.False(t, s.Cells[0].Placeholder)
	assert.Equal(t, view.NoDevice, s.Cells[1].Label)
	assert.True(t, s.Cells[1].Placeholder)
	assert.True(t, s.Cells[1].Stale)
}

// The section renders in every arrangement without the shells knowing what a
// battery is, which is the whole point of the view being data.
func TestTheSectionRendersInEveryArrangement(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{
			reading("G502 X PLUS", 86),
			reading("Arctis Nova Pro Wireless", 47),
		},
	})

	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		lines := view.Render([]view.Section{s}, a, 60)
		joined := strings.Join(lines, "\n")
		assert.Contains(t, joined, "G502 X PLUS", "in %s", a)
		assert.Contains(t, joined, "86", "in %s", a)
	}
}

// R5 (spec 022). The pane draws the placeholder too, in every arrangement, so
// both shells show the same two cells.
func TestThePaneDrawsThePlaceholder(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 86, Kind: view.KindMouse}},
	})

	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		joined := strings.Join(view.Render([]view.Section{s}, a, 60), "\n")
		assert.Contains(t, joined, "G502 X PLUS", "in %s", a)
		assert.Contains(t, joined, view.NoDevice, "in %s", a)
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

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.True(t, s.Cells[1].Placeholder, "a pair of earbuds became more than one cell")
	assert.Equal(t, "L 80  R 90  case 50", s.Cells[0].Note)
	assert.Contains(t, s.Cells[0].Value, "80")
}

// AC3. A device with one battery lists no separate batteries; its note is the
// state alone.
func TestADeviceWithOneBatteryListsNoCells(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{reading("G502 X PLUS", 86)},
	})
	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "Discharging", s.Cells[0].Note)
}

// The cells and what the battery is doing are both said, not one instead of
// the other.
func TestCellsAndChargingAreBothSaid(t *testing.T) {
	d := reading("AirPods Pro", 40)
	d.Charge = view.Filling
	d.Cells = []view.PeripheralCell{{Name: "L", Level: 40}, {Name: "R", Level: 45}}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{d}})
	require.Len(t, s.Cells, view.PeripheralSlots)

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

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "L 80  R 90", s.Cells[0].Note)
}

// detected is a device seen at a moment, live unless it is marked stale.
func detected(name string, level int, k view.Kind, at time.Time) view.PeripheralReading {
	d := reading(name, level)
	d.Kind, d.Since, d.Seen = k, at, at
	return d
}

// AC. The section never draws more than two devices, so the card does not
// change width when a pair of headphones is connected.
func TestTheSectionDrawsTwoDevicesAtMost(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	devices := []view.PeripheralReading{
		detected("G502 X PLUS", 78, view.KindMouse, t0),
		detected("Arctis Nova Pro", 47, view.KindHeadset, t0),
		detected("AirPods Pro", 81, view.KindHeadset, t0),
		detected("Keychron K4 HE", 90, view.KindKeyboard, t0),
	}

	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(devices)})

	assert.Len(t, s.Cells, 2)
}

// AC. The right slot goes to the device switched on most recently, not to the
// one whose name sorts first.
func TestTheRightSlotGoesToTheNewestLiveDevice(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	devices := []view.PeripheralReading{
		detected("G502 X PLUS", 78, view.KindMouse, t0),
		detected("Arctis Nova Pro", 47, view.KindHeadset, t0),
		detected("AirPods Pro", 81, view.KindHeadset, t0.Add(time.Minute)),
	}

	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(devices)})

	require.Len(t, s.Cells, 2)
	assert.Equal(t, "G502 X PLUS", s.Cells[0].Label, "the mouse gave up the left slot")
	assert.Equal(t, "AirPods Pro", s.Cells[1].Label,
		"the right slot went to the older device")
	assert.Contains(t, s.Note, "Arctis Nova Pro", "the device with no slot was not named")
}

// AC (spec 022). A mouse and a headset: the mouse left, the headset right.
func TestAMouseAndAHeadset(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{
			detected("Arctis Nova Pro Wireless", 47, view.KindHeadset, t0),
			detected("G502 X PLUS", 78, view.KindMouse, t0),
		})})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, []string{"G502 X PLUS", "Arctis Nova Pro Wireless"},
		[]string{s.Cells[0].Label, s.Cells[1].Label})
	assert.Empty(t, s.Note)
}

// AC (spec 022). The headset went quiet and the AirPods connected after that:
// the connection is the more recent change, so the AirPods take the slot.
func TestADeviceConnectedAfterAnotherWentQuietTakesTheSlot(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	mouse := detected("G502 X PLUS", 78, view.KindMouse, t0)
	headset := detected("Arctis Nova Pro Wireless", 47, view.KindHeadset, t0)
	headset.Stale, headset.Seen = true, t0.Add(time.Minute)
	airpods := detected("AirPods Pro", 81, view.KindHeadset, t0.Add(2*time.Minute))

	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{mouse, headset, airpods})})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "G502 X PLUS", s.Cells[0].Label)
	assert.Equal(t, "AirPods Pro", s.Cells[1].Label)
	assert.False(t, s.Cells[1].Stale)
	assert.Contains(t, s.Note, "Arctis Nova Pro Wireless")
}

// AC (spec 022). The AirPods then went quiet too, after the headset: the
// disconnection is the most recent change, so the AirPods keep the slot, dim,
// with their last level. Live is not ranked above quiet.
func TestADeviceThatWentQuietLastKeepsTheSlot(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	mouse := detected("G502 X PLUS", 78, view.KindMouse, t0)
	headset := detected("Arctis Nova Pro Wireless", 47, view.KindHeadset, t0)
	headset.Stale, headset.Seen = true, t0.Add(time.Minute)
	airpods := detected("AirPods Pro", 81, view.KindHeadset, t0.Add(2*time.Minute))
	airpods.Stale, airpods.Seen = true, t0.Add(3*time.Minute)

	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{mouse, headset, airpods})})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "AirPods Pro", s.Cells[1].Label)
	assert.True(t, s.Cells[1].Stale, "a quiet device is drawn dim")
	assert.Contains(t, s.Cells[1].Value, "81", "the last level was not kept")
}

// AC (spec 022). A quiet device whose silence is newer than another's arrival
// holds the slot over a live one: the reader's most recent change is what the
// slot shows.
func TestAQuietDeviceCanHoldTheSlotOverALiveOne(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	quiet := detected("Arctis Nova Pro", 47, view.KindHeadset, t0)
	quiet.Stale, quiet.Seen = true, t0.Add(time.Hour)
	live := detected("AirPods Pro", 81, view.KindHeadset, t0.Add(time.Minute))
	mouse := detected("G502 X PLUS", 78, view.KindMouse, t0)

	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{quiet, live, mouse})})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "Arctis Nova Pro", s.Cells[1].Label)
	assert.Contains(t, s.Note, "AirPods Pro")
}

// AC (spec 022). Three live devices: two cells and the third in the note.
func TestThreeLiveDevicesGiveTheOverflowNote(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{
			detected("G502 X PLUS", 78, view.KindMouse, t0),
			detected("Arctis Nova Pro", 47, view.KindHeadset, t0),
			detected("AirPods Pro", 81, view.KindHeadset, t0.Add(time.Minute)),
		})})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "AirPods Pro", s.Cells[1].Label)
	assert.Equal(t, "Also connected:\n  Arctis Nova Pro  47 %  Discharging", s.Note)
}

// Without a mouse the slots are filled from the rest, as before spec 022; the
// "no mouse" placeholder is only for a desk with nothing on it.
func TestWithoutAMouseTheLeftSlotIsNotLeftEmpty(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		detected("AirPods Pro", 81, view.KindHeadset, t0),
	}})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "AirPods Pro", s.Cells[0].Label)
	assert.Equal(t, view.NoDevice, s.Cells[1].Label)
}

// AC. Two devices are two cells and no note, so the ordinary desk says nothing
// about overflow.
func TestTwoDevicesHaveNoNote(t *testing.T) {
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s := view.Peripherals(view.PeripheralsReading{Devices: view.OrderPeripherals(
		[]view.PeripheralReading{
			detected("G502 X PLUS", 78, view.KindMouse, t0),
			detected("AirPods Pro", 81, view.KindHeadset, t0),
		})})

	assert.Len(t, s.Cells, 2)
	assert.Empty(t, s.Note)
}

/*
A band cell keeps the shape of a number cell.

The segments go where the percentage goes, so a row still lines up and the eye
lands in the same place; the band's word takes the quiet line. A band device is
then obviously not a measured one without a caption saying so (spec 018).
*/
func TestABandCellDrawsSegmentsWhereANumberWouldGo(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: "Logitech K800", Band: "Good", Segments: 3},
	}})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "▮▮▮▯", s.Cells[0].Value)
	assert.Equal(t, "Good", s.Cells[0].Note)
	assert.Empty(t, s.Cells[0].Unit, "there is no percent sign without a percentage")
}

// The verdict follows the band, on the same thresholds a percentage uses.
func TestTheVerdictFollowsTheBand(t *testing.T) {
	for _, c := range []struct {
		segments int
		want     view.Status
	}{{1, view.Bad}, {2, view.Warn}, {3, view.Good}, {4, view.Good}} {
		s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
			{Name: "K800", Band: "x", Segments: c.segments},
		}})

		require.Len(t, s.Cells, view.PeripheralSlots)
		assert.Equal(t, c.want, s.Cells[0].Status, "%d segments", c.segments)
	}
}

// Charging takes the quiet line: it is the more urgent fact, and the segments
// already carry the band.
func TestChargingWinsTheQuietLineFromTheBand(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: "K800", Band: "Good", Segments: 3, Charge: view.Filling},
	}})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "Charging", s.Cells[0].Note)
	assert.Equal(t, "▮▮▮▯", s.Cells[0].Value, "the band is still drawn")
}

// And a percentage device is untouched by any of it.
func TestAPercentageCellIsUnchanged(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: "G502 X PLUS", Level: 78},
	}})

	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.Equal(t, "78", s.Cells[0].Value)
	assert.Equal(t, "%", s.Cells[0].Unit)
}

// AC (spec 025). A level cell has a bar of its level over a hundred, in the
// cell's status; a band cell and a placeholder have none.
func TestALevelCellHasABarAndABandOrAPlaceholderNone(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	mouse := detected("G502 X PLUS", 18, view.KindMouse, t0)
	band := view.PeripheralReading{Name: "K800", Band: "Good", Segments: 3, Kind: view.KindKeyboard,
		Since: t0, Seen: t0}

	s := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{mouse, band}})
	require.Len(t, s.Cells, view.PeripheralSlots)
	assert.True(t, s.Cells[0].HasBar)
	assert.InDelta(t, 0.18, s.Cells[0].Bar, 1e-9)
	assert.Equal(t, view.Bad, s.Cells[0].Status, "the bar takes the cell's status")
	assert.False(t, s.Cells[1].HasBar, "a band cell has a bar under its segments")

	alone := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{mouse}})
	require.Len(t, alone.Cells, view.PeripheralSlots)
	assert.True(t, alone.Cells[1].Placeholder)
	assert.False(t, alone.Cells[1].HasBar, "a placeholder has a bar")
}
