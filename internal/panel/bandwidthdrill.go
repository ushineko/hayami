package panel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

/*
The peer breakdown's drill-downs (spec 065).

A drill is a detail whose ID carries the path to it:

	peers                        every interface
	peers/<iface>/<name>         the processes of one name on one interface
	peers/<iface>/<name>/<pid>   one process: its tree, and what it holds
	peers/<iface>/<name>/@<addr> one peer: the connections to it

The path is in the ID and nowhere else, so a drill is polled, closed and
restored exactly as the top of the list is, and the shells need no notion of
where a person has navigated to. The first line of a drill is its trail,
which is also how it is left: the trail is the back button.
*/

// Drills reports whether an ID is one of this section's drill-downs.
func (b *Bandwidth) Drills(id string) bool {
	return id == view.DetailPeers || strings.HasPrefix(id, view.DetailPeers+"/")
}

// drillPath is an ID broken into its parts: the interface, the process name,
// and either a PID or a peer address.
type drillPath struct {
	iface, name string
	pid         int
	peer        string
}

// parseDrill reads a drill ID. ok is false for the top of the list.
func parseDrill(id string) (drillPath, bool) {
	parts := strings.Split(id, "/")
	if len(parts) < 3 || parts[0] != view.DetailPeers {
		return drillPath{}, false
	}
	p := drillPath{iface: parts[1], name: parts[2]}
	if len(parts) >= 4 {
		if rest := parts[3]; strings.HasPrefix(rest, "@") {
			p.peer = strings.TrimPrefix(rest, "@")
		} else {
			p.pid, _ = strconv.Atoi(rest)
		}
	}
	return p, true
}

/*
DrillUp is where this section's way out leads.

Its levels are the root, the name on an interface (two steps), and a
process or a peer under that (three). An interface on its own is not a page
anybody asked for, so the way out of a name is the root and not it.
*/
func (b *Bandwidth) DrillUp(id string) string {
	if up := DrillOut(id); strings.Count(up, "/") >= 2 {
		return up
	}
	return view.DetailPeers
}

// trail is the first line of a drill: where the page is. The way out is a
// button in the heading, which is the same corner on every page, and not a
// row in a list of readings.
func trail(id string) []view.DetailLine {
	parts := strings.Split(id, "/")
	crumbs := []string{"Bandwidth"}
	for _, p := range parts[1:] {
		crumbs = append(crumbs, strings.TrimPrefix(p, "@"))
	}
	return []view.DetailLine{
		{Name: strings.Join(crumbs, " ▸ "), Heading: true},
	}
}

// drillDetail is a drill-down of the peer breakdown.
func (b *Bandwidth) drillDetail(key, id string, use core.NetUse) view.Detail {
	path, ok := parseDrill(id)
	if !ok {
		return b.peerDetail(key, use)
	}
	d := view.Detail{Section: key, ID: id, Title: view.ProcessTitle(view.DetailPeers)}
	d.Lines = trail(id)

	var procs []core.NetProcess
	for _, p := range use.Processes {
		name := p.Name
		if p.PID == 0 {
			name = view.OtherProcesses
		}
		if p.Iface == path.iface && name == path.name {
			procs = append(procs, p)
		}
	}
	// A process that has gone quiet is not a process that has gone. It
	// moved nothing this reading, so the list above dropped its row, and
	// the page it was opened from is still the page the reader wants: it
	// is drawn as it was, at nought, and says that it is idle. Only a
	// process that is no longer there is reported as gone, which the page
	// for one process can tell by looking for it.
	if len(procs) == 0 {
		d.Lines = append(d.Lines, view.DetailLine{Name: "idle", Heading: true})
	}

	switch {
	case path.peer != "":
		b.drillPeer(&d, procs, path.peer)
	case path.pid != 0:
		b.drillProcess(&d, procs, path.pid)
	default:
		b.drillName(&d, procs)
	}
	d.Note = use.Counts
	return d
}

