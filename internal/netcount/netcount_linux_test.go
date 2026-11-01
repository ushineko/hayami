package netcount

import (
	"io"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The programs on the real kernel (spec 072): bytes sent between two sockets of
this test are counted to this test's process, by both ends, with the IP
header included. This is the test that the counter counts at all: every
other test of the helper fakes it.

It needs root (CAP_BPF, CAP_NET_ADMIN, CAP_PERFMON) and is skipped without:

	go test -c -o /tmp/netcount.test ./internal/netcount
	sudo /tmp/netcount.test -test.run TheKernel -test.v

Loopback is not counted, so the two sockets talk over this machine's first
address that is not.
*/
func TestTheKernelCountsThisProcesssUDPAndTCP(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("loading the counter needs root")
	}
	addr := localAddress(t)
	c, err := Open()
	require.NoError(t, err)
	t.Cleanup(c.Close)
	pid := uint32(os.Getpid()) //nolint:gosec // a process ID

	// UDP: ten datagrams of a thousand bytes, each 1028 on the wire (IPv4
	// 20, UDP 8).
	rx, err := net.ListenUDP("udp4", &net.UDPAddr{IP: addr.AsSlice()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rx.Close() })
	tx, err := net.DialUDP("udp4", nil, rx.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Close() })
	buf := make([]byte, 1000)
	for range 10 {
		_, err := tx.Write(buf)
		require.NoError(t, err)
		require.NoError(t, rx.SetReadDeadline(time.Now().Add(time.Second)))
		_, _, err = rx.ReadFromUDP(make([]byte, 2000))
		require.NoError(t, err)
	}

	// TCP: a hundred thousand bytes, and the headers on top.
	l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: addr.AsSlice()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	got := make(chan int64, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			got <- -1
			return
		}
		n, _ := io.Copy(io.Discard, conn)
		_ = conn.Close()
		got <- n
	}()
	conn, err := net.DialTCP("tcp4", nil, l.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	_, err = conn.Write(make([]byte, 100_000))
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.Equal(t, int64(100_000), <-got)

	snap, err := c.Read()
	require.NoError(t, err)
	udpPort := uint16(tx.LocalAddr().(*net.UDPAddr).Port) //nolint:gosec // a port
	rxPort := uint16(rx.LocalAddr().(*net.UDPAddr).Port)  //nolint:gosec // a port
	sent := snap.Flows[Flow{PID: pid, Proto: 17, Local: addr, LocalPort: udpPort, Peer: addr, PeerPort: rxPort}]
	received := snap.Flows[Flow{PID: pid, Proto: 17, Local: addr, LocalPort: rxPort, Peer: addr, PeerPort: udpPort}]
	assert.Equal(t, uint64(10*1028), sent.Tx, "UDP sent, by this process")
	assert.Equal(t, uint64(10*1028), received.Rx, "UDP received, by this process")

	tcpPort := uint16(l.Addr().(*net.TCPAddr).Port) //nolint:gosec // a port
	var tcpOut uint64
	for f, b := range snap.Flows {
		if f.PID == pid && f.Proto == 6 && f.PeerPort == tcpPort {
			tcpOut += b.Tx
		}
	}
	assert.GreaterOrEqual(t, tcpOut, uint64(100_000), "TCP sent: every byte, and its headers")
	assert.Less(t, tcpOut, uint64(110_000), "and not twice")
}

// localAddress is this machine's first IPv4 address that is not loopback.
func localAddress(t *testing.T) netip.Addr {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	require.NoError(t, err)
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(n.IP.To4()); ok && n.IP.To4() != nil && !ip.IsLoopback() {
				return ip
			}
		}
	}
	t.Skip("no address but loopback")
	return netip.Addr{}
}
