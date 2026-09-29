package hayami_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hayami "github.com/ushineko/hayami"
)

// The embedded README is the file on disk, so About cannot show a stale copy.
func TestTheEmbeddedReadmeIsTheFileOnDisk(t *testing.T) {
	onDisk, err := os.ReadFile("README.md")
	require.NoError(t, err)
	assert.Equal(t, string(onDisk), hayami.README())
}

// It is the document and not a fragment: About is the only place a user reads
// it inside the program, so a truncated embed would be invisible until
// somebody scrolled.
func TestTheEmbeddedReadmeIsWhole(t *testing.T) {
	doc := hayami.README()
	assert.True(t, strings.HasPrefix(doc, "# hayami"), "the README no longer starts with its title")
	assert.Contains(t, doc, "## Changelog", "the changelog is missing, so the embed is truncated")
}
