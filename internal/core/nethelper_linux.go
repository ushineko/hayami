package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"
)

/*
The panel's side of the network helper on Linux (spec 072).

The helper is a systemd service, hayami-network, started by its socket: a
panel that connects to the socket starts it, and the service exits when its
last reader has gone. The socket is the one account's it was installed for,
so connecting asks nobody for anything. What comes down it is version 2 of
the protocol: flows, forwarded bytes and tunnel daemons.
*/

// NetHelperSocket is where the helper speaks.
const NetHelperSocket = "/run/hayami-network.sock"

// newHelperNet is the network by process as the helper reports it.
func newHelperNet(samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return NewFlowHelperReader(NetHelperSocket, samples)
}

/*
NewFlowHelperReader reads a helper at another socket. It is how the helper's
own tests read a server they started, with no service and no privilege.
*/
func NewFlowHelperReader(path string, samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return &flowNet{
		path: path, samples: samples, now: time.Now,
		ifaces: localIfaces, listening: listeningPorts(), counters: ReadCounters,
	}
}

type flowNet struct {
	path      string
	samples   func(context.Context) ([]ProcessSample, error)
	now       func() time.Time
	ifaces    func() map[string]string
	listening map[uint16]bool
	// counters reads the interfaces' own byte counts, at the moment the
	// flows are taken.
	counters func() (map[string]Counters, error)

	mu     sync.Mutex
	conn   net.Conn
	stream *helperStream
	lastAt time.Time
	was    map[string]Counters
}

// Read is each process's rate since the previous call. The first call
// connects, which starts the service, and has no rate.
func (h *flowNet) Read(ctx context.Context) (NetUse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if h.stream == nil {
		if err := h.connect(ctx); err != nil {
			return NetUse{}, err
		}
		h.lastAt = now
		h.was = h.readCounters()
		return NetUse{Counts: flowCounts, Complete: true}, nil
	}
	flows, err := h.stream.takeFlows()
	if err != nil {
		h.drop()
		return NetUse{}, err
	}
	counters := h.readCounters()
	elapsed := now.Sub(h.lastAt).Seconds()
	use := flowUse(ctx, h.samples, h.ifaces(), h.listening, flows, elapsed)
	if use.HasRate {
		use.Interfaces = map[string][2]float64{}
		for name, c := range counters {
			if w, ok := h.was[name]; ok && c.Rx >= w.Rx && c.Tx >= w.Tx {
				use.Interfaces[name] = [2]float64{float64(c.Rx-w.Rx) / elapsed, float64(c.Tx-w.Tx) / elapsed}
			}
		}
	}
	h.lastAt, h.was = now, counters
	return use, nil
}

// readCounters is the interfaces' byte counts now, or none.
func (h *flowNet) readCounters() map[string]Counters {
	if h.counters == nil {
		return nil
	}
	c, err := h.counters()
	if err != nil {
		return nil
	}
	return c
}

/*
connect opens the socket. No socket is no helper, which sends the chain on
to this process's own reader; a socket this account may not open is
another's helper, and one nobody answers is a helper whose socket unit was
stopped. Both of those are said rather than passed over, because the list
without the helper is a smaller one and the person would not know why.
*/
func (h *flowNet) connect(ctx context.Context) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", h.path)
	switch {
	case errors.Is(err, syscall.ENOENT):
		return errNoNetHelper
	case errors.Is(err, syscall.EACCES):
		return &Absence{Code: AbsenceNetProcessesDenied, Text: "helper is another's",
			Detail: "the network helper is installed for another account: run " +
				"`sudo hayami-tui network-helper install --allow $USER`",
			Err: err}
	case errors.Is(err, syscall.ECONNREFUSED):
		return &Absence{Code: AbsenceNetProcessesDenied, Text: "helper is stopped",
			Detail: "the network helper is installed and its socket is not listening: run " +
				"`sudo systemctl start hayami-network.socket`",
			Err: err}
	case err != nil:
		return fmt.Errorf("opening the network helper's socket: %w", err)
	}
	h.conn, h.stream = conn, &helperStream{want: NetHelperFlowsProtocol}
	go h.stream.run(conn)
	return nil
}

// drop lets the socket go. The caller holds the lock.
func (h *flowNet) drop() {
	if h.conn != nil {
		_ = h.conn.Close()
	}
	h.conn, h.stream = nil, nil
}

// Close lets the socket go, and the service exits when nobody else reads
// it. A later Read connects again.
func (h *flowNet) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop()
}
