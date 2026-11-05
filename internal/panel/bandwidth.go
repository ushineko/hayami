package panel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/keepalive"
	"github.com/ushineko/hayami/internal/view"
)

// Bandwidth is the network interfaces as a Source.
type Bandwidth struct {
	*core.BandwidthSection

	// net builds the reader of the network by process, and reader is the one
	// in use while the list is shown (spec 060). Nil for a source with no
	// list: a test's counters.
	net    func() core.NetReader
	netMu  sync.Mutex
	reader core.NetReader

	// names puts names to peers.
	names *core.PeerNames

	// keep is the file a press tells the keepalive helper through, keepErr why there
	// is none, and keepaliveItems the VPNs it is switched on for.
	keep           keepalive.Files
	keepErr        error
	keepaliveItems map[string]bool

	// vpns reads the chosen VPNs (spec 085), and labels is what the card calls
	// each. Nil on a platform with no VPN systems.
	vpns     *core.VPNSection
	labelsMu sync.Mutex
	labels   map[string]string

	// health runs the chosen VPNs' checks and keeps what they said.
	health *vpnHealth
}

// NewBandwidth builds the bandwidth source over the named interfaces. read is
// the source of counters; nil means the kernel's own table.
func NewBandwidth(names []string, read func() (map[string]core.Counters, error)) *Bandwidth {
	return &Bandwidth{BandwidthSection: core.NewBandwidthSection(names, read)}
}

// SetNetProcesses gives the source a reader of the network by process, which
// is what makes its card open a list.
func (b *Bandwidth) SetNetProcesses(build func() core.NetReader) { b.net = build }

// SetVPNSystems gives the source the systems that keep VPNs and the ones
// chosen. A platform with no systems has no VPN rows, whatever the settings say
// (spec 085 R3).
func (b *Bandwidth) SetVPNSystems(systems []core.VPNBackend, checkers core.VPNCheckers, settings config.BandwidthSettings) {
	if len(systems) == 0 {
		return
	}
	b.vpns = core.NewVPNSection(nil, systems)
	// A home directory that cannot be found leaves a press with nowhere to say so,
	// which ControlVPN reports.
	b.keep, b.keepErr = keepalive.Here()
	b.health = newVPNHealth(checkers)
	b.SetVPNs(settings)
}

// SetVPNs replaces the chosen VPNs and their checks, for a change of the settings.
func (b *Bandwidth) SetVPNs(settings config.BandwidthSettings) {
	if b.vpns == nil {
		return
	}
	names := make([]string, 0, len(settings.VPNs))
	labels := make(map[string]string, len(settings.VPNs))
	kept := make(map[string]bool, len(settings.VPNs))
	for _, c := range settings.VPNs {
		names = append(names, c.Name)
		labels[c.Name] = c.Label
		kept[c.Name] = c.Keepalive
	}
	b.labelsMu.Lock()
	b.labels = labels
	b.keepaliveItems = kept
	b.labelsMu.Unlock()
	b.vpns.SetNames(names)
	b.SetTunnels(nil)
	b.health.configure(settings)
}

// ControlVPN does what a press on a VPN's row asked. Only a chosen VPN has rows, so
// any other name is not a press.
func (b *Bandwidth) ControlVPN(ctx context.Context, action, name string, signIn core.SignIn) error {
	if b.vpns == nil || !slices.ContainsFunc(b.vpns.Readings(), func(v core.VPN) bool { return v.Name == name }) {
		return core.ErrVPNNotConfigured
	}
	if b.keepErr != nil {
		return b.keepErr
	}
	// The helper is told first. A tunnel seen down before the hold is written is a
	// tunnel it would re-dial.
	switch action {
	case view.VPNConnect:
		return b.heldWhile(ctx, name, func() error { return b.vpns.Connect(ctx, name, signIn) })
	case view.VPNDisconnect:
		if err := b.keep.Update(func(s *keepalive.State) { s.Hold(name) }); err != nil {
			return err //nolint:wrapcheck // its words are what the row says
		}
		return b.vpns.Disconnect(ctx, name) //nolint:wrapcheck // its words are what the row says
	case view.VPNBounce:
		return b.heldWhile(ctx, name, func() error { return b.vpns.Bounce(ctx, name, signIn) })
	case view.VPNReenable:
		return b.keep.Update(func(s *keepalive.State) { s.Enable(name) }) //nolint:wrapcheck // its words are what the row says
	}
	return fmt.Errorf("no VPN action %q", action)
}

