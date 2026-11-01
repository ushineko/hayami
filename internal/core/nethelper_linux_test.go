package core

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Spec 072. The panel reads a real socket: the first reading connects and
// has no rate, and the next is the flows the helper sent in between, by
// name.
func TestThePanelReadsTheHelpersSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper.sock")
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	sent, done := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		enc := json.NewEncoder(conn)
		_ = enc.Encode(flowHello)
		_ = enc.Encode(NetHelperLine{F: []NetHelperFlow{{
			PID: int32ToPID(os.Getpid()), Proto: 17, Local: "192.0.2.1", LocalPort: 5000,
			Peer: "198.51.100.9", PeerPort: 443, Rx: 4000, Tx: 400,
		}}})
		close(sent)
		<-done // the connection stays open until the test is over
	}()

	samples := func(context.Context) ([]ProcessSample, error) {
		return []ProcessSample{{PID: os.Getpid(), Name: "this test"}}, nil
	}
	r := NewFlowHelperReader(path, samples).(*flowNet)
	at := time.Unix(1000, 0)
	r.now = func() time.Time { return at }
	r.ifaces = func() map[string]string { return map[string]string{"192.0.2.1": "eth0"} }
	counters := core0()
	r.counters = func() (map[string]Counters, error) { return counters, nil }
	t.Cleanup(r.Close)

	first, err := r.Read(t.Context())
	require.NoError(t, err)
	assert.False(t, first.HasRate)
	assert.Equal(t, "every protocol", first.Counts)

	<-sent
	require.Eventually(t, func() bool {
		r.stream.mu.Lock()
		defer r.stream.mu.Unlock()
		return len(r.stream.flows.flows) > 0
	}, time.Second, time.Millisecond)
	at = at.Add(2 * time.Second)
	counters = map[string]Counters{"eth0": {Rx: 10_000, Tx: 1_000}}
	use, err := r.Read(t.Context())
	require.NoError(t, err)
	require.Len(t, use.Processes, 1)
	assert.Equal(t, "this test", use.Processes[0].Name)
	assert.Equal(t, "eth0", use.Processes[0].Iface)
	assert.InDelta(t, 2000, use.Processes[0].Rx, 0.001)
	assert.Equal(t, map[string][2]float64{"eth0": {5000, 500}}, use.Interfaces,
		"the interfaces over the same two seconds as the flows")
}

func core0() map[string]Counters { return map[string]Counters{"eth0": {}} }

func int32ToPID(pid int) uint32 { return uint32(pid) } //nolint:gosec // a process ID

// No socket is no helper, which sends the chain on. A socket this account
// may not open, and one nobody listens on, are said: the list without the
// helper is a smaller one and the person would not know why.
func TestTheHelpersSocketSaysWhyItIsNotRead(t *testing.T) {
	dir := t.TempDir()
	_, err := NewFlowHelperReader(filepath.Join(dir, "none.sock"), nil).Read(t.Context())
	require.ErrorIs(t, err, errNoNetHelper)

	// Bound and not listening: what a socket unit leaves when it is stopped
	// without removing its file.
	refused := filepath.Join(dir, "refused.sock")
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })
	require.NoError(t, unix.Bind(fd, &unix.SockaddrUnix{Name: refused}))
	_, err = NewFlowHelperReader(refused, nil).Read(t.Context())
	var absent *Absence
	require.ErrorAs(t, err, &absent)
	assert.Equal(t, "helper is stopped", absent.Text)

	if os.Geteuid() == 0 {
		t.Skip("root opens any socket")
	}
	theirs := filepath.Join(dir, "theirs.sock")
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", theirs)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	require.NoError(t, os.Chmod(theirs, 0))
	_, err = NewFlowHelperReader(theirs, nil).Read(t.Context())
	require.ErrorAs(t, err, &absent)
	assert.Equal(t, "helper is another's", absent.Text)
}
