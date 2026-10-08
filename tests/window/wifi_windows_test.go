package window_test

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/testenv"
)

// wifiSettle is how long the panel is given after its window appears: the
// bandwidth section's first poll describes the link, and its second has rates.
const wifiSettle = 5 * time.Second

// startPanel builds and starts the desktop panel on a settings file the test
// writes, in directories the test owns, and returns its window. The panel is
// killed when the test ends. Nothing of the user's settings is read.
func startPanel(t *testing.T, settings string) uintptr {
	t.Helper()
	if os.Getenv("HAYAMI_WINDOW_TEST") != "1" {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "hayami.exe")
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "..", "..")

	build := exec.CommandContext(t.Context(), "go", "build", "-tags", "migrated_fynedo", "-o", bin, "./cmd/hayami")
	build.Dir = repo
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "building the panel: %s", out)

	home := filepath.Join(dir, "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	testenv.Home(t, home)
	testenv.Cache(t, filepath.Join(home, "cache"))
	testenv.Config(t, filepath.Join(home, "config"))
	path := filepath.Join(dir, "settings.yaml")
	require.NoError(t, os.WriteFile(path, []byte(settings), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "--settings", path)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	hwnd, err := waitWindow(cmd.Process.Pid, "hayami", 20*time.Second)
	require.NoError(t, err)
	time.Sleep(wifiSettle)
	return hwnd
}

// interfacesHere are a connected Wi-Fi interface and one this build does not
// describe as Wi-Fi -- wired, or a radio Windows does not list -- on this
// machine, or a skip.
func interfacesHere(t *testing.T) (wifi, wired string) {
	t.Helper()
	radios, err := core.ReadWireless(t.Context())
	if err != nil || len(radios) == 0 {
		t.Skip("no Wi-Fi interface this build can describe")
	}
	for name, w := range radios {
		if w.Connected {
			wifi = name
		}
	}
	if wifi == "" {
		t.Skip("no connected Wi-Fi interface")
	}
	counters, err := core.ReadCounters()
	require.NoError(t, err)
	var names []string
	for name := range counters {
		if _, isRadio := radios[name]; !isRadio && core.ClassifyInterface(name) == core.KindOrdinary {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Skip("no wired interface to line up with")
	}
	// The shortest name: a long one ("Bluetooth Network Connection") fills
	// its label column up to the first arrow, and the picture can no longer
	// tell the label from the rates.
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) < len(names[j])
		}
		return names[i] < names[j]
	})
	return wifi, names[0]
}

