package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func helperLines(t *testing.T, lines ...NetHelperLine) io.Reader {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		j, err := json.Marshal(l)
		require.NoError(t, err)
		b.Write(j)
		b.WriteByte('\n')
	}
	return strings.NewReader(b.String())
}

var helperHello = NetHelperLine{V: NetHelperProtocol, Version: "1.2.3", Counts: "TCP and UDP"}

// The panel adds up what the helper sends between its own readings: two
// lines for one process are one figure, and taking it starts the count again.
func TestTheHelpersLinesAreAddedUpUntilTaken(t *testing.T) {
	s := &helperStream{}
	require.Error(t, s.read(helperLines(t, helperHello,
		NetHelperCounts(map[uint32][2]uint64{7: {100, 10}, 9: {1, 2}}),
		NetHelperCounts(map[uint32][2]uint64{7: {50, 5}}),
	)), "the pipe closing is an end, and is said")

	counts, what, err := s.take()
	require.NoError(t, err)
	assert.Equal(t, "TCP and UDP", what)
	assert.Equal(t, map[uint32][2]uint64{7: {150, 15}, 9: {1, 2}}, counts)

	counts, _, err = s.take()
	require.NoError(t, err)
	assert.Empty(t, counts, "taken once")
}

// A helper of another version of the protocol is refused by name, with what
// to do, and none of its numbers are counted.
func TestAHelperOfAnotherProtocolIsRefused(t *testing.T) {
	s := &helperStream{}
	s.run(helperLines(t, NetHelperLine{V: NetHelperProtocol + 1, Version: "9"},
		NetHelperCounts(map[uint32][2]uint64{7: {100, 10}})))
	_, _, err := s.take()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install the helper again")
}

// What the helper says went wrong is what the panel reports, and a stream
// that has ended keeps saying why.
func TestTheHelpersErrorIsThePanels(t *testing.T) {
	s := &helperStream{}
	s.run(helperLines(t, helperHello, NetHelperLine{Error: "enabling the network provider: Access is denied."}))
	for range 2 {
		_, _, err := s.take()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Access is denied")
	}

	s = &helperStream{}
	s.run(strings.NewReader("not a line\n"))
	_, _, err := s.take()
	require.Error(t, err)
}

// Bytes become rates over the time that passed. A process that has gone
// since it sent is still traffic, under no process; with no time passed
// there is no rate to give.
func TestCountsBecomeRatesByName(t *testing.T) {
	samples := func(context.Context) ([]ProcessSample, error) {
		return []ProcessSample{{PID: 7, Name: "browser"}}, nil
	}
	use := netUse(t.Context(), samples, "TCP and UDP", map[uint32][2]uint64{7: {2000, 200}, 9: {10, 20}}, 2)
	assert.True(t, use.HasRate)
	assert.Equal(t, "TCP and UDP", use.Counts)
	assert.ElementsMatch(t, []NetProcess{
		{PID: 7, Name: "browser", Rx: 1000, Tx: 100},
		{PID: 0, Rx: 5, Tx: 10},
	}, use.Processes)

	first := netUse(t.Context(), samples, "TCP and UDP", nil, 0)
	assert.False(t, first.HasRate)
	assert.Empty(t, first.Processes)
}

var flowHello = NetHelperLine{V: NetHelperFlowsProtocol, Version: "1.2.3", Counts: "every protocol"}

// Spec 072. Linux's helper speaks version 2, and a panel reading it adds up
// each flow, each forwarded pair and each tunnel daemon between readings.
func TestVersionTwoFlowsAreAddedUpUntilTaken(t *testing.T) {
	flow := NetHelperFlow{PID: 7, Proto: 17, Local: "192.0.2.1", LocalPort: 5000, Peer: "198.51.100.9", PeerPort: 443}
	once := flow
	once.Rx, once.Tx = 100, 10
	twice := flow
	twice.Rx, twice.Tx = 50, 5
	// A stream that has ended answers with why, so the lines go down a
	// pipe that stays open.
	r, w := io.Pipe()
	s := &helperStream{want: NetHelperFlowsProtocol}
	go s.run(r)
	enc := json.NewEncoder(w)
	for _, l := range []NetHelperLine{flowHello,
		{F: []NetHelperFlow{once}, H: []NetHelperHop{{In: "vnet0", Out: "eth0", Bytes: 1000}}, T: []uint32{3}},
		{F: []NetHelperFlow{twice}, H: []NetHelperHop{{In: "vnet0", Out: "eth0", Bytes: 500}}, Full: true},
	} {
		require.NoError(t, enc.Encode(l))
	}
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.flows.full
	}, time.Second, time.Millisecond)

	got, err := s.takeFlows()
	require.NoError(t, err)
	assert.Equal(t, map[NetHelperFlow][2]uint64{flow: {150, 15}}, got.flows)
	assert.Equal(t, map[[2]string]uint64{{"vnet0", "eth0"}: 1500}, got.hops)
	assert.Equal(t, map[uint32]bool{3: true}, got.tunnels)
	assert.True(t, got.full)

	again, err := s.takeFlows()
	require.NoError(t, err)
	assert.Empty(t, again.flows)
	_ = w.Close()
}

