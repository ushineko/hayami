package core

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func helperLines(t *testing.T, lines ...NetHelperLine) io.Reader {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		j, err := json.Marshal(l)
		require.NoError(t, err)
		b.Write(j)
		b.WriteByte('\n')
	}
	return strings.NewReader(b.String())
}

var helperHello = NetHelperLine{V: NetHelperProtocol, Version: "1.2.3", Counts: "TCP and UDP"}

// The panel adds up what the helper sends between its own readings: two
// lines for one process are one figure, and taking it starts the count again.
func TestTheHelpersLinesAreAddedUpUntilTaken(t *testing.T) {
	s := &helperStream{}
	require.Error(t, s.read(helperLines(t, helperHello,
		NetHelperCounts(map[uint32][2]uint64{7: {100, 10}, 9: {1, 2}}),
		NetHelperCounts(map[uint32][2]uint64{7: {50, 5}}),
	)), "the pipe closing is an end, and is said")

	counts, what, err := s.take()
	require.NoError(t, err)
	assert.Equal(t, "TCP and UDP", what)
	assert.Equal(t, map[uint32][2]uint64{7: {150, 15}, 9: {1, 2}}, counts)

	counts, _, err = s.take()
	require.NoError(t, err)
	assert.Empty(t, counts, "taken once")
}

// A helper of another version of the protocol is refused by name, with what
// to do, and none of its numbers are counted.
func TestAHelperOfAnotherProtocolIsRefused(t *testing.T) {
	s := &helperStream{}
	s.run(helperLines(t, NetHelperLine{V: NetHelperProtocol + 1, Version: "9"},
		NetHelperCounts(map[uint32][2]uint64{7: {100, 10}})))
	_, _, err := s.take()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install the helper again")
}

// What the helper says went wrong is what the panel reports, and a stream
// that has ended keeps saying why.
func TestTheHelpersErrorIsThePanels(t *testing.T) {
	s := &helperStream{}
	s.run(helperLines(t, helperHello, NetHelperLine{Error: "enabling the network provider: Access is denied."}))
	for range 2 {
		_, _, err := s.take()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Access is denied")
	}

	s = &helperStream{}
	s.run(strings.NewReader("not a line\n"))
	_, _, err := s.take()
	require.Error(t, err)
}

// Bytes become rates over the time that passed. A process that has gone
// since it sent is still traffic, under no process; with no time passed
// there is no rate to give.
func TestCountsBecomeRatesByName(t *testing.T) {
	samples := func(context.Context) ([]ProcessSample, error) {
		return []ProcessSample{{PID: 7, Name: "browser"}}, nil
	}
	use := netUse(t.Context(), samples, "TCP and UDP", map[uint32][2]uint64{7: {2000, 200}, 9: {10, 20}}, 2)
	assert.True(t, use.HasRate)
	assert.Equal(t, "TCP and UDP", use.Counts)
	assert.ElementsMatch(t, []NetProcess{
		{PID: 7, Name: "browser", Rx: 1000, Tx: 100},
		{PID: 0, Rx: 5, Tx: 10},
	}, use.Processes)

	first := netUse(t.Context(), samples, "TCP and UDP", nil, 0)
	assert.False(t, first.HasRate)
	assert.Empty(t, first.Processes)
}