/*
Spec 037. A Wi-Fi interface's row leads with its signal bars, and its rates
stay in the column a wired interface's are in.

Read off the picture. The card's lines of text are found by brightness; an
interface's row is a line whose first word starts in the label column (the
leftmost text below the heading), and the lines between are the right-aligned
totals and link. The Wi-Fi row is the first, by the settings' order.

A row ends "↓ figure unit ↑ figure unit", so its arrows are the sixth and the
third words from the end, and both rows' arrows are at the same x, to the
pixel: one column of rates. (The units' ink cannot be compared: "B/s" is padded
to the width of "KiB/s" with spaces, which have none.) The four glyphs before
the Wi-Fi row's down arrow are the bars: four runs of one width, filled ones
solid, which no four letters of a label are. The bars sit close enough to a
short label that the two read as one word at the gap that splits words, so
they are told apart by shape rather than by spacing. Bars drawn after the rates
instead would move the Wi-Fi row's arrows; bars not drawn would leave letters
before its ↓.
*/
func TestTheWiFiRowsBarsLeadItsRatesInTheWiredColumn(t *testing.T) {
	wifi, wired := interfacesHere(t)
	hwnd := startPanel(t, fmt.Sprintf("hayami:\n    sections: [bandwidth]\n    interfaces: [%q, %q]\n", wifi, wired))

	img, err := capture(hwnd)
	require.NoError(t, err)
	shot := filepath.Join(t.ArtifactDir(), "wifi.png")
	require.NoError(t, save(img, shot))
	t.Logf("picture: %s", shot)

	var text []span
	for _, l := range lines(img) {
		if l.To-l.From >= 8 {
			text = append(text, l)
		}
	}
	require.GreaterOrEqual(t, len(text), 5, "a heading, two interfaces and their lines under them")
	gap := (text[1].To - text[1].From + 1) * 3 / 4

	// The label column: the leftmost start of any line below the heading.
	left := 1 << 30
	all := make([][]span, len(text))
	for i, l := range text {
		all[i] = words(img, l, gap)
		if i > 0 && len(all[i]) > 0 && all[i][0].From < left {
			left = all[i][0].From
		}
	}
	var rows []int
	var report []string
	for i := 1; i < len(text); i++ {
		// Two words at least: the plot under the rows is one long run of ink.
		if len(all[i]) >= 2 && abs2(all[i][0].From-left) <= 1 {
			rows = append(rows, i)
		}
		report = append(report, fmt.Sprintf("%v %v", text[i], all[i]))
	}
	t.Logf("lines:\n%s", strings.Join(report, "\n"))
	require.Len(t, rows, 2, "two interface rows start in the label column")

	radio, cable := all[rows[0]], all[rows[1]]
	require.GreaterOrEqual(t, len(radio), 7, "the Wi-Fi row: a label and two rates")
	require.GreaterOrEqual(t, len(cable), 7, "the wired row: a label and two rates")
	down, up := func(w []span) span { return w[len(w)-6] }, func(w []span) span { return w[len(w)-3] }
	assert.LessOrEqual(t, abs2(down(radio).From-down(cable).From), 1,
		"the down arrows are at x=%d and x=%d: the rates are not in one column", down(radio).From, down(cable).From)
	assert.LessOrEqual(t, abs2(up(radio).From-up(cable).From), 1,
		"the up arrows are at x=%d and x=%d: the rates are not in one column", up(radio).From, up(cable).From)

	radioBars := barsBefore(img, text[rows[0]], down(radio).From)
	cableBars := barsBefore(img, text[rows[1]], down(cable).From)
	t.Logf("glyphs before ↓: Wi-Fi %v, other %v", radioBars, cableBars)
	assert.True(t, isBars(img, text[rows[0]], radioBars),
		"the four glyphs before the Wi-Fi row's ↓ are not four bars of one width: %v", radioBars)
	assert.False(t, isBars(img, text[rows[1]], cableBars),
		"the other row has bars before its ↓ too: %v", cableBars)
	assert.GreaterOrEqual(t, rows[1]-rows[0], 3,
		"the Wi-Fi row should have its totals and its link under it before the next interface")
}

// barsBefore are the last four glyphs left of x on a line: runs of ink split
// by any column without it, which a bar's gap to the next bar is.
func barsBefore(img *image.NRGBA, line span, x int) []span {
	var runs []span
	for _, w := range words(img, line, 1) {
		if w.To < x {
			runs = append(runs, w)
		}
	}
	if len(runs) > 4 {
		runs = runs[len(runs)-4:]
	}
	return runs
}

// isBars says whether glyphs are the signal's four segments: four runs of the
// same width (to a pixel), at least one of them solid -- a filled segment has
// ink in most of its box, a letter in far less.
func isBars(img *image.NRGBA, line span, glyphs []span) bool {
	if len(glyphs) != 4 {
		return false
	}
	solid := false
	for _, g := range glyphs {
		if abs2((g.To-g.From)-(glyphs[0].To-glyphs[0].From)) > 1 {
			return false
		}
		inked, top, bottom := 0, line.To, line.From
		for x := g.From; x <= g.To; x++ {
			for y := line.From; y <= line.To; y++ {
				if ink(img, x, y) {
					inked++
					top, bottom = min(top, y), max(bottom, y)
				}
			}
		}
		box := (g.To - g.From + 1) * (bottom - top + 1)
		if box > 0 && inked*10 >= box*8 {
			solid = true
		}
	}
	return solid
}

func abs2(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
