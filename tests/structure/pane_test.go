package structure_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The usage pane, drawn by the real binary in a real terminal, is laid out the
way the usage widget's --tui lays it out (issue #75).

The claim is about what a terminal shows, so it is made against one: tmux at a
fixed size, running hayami-tui, read back with capture-pane. The view's own
tests say what the lines contain; this says the program puts them on screen.

Everything it reads is invented and lives under t.TempDir(): a cache with three
accounts, a home with no credential store, and a PATH with nothing on it, so
there is nothing to fetch with and nothing is fetched.
*/
func TestTheUsagePaneIsLaidOutLikeTheWidgets(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}

	root := repoRoot(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "hayami-tui")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./cmd/hayami-tui")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s", out)

	home := filepath.Join(dir, "home")
	cache := filepath.Join(dir, "cache", "claude-usage-widget")
	require.NoError(t, os.MkdirAll(home, 0o700))
	require.NoError(t, os.MkdirAll(cache, 0o700))
	writeInventedCache(t, cache, time.Now())

	const width, height = 200, 6
	socket := "hayami-pane-test-" + fmt.Sprint(os.Getpid())
	env := []string{
		"HOME=" + home,
		"PATH=" + filepath.Join(dir, "nothing"),
		"XDG_CACHE_HOME=" + filepath.Join(dir, "cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "config"),
		"CLAUDE_USAGE_PROFILE_ROOT=" + filepath.Join(dir, "profiles"),
		"TERM=xterm-256color",
		"NO_COLOR=1",
	}
	start := exec.CommandContext(t.Context(), tmux, "-L", socket, "-f", "/dev/null",
		"new-session", "-d", "-x", fmt.Sprint(width), "-y", fmt.Sprint(height),
		bin+" --sections usage --arrangement row")
	start.Env = env
	out, err = start.CombinedOutput()
	require.NoError(t, err, "%s", out)
	// Cleanup runs after t.Context() is cancelled, so it has its own.
	t.Cleanup(func() { _ = exec.CommandContext(context.Background(), tmux, "-L", socket, "kill-server").Run() })

	var lines []string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got, err := exec.CommandContext(t.Context(), tmux, "-L", socket,
			"capture-pane", "-p").Output()
		require.NoError(t, err)
		lines = nonEmpty(string(got))
		if len(lines) == 3 && strings.Contains(lines[2], "resets") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.Len(t, lines, 3, "the pane drew %q", lines)

	// No credential store, so no badges: the name column is the widest
	// name, then two spaces, then the window, blank for a budget.
	assert.True(t, strings.HasPrefix(lines[0], "max    5h ━"), "%q", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "work      ━"), "%q", lines[1])
	assert.True(t, strings.HasPrefix(lines[2], "Codex  5h ━"), "%q", lines[2])

	assert.Contains(t, lines[0], " 12%  ·  7d 85% ")
	assert.Contains(t, lines[1], " $250.00 / $1000.00 (25%) ")
	assert.Contains(t, lines[2], " 3%  ·  individual 300.5/1200 (60%) ")

	for _, l := range lines {
		assert.Equal(t, width, len([]rune(l)), "a line does not fill the pane: %q", l)
	}
	assert.Regexp(t, `resets \d+h \d+m$`, lines[0])
	assert.Regexp(t, `resets [A-Z][a-z]{2} \d+$`, lines[1])

	// The bars end in one column, and the figures after them begin in the
	// next-but-one; the reset floats at the edge with a gap before it.
	ends := map[int]bool{}
	for _, l := range lines {
		ends[strings.LastIndexAny(l, "━─")] = true
	}
	assert.Len(t, ends, 1, "the bars end in different columns: %q", lines)
	assert.Contains(t, lines[0], "     resets", "no gap before the reset")
}

// writeInventedCache writes three accounts' readings in the shared cache's
// format. Every number is made up.
func writeInventedCache(t *testing.T, dir string, now time.Time) {
	t.Helper()
	fetched := float64(now.Unix())
	gate := fetched + 3600
	iso := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }

	files := map[string]string{
		"usage-max.json": fmt.Sprintf(`{"five_hour": {"utilization": 12, "resets_at": %q},
			"seven_day": {"utilization": 85, "resets_at": %q}}`,
			iso(2*time.Hour+30*time.Minute), iso(4*24*time.Hour)),
		"usage-work.json": `{"five_hour": null, "seven_day": null, "spend": {
			"enabled": true, "percent": 25, "severity": "normal",
			"used":  {"amount_minor": 25000, "currency": "USD", "exponent": 2},
			"limit": {"amount_minor": 100000, "currency": "USD", "exponent": 2}}}`,
		"usage-codex.json": fmt.Sprintf(`{"provider": "codex",
			"primary": {"utilization": 3, "window_minutes": 300, "resets_at": %d},
			"individual_limit": {"utilization": 60, "used": "300.50", "limit": "1200.00"}}`,
			now.Add(4*time.Hour).Unix()),
	}
	for name, data := range files {
		entry := fmt.Sprintf(`{"next_attempt_at": %f, "fetched_at": %f, "data": %s}`, gate, fetched, data)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(entry), 0o600))
	}
}

// nonEmpty is a capture's lines with the blank ones at the bottom dropped.
func nonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
