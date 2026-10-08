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
*/
package window_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/testenv"
)

// settle is how long the panel is given after its window appears: the cooler
// polls at once and its first poll takes two processor samples apart.
const settle = 4 * time.Second

// panel builds and starts the desktop panel with only the given sections, in
// directories the test owns, and returns its window. The panel is killed when
// the test ends.
func panel(t *testing.T, sections string, more ...string) uintptr {
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
	settings := filepath.Join(dir, "settings.yaml")
	require.NoError(t, os.WriteFile(settings, []byte(settingsFor(sections, more)), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "--settings", settings)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	hwnd, err := waitWindow(cmd.Process.Pid, "hayami", 20*time.Second)
	require.NoError(t, err)
	time.Sleep(settle)
	return hwnd
}

/*
Spec 034. On Windows the processor has a load and no temperature, and its row
is drawn: the load in the column the graphics card's load is in, and nothing
where the temperature would be.

Read off the picture. The card's text lines are found by brightness: the
heading first, then the processor, then the card. The processor's line ends
where the card's load ends -- the right edge of its "%" -- to the pixel, and
the card's line carries more after that point (its temperature), which the
processor's does not. A processor row that is missing puts the card's line
where the processor's should be, and the card's ends at its degrees sign, which
lines up with nothing on the line under it.
*/
func TestTheProcessorsLoadIsDrawnInTheCardsColumnWithoutATemperature(t *testing.T) {
	if !core.NewGraphicsReader().Native(t.Context()).HasTemperature {
		t.Skip("no card here reports to D3DKMT, so there is no row to line up with")
	}
	// Nothing listens on the discard port: a LibreHardwareMonitor running on
	// this desk (spec 036) would otherwise give the processor a temperature.
	hwnd := panel(t, "cooler", "lhm: http://127.0.0.1:9/data.json")

	img, err := capture(hwnd)
	require.NoError(t, err)
	shot := filepath.Join(t.ArtifactDir(), "processors.png")
	require.NoError(t, save(img, shot))
	t.Logf("picture: %s", shot)

	var text []span
	for _, l := range lines(img) {
		// The plot's line is a few pixels high; a line of text is not.
		if l.To-l.From >= 8 {
			text = append(text, l)
		}
	}
	require.GreaterOrEqual(t, len(text), 3, "a heading, the processor and the card")
	cpuLine, gpuLine := text[1], text[2]
	gap := (gpuLine.To - gpuLine.From + 1) * 3 / 4
	cpu, gpu := words(img, cpuLine, gap), words(img, gpuLine, gap)
	t.Logf("processor %v: %v", cpuLine, cpu)
	t.Logf("card      %v: %v", gpuLine, gpu)
	require.GreaterOrEqual(t, len(cpu), 2, "the processor's line is a label and a load")

	end := cpu[len(cpu)-1].To
	matched := -1
	for i, w := range gpu {
		if abs(w.To-end) <= 1 {
			matched = i
		}
	}
	require.NotEqual(t, -1, matched,
		"the processor's line ends at x=%d, where no part of the card's line below it ends", end)
	assert.Less(t, matched, len(gpu)-1,
		"the processor's line ends where the card's temperature does: its load has moved into the temperature's column")
}

// settingsFor is a settings file drawing sections, with more of its own lines.
func settingsFor(sections string, more []string) string {
	s := "hayami:\n    sections: [" + sections + "]\n"
	for _, line := range more {
		s += "    " + line + "\n"
	}
	return s
}

/*
Spec 036. With LibreHardwareMonitor running, the processor has its
temperature from it, and the row is the card's shape: the temperature ends
where the card's does.

Read off the picture as above. The processor's line now ends at its degrees
sign, which is the card's line's last word too, to the pixel; and the
processor's load still ends where the card's load does. Pointed at an address
nothing serves, the processor's line ends at its load, and the first check
fails.
*/
func TestTheProcessorsTemperatureFromLibreHardwareMonitorIsInTheCardsColumn(t *testing.T) {
	if !core.NewGraphicsReader().Native(t.Context()).HasTemperature {
		t.Skip("no card here reports to D3DKMT, so there is no row to line up with")
	}
	if _, err := core.NewLHM("").CPUTemperature(t.Context()); err != nil {
		t.Skipf("LibreHardwareMonitor gives no CPU temperature here: %v", err)
	}
	hwnd := panel(t, "cooler", "lhm: "+core.LHMURL)

	img, err := capture(hwnd)
	require.NoError(t, err)
	shot := filepath.Join(t.ArtifactDir(), "processors-lhm.png")
	require.NoError(t, save(img, shot))
	t.Logf("picture: %s", shot)

	var text []span
	for _, l := range lines(img) {
		if l.To-l.From >= 8 {
			text = append(text, l)
		}
	}
	require.GreaterOrEqual(t, len(text), 3, "a heading, the processor and the card")
	cpuLine, gpuLine := text[1], text[2]
	gap := (gpuLine.To - gpuLine.From + 1) * 3 / 4
	cpu, gpu := words(img, cpuLine, gap), words(img, gpuLine, gap)
	t.Logf("processor %v: %v", cpuLine, cpu)
	t.Logf("card      %v: %v", gpuLine, gpu)
	require.GreaterOrEqual(t, len(cpu), 3, "the processor's line is a label, a load and a temperature")
	require.GreaterOrEqual(t, len(gpu), 3)

	assert.LessOrEqual(t, abs(cpu[len(cpu)-1].To-gpu[len(gpu)-1].To), 1,
		"the processor's line ends at x=%d and the card's at x=%d: the temperatures are not one column",
		cpu[len(cpu)-1].To, gpu[len(gpu)-1].To)
	loads := 0
	for _, c := range cpu {
		for _, g := range gpu[:len(gpu)-1] {
			if abs(c.To-g.To) <= 1 {
				loads++
			}
		}
	}
	assert.Positive(t, loads, "nothing on the processor's line but its end lines up with the card's: the load has moved")
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
