package core

import (
	"context"
	"errors"
	"testing"
)

// The hinge of spec 037: a Wi-Fi interface is keyed by the name ReadCounters
// keys it by, or the bandwidth row it belongs to never finds it. Read from
// this machine's own tables; a machine with no Wi-Fi has nothing to compare.
func TestWirelessIsKeyedByTheCountersName(t *testing.T) {
	wireless, err := ReadWireless(context.Background())
	if err != nil && !errors.Is(err, ErrWirelessDenied) {
		t.Fatalf("ReadWireless: %v", err)
	}
	if len(wireless) == 0 {
		t.Skip("no Wi-Fi interface on this machine")
	}
	counters, err := ReadCounters()
	if err != nil {
		t.Fatalf("ReadCounters: %v", err)
	}
	for name, w := range wireless {
		if _, ok := counters[name]; !ok {
			t.Errorf("Wi-Fi interface %q is not among the counters' names", name)
		}
		t.Logf("%s: connected=%v signal=%d%%(%v) rssi=%d(%v) %s ch %d(%v) %q rx=%.1f tx=%.1f",
			name, w.Connected, w.Signal, w.HasSignal, w.RSSI, w.HasRSSI, w.Band(), w.Channel,
			w.HasChannel, w.Generation, w.RxRate, w.TxRate)
	}
}
