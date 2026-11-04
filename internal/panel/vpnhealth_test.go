package panel_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// healthy is a source with one connected VPN and one check, whose outcome the
// test decides. The VPN came up long enough ago to be past its grace period.
func healthy(t *testing.T, check func(context.Context, core.VPNCheck) error, settings config.BandwidthSettings) (*panel.Bandwidth, string) {
	t.Helper()
	dir := tempCache(t)
	b := panel.NewBandwidth(nil, noCounters)
	up := core.VPN{Name: "a", State: core.VPNUp, Since: time.Now().Add(-time.Hour)}
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering(up)}},
		core.VPNCheckers{"fake": check}, settings)
	return b, filepath.Join(dir, "hayami", "vpn-health.jsonl")
}

func checked(extra ...config.VPNItem) config.BandwidthSettings {
	vpns := append([]config.VPNItem{{Name: "a", Checks: []config.VPNCheckItem{{Type: "fake", Target: "invented"}}}}, extra...)
	return config.BandwidthSettings{VPNs: vpns, VPNCheckSeconds: 3600}
}

// storeLines is how many points the store holds, once a check has had time to
// write one.
func storeLines(t *testing.T, path string, want int) []map[string]any {
	t.Helper()
	var lines []map[string]any
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(path) //nolint:gosec // the test's own temporary file
		if err != nil {
			return want == 0
		}
		lines = nil
		for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var m map[string]any
			if json.Unmarshal([]byte(l), &m) == nil {
				lines = append(lines, m)
			}
		}
		return len(lines) >= want
	}, 3*time.Second, 5*time.Millisecond)
	return lines
}

func TestAPassingCheckPutsItsLatencyOnTheRow(t *testing.T) {
	b, path := healthy(t, func(context.Context, core.VPNCheck) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	}, checked())

	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	storeLines(t, path, 1)

	rows := vpnRows(b.Section())
	require.Len(t, rows, 1)
	assert.Regexp(t, regexp.MustCompile(`\d+ ms`), rows[0].Value)
	assert.Contains(t, rows[0].Value, "up")
}

// The tunnel is up and not answering: the state says one and the latency column
// the other, which is the condition the checks exist to find.
func TestAFailingCheckLeavesTheTunnelUpAndSaysFail(t *testing.T) {
	b, path := healthy(t, func(context.Context, core.VPNCheck) error {
		return errors.New("no reply")
	}, checked())

	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	storeLines(t, path, 1)

	rows := vpnRows(b.Section())
	assert.Contains(t, rows[0].Value, "up")
	assert.Contains(t, rows[0].Value, "fail")
}

// A check that could not run says nothing about the tunnel (spec 085 R4, R23).
func TestACheckThatCouldNotRunIsNotAFailure(t *testing.T) {
	b, path := healthy(t, func(context.Context, core.VPNCheck) error {
		return fmt.Errorf("%w: the socket was refused", core.ErrVPNCheckDidNotRun)
	}, checked())

	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	storeLines(t, path, 1)

	rows := vpnRows(b.Section())
	assert.NotContains(t, rows[0].Value, "fail")
	detail, err := b.PollDetail(context.Background(), view.DetailVPN)
	require.NoError(t, err)
	text := detail.Text()
	assert.Contains(t, text, "could not run")
	assert.NotContains(t, text, "Failures")
}

// A second poll inside the interval checks nothing, whichever process asks.
func TestACheckIsNotRepeatedInsideItsInterval(t *testing.T) {
	var calls atomic.Int32
	check := func(context.Context, core.VPNCheck) error {
		calls.Add(1)
		return nil
	}
	b, path := healthy(t, check, checked())
	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	storeLines(t, path, 1)

	// A second source reading the same store stands for a second panel.
	other := panel.NewBandwidth(nil, noCounters)
	up := core.VPN{Name: "a", State: core.VPNUp, Since: time.Now().Add(-time.Hour)}
	other.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering(up)}}, core.VPNCheckers{"fake": check}, checked())
	for _, s := range []*panel.Bandwidth{b, other} {
		_, err := s.Poll(context.Background())
		require.NoError(t, err)
	}
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(1), calls.Load())
}

// A tunnel just up is left alone for its grace period.
func TestATunnelInItsGracePeriodIsNotChecked(t *testing.T) {
	tempCache(t)
	var calls atomic.Int32
	b := panel.NewBandwidth(nil, noCounters)
	up := core.VPN{Name: "a", State: core.VPNUp, Since: time.Now()}
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering(up)}},
		core.VPNCheckers{"fake": func(context.Context, core.VPNCheck) error {
			calls.Add(1)
			return nil
		}}, checked())

	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	assert.Zero(t, calls.Load())
}

// A tunnel that is down has nothing to answer with.
func TestADownTunnelIsNotChecked(t *testing.T) {
	tempCache(t)
	var calls atomic.Int32
	b := panel.NewBandwidth(nil, noCounters)
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering()}},
		core.VPNCheckers{"fake": func(context.Context, core.VPNCheck) error {
			calls.Add(1)
			return nil
		}}, checked())

	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	assert.Zero(t, calls.Load())
}

// What a panel killed mid-write leaves: the last line cut short. Every sound
// line still counts.
func TestAStoreWithATruncatedLastLineLoads(t *testing.T) {
	b, path := healthy(t, func(context.Context, core.VPNCheck) error { return nil }, checked())
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	sound := fmt.Sprintf(`{"at":%q,"vpn":"a","latency_ms":12,"ok":true}`, time.Now().Format(time.RFC3339))
	require.NoError(t, os.WriteFile(path, []byte(sound+"\n"+`{"at":"20`), 0o600))

	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	assert.Contains(t, vpnRows(b.Section())[0].Value, "12 ms")
}

