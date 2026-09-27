package core_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// A table in the kernel's own shape, with the two header lines and the
// alignment the kernel writes. Invented interface names: a real machine's
// belong to a real machine.
const table = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:    1000      10    0    0    0     0          0         0     1000      10    0    0    0     0       0          0
  eth0:  204800     200    0    0    0     0          0         0   102400     100    0    0    0     0       0          0
`

func TestTheInterfaceTableIsReadIntoBytes(t *testing.T) {
	got, err := core.ParseNetDev(strings.NewReader(table))

	require.NoError(t, err)
	assert.Equal(t, core.Counters{Rx: 204800, Tx: 102400}, got["eth0"])
	assert.Equal(t, core.Counters{Rx: 1000, Tx: 1000}, got["lo"])
}

// The kernel does not always leave a space after the colon: an interface whose
// receive count is wide enough runs the number straight into it. Splitting on
// whitespace loses that interface entirely, which is a section that silently
// stops reporting on the machine that needed it most.
func TestAnInterfaceWhoseCountTouchesTheColonIsStillRead(t *testing.T) {
	line := "Inter-|\n face |bytes\n  eth0:12345678901 200    0    0    0     0          0         0   102400     100    0    0    0     0       0          0\n"

	got, err := core.ParseNetDev(strings.NewReader(line))

	require.NoError(t, err)
	assert.Equal(t, uint64(12345678901), got["eth0"].Rx)
}

// One unparseable line is one interface, not the whole table. A panel that
// went blank because a virtual device wrote something odd would be worse than
// a panel missing that device.
func TestAMalformedLineIsSkippedAndTheRestIsRead(t *testing.T) {
	bad := table + "  weird: not a number here\n"

	got, err := core.ParseNetDev(strings.NewReader(bad))

	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.NotContains(t, got, "weird")
}
