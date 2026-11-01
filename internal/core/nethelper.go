package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/ushineko/hayami/internal/safetext"
)

/*
The network helper's protocol (spec 061): what a service that may trace the
network says to a panel that may not. It is lines of JSON down a pipe, one
way. The helper reads nothing a panel writes, so a panel has nothing to send
that the helper could mistake.

The first line says who is speaking. Each line after it is the bytes every
process received and sent since the line before, about a second apart. The
panel adds them up between its own readings and divides by its own clock.

Windows' helper speaks version 1: bytes by process, nothing else. Linux's
speaks version 2 (spec 072): bytes by process and both ends, which the
Peers breakdown is made of, forwarded bytes by pair of interfaces, and which
processes carry a tunnel.

What is here has no build tag: the lines, and the adding up, which a test on
any system runs.
*/

// errNoNetHelper is the helper not being installed, which is the ordinary
// state of a desk and sends the reader on to the next way of asking.
var errNoNetHelper = errors.New("the network helper is not installed")

/*
chainNet asks the helper first and this process's own reader only where no
helper is installed. Any other answer from the helper, a reading or a
failure, is the answer, so the two never both count.
*/
type chainNet struct{ helper, own NetReader }

func (c *chainNet) Read(ctx context.Context) (NetUse, error) {
	use, err := c.helper.Read(ctx)
	if !errors.Is(err, errNoNetHelper) {
		return use, err
	}
	return c.own.Read(ctx)
}

func (c *chainNet) Close() {
	c.helper.Close()
	c.own.Close()
}

// NetHelperProtocol is the version of Windows' lines. A panel that meets
// another says so and does not guess.
const NetHelperProtocol = 1

// NetHelperFlowsProtocol is the version of Linux's lines, which carry flows.
const NetHelperFlowsProtocol = 2

// NetHelperLine is one line. The first has V, Version and Counts; the rest
// have P, or Error when the helper could not go on.
type NetHelperLine struct {
	V       int    `json:"v,omitempty"`
	Version string `json:"version,omitempty"`
	// Counts is what the numbers cover, such as "TCP and UDP".
	Counts string `json:"counts,omitempty"`
	// P is one process to an entry: its ID, bytes received, bytes sent.
	P [][3]uint64 `json:"p,omitempty"`
	// Error is why the helper stopped, in its words.
	Error string `json:"error,omitempty"`

	// F is the flows that moved bytes since the line before (version 2).
	F []NetHelperFlow `json:"f,omitempty"`
	// H is the bytes forwarded by pair of interfaces since the line before.
	H []NetHelperHop `json:"h,omitempty"`
	// T is the processes that write to a tunnel device.
	T []uint32 `json:"t,omitempty"`
	// Full reports the helper's table near its size, so that some bytes
	// were lost.
	Full bool `json:"full,omitempty"`
}

// NetHelperFlow is one process's bytes with one far end, from one near end.
// PID is zero where the helper saw nobody use the socket.
type NetHelperFlow struct {
	PID       uint32 `json:"pid"`
	Proto     uint8  `json:"proto"`
	Local     string `json:"l"`
	LocalPort uint16 `json:"lp"`
	Peer      string `json:"r"`
	PeerPort  uint16 `json:"rp"`
	Rx        uint64 `json:"rx"`
	Tx        uint64 `json:"tx"`
}

// key is a flow without its bytes.
func (f NetHelperFlow) key() NetHelperFlow {
	f.Rx, f.Tx = 0, 0
	return f
}

// NetHelperHop is bytes forwarded in by one interface and out by another.
type NetHelperHop struct {
	In    string `json:"in"`
	Out   string `json:"out"`
	Bytes uint64 `json:"bytes"`
}

// helperFlows is version 2's lines added up between two readings.
type helperFlows struct {
	flows   map[NetHelperFlow][2]uint64
	hops    map[[2]string]uint64
	tunnels map[uint32]bool
	full    bool
}

/*
helperStream is the panel's end: it reads lines until the pipe closes and
keeps the bytes counted since they were last taken.
*/
type helperStream struct {
	// want is the version this panel reads from this helper: zero is
	// NetHelperProtocol.
	want int

	mu     sync.Mutex
	hello  NetHelperLine
	counts map[uint32][2]uint64
	flows  helperFlows
	err    error
	done   bool
}

// run reads r to its end. It returns when the pipe closes or a line is not
// one this version reads.
func (s *helperStream) run(r io.Reader) {
	err := s.read(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	if s.err == nil {
		s.err = err
	}
}

func (s *helperStream) read(r io.Reader) error {
	lines := bufio.NewScanner(r)
	// Version 2's lines carry every flow that moved in a tick, about a
	// hundred bytes each: a desk with tens of thousands of peers (a torrent
	// client) writes lines of megabytes. The helper's table holds 65536
	// flows, and this is room for all of them.
	lines.Buffer(make([]byte, 0, 64<<10), 16<<20)
	first := true
	for lines.Scan() {
		var line NetHelperLine
		if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
			return fmt.Errorf("the network helper sent a line that is not one: %w", err)
		}
		if line.Error != "" {
			return fmt.Errorf("the network helper: %s", safetext.Printable(line.Error, maxHelperError)) //nolint:err113 // the helper's own words
		}
		want := s.want
		if want == 0 {
			want = NetHelperProtocol
		}
		if first {
			first = false
			if line.V != want {
				return fmt.Errorf("the network helper speaks version %d of its protocol and this panel version %d: "+ //nolint:err113 // both numbers are the message
					"install the helper again from this version's installer", line.V, want)
			}
			s.mu.Lock()
			s.hello = line
			s.mu.Unlock()
			continue
		}
		// Each version's fields and no others: what a version does not use
		// is never taken, and would be kept for as long as the pipe is open.
		s.mu.Lock()
		if want == NetHelperProtocol {
			s.addCounts(line)
		} else {
			s.flows.add(line)
		}
		s.mu.Unlock()
	}
	if err := lines.Err(); err != nil {
		return fmt.Errorf("reading the network helper: %w", err)
	}
	return errors.New("the network helper closed its pipe") //nolint:err113 // an end, said plainly
}

