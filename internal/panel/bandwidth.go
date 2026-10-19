package panel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/core"
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
}

// NewBandwidth builds the bandwidth source over the named interfaces. read is
// the source of counters; nil means the kernel's own table.
func NewBandwidth(names []string, read func() (map[string]core.Counters, error)) *Bandwidth {
	return &Bandwidth{BandwidthSection: core.NewBandwidthSection(names, read)}
}

// SetNetProcesses gives the source a reader of the network by process, which
// is what makes its card open a list.
func (b *Bandwidth) SetNetProcesses(build func() core.NetReader) { b.net = build }

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
	if len(readings) == 0 {
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
	sec.Reasons = reasons
	if b.net != nil {
		sec.Opens = view.DetailProcesses
	}
	return sec
}

// Details is the network by process, where this source was given a reader.
func (b *Bandwidth) Details() []view.DetailInfo {
	if b.net == nil {
		return nil
	}
	// Both through ProcessInfo, so both carry the label and key a switch is
	// built from. One built by hand lost them, and lost the switch with it.
	return []view.DetailInfo{
		view.ProcessInfo(view.DetailProcesses),
		view.ProcessInfo(view.DetailPeers),
	}
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

// Data is the last sample as plain values.
func (b *Bandwidth) Data() any { return b.Readings() }

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
		part.Lines = append(part.Lines, view.PeerLine("all", 0, r.RxRate, r.TxRate))

		// Summed by name, as the process list is (spec 060 R6): a browser
		// is forty processes and a person asks about the browser.
		type group struct {
			rx, tx float64
			peers  map[string]*core.NetPeer
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
			row := view.PeerLine(name, 1, g.rx, g.tx)
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
		part.Lines = append(part.Lines,
			view.PeerLine("unmeasured", 1, max(0, r.RxRate-rx), max(0, r.TxRate-tx)))
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
