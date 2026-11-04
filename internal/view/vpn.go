package view

import (
	"fmt"
	"time"
)

// VPNState is what a VPN is doing, mirroring core.VPNState as BandwidthReading
// mirrors core.Rates.
type VPNState int

// The states a row can draw.
const (
	VPNDown VPNState = iota
	VPNConnecting
	VPNUp
	VPNUnknown
)

// vpnStates is each state's word and colour. Down is the one verdict: the row
// is there because somebody chose this tunnel. Unknown is Info, because it says
// nothing about the tunnel (spec 085 R4).
var vpnStates = map[VPNState]struct {
	word   string
	status Status
}{
	VPNDown:       {"down", Warn},
	VPNConnecting: {"dialing", Info},
	VPNUp:         {"up", Good},
	VPNUnknown:    {Blank, Info},
}

// VPNReading is one chosen VPN, already read and not yet formatted.
type VPNReading struct {
	// Name is the connection's own name, and Label what the card calls it,
	// where the person gave it something shorter.
	Name  string
	Label string

	State     VPNState
	Interface string
	Connected Opt[time.Duration]

	// Stale marks a reading kept from an earlier poll.
	Stale bool
}

// The columns of a VPN's value, so the line does not move as a state, a
// latency or a duration changes.
const (
	vpnStateWidth   = 7
	vpnLatencyWidth = 6
	vpnTimeWidth    = 7
)

// VPNRows are one row per VPN, under the interfaces on the Network card. Each
// is one line and the address is not on it (spec 085 R6, R9): the card's
// question is whether the tunnel is up and answering.
//
// The latency column is kept blank until a check has measured one, so the
// figure arriving does not move the line.
func VPNRows(readings []VPNReading) []Row {
	rows := make([]Row, 0, len(readings))
	for _, r := range readings {
		rows = append(rows, vpnRow(r))
	}
	return rows
}

func vpnRow(r VPNReading) Row {
	state := vpnStates[r.State]
	parts := []Part{
		{Text: fmt.Sprintf("%-*s", vpnStateWidth, state.word), Status: state.status},
		{Text: fmt.Sprintf("  %*s", vpnLatencyWidth, Blank), Status: Info},
		{Text: fmt.Sprintf("  %*s", vpnTimeWidth, vpnElapsed(r.Connected)), Status: Info},
	}
	row := Row{Label: r.Name, Parts: parts, Stale: r.Stale, ID: "vpn:" + r.Name}
	for _, p := range parts {
		row.Value += p.Text
	}
	if r.Label != "" {
		row.Label = r.Label
		row.Tip = r.Name
	}
	if r.Interface != "" {
		row.Tip = vpnTip(row.Tip, r.Interface)
	}
	return row
}

func vpnTip(tip, iface string) string {
	if tip == "" {
		return iface
	}
	return tip + ", " + iface
}

// vpnElapsed is how long a tunnel has been up, in the two largest units that
// fit vpnTimeWidth.
func vpnElapsed(d Opt[time.Duration]) string {
	if !d.OK || d.V < 0 {
		return Blank
	}
	const day = 24 * time.Hour
	switch {
	case d.V < time.Hour:
		return fmt.Sprintf("%dm", int(d.V/time.Minute))
	case d.V < day:
		return fmt.Sprintf("%dh %02dm", int(d.V/time.Hour), int(d.V%time.Hour/time.Minute))
	case d.V < 100*day:
		return fmt.Sprintf("%dd %02dh", int(d.V/day), int(d.V%day/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d.V/day))
	}
}