// heldWhile holds a VPN for as long as a press takes, so the helper does not dial
// against it: afterwards a drop is a drop again, including one the press could not
// bring back. A press that was replaced leaves the hold to the press that replaced it,
// which may be a disconnect that means to stay down.
func (b *Bandwidth) heldWhile(ctx context.Context, name string, press func() error) error {
	if err := b.keep.Update(func(s *keepalive.State) { s.Hold(name) }); err != nil {
		return err //nolint:wrapcheck // its words are what the row says
	}
	err := press()
	if ctx.Err() == nil {
		_ = b.keep.Update(func(s *keepalive.State) { s.Release(name) })
	}
	return err
}

// vpnChosen reports whether this source draws any VPN.
func (b *Bandwidth) vpnChosen() bool { return b.vpns != nil && b.vpns.Chosen() }

// Poll reads the interfaces, and the VPNs when they are due. Either is enough
// for the card to have something to say.
//
// The VPNs go first, so the counters read in this poll include a tunnel that has
// just come up.
func (b *Bandwidth) Poll(ctx context.Context) (bool, error) {
	chosen := b.vpnChosen()
	if chosen {
		b.vpns.Poll(ctx)
		b.SetTunnels(tunnelsUp(b.vpns.Readings()))
		b.health.tick(ctx, b.vpns.Readings())
	}
	has, err := b.BandwidthSection.Poll(ctx)
	return has || chosen, err //nolint:wrapcheck // the section's own account
}

// tunnelsUp are the interfaces of the VPNs that are up, by VPN.
func tunnelsUp(vpns []core.VPN) map[string]string {
	out := make(map[string]string, len(vpns))
	for _, v := range vpns {
		if v.State == core.VPNUp && v.Interface != "" {
			out[v.Name] = v.Interface
		}
	}
	return out
}

// Section turns the last sample into rows, and the trails into a plot.
//
// HasRate is false until the second poll, because a rate is a difference. The
// distinction survives to here rather than being flattened to zero: a blank of
// the right width says "not yet", and a nought says "nothing is happening",
// and they are not the same claim.
func (b *Bandwidth) Section() view.Section {
	readings := b.Readings()
	out := make([]view.BandwidthReading, 0, len(readings))
	var reasons []view.Reason
	for _, r := range readings {
		if !r.Present {
			// An interface the user named and the kernel does not list. It
			// used to draw a row of blanks -- correct for the column widths
			// and silent about the name being wrong, which after a hardware
			// rename is the ordinary way this happens. The reason takes that
			// row's place rather than sitting under it: two lines about one
			// absent interface is one line too many.
			reasons = append(reasons, view.Reason{
				Label: r.Name, Text: "not present", Status: view.Info,
			})
			continue
		}
		trail := b.Trail(r.Name)
		w, radio := b.Wireless(r.Name)
		out = append(out, view.BandwidthReading{
			Name:     r.Name,
			RxRate:   r.RxRate,
			TxRate:   r.TxRate,
			RxTotal:  r.RxTotal,
			TxTotal:  r.TxTotal,
			HasRate:  r.HasRate,
			HasTotal: r.Present,
			RxTrail:  trail.Rx,
			TxTrail:  trail.Tx,
			Radio:    radio,
			Link:     link(w),
		})
	}
	sec := view.Bandwidth(out)
	if len(readings) == 0 && !b.vpnChosen() {
		reasons = append(reasons, view.Reason{
			Text: "no interfaces chosen", Status: view.Info,
			Detail: "pick one in the preferences, or pass --sections",
		})
	}
	if err := b.WirelessErr(); errors.Is(err, core.ErrWirelessDenied) {
		// Aside: the row already shows what it could read, blank where it
		// could not, and a line on the card about a privacy setting would be
		// there every day on every machine that keeps it. The pointer and
		// doctor say it; the words are the source's (spec 040).
		reasons = append(reasons, reason(view.Reason{Label: "Wi-Fi", Status: view.Info, Aside: true}, err))
	}
	if b.vpnChosen() {
		vpns := b.vpnReadings()
		sec.Rows = append(sec.Rows, view.VPNRows(vpns)...)
		// After the interfaces', so an interface keeps its colour as VPNs are chosen.
		sec.Trails = append(sec.Trails, view.VPNTrails(vpns, len(out))...)
		reasons = append(reasons, b.vpnReasons()...)
	}
	sec.Reasons = reasons
	if b.net != nil {
		sec.Opens = view.DetailProcesses
	} else if b.vpnChosen() {
		sec.Opens = view.DetailVPN
	}
	return sec
}