// take is the bytes counted since the previous call and what they cover, or
// why the stream ended.
func (s *helperStream) take() (map[uint32][2]uint64, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return nil, "", s.err
	}
	counts := s.counts
	s.counts = nil
	return counts, s.hello.Counts, nil
}

/*
What the panel keeps of a helper's lines between two readings is bounded:
the helper's own tables are no larger, so more than this is not a helper's.
The process serving the pipe is checked before it is read (Windows) or is
root's (Linux); the bounds are what holds if a check is ever wrong.
*/
const (
	maxHelperEntries = 65536 // processes, or flows: the helper's table size
	maxHelperHops    = 1024
	maxHelperTunnels = 256
	maxHelperText    = 64  // an address or an interface name
	maxHelperError   = 512 // the helper's account of why it stopped
)

// addCounts adds version 1's bytes by process. The caller holds the lock.
func (s *helperStream) addCounts(line NetHelperLine) {
	if s.counts == nil {
		s.counts = map[uint32][2]uint64{}
	}
	for _, p := range line.P {
		pid := uint32(p[0]) //nolint:gosec // a process ID
		c, held := s.counts[pid]
		if !held && len(s.counts) >= maxHelperEntries {
			continue
		}
		c[0] += p[1]
		c[1] += p[2]
		s.counts[pid] = c
	}
}

// takeFlows is version 2's bytes since the previous call, or why the stream
// ended.
func (s *helperStream) takeFlows() (helperFlows, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return helperFlows{}, s.err
	}
	flows := s.flows
	s.flows = helperFlows{}
	return flows, nil
}

// add adds one line's flows, hops and tunnels.
func (f *helperFlows) add(line NetHelperLine) {
	if f.flows == nil {
		f.flows = map[NetHelperFlow][2]uint64{}
		f.hops = map[[2]string]uint64{}
		f.tunnels = map[uint32]bool{}
	}
	for _, fl := range line.F {
		if len(fl.Local) > maxHelperText || len(fl.Peer) > maxHelperText {
			continue
		}
		k := fl.key()
		c, held := f.flows[k]
		if !held && len(f.flows) >= maxHelperEntries {
			f.full = true
			continue
		}
		c[0] += fl.Rx
		c[1] += fl.Tx
		f.flows[k] = c
	}
	for _, h := range line.H {
		if len(h.In) > maxHelperText || len(h.Out) > maxHelperText {
			continue
		}
		k := [2]string{h.In, h.Out}
		if _, held := f.hops[k]; !held && len(f.hops) >= maxHelperHops {
			continue
		}
		f.hops[k] += h.Bytes
	}
	for _, pid := range line.T {
		if !f.tunnels[pid] && len(f.tunnels) >= maxHelperTunnels {
			continue
		}
		f.tunnels[pid] = true
	}
	f.full = f.full || line.Full
}

// NetHelperCounts is a line of counts, for the helper to send.
func NetHelperCounts(counts map[uint32][2]uint64) NetHelperLine {
	line := NetHelperLine{P: make([][3]uint64, 0, len(counts))}
	for pid, c := range counts {
		line.P = append(line.P, [3]uint64{uint64(pid), c[0], c[1]})
	}
	return line
}

// flowCounts is what Linux's helper counts.
const flowCounts = "every protocol"

/*
flowUse makes a reading of version 2's bytes over elapsed seconds: the
flows grouped as the sockets of this process's own reader are, and the
forwarded bytes put to both interfaces of each pair, received on one and
sent on the other.
*/
func flowUse(ctx context.Context, samples func(context.Context) ([]ProcessSample, error),
	ifaces map[string]string, listening map[uint16]bool, f helperFlows, elapsed float64) NetUse {
	use := NetUse{Counts: flowCounts, Complete: true}
	if elapsed <= 0 {
		return use
	}
	use.HasRate = true

	rates := make([]socketRate, 0, len(f.flows))
	for k, c := range f.flows {
		rates = append(rates, socketRate{
			pid: int(k.PID), local: k.Local, peer: k.Peer,
			localPort: k.LocalPort, peerPort: k.PeerPort,
			rx: float64(c[0]) / elapsed, tx: float64(c[1]) / elapsed,
		})
	}
	use.Processes = byProcess(rates, processNames(ctx, samples), ifaces, listening)
	for i, p := range use.Processes {
		if p.PID != 0 && f.tunnels[uint32(p.PID)] { //nolint:gosec // a process ID
			use.Processes[i].Tunnel = true
		}
	}

	by := map[[2]string]*NetForward{}
	at := func(iface, via string) *NetForward {
		k := [2]string{iface, via}
		if by[k] == nil {
			by[k] = &NetForward{Iface: iface, Via: via}
		}
		return by[k]
	}
	for h, n := range f.hops {
		in, out, rate := h[0], h[1], float64(n)/elapsed
		if in != "" {
			at(in, out).Rx += rate
		}
		if out != "" {
			at(out, in).Tx += rate
		}
	}
	for _, fw := range by {
		use.Forwarded = append(use.Forwarded, *fw)
	}
	sort.Slice(use.Forwarded, func(i, j int) bool {
		a, b := use.Forwarded[i], use.Forwarded[j]
		if a.Iface != b.Iface {
			return a.Iface < b.Iface
		}
		return a.Rx+a.Tx > b.Rx+b.Tx
	})
	return use
}