// drillName is the processes behind one name: a browser is forty of them and
// the list above says only "vivaldi-bin".
func (b *Bandwidth) drillName(d *view.Detail, procs []core.NetProcess) {
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].Rx+procs[i].Tx > procs[j].Rx+procs[j].Tx
	})
	var rx, tx float64
	for _, p := range procs {
		rx += p.Rx
		tx += p.Tx
	}
	d.Lines = append(d.Lines, view.PeerLine("all", 0, rx, tx))
	for _, p := range procs {
		name := fmt.Sprintf("pid %d", p.PID)
		if p.PID == 0 {
			name = "not attributable"
		}
		line := view.PeerLine(name, 1, p.Rx, p.Tx)
		if p.PID != 0 {
			line.Open = fmt.Sprintf("%s/%d", d.ID, p.PID)
		}
		d.Lines = append(d.Lines, line)
	}
	// The peers are the same at this level; they are what the name is
	// talking to, whichever of its processes did the talking.
	peers := map[string]*core.NetPeer{}
	var order []string
	for _, p := range procs {
		for _, far := range p.Peers {
			f, ok := peers[far.Addr]
			if !ok {
				c := far
				peers[far.Addr] = &c
				order = append(order, far.Addr)
				continue
			}
			f.Rx += far.Rx
			f.Tx += far.Tx
		}
	}
	sort.Slice(order, func(i, j int) bool {
		a, c := peers[order[i]], peers[order[j]]
		return a.Rx+a.Tx > c.Rx+c.Tx
	})
	if len(order) > 0 {
		d.Lines = append(d.Lines, view.DetailLine{Name: "peers", Heading: true})
	}
	for _, addr := range order {
		f := peers[addr]
		if f.Rx+f.Tx < peerFloor {
			continue
		}
		label := b.names.Name(addr)
		if f.Served {
			label = "← " + label
		}
		line := view.PeerLine(label, 1, f.Rx, f.Tx)
		line.Open = fmt.Sprintf("%s/@%s", d.ID, addr)
		d.Lines = append(d.Lines, line)
	}
}

// drillProcess is one process: who it is, who started it, what it holds, and
// the offer to end it.
func (b *Bandwidth) drillProcess(d *view.Detail, procs []core.NetProcess, pid int) {
	var self core.NetProcess
	for _, p := range procs {
		if p.PID == pid {
			self = p
		}
	}
	lines := ProcessLines(pid)
	if lines == nil {
		// The page says it once. What became of a kill is the drawer's to
		// say, and this saying so too read as two different facts.
		return
	}
	d.Lines = append(d.Lines, view.PeerLine("now", 0, self.Rx, self.Tx))
	d.Lines = append(d.Lines, lines...)
}

// drillPeer is the connections to one peer: a row of figures is a number of
// sockets, and this is them.
func (b *Bandwidth) drillPeer(d *view.Detail, procs []core.NetProcess, addr string) {
	var socks []core.NetSocket
	var rx, tx float64
	served := false
	for _, p := range procs {
		for _, far := range p.Peers {
			if far.Addr != addr {
				continue
			}
			served = served || far.Served
			rx += far.Rx
			tx += far.Tx
			socks = append(socks, far.Sockets...)
		}
	}
	name := b.names.Name(addr)
	if name != addr {
		d.Lines = append(d.Lines, view.DetailLine{Name: addr, Value: name})
	}
	d.Lines = append(d.Lines, view.PeerLine("all", 0, rx, tx))
	if served {
		d.Lines = append(d.Lines, view.DetailLine{Name: "a client of ours", Heading: true})
	}
	sort.Slice(socks, func(i, j int) bool {
		return socks[i].Rx+socks[i].Tx > socks[j].Rx+socks[j].Tx
	})
	// Every connection is counted and only the moving ones are drawn: that a
	// peer is holding twenty idle sockets is worth knowing, and twenty rows
	// of noughts is not how to say it.
	moving := 0
	for _, s := range socks {
		if s.Rx+s.Tx >= peerFloor {
			moving++
		}
	}
	d.Lines = append(d.Lines, view.DetailLine{
		Name: fmt.Sprintf("%d connections, %d moving", len(socks), moving), Heading: true,
	})
	for _, s := range socks {
		if s.Rx+s.Tx < peerFloor {
			continue
		}
		d.Lines = append(d.Lines,
			view.PeerLine(fmt.Sprintf("%d → %d", s.LocalPort, s.PeerPort), 1, s.Rx, s.Tx))
	}
}