// vpnStates is core's state as the view takes it.
var vpnStates = map[core.VPNState]view.VPNState{
	core.VPNDown: view.VPNDown, core.VPNConnecting: view.VPNConnecting,
	core.VPNUp: view.VPNUp, core.VPNUnknown: view.VPNUnknown,
}

// vpnReadings are the chosen VPNs as the view takes them.
func (b *Bandwidth) vpnReadings() []view.VPNReading {
	b.labelsMu.Lock()
	labels := b.labels
	b.labelsMu.Unlock()
	now := time.Now()
	vpns := b.vpns.Readings()
	out := make([]view.VPNReading, 0, len(vpns))
	points := b.health.points()
	for _, v := range vpns {
		r := view.VPNReading{
			Name: v.Name, Label: labels[v.Name], State: vpnStates[v.State],
			Interface: v.Interface, Stale: v.Stale,
			Connected: view.Maybe(now.Sub(v.Since), v.State == core.VPNUp && !v.Since.IsZero()),
		}
		// A figure is about the tunnel that is up now. One from before it
		// went down, or before the checks stopped, says nothing of it.
		if v.State == core.VPNUp {
			if last, ok := b.health.current(points[v.Name], v.Since, now); ok {
				r.Latency, r.Failing = view.Some(last.LatencyMS), !last.OK
			}
			if rates, trail, ok := b.Tunnel(v.Name); ok {
				r.Traffic = &view.BandwidthReading{
					Name: rates.Name, RxRate: rates.RxRate, TxRate: rates.TxRate,
					RxTotal: rates.RxTotal, TxTotal: rates.TxTotal,
					HasRate: rates.HasRate, HasTotal: rates.Present,
					RxTrail: trail.Rx, TxTrail: trail.Tx,
				}
			}
		}
		out = append(out, r)
	}
	return out
}

// vpnReasons say why the rows may not be the whole story: a system that would
// not answer, or none running at all. Neither makes a row read as a tunnel down
// (spec 085 R4).
func (b *Bandwidth) vpnReasons() []view.Reason {
	if err := b.vpns.Failed(); err != nil {
		return []view.Reason{{Label: "VPN", Text: "would not answer", Detail: err.Error(), Status: view.Warn}}
	}
	if b.vpns.NoSystem() {
		return []view.Reason{{Label: "VPN", Text: "no VPN system running", Status: view.Info}}
	}
	return nil
}

// Details is the network by process, where this source was given a reader.
func (b *Bandwidth) Details() []view.DetailInfo {
	var out []view.DetailInfo
	if b.net != nil {
		// Both through ProcessInfo, so both carry the label and key a switch is
		// built from. One built by hand lost them, and lost the switch with it.
		out = append(out, view.ProcessInfo(view.DetailProcesses), view.ProcessInfo(view.DetailPeers))
	}
	if b.vpnChosen() {
		out = append(out, view.VPNDetailInfo())
	}
	return out
}

// DetailInterval is ProcessInterval.
func (b *Bandwidth) DetailInterval(string) time.Duration { return ProcessInterval }

// CloseDetail lets go of the reader, which on Windows holds a trace session
// open. The next poll of the list builds another.
func (b *Bandwidth) CloseDetail(string) {
	b.netMu.Lock()
	defer b.netMu.Unlock()
	if b.reader != nil {
		b.reader.Close()
		b.reader = nil
	}
}

