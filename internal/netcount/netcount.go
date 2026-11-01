/*
Package netcount is the network helper's counter on Linux (spec 072): eBPF
programs that count every packet reaching or leaving a socket on this
machine, by the process using the socket and the two ends, and every packet
the machine forwards, by the two interfaces.

The kernel keeps no byte counters for a UDP socket, and the TCP ones it keeps
are lost when a connection closes between two readings; counting packets as
they pass is the only way to the whole of an interface's traffic. Loading the
programs needs CAP_BPF, CAP_NET_ADMIN and CAP_PERFMON, which is why this runs
in the helper and never in a panel.

What is here has no build tag: the values, and the difference between two
readings, which a test on any system runs. The loader is netcount_linux.go.
*/
package netcount

import (
	"net/netip"
	"sort"

	"github.com/ushineko/hayami/internal/core"
)

// Flow is one process's traffic with one far end, from one near end. PID is
// zero for a socket nobody has been seen using yet.
type Flow struct {
	PID                 uint32
	Proto               uint8
	Local, Peer         netip.Addr
	LocalPort, PeerPort uint16
}

// Bytes is received and sent, IP headers included.
type Bytes struct{ Rx, Tx uint64 }

// Hop is a pair of interfaces a forwarded packet came in and went out by.
type Hop struct{ In, Out string }

// Snapshot is the counters since the programs were loaded.
type Snapshot struct {
	Flows     map[Flow]Bytes
	Forwarded map[Hop]uint64
	// Tunnels is the processes that write packets to a tunnel device.
	Tunnels []uint32
	// Full reports a table near its size, where the oldest entries are
	// being let go and their bytes since the last reading lost.
	Full bool
}

/*
Line is what moved between two snapshots, as one line of the helper's
protocol.

A counter lower than it was is an entry the table let go and made again,
and all of it is new. An entry gone from now has nothing to add.
*/
func Line(was, now Snapshot) core.NetHelperLine {
	var line core.NetHelperLine
	for f, b := range now.Flows {
		before := was.Flows[f]
		if b.Rx < before.Rx || b.Tx < before.Tx {
			before = Bytes{}
		}
		rx, tx := b.Rx-before.Rx, b.Tx-before.Tx
		if rx == 0 && tx == 0 {
			continue
		}
		line.F = append(line.F, core.NetHelperFlow{
			PID: f.PID, Proto: f.Proto,
			Local: f.Local.String(), LocalPort: f.LocalPort,
			Peer: f.Peer.String(), PeerPort: f.PeerPort,
			Rx: rx, Tx: tx,
		})
	}
	for h, n := range now.Forwarded {
		before := was.Forwarded[h]
		if n < before {
			before = 0
		}
		if n == before {
			continue
		}
		line.H = append(line.H, core.NetHelperHop{In: h.In, Out: h.Out, Bytes: n - before})
	}
	line.T = append(line.T, now.Tunnels...)
	sort.Slice(line.T, func(i, j int) bool { return line.T[i] < line.T[j] })
	line.Full = now.Full
	return line
}
