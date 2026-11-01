package netcount

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

var (
	near = netip.MustParseAddr("192.0.2.1")
	far  = netip.MustParseAddr("198.51.100.9")
)

// Spec 072. A line is what moved between two readings: an entry that did
// not move is left out, a new one counts whole, and one the table let go
// and made again, so that it reads lower, counts whole too.
func TestALineIsWhatMovedBetweenTwoReadings(t *testing.T) {
	steady := Flow{PID: 7, Proto: 6, Local: near, LocalPort: 5000, Peer: far, PeerPort: 443}
	moving := Flow{PID: 7, Proto: 17, Local: near, LocalPort: 5001, Peer: far, PeerPort: 443}
	fresh := Flow{PID: 9, Proto: 17, Local: near, LocalPort: 5002, Peer: far, PeerPort: 53}
	remade := Flow{PID: 0, Proto: 17, Local: near, LocalPort: 5003, Peer: far, PeerPort: 123}

	was := Snapshot{
		Flows: map[Flow]Bytes{
			steady: {Rx: 100, Tx: 10}, moving: {Rx: 100, Tx: 10}, remade: {Rx: 900, Tx: 900},
		},
		Forwarded: map[Hop]uint64{{In: "vnet0", Out: "eth0"}: 1000, {In: "eth0", Out: "vnet0"}: 50},
	}
	now := Snapshot{
		Flows: map[Flow]Bytes{
			steady: {Rx: 100, Tx: 10}, moving: {Rx: 350, Tx: 30}, fresh: {Rx: 40, Tx: 4}, remade: {Rx: 5, Tx: 6},
		},
		Forwarded: map[Hop]uint64{{In: "vnet0", Out: "eth0"}: 1600, {In: "eth0", Out: "vnet0"}: 50},
		Tunnels:   []uint32{12, 3},
		Full:      true,
	}

	line := Line(was, now)
	assert.ElementsMatch(t, []core.NetHelperFlow{
		{PID: 7, Proto: 17, Local: "192.0.2.1", LocalPort: 5001, Peer: "198.51.100.9", PeerPort: 443, Rx: 250, Tx: 20},
		{PID: 9, Proto: 17, Local: "192.0.2.1", LocalPort: 5002, Peer: "198.51.100.9", PeerPort: 53, Rx: 40, Tx: 4},
		{PID: 0, Proto: 17, Local: "192.0.2.1", LocalPort: 5003, Peer: "198.51.100.9", PeerPort: 123, Rx: 5, Tx: 6},
	}, line.F)
	assert.Equal(t, []core.NetHelperHop{{In: "vnet0", Out: "eth0", Bytes: 600}}, line.H)
	assert.Equal(t, []uint32{3, 12}, line.T)
	assert.True(t, line.Full)
}

// The committed object is built from the committed source. Editing the
// program without `make bpf` would ship the old program under the new
// source, and a reviewer reading the C would be reading the wrong thing.
func TestTheObjectWasBuiltFromThisSource(t *testing.T) {
	src, err := os.ReadFile("bpf/netcount.bpf.c")
	require.NoError(t, err)
	sum := sha256.Sum256(src)
	assert.Equal(t, sourceHash, hex.EncodeToString(sum[:]),
		"bpf/netcount.bpf.c changed since netcount_bpfel.o was built: run `make bpf`")
}
