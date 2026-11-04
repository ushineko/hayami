package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvedsAnswersAreAddresses(t *testing.T) {
	bus := &fakeBus{
		owners: map[string]bool{resolvedName: true},
		answers: []resolvedAddress{
			{Family: 2, Address: []byte{100, 64, 1, 5}},
			{Family: 10, Address: []byte{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
		},
	}

	got, err := resolvedLookup(context.Background(), bus, "invented.example")

	require.NoError(t, err)
	assert.Equal(t, []string{"100.64.1.5", "fd00::1"}, got)
	assert.Equal(t, uint64(1)<<12, bus.args[3], "a cached answer would pass on a dead tunnel")
}

// A machine without resolved falls back to Go's resolver, and is not made to start
// it.
func TestResolvedAbsentIsSaidWithoutStartingIt(t *testing.T) {
	bus := &fakeBus{owners: map[string]bool{}}

	_, err := resolvedLookup(context.Background(), bus, "invented.example")

	require.ErrorIs(t, err, ErrVPNBackendAbsent)
}

func TestADNSCheckNeedsANameAndAPrefix(t *testing.T) {
	err := dnsCheck(context.Background(), VPNCheck{Target: "invented.example"})
	require.ErrorIs(t, err, ErrVPNCheckDidNotRun)
}

func TestAPingCheckNeedsAHost(t *testing.T) {
	require.ErrorIs(t, pingCheck(context.Background(), VPNCheck{}), ErrVPNCheckDidNotRun)
}

// The real thing, against loopback, with whatever the kernel allows: an unprivileged
// ICMP socket that answers, or a refusal that is not a failure of the tunnel.
func TestARealPingOfLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := pingCheck(ctx, VPNCheck{Target: "127.0.0.1"})

	if errors.Is(err, ErrVPNCheckDidNotRun) {
		t.Skipf("this kernel refuses unprivileged ICMP: %v", err)
	}
	require.NoError(t, err)
}

// An address nobody answers on, in the documentation range: the check fails, by
// its timeout or the network's refusal, and does not call that not running.
func TestARealPingWithNoAnswerFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := pingCheck(ctx, VPNCheck{Target: "192.0.2.1"})

	require.Error(t, err)
	if errors.Is(err, ErrVPNCheckDidNotRun) {
		t.Skipf("this kernel refuses unprivileged ICMP: %v", err)
	}
}

// The real bus, where resolved runs: localhost resolves on every machine that has it.
func TestARealResolvedLookup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, err := resolvedLookup(ctx, systemBus{}, "localhost")

	if errors.Is(err, ErrVPNBackendAbsent) {
		t.Skip("systemd-resolved is not running")
	}
	if err != nil {
		t.Skipf("no system bus: %v", err)
	}
	assert.Contains(t, got, "127.0.0.1")
}
