package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 094 R7. This machine's memory, read for real: the cache and the page
// file come with the memory in use, and add up. A machine without a page file
// reads one of none.
func TestWindowsReadsTheCacheAndThePageFile(t *testing.T) {
	m, err := globalMemory(t.Context())
	require.NoError(t, err)

	assert.True(t, m.HasCache)
	assert.Positive(t, m.Cache)
	assert.Less(t, m.Cache, m.Total)
	assert.True(t, m.HasSwap)
	assert.LessOrEqual(t, m.SwapUsed, m.SwapTotal)
	assert.Equal(t, "page file", m.SwapName)
	t.Logf("used %d of %d, cache %d, page file %d of %d", m.Used, m.Total, m.Cache, m.SwapUsed, m.SwapTotal)
}
