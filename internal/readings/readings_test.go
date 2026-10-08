package readings_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

// saved writes a cache holding one section taken at a given time.
func saved(t *testing.T, at time.Time) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), readings.FileName)
	require.NoError(t, readings.Save(path, readings.Cache{
		"peripherals": {At: at, Section: view.Peripherals(view.PeripheralsReading{
			Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse}},
		})},
	}))
	return path
}

// AC1. What was written comes back.
func TestAReadingSurvivesTheRoundTrip(t *testing.T) {
	now := time.Now()
	c := readings.Load(saved(t, now), now)

	sec, err := c.Restore("peripherals")
	require.NoError(t, err)
	require.Len(t, sec.Cells, view.PeripheralSlots)
	assert.Equal(t, "G502 X PLUS", sec.Cells[0].Label)
	assert.Contains(t, sec.Cells[0].Value, "76")
	assert.True(t, sec.Cells[1].Placeholder, "the empty slot came back as a reading")
}

// AC1. A restored section is marked, because that is the only thing that
// distinguishes a claim about the past drawn in the present.
func TestARestoredSectionIsMarked(t *testing.T) {
	now := time.Now()
	c := readings.Load(saved(t, now), now)

	sec, err := c.Restore("peripherals")
	require.NoError(t, err)
	assert.True(t, sec.Restored)
	assert.True(t, sec.Dimmed(), "a restored section is drawn dim")
}

// AC5. Nothing older than a day is restored: a week-old battery is furniture.
func TestAReadingOlderThanADayIsNotRestored(t *testing.T) {
	now := time.Now()
	path := saved(t, now.Add(-readings.MaxAge-time.Minute))

	c := readings.Load(path, now)
	_, err := c.Restore("peripherals")
	assert.ErrorIs(t, err, readings.ErrNoEntry)
}

// AC5. One inside the bound is.
func TestAReadingFromLastNightIsRestored(t *testing.T) {
	now := time.Now()
	c := readings.Load(saved(t, now.Add(-12*time.Hour)), now)

	_, err := c.Restore("peripherals")
	assert.NoError(t, err)
}

// AC7. A cache that is not there is a cold start, not an error. The panel has
// to open whatever state this file is in.
func TestAnAbsentCacheIsAColdStart(t *testing.T) {
	c := readings.Load(filepath.Join(t.TempDir(), "nothing.json"), time.Now())
	assert.Empty(t, c)
	_, err := c.Restore("peripherals")
	assert.ErrorIs(t, err, readings.ErrNoEntry)
}

// AC7. And so is a truncated one -- a process killed mid-write, or a file
// from a version that wrote something else.
func TestATruncatedCacheIsAColdStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), readings.FileName)
	require.NoError(t, os.WriteFile(path, []byte(`{"peripherals":{"at":`), 0o600))

	assert.Empty(t, readings.Load(path, time.Now()))
}

// Spec 044. A cache written before the section's rows had IDs -- a file with
// no version, the sections at its top level -- is a cold start, not old
// sections drawn by new rules. So is one from a version this build does not
// know.
func TestACacheFromAnotherVersionIsAColdStart(t *testing.T) {
	now := time.Now()
	at := now.Format(time.RFC3339Nano)
	for name, body := range map[string]string{
		"before versions": `{"cooler":{"at":"` + at + `","section":{"Key":"cooler","Title":"Cooler"}}}`,
		"a later version": `{"version":999,"sections":{"cooler":{"at":"` + at + `","section":{"Key":"cooler","Title":"Cooler"}}}}`,
	} {
		path := filepath.Join(t.TempDir(), readings.FileName)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

		assert.Empty(t, readings.Load(path, now), name)
	}
}

// Spec 044. What Save writes is this version's, and Load reads it back.
func TestTheCacheCarriesItsVersion(t *testing.T) {
	now := time.Now()
	path := saved(t, now)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"version":`+strconv.Itoa(readings.FormatVersion))
	assert.NotEmpty(t, readings.Load(path, now))
}

// AC7. The write is atomic, so a cache that exists is a cache that parses:
// there is no window in which the file holds half a document.
func TestTheCacheIsReplacedRatherThanRewritten(t *testing.T) {
	now := time.Now()
	path := saved(t, now)

	require.NoError(t, readings.Save(path, readings.Cache{
		"cooler": {At: now, Section: view.Section{Key: "cooler", Title: "Cooler"}},
	}))

	c := readings.Load(path, now)
	_, err := c.Restore("cooler")
	require.NoError(t, err)
	_, err = c.Restore("peripherals")
	assert.ErrorIs(t, err, readings.ErrNoEntry, "the previous contents survived a replace")

	// Nothing left behind beside it.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "a temporary file was left in the cache directory")
}
