package panel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cooler"
	"github.com/ushineko/hayami/internal/panel"
)

// sources builds a cooler whose two readings the test decides, one poll at a
// time. Each call to Poll takes the next pair.
type sources struct {
	cpu    []func() (float64, error)
	liquid []func() (cooler.Liquid, error)
	at     int
}

func (s *sources) install(c *panel.Cooler) {
	panel.SetCoolerSources(c,
		func() (float64, error) { return s.cpu[s.at]() },
		func(context.Context) (cooler.Liquid, error) { return s.liquid[s.at]() },
	)
}

func degrees(v float64) func() (float64, error) {
	return func() (float64, error) { return v, nil }
}

func noSensor() (float64, error) { return 0, cooler.ErrNoSensor }

func cooling(v float64, pump int) func() (cooler.Liquid, error) {
	return func() (cooler.Liquid, error) {
		return cooler.Liquid{Coolant: v, PumpRPM: pump, HasPump: true}, nil
	}
}

func contended() (cooler.Liquid, error) {
	return cooler.Liquid{}, errors.New("hidraw is busy")
}

func noCooler() (cooler.Liquid, error) { return cooler.Liquid{}, cooler.ErrNoCooler }

// The bug this is about: liquidctl opens a hidraw node and this machine has a
// history of contention on those, so a poll comes back empty several times an
// hour. Replacing the reading with what just arrived took the coolant row
// away, shortened the card and changed the height of the panel, for a second,
// at random.
func TestACoolerThatMissesAPollKeepsWhatItKnew(t *testing.T) {
	c := panel.NewCooler()
	s := &sources{
		cpu:    []func() (float64, error){degrees(60), degrees(61)},
		liquid: []func() (cooler.Liquid, error){cooling(38.9, 2650), contended},
	}
	s.install(c)

	drawn, err := c.Poll(t.Context())
	require.NoError(t, err)
	require.True(t, drawn)
	require.False(t, c.Section().Gone)

	s.at = 1
	drawn, err = c.Poll(t.Context())

	require.Error(t, err, "the failure still reaches the caller, which logs it")
	assert.True(t, drawn, "a section that was drawn stays drawn")

	sec := c.Section()
	assert.True(t, sec.Gone, "the heading says the numbers are not live")
	require.Len(t, sec.Rows, 3)
	assert.Contains(t, sec.Rows[1].Value, "38.9", "the coolant kept its last value")
	assert.Contains(t, sec.Rows[2].Value, "2650", "and so did the pump")
}

// Recovery clears it. A marker that never goes away is a marker nobody reads.
func TestACoolerThatComesBackIsNotGoneAnyMore(t *testing.T) {
	c := panel.NewCooler()
	s := &sources{
		cpu:    []func() (float64, error){degrees(60), degrees(61), degrees(62)},
		liquid: []func() (cooler.Liquid, error){cooling(38.9, 2650), contended, cooling(39.4, 2700)},
	}
	s.install(c)

	for s.at = range 3 {
		_, _ = c.Poll(t.Context())
	}

	sec := c.Section()
	assert.False(t, sec.Gone)
	assert.Contains(t, sec.Rows[1].Value, "39.4")
}

// Gone is for a source that answered and has stopped. A machine with no
// liquid cooler has never had one, and a panel that marked it stale would be
// claiming to have lost something it never had.
func TestAMachineWithNoCoolerIsNotGoneItIsAMachineWithNoCooler(t *testing.T) {
	c := panel.NewCooler()
	s := &sources{
		cpu:    []func() (float64, error){degrees(60), degrees(61)},
		liquid: []func() (cooler.Liquid, error){noCooler, noCooler},
	}
	s.install(c)

	_, err := c.Poll(t.Context())
	require.NoError(t, err, "no cooler is not a failure")
	s.at = 1
	_, _ = c.Poll(t.Context())

	sec := c.Section()
	assert.False(t, sec.Gone)
	require.Len(t, sec.Rows, 1)
	assert.Equal(t, "CPU", sec.Rows[0].Label)
}

// A machine with neither has no section at all, rather than an empty one.
func TestAMachineWithNeitherDrawsNothing(t *testing.T) {
	c := panel.NewCooler()
	s := &sources{
		cpu:    []func() (float64, error){noSensor},
		liquid: []func() (cooler.Liquid, error){noCooler},
	}
	s.install(c)

	drawn, err := c.Poll(t.Context())

	require.NoError(t, err)
	assert.False(t, drawn)
	assert.False(t, c.Section().Gone)
}

// The plot is a record of what was measured. A trail fed the value it already
// held would draw a flat line through an outage and call it a steady
// temperature, which is the one thing a trend line must not do.
func TestAMissedPollPutsNoSampleOnThePlot(t *testing.T) {
	c := panel.NewCooler()
	s := &sources{
		cpu:    []func() (float64, error){degrees(60), degrees(61)},
		liquid: []func() (cooler.Liquid, error){cooling(38.9, 2650), contended},
	}
	s.install(c)

	_, _ = c.Poll(t.Context())
	before := c.Section().Trails
	require.Len(t, before, 2, "the coolant and the processor")

	s.at = 1
	_, _ = c.Poll(t.Context())
	after := c.Section().Trails

	require.Len(t, after, 2)
	assert.Len(t, after[0].Samples, len(before[0].Samples), "the coolant gained a made-up sample")
	assert.Len(t, after[1].Samples, len(before[1].Samples))
}

// Two series, named and in order: the coolant is the primary trace and is
// given to the plot first, because a plot draws them in the order it gets them
// and the coolant is what the eye should land on.
func TestTheCoolerPlotsTheCoolantAndTheProcessor(t *testing.T) {
	c := panel.NewCooler()
	s := &sources{
		cpu:    []func() (float64, error){degrees(60)},
		liquid: []func() (cooler.Liquid, error){cooling(38.9, 2650)},
	}
	s.install(c)

	_, _ = c.Poll(t.Context())

	trails := c.Section().Trails
	require.Len(t, trails, 2)
	assert.Equal(t, "Coolant", trails[0].Name)
	assert.Equal(t, "CPU", trails[1].Name)
	assert.InDelta(t, 60.0, trails[1].Samples[0], 0.001,
		"a partial window is averaged as it stands, so the trace starts on the first sample")
}
