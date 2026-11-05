package core_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// tunnelSection is a section watching no interface, over a table the test sets, on
// a clock that moves a second a read.
func tunnelSection(table *map[string]core.Counters) *core.BandwidthSection {
	s := core.NewBandwidthSection(nil, func() (map[string]core.Counters, error) { return *table, nil })
	at := time.Unix(1000, 0)
	s.SetClock(func() time.Time {
		at = at.Add(time.Second)
		return at
	})
	return s
}

// A tunnel's interface is read with no interface chosen, as a rate, and the totals
// are the kernel's (spec 086 R5, R7).
func TestATunnelIsReadWithNoInterfaceChosen(t *testing.T) {
	table := map[string]core.Counters{"tun0": {Rx: 1000, Tx: 500}}
	s := tunnelSection(&table)

	has, err := s.Poll(t.Context())
	require.NoError(t, err)
	assert.False(t, has, "nothing is watched before a tunnel is named")

	s.SetTunnels(map[string]string{"vpn": "tun0"})
	has, err = s.Poll(t.Context())
	require.NoError(t, err)
	assert.True(t, has)
	r, trail, ok := s.Tunnel("vpn")
	require.True(t, ok)
	assert.Equal(t, uint64(1000), r.RxTotal)
	assert.Empty(t, trail.Rx)

	table = map[string]core.Counters{"tun0": {Rx: 3000, Tx: 600}}
	_, err = s.Poll(t.Context())
	require.NoError(t, err)
	r, trail, ok = s.Tunnel("vpn")
	require.True(t, ok)
	assert.True(t, r.HasRate)
	assert.InDelta(t, 2000, r.RxRate, 0.001)
	assert.InDelta(t, 100, r.TxRate, 0.001)
	assert.Equal(t, []float64{2000}, trail.Rx)
	assert.Empty(t, s.Readings(), "a tunnel is not one of the watched interfaces")
}

// A tunnel that goes down loses its figures and its trail, since they are about the
// tunnel that was up.
func TestATunnelGoingDownForgetsItsFigures(t *testing.T) {
	table := map[string]core.Counters{"tun0": {Rx: 1000}}
	s := tunnelSection(&table)
	s.SetTunnels(map[string]string{"vpn": "tun0"})
	for range 2 {
		_, err := s.Poll(t.Context())
		require.NoError(t, err)
	}

	s.SetTunnels(map[string]string{})
	_, _, ok := s.Tunnel("vpn")
	assert.False(t, ok)

	s.SetTunnels(map[string]string{"vpn": "tun0"})
	_, err := s.Poll(t.Context())
	require.NoError(t, err)
	_, trail, ok := s.Tunnel("vpn")
	require.True(t, ok)
	assert.Len(t, trail.Rx, 1, "the trail starts again")
}

// An interface that is not there yet is no reading, and a tunnel re-created between
// polls counts from nought, which is no rate rather than a vast negative one.
func TestATunnelsCountersAreTakenAsAnInterfacesAre(t *testing.T) {
	table := map[string]core.Counters{}
	s := tunnelSection(&table)
	s.SetTunnels(map[string]string{"vpn": "tun0"})
	_, err := s.Poll(t.Context())
	require.NoError(t, err)
	_, _, ok := s.Tunnel("vpn")
	assert.False(t, ok, "the interface is not there")

	table = map[string]core.Counters{"tun0": {Rx: 5000, Tx: 5000}}
	_, err = s.Poll(t.Context())
	require.NoError(t, err)
	table = map[string]core.Counters{"tun0": {Rx: 10, Tx: 10}}
	_, err = s.Poll(t.Context())
	require.NoError(t, err)
	r, _, ok := s.Tunnel("vpn")
	require.True(t, ok)
	assert.Zero(t, r.RxRate)
	assert.Zero(t, r.TxRate)
	assert.Equal(t, uint64(10), r.RxTotal)
}

// A tunnel whose interface is also chosen reads the same figures in both places, and
// the interface's own row is undisturbed.
func TestATunnelAlsoChosenAsAnInterfaceReadsTheSame(t *testing.T) {
	table := map[string]core.Counters{"tun0": {Rx: 100, Tx: 50}, "eno2": {Rx: 7}}
	s := core.NewBandwidthSection([]string{"eno2", "tun0"}, func() (map[string]core.Counters, error) { return table, nil })
	s.SetTunnels(map[string]string{"vpn": "tun0"})
	_, err := s.Poll(t.Context())
	require.NoError(t, err)

	readings := s.Readings()
	require.Len(t, readings, 2)
	assert.Equal(t, "eno2", readings[0].Name)
	assert.Equal(t, "tun0", readings[1].Name)
	r, _, ok := s.Tunnel("vpn")
	require.True(t, ok)
	assert.Equal(t, readings[1].RxTotal, r.RxTotal)
}
