package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// newNetReader is Linux's network by process: every TCP socket's counters
// from sock_diag, each put to the process that holds it.
func newNetReader(samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return &socketNet{
		sockets:   tcpSockets,
		owners:    ProcSocketOwners(ProcPath),
		samples:   samples,
		counts:    "TCP only",
		now:       time.Now,
		ifaces:    localIfaces,
		listening: listeningPorts(),
	}
}

// The sock_diag request and reply (linux/sock_diag.h, linux/inet_diag.h).
const (
	sockDiagByFamily = 20
	inetDiagInfo     = 2

	// inetDiagReqLen is struct inet_diag_req_v2: four bytes, the states, and
	// a 48-byte socket ID.
	inetDiagReqLen = 56
	// inetDiagMsgLen is struct inet_diag_msg, which every reply starts with.
	inetDiagMsgLen = 72

	// Offsets into inet_diag_msg: the address family, the source and
	// destination addresses and the cookie inside its socket ID, and the
	// inode.
	msgFamily = 0
	msgSport  = 4
	msgDport  = 6
	msgSrc    = 8
	msgDst    = 24
	msgCookie = 44
	msgInode  = 68

	// Offsets into struct tcp_info of the two counters read.
	tcpInfoBytesAcked    = 120
	tcpInfoBytesReceived = 128

	// tcpListen is the state of a socket that carries nothing itself.
	tcpListen = 10
)

/*
tcpSockets asks the kernel for every TCP socket and its counters, over
NETLINK_SOCK_DIAG, which needs no privilege: it is what `ss -ti` does.

Each reply is the socket's ID and a tcp_info, of which bytes_acked and
bytes_received are read. The addresses are looked at for one thing, whether
the connection is to this machine itself, and are not kept: a loopback
connection is not traffic on an interface, and counting it would put every
local proxy at the head of the list.

UDP has no counters here, which is why the list says "TCP only".
*/
func tcpSockets(ctx context.Context) ([]SocketCount, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, fmt.Errorf("opening sock_diag: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 2}); err != nil {
		return nil, fmt.Errorf("sock_diag: %w", err)
	}

	var out []SocketCount
	for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		if err := ctx.Err(); err != nil {
			return nil, err //nolint:wrapcheck // the context's own message is the whole story
		}
		found, err := dumpTCP(fd, family)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

// dumpTCP is one family's sockets.
func dumpTCP(fd int, family uint8) ([]SocketCount, error) {
	req := make([]byte, unix.SizeofNlMsghdr+inetDiagReqLen)
	binary.NativeEndian.PutUint32(req[0:], uint32(len(req))) //nolint:gosec // a constant length
	binary.NativeEndian.PutUint16(req[4:], sockDiagByFamily)
	binary.NativeEndian.PutUint16(req[6:], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:], 1)
	body := req[unix.SizeofNlMsghdr:]
	body[0] = family
	body[1] = unix.IPPROTO_TCP
	body[2] = 1 << (inetDiagInfo - 1)
	// Every state but listening.
	binary.NativeEndian.PutUint32(body[4:], ^uint32(0)&^(1<<tcpListen))

	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("asking sock_diag: %w", err)
	}

	var out []SocketCount
	buf := make([]byte, 1<<16)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return nil, fmt.Errorf("reading sock_diag: %w", err)
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("reading sock_diag: %w", err)
		}
		for _, m := range msgs {
			switch m.Header.Type {
			case unix.NLMSG_DONE:
				return out, nil
			case unix.NLMSG_ERROR:
				return nil, fmt.Errorf("sock_diag refused the request for family %d", family)
			}
			if c, ok := parseInetDiag(m.Data); ok {
				out = append(out, c)
			}
		}
	}
}

// parseInetDiag reads one reply, and reports false for a socket that is
// loopback or carries no tcp_info.
func parseInetDiag(data []byte) (SocketCount, bool) {
	if len(data) < inetDiagMsgLen {
		return SocketCount{}, false
	}
	if loopback(data[msgFamily], data[msgSrc:msgSrc+16]) || loopback(data[msgFamily], data[msgDst:msgDst+16]) {
		return SocketCount{}, false
	}
	c := SocketCount{
		Inode:     binary.NativeEndian.Uint32(data[msgInode:]),
		Cookie:    binary.NativeEndian.Uint64(data[msgCookie:]),
		Local:     addrText(data[msgFamily], data[msgSrc:msgSrc+16]),
		Peer:      addrText(data[msgFamily], data[msgDst:msgDst+16]),
		LocalPort: binary.BigEndian.Uint16(data[msgSport:]),
		PeerPort:  binary.BigEndian.Uint16(data[msgDport:]),
	}
	// The attributes follow, each a length, a type and a payload, padded to
	// four bytes.
	for rest := data[inetDiagMsgLen:]; len(rest) >= 4; {
		length := int(binary.NativeEndian.Uint16(rest[0:]))
		kind := binary.NativeEndian.Uint16(rest[2:])
		if length < 4 || length > len(rest) {
			break
		}
		if kind == inetDiagInfo {
			info := rest[4:length]
			if len(info) < tcpInfoBytesReceived+8 {
				return SocketCount{}, false // a kernel older than the counters
			}
			c.Tx = binary.NativeEndian.Uint64(info[tcpInfoBytesAcked:])
			c.Rx = binary.NativeEndian.Uint64(info[tcpInfoBytesReceived:])
			return c, true
		}
		rest = rest[min((length+3)&^3, len(rest)):]
	}
	return SocketCount{}, false
}

// addrText is a socket ID address as text, v4 or v6. A v4-mapped v6 address
// is written as the v4 it is, so one peer is one name however it arrived.
func addrText(family uint8, addr []byte) string {
	if family == unix.AF_INET {
		a, _ := netip.AddrFromSlice(addr[:4])
		return a.String()
	}
	var b [16]byte
	copy(b[:], addr[:16])
	a := netip.AddrFrom16(b)
	if a.Is4In6() {
		a = a.Unmap()
	}
	return a.String()
}

// loopback reports whether an address from a socket ID is this machine's
// own: 127.0.0.0/8, ::1, or the first written as an IPv6 address.
func loopback(family uint8, addr []byte) bool {
	if family == unix.AF_INET {
		return addr[0] == 127
	}
	for _, b := range addr[:10] {
		if b != 0 {
			return false
		}
	}
	if addr[10] == 0xff && addr[11] == 0xff {
		return addr[12] == 127
	}
	return addr[10] == 0 && addr[11] == 0 && addr[12] == 0 && addr[13] == 0 && addr[14] == 0 && addr[15] == 1
}
