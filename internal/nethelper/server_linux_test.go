package nethelper

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/netcount"
)

// fakeCounter counts what the server asks of the programs. Each Read moves
// one flow on by a thousand bytes received and a hundred sent.
type fakeCounter struct {
	mu     sync.Mutex
	opens  int
	closes int
	reads  uint64
	err    error
}

var testFlow = netcount.Flow{
	PID: 7, Proto: 17, Local: netip.MustParseAddr("192.0.2.1"), LocalPort: 5000,
	Peer: netip.MustParseAddr("198.51.100.9"), PeerPort: 443,
}

func (f *fakeCounter) open() (counter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.opens++
	f.reads = 0
	return f, nil
}

func (f *fakeCounter) Read() (netcount.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return netcount.Snapshot{Flows: map[netcount.Flow]netcount.Bytes{
		testFlow: {Rx: 1000 * f.reads, Tx: 100 * f.reads},
	}}, nil
}

func (f *fakeCounter) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
}

func (f *fakeCounter) seen() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens, f.closes
}

var testHello = core.NetHelperLine{V: core.NetHelperFlowsProtocol, Version: "1.2.3", Counts: "every protocol"}

// testServer is a server on a socket of its own, for this account, on a
// short clock. No systemd and no root: the socket is a real one and only the
// programs are faked.
func testServer(t *testing.T, c *fakeCounter, allow uint32) (*server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "helper.sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	return &server{
		listener: l, allow: allow, hello: testHello, open: c.open,
		tick: 40 * time.Millisecond, idle: 200 * time.Millisecond,
	}, path
}

func me() uint32 { return uint32(os.Getuid()) } //nolint:gosec // a user ID

func readLine(t *testing.T, r *bufio.Scanner) core.NetHelperLine {
	t.Helper()
	require.True(t, r.Scan(), "the socket closed: %v", r.Err())
	var line core.NetHelperLine
	require.NoError(t, json.Unmarshal(r.Bytes(), &line))
	return line
}

func connect(t *testing.T, path string) (net.Conn, *bufio.Scanner) {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", path)
	require.NoError(t, err)
	return conn, bufio.NewScanner(conn)
}

// Spec 072. A reader is told who is speaking and then sent what moved, a
// line a tick. The programs are loaded only when somebody reads and
// detached when the last reader leaves, and with nobody reading the helper
// ends by itself.
func TestTheServerCountsOnlyWhileSomebodyReads(t *testing.T) {
	c := &fakeCounter{}
	s, path := testServer(t, c, me())
	// Long enough that letting the programs go is seen to happen when the
	// last reader leaves, and not only when the server ends.
	s.idle = time.Second
	done := make(chan error, 1)
	go func() { done <- s.serve(t.Context()) }()

	time.Sleep(3 * s.tick)
	opens, _ := c.seen()
	assert.Zero(t, opens, "nothing is loaded with nobody reading")

	conn, lines := connect(t, path)
	assert.Equal(t, testHello, readLine(t, lines))
	line := readLine(t, lines)
	require.Len(t, line.F, 1)
	assert.Equal(t, core.NetHelperFlow{
		PID: 7, Proto: 17, Local: "192.0.2.1", LocalPort: 5000, Peer: "198.51.100.9", PeerPort: 443, Rx: 1000, Tx: 100,
	}, line.F[0], "a line is what moved since the one before, not the total")

	// A second reader shares the programs loaded for the first.
	second, secondLines := connect(t, path)
	assert.Equal(t, testHello, readLine(t, secondLines))
	opens, _ = c.seen()
	assert.Equal(t, 1, opens)

	_ = conn.Close()
	_ = second.Close()
	require.Eventually(t, func() bool {
		_, closes := c.seen()
		return closes == 1
	}, s.idle/2, 10*time.Millisecond, "the programs are detached when the last reader leaves")
	select {
	case <-done:
		t.Fatal("the server ended before its idle time")
	default:
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("the server did not end with nobody reading")
	}
}

// A reader that is not the account the helper serves is told so and let
// go, and nothing is loaded for it.
func TestTheServerServesOneAccount(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root is always let in")
	}
	c := &fakeCounter{}
	s, path := testServer(t, c, me()+1)
	go func() { _ = s.serve(t.Context()) }()

	conn, lines := connect(t, path)
	t.Cleanup(func() { _ = conn.Close() })
	assert.Equal(t, "this helper serves another account", readLine(t, lines).Error)
	assert.False(t, lines.Scan(), "and the socket is closed")
	opens, _ := c.seen()
	assert.Zero(t, opens)
}

// Programs that will not load are said to the reader in the helper's words,
// and the helper ends.
func TestAProgramThatWillNotLoadIsSaid(t *testing.T) {
	c := &fakeCounter{err: errors.New("the kernel refused the counter's programs: invalid argument")}
	s, path := testServer(t, c, me())
	done := make(chan error, 1)
	go func() { done <- s.serve(t.Context()) }()

	conn, lines := connect(t, path)
	t.Cleanup(func() { _ = conn.Close() })
	assert.Equal(t, testHello, readLine(t, lines))
	assert.Contains(t, readLine(t, lines).Error, "the kernel refused")
	require.Error(t, <-done)
}