/*
PollDetail reads the network by process.

**The list is partial and says so** (spec 060 R11). Its note names what it is
made of, the reader's own words, and the share of the watched interfaces'
traffic the list adds up to, so a list that comes to less than the rows above
it reads as partial and not as wrong.
*/
func (b *Bandwidth) PollDetail(ctx context.Context, id string) (view.Detail, error) {
	if id == view.DetailVPN && b.vpnChosen() {
		return b.vpnDetail(), nil
	}
	key, title := b.Key(), view.ProcessTitle(id)
	if (id != view.DetailProcesses && !b.Drills(id)) || b.net == nil {
		return view.Detail{}, fmt.Errorf("the %s section has no detail %q", key, id)
	}
	b.netMu.Lock()
	if b.reader == nil {
		b.reader = b.net()
	}
	reader := b.reader
	b.netMu.Unlock()

	use, err := reader.Read(ctx)
	if err != nil {
		return unread(key, id, title, err), err //nolint:wrapcheck // the account the reader gives of the absence
	}
	if b.Drills(id) {
		if b.names == nil {
			b.names = core.NewPeerNames()
		}
		return b.drillDetail(key, id, use), nil
	}
	uses := make([]view.ProcessUse, 0, len(use.Processes))
	listed := 0.0
	for _, p := range use.Processes {
		name := p.Name
		if p.PID == 0 {
			name = ""
		}
		uses = append(uses, view.ProcessUse{Name: name, Value: p.Rx, Second: p.Tx})
		listed += p.Rx + p.Tx
	}
	d := view.Processes(key, id, uses)
	d.Note = netNote(use, listed, b.Readings())
	return d, nil
}

// shareFloor is the least an interface total has to be, in bytes per second,
// before the share of it the list accounts for is worth saying: under it the
// figure is one packet either way.
const shareFloor = 1024

// netNote is the line under the list: what it counts, and the share of the
// interfaces' traffic it comes to.
func netNote(use core.NetUse, listed float64, readings []core.Rates) string {
	if !use.HasRate {
		return use.Counts + ", measuring"
	}
	total, known := 0.0, false
	for _, r := range readings {
		if r.Present && r.HasRate {
			total += r.RxRate + r.TxRate
			known = true
		}
	}
	if !known || total < shareFloor {
		return use.Counts
	}
	share := min(100*listed/total, 100)
	return fmt.Sprintf("%s, %.0f %% of the interfaces' traffic", use.Counts, share)
}

// link is a Wi-Fi description as the view takes it.
func link(w core.Wireless) view.LinkReading {
	return view.LinkReading{
		Connected: w.Connected,
		RSSI:      w.RSSI, HasRSSI: w.HasRSSI,
		Signal: w.Signal, HasSignal: w.HasSignal,
		Band:    w.Band(),
		Channel: w.Channel, HasChannel: w.HasChannel,
		Generation: w.Generation,
		RxRate:     w.RxRate, TxRate: w.TxRate, HasRx: w.HasRx, HasTx: w.HasTx,
	}
}

// Key names the section.
func (b *Bandwidth) Key() string { return view.BandwidthInfo.Key }

// Data is the last sample as plain values. With no VPN chosen it is the list of
// interfaces it always was, so a script reading it sees no change (spec 085 R7).
func (b *Bandwidth) Data() any {
	if !b.vpnChosen() {
		return b.Readings()
	}
	type vpnData struct {
		core.VPN
		Rates *core.Rates `json:"rates,omitempty"`
	}
	vpns := b.vpns.Readings()
	out := make([]vpnData, 0, len(vpns))
	for _, v := range vpns {
		d := vpnData{VPN: v}
		if rates, _, ok := b.Tunnel(v.Name); ok && v.State == core.VPNUp {
			d.Rates = &rates
		}
		out = append(out, d)
	}
	return struct {
		Interfaces []core.Rates `json:"interfaces"`
		VPNs       []vpnData    `json:"vpns"`
	}{b.Readings(), out}
}

