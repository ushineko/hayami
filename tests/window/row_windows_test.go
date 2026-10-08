package window_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/testenv"
)

// startArranged is start with an arrangement set in the settings file: the
// shared panelSettings has no arrangement, and this is the one test that
// needs one. It writes what start writes and one line more.
func startArranged(t *testing.T, s panelSettings, arrangement string) uintptr {
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
	body := s.yaml() + "    arrangement: " + arrangement + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "--settings", path)
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

/*
Spec 049. The window draws the row arrangement: one line per reading, with no
card heading and no plot (fynedesygn's glance.Lines). Read off the picture:
in stack the cooler's card is its heading and a line per row the section
gives; in row it is the rows alone, so the picture has one line fewer than
the stacked one, and as many as the card has rows. Before this spec the
window stacked a row setting and the two pictures were the same.

The plot draws no text: its blue-led line is not ink (geometry_windows_test),
so it does not count either way; that it is hidden is the library's to test.
*/
func TestTheWindowDrawsRowAsALinePerReading(t *testing.T) {
	// Nothing listens on the discard port: the cooler's rows are the load and
	// card rows and the coolant's reason, whatever this desk runs.
	s := panelSettings{Sections: []string{"cooler"}, LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle}

	stacked := shoot(t, startArranged(t, s, "stack"), "cooler-stack.png")
	t.Logf("stack: %s, lines %v", stacked.path, stacked.text)
	c := cardOf(t, "cooler", s)
	require.Len(t, stacked.text, len(c)+1,
		"the stacked card is its heading and %d lines: %v", len(c), c.labels())

	row := shoot(t, startArranged(t, s, "row"), "cooler-row.png")
	t.Logf("row: %s, lines %v", row.path, row.text)
	require.Len(t, row.text, len(c),
		"in row the card is its %d lines with no heading: %v (picture %s)", len(c), c.labels(), row.path)
}
