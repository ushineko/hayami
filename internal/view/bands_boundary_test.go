package view

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
Every reading's status at and around each of its thresholds (spec 039).

Written against the hand-written ladders before they became tables, and kept
unchanged after: a table that draws one status differently from the ladder it
replaced fails here. Each case sits on a threshold, just below it and just
above it, because that is where a ">" and a ">=" differ.
*/

func TestTheCoolantsStatusAtEveryThreshold(t *testing.T) {
	cases := []struct {
		v    float64
		want Status
	}{
		{-10, Good}, {49.9, Good}, {50, Warn}, {50.1, Warn},
		{54.9, Warn}, {55, Bad}, {55.1, Bad}, {90, Bad},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, coolant(c.v), "coolant %v", c.v)
	}
}

func TestTheQuotasStatusAtEveryThreshold(t *testing.T) {
	cases := []struct {
		v    float64
		want Status
	}{
		{0, Good}, {0.4999, Good}, {0.50, Warn}, {0.5001, Warn},
		{0.7999, Warn}, {0.80, Warn}, {0.8001, Bad}, {1, Bad}, {1.5, Bad},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, quota(c.v), "quota %v", c.v)
	}
}

func TestTheBatterysStatusAtEveryThreshold(t *testing.T) {
	cases := []struct {
		level  int
		charge Charge
		want   Status
	}{
		{0, Draining, Bad}, {19, Draining, Bad}, {20, Draining, Bad},
		{21, Draining, Warn}, {49, Draining, Warn}, {50, Draining, Warn},
		{51, Draining, Good}, {100, Draining, Good},
		// Anything but draining is no verdict, whatever the level.
		{5, Draining + 1, Info}, {100, Draining + 1, Info},
	}
	for _, c := range cases {
		got := peripheral(PeripheralReading{Name: "x", Level: c.level, Charge: c.charge})
		assert.Equal(t, c.want, got.Status, "battery %d%% charge %d", c.level, c.charge)
	}
}

func TestTheBatteryBandsStatusAtEveryStep(t *testing.T) {
	cases := []struct {
		segments int
		charge   Charge
		want     Status
	}{
		{1, Draining, Bad}, {2, Draining, Warn}, {3, Draining, Good}, {4, Draining, Good},
		{1, Draining + 1, Info}, {4, Draining + 1, Info},
	}
	for _, c := range cases {
		got := peripheral(PeripheralReading{Name: "x", Segments: c.segments, Charge: c.charge})
		assert.Equal(t, c.want, got.Status, "band %d charge %d", c.segments, c.charge)
	}
}

func TestTheRatesStatusAtEveryThreshold(t *testing.T) {
	cases := []struct {
		v    float64
		has  bool
		want Status
	}{
		{0, true, Info}, {RateNotable - 1, true, Info}, {RateNotable, true, Accent},
		{RateBusy - 1, true, Accent}, {RateBusy, true, Warn},
		{RateFlatOut - 1, true, Warn}, {RateFlatOut, true, Strong}, {1 << 40, true, Strong},
		{RateFlatOut, false, Info},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, RateBand(c.v, c.has), "rate %v has %v", c.v, c.has)
	}
}

func TestTheSignalsLevelAndStatusAtEveryThreshold(t *testing.T) {
	cases := []struct {
		l      LinkReading
		level  int
		status Status
	}{
		{LinkReading{}, 0, Info},
		{LinkReading{Connected: true}, 0, Info},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -90}, 1, Warn},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -76}, 1, Warn},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -75}, 2, Info},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -68}, 2, Info},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -67}, 3, Info},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -56}, 3, Info},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -55}, 4, Info},
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -30}, 4, Info},
		// The RSSI wins over the percentage when both are there.
		{LinkReading{Connected: true, HasRSSI: true, RSSI: -90, HasSignal: true, Signal: 99}, 1, Warn},
		{LinkReading{Connected: true, HasSignal: true, Signal: 0}, 1, Warn},
		{LinkReading{Connected: true, HasSignal: true, Signal: 24}, 1, Warn},
		{LinkReading{Connected: true, HasSignal: true, Signal: 25}, 2, Info},
		{LinkReading{Connected: true, HasSignal: true, Signal: 49}, 2, Info},
		{LinkReading{Connected: true, HasSignal: true, Signal: 50}, 3, Info},
		{LinkReading{Connected: true, HasSignal: true, Signal: 74}, 3, Info},
		{LinkReading{Connected: true, HasSignal: true, Signal: 75}, 4, Info},
		{LinkReading{Connected: true, HasSignal: true, Signal: 100}, 4, Info},
	}
	for _, c := range cases {
		name := fmt.Sprintf("%+v", c.l)
		assert.Equal(t, c.level, SignalLevel(c.l), "level %s", name)
		assert.Equal(t, c.status, signalStatus(c.l), "status %s", name)
	}
}
