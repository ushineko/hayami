package core

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

/*
PeerNames puts a name to a peer's address, out of the way of the reading.

A lookup can take as long as a DNS server takes to answer, and a panel that
waited for one would stop drawing. So a miss answers with the address and
asks in the background; the name arrives in a later reading, which for a
list redrawn every couple of seconds is soon enough to look immediate.

An address that has no name is remembered as having none, so a peer without
a PTR record is asked about once and not on every reading.
*/
type PeerNames struct {
	mu      sync.Mutex
	known   map[string]string
	asking  map[string]bool
	lookup  func(context.Context, string) ([]string, error)
	timeout time.Duration
}

// NewPeerNames builds a resolver with an empty cache.
func NewPeerNames() *PeerNames {
	return &PeerNames{
		known:   map[string]string{},
		asking:  map[string]bool{},
		lookup:  net.DefaultResolver.LookupAddr,
		timeout: 2 * time.Second,
	}
}

// Name is the address's name if one is known, and the address itself if not.
// It never blocks.
func (n *PeerNames) Name(addr string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if name, ok := n.known[addr]; ok {
		if name == "" {
			return addr
		}
		return name
	}
	if !n.asking[addr] {
		n.asking[addr] = true
		go n.ask(addr)
	}
	return addr
}

func (n *PeerNames) ask(addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	names, err := n.lookup(ctx, addr)
	name := ""
	if err == nil && len(names) > 0 {
		name = Shorten(strings.TrimSuffix(names[0], "."))
	}
	n.mu.Lock()
	n.known[addr] = name
	delete(n.asking, addr)
	n.mu.Unlock()
}

/*
Shorten cuts a name to what a narrow column can show and a person reads.

A machine of this network is its first label: the tailnet and the LAN put
their own suffix on every name, and repeating it down a column says nothing.
Anything else keeps its last two labels, which is the part that names who
is at the far end rather than which of their machines it was.
*/
func Shorten(name string) string {
	for _, suffix := range []string{".ts.net", ".lan", ".local", ".home.arpa"} {
		if strings.HasSuffix(name, suffix) {
			first, _, _ := strings.Cut(name, ".")
			return first
		}
	}
	labels := strings.Split(name, ".")
	if len(labels) <= 2 {
		return name
	}
	return strings.Join(labels[len(labels)-2:], ".")
}
