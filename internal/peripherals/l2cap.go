package peripherals

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// AAPTimeout bounds the whole exchange.
//
// A device that connects and never reports must not take the poll loop with
// it, the same rule liquidctl and the app-server are held to. The real
// exchange finishes well inside a second.
const AAPTimeout = 6 * time.Second

// channel is an open L2CAP conversation, or a test's stand-in for one.
type channel interface {
	Send([]byte) error
	Receive(time.Duration) ([]byte, error)
	Close() error
}

// errNoPacket is the channel having nothing to say before its deadline.
var errNoPacket = errors.New("no packet")

// l2cap is a real channel to a real device.
type l2cap struct{ fd int }

/*
dial opens an L2CAP channel to a device's accessory protocol port.

Two things here are not obvious and both of them read as the device refusing
the connection when they are wrong:

**The address goes in written order.** [unix.SockaddrL2] reverses it on the way
to the kernel, so the six bytes here are the six bytes of the printed address,
left to right. Reversing them first is what a raw sockaddr_l2 wants and what
every C example does, and doing it dials an address nothing answers on — for
which the kernel's answer is ECONNREFUSED. That is indistinguishable, from the
outside, from AirPods declining the channel.

**The connect completes asynchronously.** It reports EINPROGRESS and the real
answer arrives later, in SO_ERROR, once the socket is writable. Treating
EINPROGRESS as the failure reports a working device as broken.
*/
func dial(addr [6]byte, timeout time.Duration) (channel, error) {
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_SEQPACKET, unix.BTPROTO_L2CAP)
	if err != nil {
		return nil, fmt.Errorf("opening an l2cap socket: %w", err)
	}

	err = unix.Connect(fd, &unix.SockaddrL2{PSM: AAPPSM, Addr: addr})
	if errors.Is(err, unix.EINPROGRESS) {
		err = awaitConnect(fd, timeout)
	}
	if err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("connecting to the accessory protocol: %w", err)
	}
	return &l2cap{fd: fd}, nil
}

// awaitConnect waits for an asynchronous connect and reports how it went.
func awaitConnect(fd int, timeout time.Duration) error {
	n, err := poll(fd, unix.POLLOUT, timeout)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("the connect did not complete within %s", timeout)
	}

	code, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
	if err != nil {
		return fmt.Errorf("asking how the connect went: %w", err)
	}
	if code != 0 {
		return unix.Errno(code)
	}
	return nil
}

// Send writes one packet.
func (c *l2cap) Send(p []byte) error {
	if _, err := unix.Write(c.fd, p); err != nil {
		return fmt.Errorf("sending on the accessory channel: %w", err)
	}
	return nil
}

// Receive waits for one packet.
func (c *l2cap) Receive(within time.Duration) ([]byte, error) {
	if within <= 0 {
		return nil, errNoPacket
	}

	n, err := poll(c.fd, unix.POLLIN, within)
	if err != nil {
		return nil, fmt.Errorf("waiting on the accessory channel: %w", err)
	}
	if n == 0 {
		return nil, errNoPacket
	}

	buf := make([]byte, 1024)
	var read int
	for {
		read, err = unix.Read(c.fd, buf)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading the accessory channel: %w", err)
	}
	return buf[:read], nil
}

/*
poll waits for a socket, retrying the interruptions Go causes itself.

**EINTR is not a failure here and it is not rare.** The Go runtime preempts
goroutines with signals, and a signal delivered while poll(2) is waiting
returns EINTR. Treating that as an error made the accessory protocol fail
against a device that was answering perfectly well, with a message — "waiting
on the accessory channel: interrupted system call" — that says nothing about
what actually happened.

The deadline is recomputed on each retry so the interruptions cannot extend
the wait past what the caller asked for.
*/
func poll(fd int, events int16, within time.Duration) (int, error) {
	if fd < 0 || fd > math.MaxInt32 {
		return 0, fmt.Errorf("%d is not a file descriptor", fd)
	}
	deadline := time.Now().Add(within)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, nil
		}

		// The wait is in milliseconds and is bounded by the caller's own
		// timeout, so it cannot reach the width of the argument.
		ms := left.Milliseconds()
		if ms > int64(maxPollWait) {
			ms = int64(maxPollWait)
		}

		pfd := []unix.PollFd{{Fd: int32(fd), Events: events}} //nolint:gosec // bounded above
		n, err := unix.Poll(pfd, int(ms))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("waiting on an l2cap socket: %w", err)
		}
		return n, nil
	}
}

// maxPollWait caps one wait, so the conversion into poll's argument cannot
// overflow however long a caller asks for.
const maxPollWait = int32(60_000)

// Close ends the conversation.
func (c *l2cap) Close() error {
	if err := unix.Close(c.fd); err != nil {
		return fmt.Errorf("closing the accessory channel: %w", err)
	}
	return nil
}

/*
readAAP runs the exchange and returns the first battery packet's cells.

The device is talked through three steps and then reports on its own. Packets
that are neither an acknowledgement nor a battery are the device's ordinary
chatter — it has a great deal to say about its case, its firmware and its
serial numbers — and are waited through.
*/
func readAAP(c channel, timeout time.Duration) ([]CellReading, error) {
	if err := c.Send(aapHandshake); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	sentFeatures, sentNotifications := false, false

	for {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrNoBatteryPacket
		}

		pkt, err := c.Receive(left)
		if errors.Is(err, errNoPacket) {
			continue
		}
		if err != nil {
			return nil, err
		}

		switch {
		case startsWith(pkt, aapBattery):
			return decodeAAPBattery(pkt)

		case startsWith(pkt, aapHandshakeAck) && !sentFeatures:
			if err := c.Send(aapSetFeatures); err != nil {
				return nil, err
			}
			sentFeatures = true

		case startsWith(pkt, aapFeaturesAck) && !sentNotifications:
			if err := c.Send(aapNotifications); err != nil {
				return nil, err
			}
			sentNotifications = true
		}
	}
}

// startsWith reports whether a packet opens with a prefix.
func startsWith(pkt, prefix []byte) bool {
	return len(pkt) >= len(prefix) && string(pkt[:len(prefix)]) == string(prefix)
}

// parseAddress reads a printed Bluetooth address into the order
// [unix.SockaddrL2] wants, which is the order it is written.
func parseAddress(s string) ([6]byte, error) {
	var out [6]byte
	parts := strings.Split(s, ":")
	if len(parts) != len(out) {
		return out, fmt.Errorf("%q is not a bluetooth address", s)
	}
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return out, fmt.Errorf("%q is not a bluetooth address: %w", s, err)
		}
		out[i] = byte(v)
	}
	return out, nil
}
