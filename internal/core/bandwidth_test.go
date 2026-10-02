package core_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/ushineko/hayami/internal/core"
)

// A rate is a difference, so the first sample has none. It is a real state and
// the view has a blank of the same width for it; reporting zero instead would
// draw a confident nought where there is no answer yet.
func TestTheFirstSampleHasTotalsAndNoRate(t *testing.T) {
	b := core.NewBandwidth()
	now := time.Now()

	got := b.Sample(now, []string{"eth0"}, map[string]core.Counters{"eth0": {Rx: 1000, Tx: 500}})

	assert.Equal(t, uint64(1000), got[0].RxTotal)
	assert.Zero(t, got[0].RxRate)
}

func TestTheSecondSampleIsTheDifferenceOverTheElapsedTime(t *testing.T) {
	b := core.NewBandwidth()
	start := time.Now()
	b.Sample(start, []string{"eth0"}, map[string]core.Counters{"eth0": {Rx: 1000, Tx: 500}})

	got := b.Sample(start.Add(2*time.Second), []string{"eth0"},
		map[string]core.Counters{"eth0": {Rx: 3048, Tx: 1524}})

	assert.InDelta(t, 1024, got[0].RxRate, 0.001, "2048 bytes over two seconds")
	assert.InDelta(t, 512, got[0].TxRate, 0.001)
}

// An interface that came back up, or a machine that slept, has a counter lower
// than the one before it. The difference would be enormous and negative; the
// reading is simply not a rate.
func TestACounterGoingBackwardsIsNotAVastNegativeRate(t *testing.T) {
	b := core.NewBandwidth()
	start := time.Now()
	b.Sample(start, []string{"eth0"}, map[string]core.Counters{"eth0": {Rx: 5000}})

	got := b.Sample(start.Add(time.Second), []string{"eth0"},
		map[string]core.Counters{"eth0": {Rx: 10}})

	assert.Zero(t, got[0].RxRate)
	assert.Equal(t, uint64(10), got[0].RxTotal, "the new total is still the truth")
}

// The order is the user's, not a map's. A panel whose rows changed places
// between two paints would be unreadable.
func TestTheReadingsAreInTheOrderTheNamesWereGiven(t *testing.T) {
	b := core.NewBandwidth()
	counters := map[string]core.Counters{"eth0": {}, "wg0": {}, "lo": {}}

	got := b.Sample(time.Now(), []string{"wg0", "lo", "eth0"}, counters)

	assert.Equal(t, []string{"wg0", "lo", "eth0"},
		[]string{got[0].Name, got[1].Name, got[2].Name})
}

// An interface the user named and the kernel does not have keeps its place, so
// the section can say it is gone rather than quietly losing a row.
func TestAnInterfaceTheKernelDoesNotHaveKeepsItsRow(t *testing.T) {
	b := core.NewBandwidth()

	got := b.Sample(time.Now(), []string{"gone0"}, map[string]core.Counters{})

	assert.Len(t, got, 1)
	assert.Equal(t, "gone0", got[0].Name)
	assert.Zero(t, got[0].RxTotal)
}

// "Not yet" and "nothing is happening" are different claims. The first sample
// has no rate at all; a second sample with identical counters has a rate, and
// it is zero.
func TestARateThatHasNotArrivedIsNotARateOfZero(t *testing.T) {
	b := core.NewBandwidth()
	start := time.Now()
	counters := map[string]core.Counters{"eth0": {Rx: 1000}}

	first := b.Sample(start, []string{"eth0"}, counters)
	second := b.Sample(start.Add(time.Second), []string{"eth0"}, counters)

	assert.False(t, first[0].HasRate, "the first sample has nothing to difference against")
	assert.True(t, second[0].HasRate)
	assert.Zero(t, second[0].RxRate, "a quiet interface has a rate, and it is zero")
}

func TestAnInterfaceTheKernelDoesNotHaveIsNotPresent(t *testing.T) {
	b := core.NewBandwidth()

	got := b.Sample(time.Now(), []string{"gone0"}, map[string]core.Counters{})

	assert.False(t, got[0].Present)
}

// A chooser groups interfaces by kind, and the names it groups are the
// platform's own. Windows' are the defaults it gives every machine, not any
// one machine's: its loopback, its numbered virtual adapters and its IPv6
// transition tunnels are churn, and its tailscale interface is capitalised.
func TestInterfacesAreClassifiedOnEitherPlatform(t *testing.T) {
	cases := map[string]core.InterfaceKind{
		"eth0":                              core.KindOrdinary,
		"wlp3s0":                            core.KindOrdinary,
		"lo":                                core.KindVirtual,
		"veth1a2b3c":                        core.KindVirtual,
		"docker0":                           core.KindVirtual,
		"tailscale0":                        core.KindTunnel,
		"wg0":                               core.KindTunnel,
		"Ethernet":                          core.KindOrdinary,
		"Ethernet 2":                        core.KindOrdinary,
		"Wi-Fi":                             core.KindOrdinary,
		"Tailscale":                         core.KindTunnel,
		"Loopback Pseudo-Interface 1":       core.KindVirtual,
		"Local Area Connection* 3":          core.KindVirtual,
		"vEthernet (Default Switch)":        core.KindVirtual,
		"6to4 Adapter":                      core.KindVirtual,
		"Teredo Tunneling Pseudo-Interface": core.KindVirtual,
		"Ethernet (Kernel Debugger)":        core.KindVirtual,
	}
	for name, want := range cases {
		assert.Equal(t, want, core.ClassifyInterface(name), "%q", name)
	}
}
