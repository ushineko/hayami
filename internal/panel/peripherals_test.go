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

// source builds a Peripherals whose two sources are the test's.
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

// devices reads the section's rows back as the view sees them.
func devices(p *panel.Peripherals) []view.PeripheralReading {
	return p.Data().(view.PeripheralsReading).Devices
}

// AC5. A device that answers and then stops keeps its level and is marked
// stale, because the reader's question is whether what it said last is still
// true.
func TestADeviceThatStopsAnsweringKeepsItsLevelAndIsStale(t *testing.T) {
	c := &clock{at: time.Now()}
	present := true
	p := source(t, c, func() []peripherals.Battery {
		if !present {
			return nil
		}
		return []peripherals.Battery{{Name: "G502 X PLUS", Level: 86, HasLevel: true}}
	})

	ok, err := p.Poll(context.Background())
	require.NoError(t, err)
	assert.True(t, ok)

	present = false
	c.tick(time.Minute)
	ok, err = p.Poll(context.Background())
	require.NoError(t, err)
	assert.True(t, ok, "a device that has gone quiet is still drawn")

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, 86, found[0].Level, "the last level was dropped rather than kept")
	assert.True(t, found[0].Stale)
}

// AC5. A device that has never answered is not drawn at all. There is nothing
// to say about it and a row of dashes is furniture.
func TestADeviceThatNeverAnsweredIsNotDrawn(t *testing.T) {
	c := &clock{at: time.Now()}
	p := source(t, c, func() []peripherals.Battery { return nil })

	ok, err := p.Poll(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, devices(p))
}

// AC5. A device quiet for long enough is gone rather than quiet, and stops
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

// AC6. A device that is connected and not saying how full it is keeps the
// level it last gave.
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
	assert.True(t, found[0].HasLevel)
	assert.Equal(t, 72, found[0].Level)
	assert.False(t, found[0].Stale, "the device is answering; it is its battery that is quiet")
}

// AC6. The remembered level is dropped when the battery crosses between
// charging and discharging. Then it is not stale, it is wrong: a headset that
// has been on its cable is not at the level it came off the head with.
func TestTheRememberedLevelIsDroppedWhenTheChargeStateChanges(t *testing.T) {
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

	// On the cable now, and not saying how full it is.
	state, saying = peripherals.Charging, false
	c.tick(time.Minute)
	_, err = p.Poll(context.Background())
	require.NoError(t, err)

	found := devices(p)
	require.Len(t, found, 1)
	assert.False(t, found[0].HasLevel, "a level from before the cable was carried over")
	assert.Equal(t, view.Filling, found[0].Charge)
}

// AC6. A different device does not inherit the last one's level. The name is
// what tells them apart, and without that guard a headset swapped for another
// would show the first one's battery.
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
		if d.Name == "Some Other Headset" {
			assert.False(t, d.HasLevel, "the previous device's level bled onto a different one")
		}
	}
}

// AC5. Rows keep their order as devices come and go, because a panel is read
// at a glance and a row that moves has to be found again.
func TestRowsKeepTheirOrderAsDevicesComeAndGo(t *testing.T) {
	c := &clock{at: time.Now()}
	both := false
	p := source(t, c, func() []peripherals.Battery {
		out := []peripherals.Battery{{Name: "G502 X PLUS", Level: 86, HasLevel: true}}
		if both {
			// Deliberately given ahead of the mouse, as a source may.
			out = append([]peripherals.Battery{{Name: "Arctis", Level: 50, HasLevel: true}}, out...)
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
	assert.Equal(t, "Arctis", found[0].Name)
	assert.Equal(t, "G502 X PLUS", found[1].Name)
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
