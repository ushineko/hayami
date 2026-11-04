package view

import (
	"fmt"
	"slices"
)

// DetailVPN is the VPNs behind the Network card's rows (spec 085 R13).
const DetailVPN = "vpn"

// detailTrailWidth is how many characters a trail takes in `detail`'s text.
const detailTrailWidth = 40

// VPNDetailInfo is how a person chooses the VPN detail beside the process lists.
func VPNDetailInfo() DetailInfo {
	return DetailInfo{ID: DetailVPN, Title: "VPNs", Label: "VPN", Key: "v", OnRequest: true}
}

// VPNStats are what a VPN's stored history adds up to.
type VPNStats struct {
	Checks    int
	Failures  int
	AverageMS float64
}

// VPNCheckLine is one check of the last cycle.
type VPNCheckLine struct {
	Type      string
	Latency   float64
	OK        bool
	DidNotRun bool
}

// VPNDetailReading is everything the drawer says of one VPN.
type VPNDetailReading struct {
	VPNReading

	Addresses []string
	Gateway   string
	Routes    []string

	Stats      VPNStats
	LastChecks []VPNCheckLine

	// Samples are the recent latencies, oldest first, and Failed says which of
	// them did not pass.
	Samples []float64
	Failed  []bool
}

/*
VPNDetail is each chosen VPN's state, addresses and health, one part per VPN.

**A part each, as usage has for its accounts.** The window draws a rule between
parts and each has its own heading and plot. On one plot, the VPNs were told apart
by colours nothing named; one list under it read as one long VPN.

**A failure is a spike in the verdict colour.** A check that fails quickly has a
low latency, so a line of latency alone can hide it. A VPN that has failed gets a
second trace at zero, rising to the top of its plot at each failure. The design
system's sparkline has no markers, and this needs none.
*/
func VPNDetail(section string, vpns []VPNDetailReading) Detail {
	d := Detail{Section: section, ID: DetailVPN, Title: VPNDetailInfo().Title}
	for _, v := range vpns {
		state := vpnStates[v.State]
		d.Parts = append(d.Parts, Detail{
			Section: section, ID: DetailVPN + "/" + v.Name, Title: vpnName(v.VPNReading),
			TitleParts: []Part{{Text: state.word, Status: state.status}},
			Trails:     vpnTrails(v),
			Lines:      vpnLines(v),
		})
	}
	return d
}

func vpnName(v VPNReading) string {
	if v.Label != "" {
		return v.Label
	}
	return v.Name
}

func vpnTrails(v VPNDetailReading) []Trail {
	if len(v.Samples) == 0 {
		return nil
	}
	out := []Trail{{Name: "latency", Samples: v.Samples, Status: Info, Coloured: true}}
	if !slices.Contains(v.Failed, true) {
		return out
	}
	top := slices.Max(v.Samples)
	spikes := make([]float64, len(v.Failed))
	for i, failed := range v.Failed {
		if failed {
			spikes[i] = top
		}
	}
	return append(out, Trail{Name: "failed", Samples: spikes, Status: Bad})
}

func vpnLines(v VPNDetailReading) []DetailLine {
	var lines []DetailLine
	add := func(name, value string) {
		if value != "" {
			lines = append(lines, DetailLine{Name: name, Value: clipMiddle(value, vpnValueWidth)})
		}
	}
	if v.Label != "" {
		add("Connection", v.Name)
	}
	add("Interface", v.Interface)
	if v.Connected.OK {
		add("Connected", vpnElapsed(v.Connected))
	}
	if v.Stats.Checks > 0 {
		passed := float64(v.Stats.Checks-v.Stats.Failures) / float64(v.Stats.Checks) * 100
		add("Latency", formatMS(Some(v.Stats.AverageMS))+" average")
		add("Failures", fmt.Sprintf("%d of %d", v.Stats.Failures, v.Stats.Checks))
		add("Uptime", fmt.Sprintf("%.1f %%", passed))
	}
	for _, c := range v.LastChecks {
		lines = append(lines, vpnCheckDetail(c))
	}
	for _, a := range v.Addresses {
		add("Address", a)
	}
	add("Gateway", v.Gateway)
	return append(lines, routeLines(v.Routes)...)
}

// vpnValueWidth is the most characters a value in this detail takes. A scrolling
// drawer is as wide as its widest row, so one provider's generated name or one full
// IPv6 address pushes every row's value past the drawer's edge.
const vpnValueWidth = 28

// clipMiddle cuts a value to a width from its middle, keeping both ends: the start
// of a name and the end of one, a network and its prefix length.
func clipMiddle(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	keep := width - 1
	return string(r[:keep-keep/2]) + "…" + string(r[len(r)-keep/2:])
}

// vpnRouteLines is how many of a tunnel's routes the drawer lists. A corporate
// tunnel can push dozens, which would leave no room for the next VPN, or for the
// health that this view is for.
const vpnRouteLines = 4

func routeLines(routes []string) []DetailLine {
	shown := routes[:min(len(routes), vpnRouteLines)]
	lines := make([]DetailLine, 0, len(shown)+1)
	for _, r := range shown {
		lines = append(lines, DetailLine{Name: "Route", Value: r})
	}
	if more := len(routes) - len(shown); more > 0 {
		lines = append(lines, DetailLine{Name: "Route", Value: fmt.Sprintf("and %d more", more)})
	}
	return lines
}

func vpnCheckDetail(c VPNCheckLine) DetailLine {
	line := DetailLine{Name: "Check " + c.Type}
	switch {
	case c.DidNotRun:
		line.Value, line.Status = "could not run", Info
	case c.OK:
		line.Value, line.Status = formatMS(Some(c.Latency)), Good
	default:
		line.Value, line.Status = "failed", Warn
	}
	return line
}
