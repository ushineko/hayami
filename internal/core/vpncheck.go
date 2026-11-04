package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// VPNCheck is one check of a tunnel's health (spec 085 R10): what kind, and
// what it is aimed at. Target is the host to ping or the name to look up; Prefix
// is what a looked-up address must start with.
type VPNCheck struct {
	Type    string
	Target  string
	Prefix  string
	Timeout time.Duration
}

// VPNCheckDefaultTimeout is how long a check is given when its settings name no
// limit: vpn-toggle's five seconds for a ping.
const VPNCheckDefaultTimeout = 5 * time.Second

// ErrVPNCheckDidNotRun is a check that could not be attempted, which says
// nothing about the tunnel: the system refused a socket, or the check's settings
// cannot be acted on. A timeout is not this. A timeout is a failure, because it
// is what an unresponsive tunnel looks like.
var ErrVPNCheckDidNotRun = errors.New("the check could not run")

// VPNCheckers are the checks this platform can make, by type: a table on the
// host, so a check type is a row and not a case (docs/architecture.md rule 2).
type VPNCheckers map[string]func(context.Context, VPNCheck) error

// VPNCheckResult is one check as it came out.
type VPNCheckResult struct {
	Type      string  `json:"type"`
	LatencyMS float64 `json:"latency_ms"`
	OK        bool    `json:"ok"`

	// DidNotRun marks a check that said nothing about the tunnel.
	DidNotRun bool `json:"did_not_run,omitempty"`
}

// Run makes one check and times it.
func (c VPNCheckers) Run(ctx context.Context, check VPNCheck) VPNCheckResult {
	result := VPNCheckResult{Type: check.Type}
	run, ok := c[check.Type]
	if !ok {
		result.DidNotRun = true
		return result
	}
	timeout := check.Timeout
	if timeout <= 0 {
		timeout = VPNCheckDefaultTimeout
	}
	limited, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	err := run(limited, check)
	result.LatencyMS = float64(time.Since(start)) / float64(time.Millisecond)
	result.OK = err == nil
	result.DidNotRun = errors.Is(err, ErrVPNCheckDidNotRun)
	return result
}

// MatchesPrefix reports whether any address starts with the expected prefix, as
// vpn-toggle's DNS check does: "100." matches "100.64.1.5".
func MatchesPrefix(addresses []string, prefix string) bool {
	for _, a := range addresses {
		if strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

// TunnelAddressing is what the drawer says of a tunnel's addresses. It is read
// only while the drawer is shown, and goes on no card (spec 085 R9).
type TunnelAddressing struct {
	Addresses []string `json:"addresses,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	Routes    []string `json:"routes,omitempty"`
}

// ReadTunnelAddressing reads an interface's addresses and IPv4 routes.
func ReadTunnelAddressing(iface string) (TunnelAddressing, error) {
	var out TunnelAddressing
	nic, err := net.InterfaceByName(iface)
	if err != nil {
		return out, fmt.Errorf("finding %s: %w", iface, err)
	}
	addrs, err := nic.Addrs()
	if err != nil {
		return out, fmt.Errorf("reading %s's addresses: %w", iface, err)
	}
	for _, a := range addrs {
		// A link-local address is every interface's, and says nothing of the
		// tunnel.
		if ip, ok := a.(*net.IPNet); ok && ip.IP.IsLinkLocalUnicast() {
			continue
		}
		out.Addresses = append(out.Addresses, a.String())
	}
	f, err := os.Open(routeTablePath)
	if err != nil {
		// The addresses are still worth saying.
		return out, nil //nolint:nilerr // a table this machine does not have is no routes
	}
	defer func() { _ = f.Close() }()
	out.Gateway, out.Routes = ParseRouteTable(io.LimitReader(f, routeTableMax), iface)
	return out, nil
}

const (
	routeTablePath = "/proc/net/route"
	routeTableMax  = 1 << 20
)

// routeUp and routeGateway are RTF_UP and RTF_GATEWAY from the table's flags.
const (
	routeUp      = 0x1
	routeGateway = 0x2
)

/*
ParseRouteTable reads /proc/net/route for one interface: its gateway, where a
route names one, and its routes as prefixes.

The kernel writes each address as eight hexadecimal digits in the machine's byte
order, which /proc's own table assumes to be little-endian on everything hayami
is built for. A line that does not parse is skipped.
*/
func ParseRouteTable(r io.Reader, iface string) (gateway string, routes []string) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 8 || f[0] != iface {
			continue
		}
		dest, ok := routeAddress(f[1])
		mask, okMask := routeAddress(f[7])
		via, okVia := routeAddress(f[2])
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if !ok || !okMask || !okVia || err != nil || flags&routeUp == 0 {
			continue
		}
		if flags&routeGateway != 0 && gateway == "" {
			gateway = via.String()
		}
		bits, _ := net.IPMask(mask.To4()).Size()
		routes = append(routes, fmt.Sprintf("%s/%d", dest, bits))
	}
	// A table cut short still says what it said up to there.
	_ = sc.Err()
	return gateway, routes
}

func routeAddress(s string) (net.IP, bool) {
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 4 {
		return nil, false
	}
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, binary.LittleEndian.Uint32(raw))
	return ip, true
}
