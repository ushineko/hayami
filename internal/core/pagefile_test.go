package core

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageFileEntry is one SYSTEM_PAGEFILE_INFORMATION as amd64 lays it out: the
// three counts, a UNICODE_STRING for the name, and the name's bytes, which
// the offset steps over.
func pageFileEntry(last bool, size, inUse uint32) []byte {
	b := make([]byte, 48)
	if !last {
		binary.LittleEndian.PutUint32(b, uint32(len(b)))
	}
	binary.LittleEndian.PutUint32(b[4:], size)
	binary.LittleEndian.PutUint32(b[8:], inUse)
	binary.LittleEndian.PutUint32(b[12:], inUse/2) // the peak, unread
	return b
}

// Every page file counts, in pages times the page size (spec 094 R7).
func TestThePageFilesAreSummed(t *testing.T) {
	buf := append(pageFileEntry(false, 1000, 200), pageFileEntry(true, 500, 50)...)

	used, total, err := pageFiles(buf, 4096)
	require.NoError(t, err)
	assert.Equal(t, uint64(250*4096), used)
	assert.Equal(t, uint64(1500*4096), total)
}

// No page file is an empty answer, which is none and not an error.
func TestNoPageFileIsNone(t *testing.T) {
	used, total, err := pageFiles(nil, 4096)
	require.NoError(t, err)
	assert.Zero(t, used)
	assert.Zero(t, total)
}

// An answer that does not add up is an error, not a figure: an entry cut
// short, an offset past the end, more in use than there is.
func TestAPageFileAnswerThatDoesNotAddUpIsAnError(t *testing.T) {
	cut := pageFileEntry(true, 10, 5)[:8]
	past := pageFileEntry(false, 10, 5)
	binary.LittleEndian.PutUint32(past, 4096)
	over := pageFileEntry(true, 10, 50)
	for name, buf := range map[string][]byte{"cut": cut, "past": past, "over": over} {
		_, _, err := pageFiles(buf, 4096)
		assert.Error(t, err, name)
	}
}

// The walk over the page file answer neither panics nor loops on any bytes,
// and what it sums adds up: no more in use than there is (spec 094).
func FuzzPageFiles(f *testing.F) {
	f.Add(append(pageFileEntry(false, 1000, 200), pageFileEntry(true, 500, 50)...))
	f.Add([]byte{})
	f.Add(pageFileEntry(true, 10, 5)[:8])
	f.Fuzz(func(t *testing.T, buf []byte) {
		used, total, err := pageFiles(buf, 4096)
		if err == nil && used > total {
			t.Fatalf("%d in use of %d", used, total)
		}
	})
}
