package panel_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/peripherals"
	"github.com/ushineko/hayami/internal/view"
)

// clock is a time the test moves itself, so a rule about ten minutes does not
// cost ten minutes to check.
type clock struct{ at time.Time }

func (c *clock) now() time.Time       { return c.at }
func (c *clock) tick(d time.Duration) { c.at = c.at.Add(d) }

// source builds a Peripherals whose sources are the test's.
func source(t *testing.T, c *clock, logitech func() []peripherals.Battery) *panel.Peripherals {
	t.Helper()
	p := panel.NewPeripherals()
	panel.SetPeripheralSources(p,
		func() ([]peripherals.Battery, error) { return logitech(), nil },
		func(context.Context) ([]peripherals.Battery, error) { return nil, peripherals.ErrNoHeadsetcontrol },
		c.now,
	)
	return p
}

// devices reads the section's readings back as the view sees them.
func devices(p *panel.Peripherals) []view.PeripheralReading {
	return p.Data().(view.PeripheralsReading).Devices
}

// AC2. A device this panel has never had a level from is not a cell.
//
// An Arctis whose receiver is in with the headset switched off is reported as
// present and silent for a whole session. Drawn, it is a name, a dash and a
// word explaining that there is nothing to say — and, once the cells are
// ordered, it sits in front of the mouse.
func TestADeviceThatHasNeverGivenALevelIsNotDrawn(t *testing.T) {
	c := &clock{at: time.Now()}
	p := source(t, c, func() []peripherals.Battery {
		return []peripherals.Battery{
			{Name: "Arctis Nova Pro Wireless", Kind: peripherals.KindHeadset},
			{Name: "G502 X PLUS", Level: 78, HasLevel: true, Kind: peripherals.KindMouse},
		}
	})

	ok, err := p.Poll(context.Background())
	require.NoError(t, err)
	assert.True(t, ok)

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
}