// A panel that reads version 2 refuses a helper of version 1, and the other
// way about, by both numbers.
func TestEachPanelReadsItsOwnHelpersVersion(t *testing.T) {
	s := &helperStream{want: NetHelperFlowsProtocol}
	s.run(helperLines(t, helperHello))
	_, err := s.takeFlows()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version 1 of its protocol and this panel version 2")

	s = &helperStream{}
	s.run(helperLines(t, flowHello))
	_, _, err = s.take()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version 2 of its protocol and this panel version 1")
}

/*
Flows become the same breakdown as sockets do: by process and interface,
then by peer, then by port. A process with no name is "other". A tunnel
daemon is marked. Forwarded bytes are put to both interfaces, received on
the one they came in by and sent on the one they left by.
*/
func TestFlowsBecomeTheBreakdown(t *testing.T) {
	samples := func(context.Context) ([]ProcessSample, error) {
		return []ProcessSample{{PID: 7, Name: "browser"}, {PID: 3, Name: "tailscaled"}}, nil
	}
	ifaces := map[string]string{"192.0.2.1": "eth0"}
	f := helperFlows{
		flows: map[NetHelperFlow][2]uint64{
			{PID: 7, Proto: 17, Local: "192.0.2.1", LocalPort: 5000, Peer: "198.51.100.9", PeerPort: 443}:   {2000, 200},
			{PID: 7, Proto: 6, Local: "192.0.2.1", LocalPort: 5001, Peer: "198.51.100.9", PeerPort: 443}:    {1000, 100},
			{PID: 3, Proto: 17, Local: "192.0.2.1", LocalPort: 41641, Peer: "203.0.113.5", PeerPort: 41641}: {400, 400},
			{PID: 99, Proto: 17, Local: "192.0.2.1", LocalPort: 53, Peer: "203.0.113.1", PeerPort: 5353}:    {20, 0},
		},
		hops:    map[[2]string]uint64{{"vnet0", "eth0"}: 600, {"eth0", "vnet0"}: 4000},
		tunnels: map[uint32]bool{3: true},
	}
	use := flowUse(t.Context(), samples, ifaces, map[uint16]bool{53: true}, f, 2)
	assert.True(t, use.HasRate)
	assert.True(t, use.Complete)
	assert.Equal(t, "every protocol", use.Counts)

	byName := map[string]NetProcess{}
	for _, p := range use.Processes {
		byName[p.Name] = p
	}
	browser := byName["browser"]
	assert.Equal(t, "eth0", browser.Iface)
	assert.InDelta(t, 1500, browser.Rx, 0.001)
	assert.InDelta(t, 150, browser.Tx, 0.001)
	require.Len(t, browser.Peers, 1)
	assert.Len(t, browser.Peers[0].Sockets, 2, "UDP and TCP to one peer are two sockets under it")
	assert.False(t, browser.Tunnel)
	assert.True(t, byName["tailscaled"].Tunnel)

	other := byName[""]
	assert.Equal(t, 0, other.PID, "a process with no name is counted under no process")
	require.Len(t, other.Peers, 1)
	assert.True(t, other.Peers[0].Served, "a near end on a listening port is served")

	assert.ElementsMatch(t, []NetForward{
		{Iface: "eth0", Via: "vnet0", Rx: 2000, Tx: 300},
		{Iface: "vnet0", Via: "eth0", Rx: 300, Tx: 2000},
	}, use.Forwarded)

	first := flowUse(t.Context(), samples, ifaces, nil, f, 0)
	assert.False(t, first.HasRate)
	assert.Empty(t, first.Processes)
}

// The helper is asked first. Only its absence sends the chain on; a helper
// that fails is the answer, so the two readers never both count.
func TestTheChainAsksTheHelperFirst(t *testing.T) {
	own := &fakeNet{use: NetUse{Counts: "TCP only"}}
	c := &chainNet{helper: &fakeNet{err: errNoNetHelper}, own: own}
	use, err := c.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "TCP only", use.Counts)
	assert.Equal(t, 1, own.reads)

	own = &fakeNet{use: NetUse{Counts: "TCP only"}}
	c = &chainNet{helper: &fakeNet{err: errors.New("the kernel refused the counter's programs")}, own: own}
	_, err = c.Read(t.Context())
	require.Error(t, err)
	assert.Equal(t, 0, own.reads, "a helper's failure is not followed by the panel's own reading")

	own = &fakeNet{}
	c = &chainNet{helper: &fakeNet{use: NetUse{Counts: "every protocol"}}, own: own}
	use, err = c.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "every protocol", use.Counts)
	assert.Equal(t, 0, own.reads)
}

type fakeNet struct {
	use   NetUse
	err   error
	reads int
}