// The store is bounded: it is a cache, and runs for as long as the machine does.
func TestTheStoreDoesNotPassItsCap(t *testing.T) {
	const limit, slack = 10_000, 500
	b, path := healthy(t, func(context.Context, core.VPNCheck) error { return nil }, checked())
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	old := fmt.Sprintf(`{"at":%q,"vpn":"a","latency_ms":1,"ok":true}`, time.Now().Add(-24*time.Hour).Format(time.RFC3339))
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(old+"\n", limit+slack)), 0o600))

	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(path) //nolint:gosec // the test's own temporary file
		return err == nil && strings.Count(string(raw), "\n") == limit
	}, 5*time.Second, 10*time.Millisecond)
}

// The drawer has what the card has no room for, and the card and JSON do not
// (spec 085 R9, R13).
func TestTheDetailPlotsLatencyAndMarksFailures(t *testing.T) {
	b, path := healthy(t, func(context.Context, core.VPNCheck) error { return nil }, checked())
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	at := time.Now().Add(-2 * time.Hour)
	var lines []string
	for i, ok := range []bool{true, false, true} {
		lines = append(lines, fmt.Sprintf(`{"at":%q,"vpn":"a","latency_ms":%d,"ok":%v}`,
			at.Add(time.Duration(i)*time.Minute).Format(time.RFC3339), 10+i, ok))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	d, err := b.PollDetail(context.Background(), view.DetailVPN)
	require.NoError(t, err)

	require.Len(t, d.Parts, 1, "a part per VPN")
	trails := d.Parts[0].Trails
	require.Len(t, trails, 2)
	assert.Equal(t, []float64{10, 11, 12}, trails[0].Samples[:3])
	assert.Equal(t, view.Bad, trails[1].Status)
	assert.Zero(t, trails[1].Samples[0])
	assert.Positive(t, trails[1].Samples[1], "the failure rises to the top of the plot")
	assert.Contains(t, d.Text(), "1 of ")

	// The poll started a cycle, whose point is on its way to the test's directory,
	// and the test's end removes that directory.
	assert.Eventually(t, func() bool {
		raw, err := os.ReadFile(path)
		return err == nil && strings.Count(string(raw), "\n") == len(lines)+1
	}, 5*time.Second, time.Millisecond)
}

func TestTheVPNDetailIsOfferedOnlyWhenOneIsChosen(t *testing.T) {
	none := network(t, []string{"eth0"}, answering())
	assert.Empty(t, none.Details())

	some := network(t, nil, answering(), config.VPNItem{Name: "a"})
	require.Len(t, some.Details(), 1)
	assert.Equal(t, view.DetailVPN, some.Details()[0].ID)
	assert.Equal(t, view.DetailVPN, some.Section().Opens, "the card opens it where there is no process list")
}

// helperEnv makes this test binary a panel in another process.
const helperEnv = "HAYAMI_VPN_HEALTH_HELPER"

// TestVPNHealthHelper is not a test: it is a process for the one below, polling
// until killed.
func TestVPNHealthHelper(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		t.Skip("a helper for TestOneSetOfProbesHoweverManyPanels")
	}
	b := panel.NewBandwidth(nil, noCounters)
	up := core.VPN{Name: "a", State: core.VPNUp, Since: time.Now().Add(-time.Hour)}
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering(up)}},
		core.VPNCheckers{"fake": func(context.Context, core.VPNCheck) error {
			return nil
		}}, config.BandwidthSettings{
			VPNCheckSeconds: 1,
			VPNs:            []config.VPNItem{{Name: "a", Checks: []config.VPNCheckItem{{Type: "fake"}}}},
		})
	for {
		// Poll waits out VPNInterval only for the VPN state: the checks' own
		// interval is what this exercises.
		_, _ = b.Poll(context.Background())
		time.Sleep(20 * time.Millisecond)
	}
}

// Real processes, a real flock and a real file: any number of panels make one set
// of probes per interval, and it does not depend on any one of them staying
// (spec 085 R11).
func TestOneSetOfProbesHoweverManyPanels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no flock, and no VPN systems to use it for (spec 085 R3)")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hayami", "vpn-health.jsonl")

	start := func() *exec.Cmd {
		cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestVPNHealthHelper$") //nolint:gosec // this test binary
		cmd.Env = append(os.Environ(), helperEnv+"=1", "XDG_CACHE_HOME="+dir, "LOCALAPPDATA="+dir)
		require.NoError(t, cmd.Start())
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
		return cmd
	}
	panels := []*exec.Cmd{start(), start(), start()}

	points := func() []time.Time {
		raw, err := os.ReadFile(path) //nolint:gosec // the test's own temporary file
		if err != nil {
			return nil
		}
		var out []time.Time
		for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var p struct {
				At time.Time `json:"at"`
			}
			if json.Unmarshal([]byte(l), &p) == nil {
				out = append(out, p.At)
			}
		}
		return out
	}
	require.Eventually(t, func() bool { return len(points()) >= 3 }, 15*time.Second, 50*time.Millisecond)

	got := points()
	for i := 1; i < len(got); i++ {
		assert.GreaterOrEqual(t, got[i].Sub(got[i-1]), 900*time.Millisecond,
			"point %d came inside the interval of the one before: two panels probed", i)
	}

	// Whichever of them made the last check, each may go; the rest carry on.
	before := len(points())
	require.NoError(t, panels[0].Process.Kill())
	require.NoError(t, panels[1].Process.Kill())
	require.Eventually(t, func() bool { return len(points()) >= before+2 }, 15*time.Second, 50*time.Millisecond)
}