/*
peerDetail is the breakdown: one part per interface, each the
processes that used it with their peers under them, and the interface's
traffic the list does not account for as its last line.

Parts and not Lines, deliberately. The Lines path cuts the list to the
drawer's height (gui.Panel.render, detail.Top) and never scrolls, and the cut
is a prefix -- so the remainder, which is last, would be the first row
dropped, and a list whose job is to say it is incomplete would go quiet about
it. The Parts path scrolls and draws the whole of what it is given.
*/
func (b *Bandwidth) peerDetail(key string, use core.NetUse) view.Detail {
	d := view.Detail{Section: key, ID: view.DetailPeers, Title: view.ProcessTitle(view.DetailPeers)}

	byIface := map[string][]core.NetProcess{}
	for _, p := range use.Processes {
		name := p.Iface
		if name == "" {
			name = "elsewhere"
		}
		byIface[name] = append(byIface[name], p)
	}

	for _, r := range b.Readings() {
		if !r.Present || !r.HasRate {
			continue
		}
		part := view.Detail{Section: key, ID: view.DetailPeers, Title: r.Name}
		// The interface's rate over the reader's own interval, where it
		// gives one, so the remainder is not one second set against another.
		allRx, allTx := r.RxRate, r.TxRate
		if rate, ok := use.Interfaces[r.Name]; ok {
			allRx, allTx = rate[0], rate[1]
		}
		part.Lines = append(part.Lines, view.PeerLine("all", 0, allRx, allTx))

		// Summed by name, as the process list is (spec 060 R6): a browser
		// is forty processes and a person asks about the browser.
		type group struct {
			rx, tx float64
			peers  map[string]*core.NetPeer
			tunnel bool
		}
		byName := map[string]*group{}
		var order []string
		for _, p := range byIface[r.Name] {
			name := p.Name
			if p.PID == 0 {
				name = view.OtherProcesses
			}
			g, ok := byName[name]
			if !ok {
				g = &group{peers: map[string]*core.NetPeer{}}
				byName[name] = g
				order = append(order, name)
			}
			g.rx += p.Rx
			g.tx += p.Tx
			g.tunnel = g.tunnel || p.Tunnel
			for _, far := range p.Peers {
				f, ok := g.peers[far.Addr]
				if !ok {
					c := far
					g.peers[far.Addr] = &c
					continue
				}
				f.Rx += far.Rx
				f.Tx += far.Tx
			}
		}
		sort.Slice(order, func(i, j int) bool {
			a, b := byName[order[i]], byName[order[j]]
			return a.rx+a.tx > b.rx+b.tx
		})

		var rx, tx float64
		for _, name := range order {
			g := byName[name]
			rx += g.rx
			tx += g.tx
			// A row that is moving nothing says nothing, and in a list long
			// enough to scroll it costs a line that a row with a figure in
			// it could have had.
			if g.rx+g.tx < peerFloor {
				continue
			}
			label := name
			if g.tunnel {
				// Its bytes here are a tunnel interface's over again.
				label += " (tunnel)"
			}
			row := view.PeerLine(label, 1, g.rx, g.tx)
			row.Open = view.DetailPeers + "/" + r.Name + "/" + name
			part.Lines = append(part.Lines, row)
			var fars []core.NetPeer
			for _, f := range g.peers {
				if f.Rx+f.Tx >= peerFloor {
					fars = append(fars, *f)
				}
			}
			sort.Slice(fars, func(i, j int) bool {
				return fars[i].Rx+fars[i].Tx > fars[j].Rx+fars[j].Tx
			})
			for _, far := range fars {
				label := b.names.Name(far.Addr)
				if far.Served {
					label = "\u2190 " + label
				}
				line := view.PeerLine(label, 3, far.Rx, far.Tx)
				line.Open = view.DetailPeers + "/" + r.Name + "/" + name + "/@" + far.Addr
				part.Lines = append(part.Lines, line)
			}
		}
		// Traffic this machine passed between two of its interfaces, which
		// no process sent: a virtual machine's, a container's, an exit
		// node's (spec 072).
		// Grouped under one line, as peers are under a process: an
		// interface's name is up to fifteen characters, and with a word in
		// front of it a row would run into its figures.
		var fwRx, fwTx float64
		var vias []view.DetailLine
		for _, fw := range use.Forwarded {
			if fw.Iface != r.Name || fw.Rx+fw.Tx < peerFloor {
				continue
			}
			fwRx += fw.Rx
			fwTx += fw.Tx
			vias = append(vias, view.PeerLine("\u2194 "+fw.Via, 3, fw.Rx, fw.Tx))
		}
		if len(vias) > 0 {
			rx += fwRx
			tx += fwTx
			part.Lines = append(part.Lines, view.PeerLine("forwarded", 1, fwRx, fwTx))
			part.Lines = append(part.Lines, vias...)
		}
		// What is left is traffic the reader cannot see, or, from one that
		// sees everything, headers and multicast.
		rest := "unmeasured"
		if use.Complete {
			rest = "headers / multicast"
		}
		part.Lines = append(part.Lines,
			view.PeerLine(rest, 1, max(0, allRx-rx), max(0, allTx-tx)))
		d.Parts = append(d.Parts, part)
	}
	if len(d.Parts) == 0 {
		d.Note = use.Counts + ", measuring"
	}
	return d
}

// peerFloor is the least a peer has to be moving, in bytes per second, to be
// worth a line of its own: under it the figure is one packet either way and
// the row is noise in a list that is already long.
// Anything at all, so a row that is moving nothing is not drawn.
const peerFloor = 1
