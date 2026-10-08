package core

import (
	"strings"
	"time"
)

// Rates is one interface's reading: what it is doing now and what it has done
// since the counters were last zero.
//
// A rate is bytes per second as a float, not a formatted string. Formatting is
// the view's, and a core that returned "1.5 MiB/s" would be a core that had
// decided how wide a column is.
type Rates struct {
	Name    string  `json:"name"`
	RxRate  float64 `json:"rx_rate"`
	TxRate  float64 `json:"tx_rate"`
	RxTotal uint64  `json:"rx_total"`
	TxTotal uint64  `json:"tx_total"`

	// Present is false for an interface the kernel does not have. The reading
	// keeps its place so the section can say the interface is gone rather
	// than losing the row.
	Present bool `json:"present"`

	// HasRate is false until there are two samples to difference. It is not
	// the same as a rate of zero: one says "not yet" and the other says
	// "nothing is happening", and a panel that conflated them would draw a
	// confident nought where it has no answer.
	HasRate bool `json:"has_rate"`

	// Wireless is the link of a Wi-Fi interface (spec 037), and nil for
	// every other. The sampler never sets it; the section does.
	Wireless *Wireless `json:"wireless,omitempty"`
}

// Bandwidth turns counters into rates. It holds the previous sample, because a
// rate is a difference and one reading of a counter is not a rate.
//
// The first Sample after construction returns totals with no rates: there is
// nothing to difference against yet. That is a real state and not an error,
// and it is why the view has a blank form of every value.
type Bandwidth struct {
	prev   map[string]Counters
	prevAt time.Time
}

// NewBandwidth builds a sampler with no history.
func NewBandwidth() *Bandwidth { return &Bandwidth{} }

// Sample differences the counters against the previous call and returns one
// reading per name, in the order the names are given, so the panel's order is
// the user's and not a map's.
//
// A counter that went backwards is treated as a restart — an interface that
// came back up, or a machine that slept — and contributes no rate rather than
// a vast negative one. A name that is not in the table contributes a reading
// with no data, so a section keeps its row and says the interface is gone.
func (b *Bandwidth) Sample(now time.Time, names []string, counters map[string]Counters) []Rates {
	out := make([]Rates, 0, len(names))
	elapsed := now.Sub(b.prevAt).Seconds()
	first := b.prev == nil || elapsed <= 0

	for _, name := range names {
		c, ok := counters[name]
		if !ok {
			out = append(out, Rates{Name: name})
			continue
		}
		r := Rates{Name: name, RxTotal: c.Rx, TxTotal: c.Tx, Present: true}
		if p, had := b.prev[name]; had && !first {
			r.HasRate = true
			if c.Rx >= p.Rx {
				r.RxRate = float64(c.Rx-p.Rx) / elapsed
			}
			if c.Tx >= p.Tx {
				r.TxRate = float64(c.Tx-p.Tx) / elapsed
			}
		}
		out = append(out, r)
	}

	b.prev = counters
	b.prevAt = now
	return out
}

// InterfaceNames lists every interface the kernel reports, for a program
// offering the user a choice. The loopback is included: hiding it here would
// be this package deciding what is interesting.
func InterfaceNames(counters map[string]Counters) []string {
	names := make([]string, 0, len(counters))
	for name := range counters {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

/*
InterfaceKind is what sort of interface a name belongs to, for a program
deciding which ones to put in front of somebody.

Not for deciding what to *measure*: every interface the kernel reports can be
watched, and this package has no business narrowing that. It is for the order
and the prominence a chooser gives them, which is a different question and one
a chooser cannot answer from a name alone without this.
*/
type InterfaceKind int

const (
	// KindOrdinary is a real interface: ethernet, wireless, anything the
	// machine talks to the world through. The default, because a name this
	// package does not recognise is more likely to be somebody's unusual
	// hardware than a container.
	KindOrdinary InterfaceKind = iota
	// KindTunnel is a VPN or overlay: tailscale, wireguard, tun. Real
	// traffic, and usually worth watching.
	KindTunnel
	// KindVirtual is the churn a container runtime leaves behind: veth
	// pairs, docker and libvirt bridges, and the loopback. Individually
	// meaningless and numerous -- 73 of 77 on the machine this was written
	// for, which is what made the chooser unusable.
	KindVirtual
)

// virtualPrefixes are the names a container or VM runtime creates, and the
// ones Windows gives its own pseudo-interfaces: the loopback, the numbered
// "Local Area Connection*" adapters (Wi-Fi Direct and the WAN miniports), the
// IPv6 transition tunnels nothing on a modern network carries, and the
// kernel-debugger NIC. Hyper-V's "vEthernet (...)" switches match "veth".
var virtualPrefixes = []string{
	"veth", "br-", "docker", "virbr", "vnet", "cni", "flannel", "kube",
	"loopback pseudo-interface", "local area connection*", "6to4 adapter",
	"teredo tunneling", "microsoft ip-https", "isatap", "ethernet (kernel debugger)",
}

// tunnelPrefixes are the overlays worth putting near the top.
var tunnelPrefixes = []string{"tailscale", "wg", "tun", "ppp", "zt"}

// ClassifyInterface says what sort of interface a name is.
//
// By name, because that is all there is here: /proc/net/dev has no notion of
// a container. The prefixes are the conventions the runtimes follow and they
// are a heuristic -- a machine that names its ethernet "tunnel0" is misread,
// and the cost of that is one row in the wrong group of a list that has a
// "show everything" beside it. Case is ignored, because Windows capitalises
// what Linux does not: its tailscale interface is "Tailscale".
func ClassifyInterface(name string) InterfaceKind {
	if name == "lo" {
		return KindVirtual
	}
	name = strings.ToLower(name)
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return KindVirtual
		}
	}
	for _, p := range tunnelPrefixes {
		if strings.HasPrefix(name, p) {
			return KindTunnel
		}
	}
	return KindOrdinary
}
