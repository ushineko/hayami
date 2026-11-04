package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	resolvedName = "org.freedesktop.resolve1"
	resolvedPath = "/org/freedesktop/resolve1"
)

// linuxVPNCheckers are the checks this platform makes, none by running a
// program (spec 085 R2).
func linuxVPNCheckers() VPNCheckers {
	return VPNCheckers{"ping": pingCheck, "dns": dnsCheck}
}

/*
lookup finds a name the way the system would, as far as a program built without
cgo can.

**systemd-resolved first, over the bus.** vpn-toggle moved to `getent` so that
lookups pass through nsswitch, and a tunnel's DNS is often only known to
resolved, per link. Go's resolver reads /etc/resolv.conf, and on the desk that
lists the LAN's servers and not resolved's stub, so it would never ask the
tunnel's. Asking resolved gives getent's answer with no subprocess. Go's own
resolver is the fallback for a machine that does not run it.

**Past the cache** (resolvedNoCache). The check exists to find a tunnel that is on
and not answering, and a cached answer passes on a dead tunnel until the record
expires. This is a decided departure from vpn-toggle (spec 085 R10).
*/
func lookup(ctx context.Context, name string) ([]string, error) {
	addresses, err := resolvedLookup(ctx, systemBus{}, name)
	if errors.Is(err, ErrVPNBackendAbsent) {
		var r net.Resolver
		r.PreferGo = true
		addresses, err = r.LookupHost(ctx, name)
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // a failed lookup is the check's whole account
	}
	return addresses, nil
}

// resolvedNoCache is SD_RESOLVED_NO_CACHE: ask the network, not the cache.
const resolvedNoCache = uint64(1) << 12

// resolvedAddress is one answer from ResolveHostname.
type resolvedAddress struct {
	IfIndex int32
	Family  int32
	Address []byte
}

// resolvedLookup asks systemd-resolved, and reports it absent where it is not
// running, without starting it.
func resolvedLookup(ctx context.Context, bus vpnBus, name string) ([]string, error) {
	has, err := bus.hasOwner(ctx, resolvedName)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrVPNBackendAbsent
	}
	var (
		answers   []resolvedAddress
		canonical string
		flags     uint64
	)
	err = bus.callArgs(ctx, resolvedName, resolvedPath, resolvedName+".Manager.ResolveHostname",
		[]any{int32(0), name, int32(syscall.AF_UNSPEC), resolvedNoCache}, &answers, &canonical, &flags)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(answers))
	for _, a := range answers {
		out = append(out, net.IP(a.Address).String())
	}
	return out, nil
}

// dnsCheck looks the target up and requires an address with the expected prefix.
func dnsCheck(ctx context.Context, c VPNCheck) error {
	if c.Target == "" || c.Prefix == "" {
		return fmt.Errorf("%w: a DNS check needs a name and a prefix", ErrVPNCheckDidNotRun)
	}
	addresses, err := lookup(ctx, c.Target)
	if err != nil {
		return err
	}
	if !MatchesPrefix(addresses, c.Prefix) {
		return errors.New("no address with the expected prefix")
	}
	return nil
}

// icmpFamily is what differs between pinging over IPv4 and over IPv6.
type icmpFamily struct {
	network, listen string
	protocol        int
	request, reply  icmp.Type
}

var (
	icmpV4 = icmpFamily{"udp4", "0.0.0.0", 1, ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply}
	icmpV6 = icmpFamily{"udp6", "::", 58, ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply}
)

/*
pingCheck sends one echo request, as vpn-toggle's `ping -c 1` does, over an
unprivileged ICMP datagram socket.

A kernel configured not to allow those (`net.ipv4.ping_group_range`) refuses the
socket. That is the check not running, and says nothing of the tunnel: there is
no fallback to the `ping` program, which is the point of doing this natively.
*/
func pingCheck(ctx context.Context, c VPNCheck) error {
	if c.Target == "" {
		return fmt.Errorf("%w: a ping check needs a host", ErrVPNCheckDidNotRun)
	}
	host := c.Target
	if net.ParseIP(host) == nil {
		addresses, err := lookup(ctx, host)
		if err != nil {
			return err
		}
		if len(addresses) == 0 {
			return errors.New("the name has no address")
		}
		host = addresses[0]
	}
	ip := net.ParseIP(host)
	family := icmpV4
	if ip.To4() == nil {
		family = icmpV6
	}

	conn, err := icmp.ListenPacket(family.network, family.listen)
	if err != nil {
		if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) ||
			errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EAFNOSUPPORT) {
			return fmt.Errorf("%w: %w", ErrVPNCheckDidNotRun, err)
		}
		return fmt.Errorf("opening an ICMP socket: %w", err)
	}
	defer func() { _ = conn.Close() }()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(VPNCheckDefaultTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("setting the ICMP deadline: %w", err)
	}
	// The poll being cancelled closes the socket, so a read waiting on a tunnel
	// that says nothing stops with it.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	seq := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: family.request,
		Body: &icmp.Echo{ID: seq, Seq: seq, Data: []byte("hayami")},
	}
	raw, err := msg.Marshal(nil)
	if err != nil {
		return fmt.Errorf("building the echo request: %w", err)
	}
	to := &net.UDPAddr{IP: ip}
	if _, err := conn.WriteTo(raw, to); err != nil {
		return fmt.Errorf("sending the echo request: %w", err)
	}

	buf := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			return fmt.Errorf("waiting for the echo reply: %w", err)
		}
		reply, err := icmp.ParseMessage(family.protocol, buf[:n])
		if err != nil || reply.Type != family.reply {
			continue
		}
		echo, ok := reply.Body.(*icmp.Echo)
		if ok && echo.Seq == seq && strings.EqualFold(hostOf(from), ip.String()) {
			return nil
		}
	}
}

func hostOf(a net.Addr) string {
	if u, ok := a.(*net.UDPAddr); ok {
		return u.IP.String()
	}
	return a.String()
}