func (f *fakeNet) Read(context.Context) (NetUse, error) {
	f.reads++
	return f.use, f.err
}

func (f *fakeNet) Close() {}

// GHSA-94c3-mf55-vv8v. A stream keeps each version's fields and no others:
// what a version does not use is never taken, and a server that sends it
// anyway would otherwise grow the panel for as long as the pipe is open.
func TestAStreamKeepsOnlyItsOwnVersionsFields(t *testing.T) {
	flow := NetHelperFlow{PID: 7, Proto: 17, Local: "192.0.2.1", Peer: "198.51.100.9", Rx: 1, Tx: 1}
	foreign := NetHelperLine{
		P: [][3]uint64{{7, 10, 1}},
		F: []NetHelperFlow{flow}, H: []NetHelperHop{{In: "a", Out: "b", Bytes: 1}}, T: []uint32{3},
	}

	v1 := &helperStream{}
	require.NoError(t, feed(t, v1, helperHello, foreign))
	assert.Len(t, v1.counts, 1)
	assert.Empty(t, v1.flows.flows, "a version 1 stream keeps no flows")
	assert.Empty(t, v1.flows.hops)
	assert.Empty(t, v1.flows.tunnels)

	v2 := &helperStream{want: NetHelperFlowsProtocol}
	require.NoError(t, feed(t, v2, flowHello, foreign))
	assert.Empty(t, v2.counts, "a version 2 stream keeps no counts by process")
	assert.Len(t, v2.flows.flows, 1)
}

// What is kept between two readings is bounded by the helper's own table
// sizes, and text longer than an address or an interface name is not kept.
func TestWhatIsKeptBetweenReadingsIsBounded(t *testing.T) {
	many := NetHelperLine{}
	for pid := range maxHelperEntries + 100 {
		many.P = append(many.P, [3]uint64{uint64(pid), 1, 1})
	}
	v1 := &helperStream{}
	require.NoError(t, feed(t, v1, helperHello, many))
	assert.Len(t, v1.counts, maxHelperEntries)

	flows := NetHelperLine{}
	for port := range maxHelperEntries + 100 {
		flows.F = append(flows.F, NetHelperFlow{PID: 1, Local: "192.0.2.1", LocalPort: uint16(port % 65536), //nolint:gosec // a port
			Peer: "198.51.100.9", PeerPort: uint16(port / 65536), Rx: 1}) //nolint:gosec // a port
	}
	long := strings.Repeat("x", maxHelperText+1)
	flows.F = append(flows.F, NetHelperFlow{PID: 1, Local: long, Peer: "198.51.100.9"})
	for i := range maxHelperHops + 10 {
		flows.H = append(flows.H, NetHelperHop{In: "eth0", Out: fmt.Sprintf("veth%d", i), Bytes: 1})
	}
	flows.H = append(flows.H, NetHelperHop{In: long, Out: "eth0", Bytes: 1})
	for pid := range maxHelperTunnels + 10 {
		flows.T = append(flows.T, uint32(pid)) //nolint:gosec // a process ID
	}
	v2 := &helperStream{want: NetHelperFlowsProtocol}
	require.NoError(t, feed(t, v2, flowHello, flows))
	assert.Len(t, v2.flows.flows, maxHelperEntries)
	assert.True(t, v2.flows.full, "a table at its bound says some bytes were lost")
	assert.Len(t, v2.flows.hops, maxHelperHops)
	assert.Len(t, v2.flows.tunnels, maxHelperTunnels)
	for k := range v2.flows.flows {
		assert.NotEqual(t, long, k.Local)
	}
}

// The helper's account of why it stopped reaches a terminal as words: no
// control character survives, so no escape sequence a terminal would act on.
func TestTheHelpersErrorReachesTheTerminalAsWords(t *testing.T) {
	s := &helperStream{}
	s.run(helperLines(t, helperHello,
		NetHelperLine{Error: "boom\x1b]52;c;aGVsbG8=\x07\x1b[2J\u009b31m done" + strings.Repeat("y", 2*maxHelperError)}))
	_, _, err := s.take()
	require.Error(t, err)
	msg := err.Error()
	for _, r := range msg {
		assert.False(t, unicode.IsControl(r), "control character %U in %q", r, msg)
	}
	assert.Contains(t, msg, "boom]52;c;aGVsbG8=[2J31m done")
	assert.LessOrEqual(t, len(msg), len("the network helper: ")+maxHelperError+utf8.UTFMax)
}

// feed runs the stream over lines to the pipe's end, so what was kept can be
// looked at: an ended stream's tables are left as they were.
func feed(t *testing.T, s *helperStream, lines ...NetHelperLine) error {
	t.Helper()
	r, w := io.Pipe()
	go s.run(r)
	enc := json.NewEncoder(w)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return err
		}
	}
	_ = w.Close()
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.done
	}, 5*time.Second, time.Millisecond)
	return nil
}
