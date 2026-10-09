package window_test

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
Spec 054. A stale usage reading takes the lines a fresh one takes.

The usage section added a "read … ago" line above its meter once the cache was
five minutes old, and took it away when the cache was refreshed: in a pane one
line tall that line was all there was, and every time it came or went the
panel moved (#172). Now a stale reading is drawn dim in place, its age a hover
away. The panel is run twice over an invented cache -- read a minute ago, then
twelve -- and its window must be the same height, with the same bands of text,
both times.

The bands are counted at faintLevel, not inkLevel: the usage card's heading,
its stats line and the line the old code added are all set dim, and counting
only bright ink saw one line in every picture, which no change could fail.
*/
func TestAStaleUsageReadingTakesTheLinesAFreshOneTakes(t *testing.T) {
	type run struct{ height, bands int }
	shot := func(age time.Duration, name string) run {
		t.Helper()
		s := panelSettings{Sections: []string{"usage"}, Settle: 3 * time.Second, Cache: func(dir string) {
			writeUsageCache(t, filepath.Join(dir, "claude-usage-widget"), time.Now(), age)
		}}
		hwnd := start(t, s)
		p := shoot(t, hwnd, name)
		// The panel's own source, polled here over the same cache, says
		// whether this run is the stale one: a picture that looks the same
		// because the cache was not read as stale would prove nothing.
		sec := usageSection(t)
		require.NotEmpty(t, sec.Meters, "the invented cache drew no meter")
		require.Equal(t, age > view.UsageStale, sec.Stale, "read %v ago", age)
		r := run{height: p.img.Bounds().Dy(), bands: len(bands(p.img, faintLevel))}
		t.Logf("read %v ago: %d px tall, %d bands of text, picture %s", age, r.height, r.bands, p.path)
		return r
	}
	fresh := shot(time.Minute, "usage-fresh.png")
	stale := shot(12*time.Minute, "usage-stale.png")
	require.GreaterOrEqual(t, fresh.bands, 3, "the usage card drew less than a heading, a meter and its stats")
	require.Equal(t, fresh, stale, "a stale reading changed the usage card's size or lines")
}

// faintLevel is above the card's background and below its dimmest text.
const faintLevel = 90

// bands are the picture's horizontal runs of anything brighter than level:
// text of any weight, and a meter's bar.
func bands(img *image.NRGBA, level float64) []span {
	b := img.Bounds()
	var out []span
	in := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		has := false
		for x := b.Min.X; x < b.Max.X && !has; x++ {
			has = luminance(img.At(x, y)) > level
		}
		switch {
		case has && !in:
			out = append(out, span{y, y})
			in = true
		case has:
			out[len(out)-1].To = y
		default:
			in = false
		}
	}
	return out
}

// usageSection polls the usage source in this process, over the cache the
// test's environment points at.
func usageSection(t *testing.T) view.Section {
	t.Helper()
	sources := panel.Sources([]string{"usage"}, panel.Env{Settings: config.Config{}})
	require.Len(t, sources, 1)
	_, _ = sources[0].Poll(t.Context())
	return sources[0].Section()
}

// writeUsageCache is one invented Claude account, read age before now, with
// a gate an hour away so the panel asks nobody for a fresh one.
func writeUsageCache(t *testing.T, dir string, now time.Time, age time.Duration) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	fetched := float64(now.Add(-age).Unix())
	gate := float64(now.Add(time.Hour).Unix())
	iso := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }
	data := fmt.Sprintf(`{"five_hour": {"utilization": 12, "resets_at": %q}, "seven_day": {"utilization": 40, "resets_at": %q}}`,
		iso(2*time.Hour), iso(4*24*time.Hour))
	entry := fmt.Sprintf(`{"next_attempt_at": %f, "fetched_at": %f, "data": %s}`, gate, fetched, data)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"), []byte(entry), 0o600))
}