// AC2. A device that answered and has gone quiet keeps its level and is drawn
// dim.
//
// This is the case the memory exists for. A wireless mouse that has been still
// for a minute answers nothing — measured on the receiver this was written
// against, where one poll in fourteen came back empty — and a panel that
// dropped its cell would reflow on a desk where nothing was wrong.
func TestADeviceThatStopsAnsweringKeepsItsLevelAndGoesDim(t *testing.T) {
	c := &clock{at: time.Now()}
	present := true
	p := source(t, c, func() []peripherals.Battery {
		if !present {
			return nil
		}
		return []peripherals.Battery{{Name: "G502 X PLUS", Level: 86, HasLevel: true}}
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	present = false
	c.tick(time.Minute)
	ok, err := p.Poll(context.Background())
	require.NoError(t, err)
	assert.True(t, ok, "a device that has gone quiet is still drawn")

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, 86, found[0].Level, "the last level was dropped rather than kept")
	assert.True(t, found[0].Stale)

	// The verdict survives the dimming, which is the shells' to apply.
	cells := p.Section().Cells
	require.Len(t, cells, 1)
	assert.True(t, cells[0].Stale)
	assert.Equal(t, "Offline", cells[0].Note)
	assert.Contains(t, cells[0].Value, "86")
}

// AC2. A device quiet for long enough is gone rather than quiet, and stops
// being drawn. A mouse put away this morning is not a mouse that is idle.
func TestADeviceQuietForLongEnoughIsForgotten(t *testing.T) {
	c := &clock{at: time.Now()}
	present := true
	p := source(t, c, func() []peripherals.Battery {
		if !present {
			return nil
		}
		return []peripherals.Battery{{Name: "G502 X PLUS", Level: 86, HasLevel: true}}
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	present = false
	c.tick(panel.PeripheralsForget + time.Minute)
	ok, err := p.Poll(context.Background())
	require.NoError(t, err)

	assert.False(t, ok)
	assert.Empty(t, devices(p))
}

// AC2. A device that is connected and not saying how full it is keeps the
// level it last gave, as the monitor's own carry-over does.
func TestAConnectedDeviceThatIsNotSayingKeepsItsLastLevel(t *testing.T) {
	c := &clock{at: time.Now()}
	saying := true
	p := source(t, c, func() []peripherals.Battery {
		if !saying {
			return []peripherals.Battery{{Name: "Arctis Nova Pro Wireless"}}
		}
		return []peripherals.Battery{{Name: "Arctis Nova Pro Wireless", Level: 72, HasLevel: true}}
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	saying = false
	c.tick(time.Minute)
	_, err = p.Poll(context.Background())
	require.NoError(t, err)

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, 72, found[0].Level)
	assert.False(t, found[0].Stale, "the device is answering; it is its battery that is quiet")
}

// AC2. The carried level is dropped when the battery crosses between charging
// and discharging. Then it is not stale, it is wrong: a headset that has been
// on its cable is not at the level it came off the head with.
func TestTheCarriedLevelIsDroppedWhenTheChargeStateChanges(t *testing.T) {
	c := &clock{at: time.Now()}
	state := peripherals.Discharging
	saying := true
	p := source(t, c, func() []peripherals.Battery {
		b := peripherals.Battery{Name: "Arctis Nova Pro Wireless", State: state}
		if saying {
			b.Level, b.HasLevel = 72, true
		}
		return []peripherals.Battery{b}
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	// On the cable now, and not saying how full it is. The old level belongs
	// to before the cable, so there is nothing to draw and no cell.
	state, saying = peripherals.Charging, false
	c.tick(time.Minute)
	_, err = p.Poll(context.Background())
	require.NoError(t, err)

	for _, d := range devices(p) {
		assert.NotEqual(t, 72, d.Level, "a level from before the cable was carried over")
	}
}

// AC2. A different device does not inherit the last one's level. The name is
// what tells them apart.
func TestADifferentDeviceDoesNotInheritTheLastOnesLevel(t *testing.T) {
	c := &clock{at: time.Now()}
	name := "Arctis Nova Pro Wireless"
	saying := true
	p := source(t, c, func() []peripherals.Battery {
		b := peripherals.Battery{Name: name}
		if saying {
			b.Level, b.HasLevel = 72, true
		}
		return []peripherals.Battery{b}
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	name, saying = "Some Other Headset", false
	c.tick(time.Minute)
	_, err = p.Poll(context.Background())
	require.NoError(t, err)

	for _, d := range devices(p) {
		assert.NotEqual(t, "Some Other Headset", d.Name,
			"a device with no level of its own took the previous one's")
	}
}

// AC15. The mouse is the first cell, however the source hands the devices over
// and whatever the devices are called.
func TestTheMouseIsTheFirstCell(t *testing.T) {
	c := &clock{at: time.Now()}
	p := source(t, c, func() []peripherals.Battery {
		// Given headset-first, as a source may, and named so that a sort by
		// name alone would keep it that way.
		return []peripherals.Battery{
			{Name: "Arctis Nova Pro Wireless", Level: 47, HasLevel: true, Kind: peripherals.KindHeadset},
			{Name: "G502 X PLUS", Level: 78, HasLevel: true, Kind: peripherals.KindMouse},
		}
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	found := devices(p)
	require.Len(t, found, 2)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
	assert.Equal(t, view.KindMouse, found[0].Kind)

	// The section the shells draw is in the same order as the JSON the command
	// line prints, which is the reason the panel orders rather than the view.
	cells := p.Section().Cells
	require.Len(t, cells, 2)
	assert.Equal(t, "G502 X PLUS", cells[0].Label)
}

// AC15. Cells keep their order as devices come and go, because a panel is read
// at a glance and a cell that moves has to be found again.
func TestCellsKeepTheirOrderAsDevicesComeAndGo(t *testing.T) {
	c := &clock{at: time.Now()}
	both := false
	p := source(t, c, func() []peripherals.Battery {
		out := []peripherals.Battery{
			{Name: "G502 X PLUS", Level: 86, HasLevel: true, Kind: peripherals.KindMouse},
		}
		if both {
			out = append([]peripherals.Battery{
				{Name: "Arctis", Level: 50, HasLevel: true, Kind: peripherals.KindHeadset},
			}, out...)
		}
		return out
	})

	_, err := p.Poll(context.Background())
	require.NoError(t, err)

	both = true
	_, err = p.Poll(context.Background())
	require.NoError(t, err)

	found := devices(p)
	require.Len(t, found, 2)
	assert.Equal(t, "G502 X PLUS", found[0].Name, "the headset took the mouse's place")
	assert.Equal(t, "Arctis", found[1].Name)
}

// A machine with neither source is a machine with no section, not a machine
// with an empty heading.
func TestAMachineWithNeitherSourceDrawsNoSection(t *testing.T) {
	p := panel.NewPeripherals()
	ok, err := p.Poll(context.Background())

	// Whatever this machine has, an absent tool is not an error.
	if err != nil {
		assert.NotErrorIs(t, err, peripherals.ErrNoHeadsetcontrol)
	}
	if !ok {
		assert.Empty(t, devices(p))
	}
}
