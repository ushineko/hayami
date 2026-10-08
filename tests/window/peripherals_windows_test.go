package window_test

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/aula"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/logitech"
	"github.com/ushineko/sanshoku/razer"
	"github.com/ushineko/sanshoku/steelseries"

	"github.com/ushineko/hayami/internal/testenv"
)

// peripheralsSettle is how long the panel is given after its window appears
// for the peripherals' first poll, which asks every device and may ask a
// sleeping one twice.
const peripheralsSettle = 6 * time.Second

// launch builds and starts the desktop panel with only the given sections, in
// directories the test owns -- a throwaway settings file, home and cache, so
// nothing of the user's is read or written -- and returns its window.
func launch(t *testing.T, sections string, settle time.Duration) uintptr {
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
	require.NoError(t, os.WriteFile(settings, []byte("hayami:\n    sections: ["+sections+"]\n"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "--settings", settings)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	// The process's own window, by its id: another panel on the desktop is
	// not this one.
	hwnd, err := waitWindow(cmd.Process.Pid, "hayami", 20*time.Second)
	require.NoError(t, err)
	time.Sleep(settle)
	return hwnd
}

// answering is the names of the devices on this desk that give a level now,
// through the drivers hayami asks on Windows. It asks what the panel will ask,
// getters only, so the test knows what the picture should hold.
//
// Asked up to three times a second apart: a keyboard idle on its receiver
// misses the first questions while its link wakes, and the asking wakes it.
func answering(t *testing.T) []string {
	t.Helper()
	for range 3 {
		if names := askDesk(t); len(names) > 0 {
			return names
		}
		time.Sleep(time.Second)
	}
	return nil
}

// askDesk is one round of answering's questions.
func askDesk(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	found, _ := sanshoku.Scan(ctx, logitech.Driver{}, razer.Driver{}, steelseries.Driver{}, aula.Driver{})
	var names []string
	for _, c := range found {
		dev, err := c.Open(ctx)
		if err != nil {
			continue
		}
		if src, ok := dev.(battery.Source); ok {
			got, _ := src.Batteries(ctx)
			for _, b := range got {
				if b.HasLevel || b.HasBand {
					names = append(names, b.Name)
				}
			}
		}
		_ = dev.Close()
	}
	return names
}

/*
Spec 035. On Windows the peripherals section draws the devices on the desk.

Read off the picture. Which devices answer depends on the desk at that moment
-- a mouse asleep in its dock reads nothing, a keyboard switched to its cable
says nothing through its receiver -- so the test first asks the devices itself
and skips when none gives a level. Then the panel, started with only that
section, must draw a card: a heading line and, under it, a line with a cell's
name and level on it. Before spec 035 the section found no device on Windows,
drew nothing, and the window held no line of text at all.
*/
func TestThePeripheralsAreDrawnOnWindows(t *testing.T) {
	if os.Getenv("HAYAMI_WINDOW_TEST") != "1" {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	live := answering(t)
	if len(live) == 0 {
		t.Skip("no device on this desk gives a level now: wake the mouse or keyboard and run again")
	}
	t.Logf("answering: %v", live)

	hwnd := launch(t, "peripherals", peripheralsSettle)

	// A keyboard idle on its receiver can miss a poll while its link wakes,
	// and the section polls every fifteen seconds; so the picture is taken
	// until it holds a device, for three polls at most.
	var text []span
	var shot string
	for deadline := time.Now().Add(3 * 15 * time.Second); ; time.Sleep(3 * time.Second) {
		img, err := capture(hwnd)
		require.NoError(t, err)
		shot = filepath.Join(t.ArtifactDir(), "peripherals.png")
		require.NoError(t, save(img, shot))
		text = textLines(img)
		if drawsDevices(img, text) || time.Now().After(deadline) {
			break
		}
	}
	t.Logf("picture: %s", shot)
	t.Logf("text lines: %v", text)

	// A card drawing a device says nothing about what is absent: the reasons
	// are dropped. A heading, the cells' names and their levels is three
	// lines at most; a card with no device is the heading, a reason per
	// vendor and two placeholder cells.
	require.NotEmpty(t, text, "no line of text at all")
	require.LessOrEqual(t, len(text), 3,
		"the card is reasons and placeholders, not devices: the section drew none of %v", live)
}

// textLines are the picture's lines of text: bands of ink tall enough to be
// letters rather than a rule.
func textLines(img *image.NRGBA) []span {
	var text []span
	for _, l := range lines(img) {
		if l.To-l.From >= 8 {
			text = append(text, l)
		}
	}
	return text
}

// drawsDevices is whether the card is a heading and cells only.
func drawsDevices(img *image.NRGBA, text []span) bool {
	if len(text) == 0 || len(text) > 3 {
		return false
	}
	last := text[len(text)-1]
	return len(words(img, last, (last.To-last.From+1)*3/4)) >= 1
}
