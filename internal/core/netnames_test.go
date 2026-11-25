package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// known waits for the resolver's answer about addr to land.
func known(t *testing.T, n *PeerNames, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		_, ok := n.known[addr]
		n.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no answer about %s", addr)
}

// Spec 113 R12. A peer's name is kept whole, which is the fact, and
// shortened only for a column: Name is what a line says, Full what a reading
// holds.
func TestAPeersNameIsKeptWholeAndShortenedForAColumn(t *testing.T) {
	n := NewPeerNames()
	n.lookup = func(_ context.Context, addr string) ([]string, error) {
		if addr == "192.0.2.10" {
			return []string{"edge-7.cache.example.com."}, nil
		}
		return nil, errors.New("no such host")
	}

	if got := n.Full("192.0.2.10"); got != "192.0.2.10" {
		t.Fatalf("a first ask answers with the address, got %q", got)
	}
	known(t, n, "192.0.2.10")

	if got := n.Full("192.0.2.10"); got != "edge-7.cache.example.com" {
		t.Errorf("Full = %q, want the whole name without its dot", got)
	}
	if got := n.Name("192.0.2.10"); got != "example.com" {
		t.Errorf("Name = %q, want the name as a column shows it", got)
	}

	n.Full("192.0.2.20")
	known(t, n, "192.0.2.20")
	if got, short := n.Full("192.0.2.20"), n.Name("192.0.2.20"); got != "192.0.2.20" || short != "192.0.2.20" {
		t.Errorf("an address with no name is itself: Full %q, Name %q", got, short)
	}
}

func TestShortNameIsTheAddressWhereThereIsNoName(t *testing.T) {
	for _, c := range []struct{ addr, full, want string }{
		{"192.0.2.1", "", "192.0.2.1"},
		{"192.0.2.1", "192.0.2.1", "192.0.2.1"},
		{"192.0.2.1", "desk.lan", "desk"},
		{"192.0.2.1", "a.b.example.org", "example.org"},
	} {
		if got := ShortName(c.addr, c.full); got != c.want {
			t.Errorf("ShortName(%q, %q) = %q, want %q", c.addr, c.full, got, c.want)
		}
	}
}
