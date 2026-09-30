package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// counting is a counter table that grows on every read, for the interfaces
// present in it.
type counting struct {
	present map[string]bool
	n       uint64
}

func (c *counting) read() (map[string]core.Counters, error) {
	c.n++
	out := map[string]core.Counters{}
	for name, ok := range c.present {
		if ok {
			out[name] = core.Counters{Rx: c.n * 1000, Tx: c.n * 10}
		}
	}
	return out, nil
}

// section builds a section over a counter table with a clock that moves a
// poll interval every time it is read.
func section(names []string, src *counting) *core.BandwidthSection {
	b := core.NewBandwidthSection(names, src.read)
	at := time.Unix(0, 0)
	b.SetClock(func() time.Time {
		at = at.Add(core.BandwidthInterval)
		return at
	})
	return b
}

func poll(t *testing.T, b *core.BandwidthSection, times int) {
	t.Helper()
	for range times {
		_, err := b.Poll(context.Background())
		require.NoError(t, err)
	}
}

// The first poll has no rate and adds nothing; every poll after it adds one
// sample per direction, up to BandwidthTrail, oldest dropped first.
func TestTheTrailHoldsOneSamplePerRatedPollUpToItsLength(t *testing.T) {
	for _, rated := range []int{1, 5, core.BandwidthTrail, core.BandwidthTrail + 7} {
		src := &counting{present: map[string]bool{"eno2": true}}
		b := section([]string{"eno2"}, src)

		poll(t, b, 1+rated)

		got := b.Trail("eno2")
		want := min(rated, core.BandwidthTrail)
		assert.Len(t, got.Rx, want, "after %d rated polls", rated)
		assert.Len(t, got.Tx, want, "after %d rated polls", rated)
		// A thousand bytes down and ten up per two-second poll, in bytes
		// per second as Rates reports them.
		for i := range got.Rx {
			assert.InDelta(t, 500.0, got.Rx[i], 1e-9)
			assert.InDelta(t, 5.0, got.Tx[i], 1e-9)
		}
	}
}

// A poll that read nothing for an interface records nothing for it: a gap
// compresses rather than being drawn as a zero.
func TestAnUnreadInterfaceAddsNothingToItsTrail(t *testing.T) {
	src := &counting{present: map[string]bool{"eno2": true, "wlan0": true}}
	b := section([]string{"eno2", "wlan0"}, src)
	poll(t, b, 3)
	require.Len(t, b.Trail("wlan0").Rx, 2)

	src.present["wlan0"] = false
	poll(t, b, 4)

	assert.Len(t, b.Trail("wlan0").Rx, 2, "the absent interface kept what it had and gained nothing")
	assert.Len(t, b.Trail("eno2").Rx, 6)
}

// An interface that leaves the watched set loses its samples, and one that is
// never watched has none.
func TestARemovedInterfaceIsForgotten(t *testing.T) {
	src := &counting{present: map[string]bool{"eno2": true, "wlan0": true}}
	b := section([]string{"eno2", "wlan0"}, src)
	poll(t, b, 3)
	require.NotEmpty(t, b.Trail("wlan0").Rx)

	b.SetInterfaces([]string{"eno2"})

	assert.Empty(t, b.Trail("wlan0").Rx)
	assert.Empty(t, b.Trail("wlan0").Tx)
	assert.Len(t, b.Trail("eno2").Rx, 2, "the interface still watched keeps its trail")
	assert.Empty(t, b.Trail("lo").Rx)
}

// A trail handed out is a copy: a shell that edits it does not edit the plot.
func TestATrailIsACopy(t *testing.T) {
	src := &counting{present: map[string]bool{"eno2": true}}
	b := section([]string{"eno2"}, src)
	poll(t, b, 3)

	got := b.Trail("eno2")
	got.Rx[0] = -1

	assert.NotEqual(t, -1.0, b.Trail("eno2").Rx[0])
}
