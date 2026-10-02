package core_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

/*
Windows' table is read, and its filter rows are not in it.

Every adapter carries a stack of lightweight filters -- WFP, the QoS packet
scheduler, a capture driver if one is installed -- each listed as an interface
of its own named after the adapter with the filter's name appended, counting
the same bytes again. A chooser that offered them offered each adapter four
times. This reads the machine it runs on, which always has at least the
loopback, and says nothing about which interfaces those are.
*/
func TestWindowsInterfacesAreReadWithoutTheirFilters(t *testing.T) {
	counters, err := core.ReadCounters()
	require.NoError(t, err)
	require.NotEmpty(t, counters, "not even the loopback was read")
	for name := range counters {
		assert.NotContains(t, name, "-WFP ", "a filter row was offered as an interface")
		assert.NotContains(t, name, "-QoS Packet Scheduler", "a filter row was offered as an interface")
		assert.False(t, strings.HasSuffix(name, "-0000"), "a filter row was offered as an interface")
	}
}
