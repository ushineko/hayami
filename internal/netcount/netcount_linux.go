package netcount

import (
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
)

// cgroupRoot is the cgroup v2 hierarchy's root, where a program sees every
// process on the machine.
const cgroupRoot = "/sys/fs/cgroup"

// forwardPriority is where on the forward hook the counter sits. The kernel
// refuses a program a priority a firewall's chain already holds ("device or
// resource busy" at 0, where nftables' filter chains are); this one is
// early, ahead of the filters, so it counts what reaches the hook whether
// or not a rule then drops it.
const forwardPriority = -1000

// fullAt is the share of a table's entries past which Snapshot.Full is set.
const fullAt = 0.9

// Counter is the programs, loaded and attached. Close detaches them; nothing
// is pinned, so a helper that dies leaves nothing behind.
type Counter struct {
	objs  netcountObjects
	links []link.Link
	names map[uint32]string

	// Missing is what could not be attached and what the counts lack for
	// it, in a few words, or "".
	Missing string
}

/*
Open loads the programs and attaches them. The socket counters and the
forward hook are the reading and their failure is Open's; the hooks that
find a socket's user after it was made, and the one that finds tunnel
daemons, only sharpen it, and one that will not attach is said in Missing.
*/
func Open() (*Counter, error) {
	_ = rlimit.RemoveMemlock() // kernels before 5.11 charge maps to it

	spec, err := loadNetcount()
	if err != nil {
		return nil, fmt.Errorf("reading the counter's programs: %w", err)
	}
	host, err := netnsCookie()
	if err != nil {
		return nil, err
	}
	if err := spec.Variables[netcountVarHostNetns].Set(host); err != nil {
		return nil, fmt.Errorf("setting the counter's namespace: %w", err)
	}
	c := &Counter{names: map[uint32]string{}}
	if err := spec.LoadAndAssign(&c.objs, nil); err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			// %+v is the verifier's whole log, which is the only account of
			// why; the error itself is in it.
			return nil, fmt.Errorf("the kernel refused the counter's programs: %+v", ve) //nolint:errorlint // the log, not the chain
		}
		return nil, fmt.Errorf("loading the counter's programs: %w", err)
	}

	fail := func(err error) (*Counter, error) {
		c.Close()
		return nil, err
	}
	for _, a := range []struct {
		prog *ebpf.Program
		at   ebpf.AttachType
	}{
		{c.objs.SockCreate, ebpf.AttachCGroupInetSockCreate},
		{c.objs.SkbIn, ebpf.AttachCGroupInetIngress},
		{c.objs.SkbOut, ebpf.AttachCGroupInetEgress},
	} {
		l, err := link.AttachCgroup(link.CgroupOptions{Path: cgroupRoot, Attach: a.at, Program: a.prog})
		if err != nil {
			return fail(fmt.Errorf("attaching to %s: %w", cgroupRoot, err))
		}
		c.links = append(c.links, l)
	}
	for _, family := range []link.NetfilterProtocolFamily{link.NetfilterProtoIPv4, link.NetfilterProtoIPv6} {
		l, err := link.AttachNetfilter(link.NetfilterOptions{
			Program: c.objs.Forward, ProtocolFamily: family, Hook: link.NetfilterInetForward,
			Priority: forwardPriority,
		})
		if err != nil {
			return fail(fmt.Errorf("attaching to the forward hook (Linux 6.4 or later): %w", err))
		}
		c.links = append(c.links, l)
	}

	for _, p := range []*ebpf.Program{
		c.objs.OwnTcpSendmsg, c.objs.OwnTcpRecvmsg, c.objs.OwnUdpSendmsg,
		c.objs.OwnUdpRecvmsg, c.objs.OwnUdpv6Sendmsg, c.objs.OwnUdpv6Recvmsg,
	} {
		l, err := link.AttachTracing(link.TracingOptions{Program: p})
		if err != nil {
			// Kernel lockdown refuses tracing; a socket is then counted to
			// the process that made it.
			c.Missing = "a socket is counted to the process that made it, not the one using it"
			continue
		}
		c.links = append(c.links, l)
	}
	if l, err := link.Kprobe("tun_chr_write_iter", c.objs.Tunnel, nil); err == nil {
		c.links = append(c.links, l)
	}
	return c, nil
}

// Read is the counters now.
func (c *Counter) Read() (Snapshot, error) {
	s := Snapshot{Flows: map[Flow]Bytes{}, Forwarded: map[Hop]uint64{}}

	var f netcountFlow
	var b netcountBytes
	n := 0
	it := c.objs.Flows.Iterate()
	for it.Next(&f, &b) {
		n++
		s.Flows[flowOf(f)] = Bytes{Rx: b.Rx, Tx: b.Tx}
	}
	if err := it.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("reading the counter's flows: %w", err)
	}
	s.Full = float64(n) > fullAt*float64(c.objs.Flows.MaxEntries())

	var h netcountHop
	var bytes uint64
	hops := c.objs.Forwarded.Iterate()
	for hops.Next(&h, &bytes) {
		s.Forwarded[Hop{In: c.name(h.In), Out: c.name(h.Out)}] += bytes
	}
	if err := hops.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("reading the counter's forwarded bytes: %w", err)
	}

	var pid uint32
	var one uint8
	tunnels := c.objs.Tunnels.Iterate()
	for tunnels.Next(&pid, &one) {
		s.Tunnels = append(s.Tunnels, pid)
	}
	return s, nil
}

// Close detaches the programs and lets the tables go.
func (c *Counter) Close() {
	for _, l := range c.links {
		_ = l.Close()
	}
	c.links = nil
	_ = c.objs.Close()
}

// flowOf is a table's key as a Flow.
func flowOf(f netcountFlow) Flow {
	out := Flow{PID: f.Tgid, Proto: f.Proto, LocalPort: f.Lport, PeerPort: f.Rport}
	if f.Family == 4 {
		out.Local = netip.AddrFrom4([4]byte(f.Laddr[:4]))
		out.Peer = netip.AddrFrom4([4]byte(f.Raddr[:4]))
	} else {
		out.Local = netip.AddrFrom16(f.Laddr).Unmap()
		out.Peer = netip.AddrFrom16(f.Raddr).Unmap()
	}
	return out
}

// name is an interface's name by its index, remembered: an interface that
// has gone since keeps the name it had.
func (c *Counter) name(index uint32) string {
	if index == 0 {
		return ""
	}
	if n, ok := c.names[index]; ok {
		return n
	}
	ifc, err := net.InterfaceByIndex(int(index))
	if err != nil {
		return fmt.Sprintf("if%d", index)
	}
	c.names[index] = ifc.Name
	return ifc.Name
}

// netnsCookie is the cookie of this process's network namespace, the one
// the kernel gives a socket in it (SO_NETNS_COOKIE, Linux 5.14).
func netnsCookie() (uint64, error) {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return 0, fmt.Errorf("asking for this namespace: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	cookie, err := unix.GetsockoptUint64(fd, unix.SOL_SOCKET, unix.SO_NETNS_COOKIE)
	if err != nil {
		return 0, fmt.Errorf("asking for this namespace's cookie: %w", err)
	}
	return cookie, nil
}
