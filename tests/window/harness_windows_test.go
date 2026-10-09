/*
Package window_test drives the real panel on a real Windows desktop and reads
what it drew from a picture of the window: the project's rule that a claim
about what the program looks like is made against a screenshot of the
program, or it is not made.

It needs a desktop session, a mingw gcc for the cgo build, and minutes rather
than seconds, so it runs only when asked:

	$env:HAYAMI_WINDOW_TEST = "1"; go test ./tests/window/ -v

Each run names its picture in the log, for a person to look at; with
-artifacts it is kept:

	$env:HAYAMI_WINDOW_TEST = "1"; go test ./tests/window/ -v -artifacts -outputdir $env:TEMP

This file is the harness every test uses (spec 041): one build of the panel,
one launcher over a throwaway settings file, one way to take and save the
picture, and rows found by their label rather than their position.
*/
package window_test

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/view"
)

// enabled is whether this run drives real windows.
func enabled() bool { return os.Getenv("HAYAMI_WINDOW_TEST") == "1" }

// binary is the panel built once for the whole run, in a directory TestMain
// owns. Empty when the run does not drive windows.
var binary string

// TestMain builds the panel once. Each test used to build its own, which was
// a minute of a run spent compiling the same program four times.
func TestMain(m *testing.M) {
	if !enabled() {
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "hayami-window-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "making the build directory:", err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "hayami.exe")
	_, file, _, _ := runtime.Caller(0)
	build := exec.Command("go", "build", "-tags", "migrated_fynedo", "-o", binary, "./cmd/hayami")
	build.Dir = filepath.Join(filepath.Dir(file), "..", "..")
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the panel: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// panelSettings is what a test asks the panel to draw. Everything else is the
// panel's default, in a settings file nobody else reads.
type panelSettings struct {
	// Sections are drawn in this order.
	Sections []string
	// Interfaces are the bandwidth section's chosen interfaces.
	Interfaces []string
	// LHM is the LibreHardwareMonitor address (spec 036); empty is the
	// default.
	LHM string
	// Settle is how long the panel is given after its window appears, for
	// its first polls.
	Settle time.Duration
	// HideForFullscreen turns on hiding while a full-screen app is in front
	// (spec 051). Off unless a test asks: a panel that hid itself because
	// someone has a game or a film in front could not be photographed.
	HideForFullscreen bool
	// Preferences opens the preferences window as well, on this page.
	Preferences string

	// x and y are where the window opens, set by start: away from the
	// pointer, so the panel never opens under it and shows a row's tip over
	// the card it is reading.
	x, y int
}

// yaml is the settings file for s.
func (s panelSettings) yaml() string {
	var b strings.Builder
	b.WriteString("hayami:\n    sections: [" + strings.Join(s.Sections, ", ") + "]\n")
	// Each section's own settings, in the shape spec 046 writes; the root
	// fields older files carry are the config tests' to cover.
	if len(s.Interfaces) > 0 || s.LHM != "" {
		b.WriteString("    sectionSettings:\n")
	}
	if len(s.Interfaces) > 0 {
		quoted := make([]string, len(s.Interfaces))
		for i, n := range s.Interfaces {
			quoted[i] = fmt.Sprintf("%q", n)
		}
		b.WriteString("        bandwidth:\n            interfaces: [" + strings.Join(quoted, ", ") + "]\n")
	}
	if s.LHM != "" {
		b.WriteString("        cooler:\n            lhm: " + s.LHM + "\n")
	}
	fmt.Fprintf(&b, "    hideForFullscreen: %t\n", s.HideForFullscreen)
	// Placed says the position is one: zero is a legal coordinate.
	fmt.Fprintf(&b, "    x: %d\n    y: %d\n    placed: true\n", s.x, s.y)
	return b.String()
}

// start runs the panel on s, in a home, cache and config the test owns, and
// returns its window -- this process's own, by its id, so another panel on
// the desktop is never the one read. The panel is killed when the test ends.
func start(t *testing.T, s panelSettings) uintptr {
	t.Helper()
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	testenv.Home(t, home)
	testenv.Cache(t, filepath.Join(home, "cache"))
	testenv.Config(t, filepath.Join(home, "config"))
	s.x, s.y = awayFromPointer()
	path := filepath.Join(dir, "settings.yaml")
	require.NoError(t, os.WriteFile(path, []byte(s.yaml()), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	args := []string{"--settings", path}
	if s.Preferences != "" {
		args = append(args, "--preferences="+s.Preferences)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	hwnd, err := waitWindow(cmd.Process.Pid, "hayami", 20*time.Second)
	require.NoError(t, err)
	time.Sleep(s.Settle)
	return hwnd
}

// picture is one capture of the window, saved for a person to look at.
type picture struct {
	img  *image.NRGBA
	path string
	// text are its lines of text, top to bottom: bands of ink tall enough to
	// be letters rather than the plot's line.
	text []span
}

// shoot captures the window and saves it as name in the test's artifacts.
func shoot(t *testing.T, hwnd uintptr, name string) *picture {
	t.Helper()
	img, err := capture(hwnd)
	require.NoError(t, err)
	path := filepath.Join(t.ArtifactDir(), name)
	require.NoError(t, save(img, path))
	return &picture{img: img, path: path, text: textLines(img)}
}

// textLines are the picture's lines of text: the plot's line is a few pixels
// high, a line of text is not.
func textLines(img *image.NRGBA) []span {
	var text []span
	for _, l := range lines(img) {
		if l.To-l.From >= 8 {
			text = append(text, l)
		}
	}
	return text
}

// gap is the column gap that splits a line into words: three quarters of a
// line's height, wider than the space between two letters and narrower than
// the one between a label and a value.
func (p *picture) gap() int {
	if len(p.text) == 0 {
		return 0
	}
	// The heading is set in another size; the first row is the body's.
	l := p.text[min(1, len(p.text)-1)]
	return (l.To - l.From + 1) * 3 / 4
}

// words are a line's words at the picture's gap.
func (p *picture) words(line span) []span { return words(p.img, line, p.gap()) }

/*
card is what one section's card holds, line by line, as the window draws it:
the section's own lines (rows, then the reasons that stay on the card) with
each row's detail lines under it, which is what the window's flatten does.

It comes from the program, not from the test: the same sources the panel
runs, polled here in the test's own process, give the same rows in the same
order. So a row is found by its label -- the line whose index in the card is
the index of the row with that label -- and a row added, removed or reordered
by the program moves the expectation with it, where a test that said "the
second line is the processor" would read the wrong line or fail for a reason
unrelated to what it checks.
*/
type card []view.Row

// cardOf polls the section key the way the panel does, with s's settings, and
// returns its card. Bandwidth is polled twice, a moment apart, because its
// rows' detail lines are complete only from the second poll.
func cardOf(t *testing.T, key string, s panelSettings) card {
	t.Helper()
	// A nil reader and scan are the panel's own: the counters, the Wi-Fi
	// descriptions and the devices on the desk.
	settings := config.Bandwidth.Set(config.Config{}, config.BandwidthSettings{Interfaces: s.Interfaces})
	settings = config.Cooler.Set(settings, config.CoolerSettings{LHM: s.LHM})
	sources := panel.Sources([]string{key}, panel.Env{Settings: settings})
	require.Len(t, sources, 1, "no source for section %q", key)
	src := sources[0]
	polls := 1
	if key == "bandwidth" {
		polls = 2
	}
	for i := range polls {
		if i > 0 {
			time.Sleep(time.Second)
		}
		_, _ = src.Poll(t.Context())
	}
	var out card
	for _, r := range src.Section().Lines() {
		bare := r
		bare.Detail = ""
		out = append(out, bare)
		for _, d := range r.DetailLines() {
			out = append(out, view.Row{Value: d})
		}
	}
	return out
}

// labels are the card's lines' labels, for a failure message.
func (c card) labels() []string {
	out := make([]string, len(c))
	for i, r := range c {
		out[i] = fmt.Sprintf("%q", r.Label)
	}
	return out
}

/*
row is the picture's line for the card's row labelled label.

The picture's lines are the card's heading and then one per card line, so the
row's line is the heading's plus its index. A picture with a different number
of lines from the card is a window drawing something other than what the
program says the section holds -- a row missing, a reason the model dropped --
and is a failure rather than a guess.
*/
func (p *picture) row(t *testing.T, c card, label string) span {
	t.Helper()
	require.Len(t, p.text, len(c)+1,
		"the picture has %d lines of text and the card a heading and %d lines: %v (picture %s)",
		len(p.text), len(c), c.labels(), p.path)
	for i, r := range c {
		if r.Label == label {
			return p.text[i+1]
		}
	}
	require.Failf(t, "no row", "the card has no row labelled %q: %v", label, c.labels())
	return span{}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var (
	getCursorPos     = user32.NewProc("GetCursorPos")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
)

// The primary screen's size, for GetSystemMetrics.
const (
	smCXScreen = 0
	smCYScreen = 1
)

// panelReach is more than the panel's size with any of the sections the tests
// draw: a corner this far from the pointer keeps the window clear of it.
const panelReach = 600

/*
awayFromPointer is a top-left corner for the window in the quadrant of the
primary screen farthest from the pointer.

A window that opens under a resting pointer gets a hover, and a row with a
tip shows it over the card: the picture then holds the tip's lines, not the
card's. That happened on a desk whose pointer rested where the panel opens,
and a test that moved the person's pointer would be worse than one that moves
its own window.
*/
func awayFromPointer() (x, y int) {
	var p struct{ X, Y int32 }
	_, _, _ = getCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	pw, _, _ := getSystemMetrics.Call(smCXScreen)
	ph, _, _ := getSystemMetrics.Call(smCYScreen)
	w, h := int(pw), int(ph)
	x, y = 40, 40
	if int(p.X) < w/2 {
		x = w - panelReach
	}
	if int(p.Y) < h/2 {
		y = h - panelReach
	}
	return max(x, 0), max(y, 0)
}
