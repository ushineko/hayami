package core

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

/*
Spec 074. What hayami parses from sources it does not control is fuzzed:
no input panics, and what the helper's stream keeps stays inside its bounds.
Each target is seeded with what the example tests read; `make fuzz` runs each
for a while, and CI does on every PR.
*/

// FuzzParseNetDev: /proc/net/dev, or anything in its place.
func FuzzParseNetDev(f *testing.F) {
	f.Add("Inter-|   Receive\n face |bytes    packets\n  eth0:  204800     200    0    0    0     0          0         0   102400     100    0    0    0     0       0          0\n")
	f.Add("Inter-|\n face |bytes\n  eth0:12345678901 200    0    0    0     0          0         0   102400     100    0    0    0     0       0          0\n")
	f.Add("")
	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = ParseNetDev(strings.NewReader(s))
	})
}

// FuzzHelperStream: the lines a network helper sends, read as either
// protocol. Whatever arrives, what is kept stays inside its bounds.
func FuzzHelperStream(f *testing.F) {
	for _, l := range [][]NetHelperLine{
		{{V: NetHelperProtocol, Version: "1", Counts: "TCP and UDP"}, {P: [][3]uint64{{7, 100, 10}}}},
		{{V: NetHelperFlowsProtocol, Version: "1", Counts: "every protocol"}, {F: []NetHelperFlow{{PID: 7, Proto: 17,
			Local: "192.0.2.1", LocalPort: 5000, Peer: "198.51.100.9", PeerPort: 443, Rx: 1, Tx: 2}},
			H: []NetHelperHop{{In: "vnet0", Out: "eth0", Bytes: 3}}, T: []uint32{3}, Full: true}},
		{{V: NetHelperProtocol}, {Error: "boom\x1b]52;c;aGk=\x07"}},
	} {
		var b bytes.Buffer
		for _, line := range l {
			j, _ := json.Marshal(line)
			b.Write(j)
			b.WriteByte('\n')
		}
		f.Add(b.Bytes(), false)
		f.Add(b.Bytes(), true)
	}
	f.Fuzz(func(t *testing.T, data []byte, v2 bool) {
		s := &helperStream{}
		if v2 {
			s.want = NetHelperFlowsProtocol
		}
		s.run(bytes.NewReader(data))
		if len(s.counts) > maxHelperEntries || len(s.flows.flows) > maxHelperEntries ||
			len(s.flows.hops) > maxHelperHops || len(s.flows.tunnels) > maxHelperTunnels {
			t.Fatalf("kept past its bounds: %d counts, %d flows, %d hops, %d tunnels",
				len(s.counts), len(s.flows.flows), len(s.flows.hops), len(s.flows.tunnels))
		}
		if s.err != nil && strings.ContainsAny(s.err.Error(), "\x1b\x07") {
			t.Fatalf("an escape reached the error: %q", s.err)
		}
	})
}

// FuzzLHMTree: LibreHardwareMonitor's data.json, decoded and searched for
// the processor's temperature as the reader does.
func FuzzLHMTree(f *testing.F) {
	if b, err := os.ReadFile("testdata/lhm.json"); err == nil {
		f.Add(b)
	}
	f.Add([]byte(`{"Text":"Sensor","Children":[{"Text":"Core (Tctl/Tdie)","Value":"45,5 °C","Children":[]}]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		var root LHMNode
		if json.Unmarshal(data, &root) != nil {
			return
		}
		_, _, _ = LHMCPUTemperature(root)
	})
}

// FuzzParseLHMValue: one sensor's value, as LibreHardwareMonitor writes it
// in whatever locale.
func FuzzParseLHMValue(f *testing.F) {
	for _, s := range []string{"45,5 °C", "45.5 °C", "1 234,5 RPM", "", "-", "NaN °C"} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = ParseLHMValue(s)
	})
}
