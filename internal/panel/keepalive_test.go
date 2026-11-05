package panel_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/keepalive"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// fakeTunnels is a VPN system whose tunnels go up and down as they are asked, and
// says what it was asked. A connect waits on slow, when there is one.
type fakeTunnels struct {
	mu    sync.Mutex
	up    map[string]bool
	calls []string
	slow  chan struct{}
}

func (f *fakeTunnels) backend(name string, unattended bool) core.VPNBackend {
	return core.VPNBackend{
		Name: name, Unattended: unattended,
		Active: func(context.Context) ([]core.VPN, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []core.VPN
			for n, on := range f.up {
				if on {
					out = append(out, core.VPN{Name: n, State: core.VPNUp})
				}
			}
			return out, nil
		},
		Configured: func(context.Context) ([]string, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []string
			for n := range f.up {
				out = append(out, n)
			}
			return out, nil
		},
		Connect: func(_ context.Context, vpn string, _ core.SignIn) error {
			f.mu.Lock()
			f.calls = append(f.calls, "connect "+vpn)
			slow := f.slow
			f.mu.Unlock()
			if slow != nil {
				<-slow
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.up[vpn] = true
			return nil
		},
		Disconnect: func(_ context.Context, vpn string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.calls = append(f.calls, "disconnect "+vpn)
			f.up[vpn] = false
			return nil
		},
	}
}

func (f *fakeTunnels) said() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeTunnels) drop(vpn string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.up[vpn] = false
}

// helperBench runs the helper over fake tunnels with the clock racing, so its intervals
// pass in a moment. The locks and the files are real.
type helperBench struct {
	t        *testing.T
	tunnels  *fakeTunnels
	files    keepalive.Files
	checks   atomic.Int64
	pass     atomic.Bool
	notes    atomic.Int64
	settings config.BandwidthSettings
	systems  []core.VPNBackend
	stop     func()
	done     chan error
}

func newHelperBench(t *testing.T, unattended bool, items ...config.VPNItem) *helperBench {
	t.Helper()
	dir := tempCache(t)
	b := &helperBench{
		t: t, tunnels: &fakeTunnels{up: map[string]bool{}}, files: keepalive.At(filepath.Join(dir, "hayami")),
		settings: config.BandwidthSettings{VPNs: items, VPNCheckSeconds: 1, VPNGraceSeconds: 1},
	}
	for _, item := range items {
		b.tunnels.up[item.Name] = true
	}
	b.pass.Store(true)
	b.systems = []core.VPNBackend{b.tunnels.backend("fake", unattended)}
	return b
}

func (b *helperBench) checkers() core.VPNCheckers {
	return core.VPNCheckers{"ping": func(context.Context, core.VPNCheck) error {
		b.checks.Add(1)
		if b.pass.Load() {
			return nil
		}
		return errors.New("no answer") //nolint:err113 // a test's failure
	}}
}

func (b *helperBench) start() {
	b.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var clock atomic.Int64
	base := time.Now()
	b.done = make(chan error, 1)
	go func() {
		b.done <- panel.RunKeepalive(ctx, panel.KeepaliveOptions{
			Systems: b.systems, Checkers: b.checkers(), Files: b.files,
			Settings: func() config.BandwidthSettings { return b.settings },
			Notify:   func(context.Context, string) { b.notes.Add(1) },
			Log:      func(string) {},
			Tick:     time.Millisecond,
			Now:      func() time.Time { return base.Add(time.Duration(clock.Add(1)) * 100 * time.Millisecond) },
		})
	}()
	stopped := false
	b.stop = func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		require.NoError(b.t, <-b.done)
	}
	b.t.Cleanup(b.stop)
}

func (b *helperBench) await(what string, cond func() bool) {
	b.t.Helper()
	require.Eventually(b.t, cond, 20*time.Second, time.Millisecond, what)
}

func checkedVPN(name string, keep bool) config.VPNItem {
	return config.VPNItem{
		Name: name, Keepalive: keep,
		Checks: []config.VPNCheckItem{{Type: "ping", Target: "192.0.2.1"}},
	}
}

func TestAHelperWithNoSystemsSaysSo(t *testing.T) {
	err := panel.RunKeepalive(context.Background(), panel.KeepaliveOptions{})

	assert.ErrorIs(t, err, panel.ErrKeepaliveNoSystems)
}

func TestTheHelperBouncesAFailingTunnelThenDisablesIt(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.pass.Store(false)
	b.start()

	b.await("the disable", func() bool {
		s := b.files.Load()
		return s.IsDisabled("kept")
	})
	b.await("the disconnect", func() bool {
		said := b.tunnels.said()
		return len(said) == 5 && said[4] == "disconnect kept"
	})
	assert.Equal(t, []string{
		"disconnect kept", "connect kept", "disconnect kept", "connect kept", "disconnect kept",
	}, b.tunnels.said(), "two bounces, then down")
	b.await("the notification", func() bool { return b.notes.Load() == 1 })

	// And it does nothing further.
	checks := b.checks.Load()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, checks, b.checks.Load())
	assert.Len(t, b.tunnels.said(), 5)
}

func TestTheHelperRedialsATunnelThatDropped(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.start()
	b.await("a check", func() bool { return b.checks.Load() > 0 })

	b.tunnels.drop("kept")

	b.await("the re-dial", func() bool {
		said := b.tunnels.said()
		return len(said) == 1 && said[0] == "connect kept"
	})
}

func TestTheHelperLeavesAloneOneDownWhenItStarted(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.tunnels.drop("kept")
	b.start()

	time.Sleep(200 * time.Millisecond)

	assert.Empty(t, b.tunnels.said())
}

// A VPN chosen and not switched on is still checked, because no panel can be while the
// helper holds the lock, and is never acted on.
func TestAVPNNotSwitchedOnIsCheckedAndNeverActedOn(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("watched", false))
	b.pass.Store(false)
	b.start()

	b.await("checks", func() bool { return b.checks.Load() > 3 })
	b.tunnels.drop("watched")
	time.Sleep(100 * time.Millisecond)

	assert.Empty(t, b.tunnels.said())
	assert.Zero(t, b.notes.Load())
}

func TestASignInSystemIsNotifiedOnAndNeverActedOn(t *testing.T) {
	b := newHelperBench(t, false, checkedVPN("signin", true))
	b.pass.Store(false)
	b.start()

	b.await("the notification of the failed check", func() bool { return b.notes.Load() == 1 })
	b.tunnels.drop("signin")
	b.await("the notification of the drop", func() bool { return b.notes.Load() == 2 })

	assert.Empty(t, b.tunnels.said(), "no connect and no disconnect")
	s := b.files.Load()
	assert.False(t, s.IsDisabled("signin"))
}

// A second helper finds the first, and the first's end lets another start.
func TestOnlyOneHelperRunsAtATime(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.start()
	b.await("the first to run", b.files.Running)

	err := panel.RunKeepalive(context.Background(), panel.KeepaliveOptions{
		Systems: b.systems, Files: b.files, Log: func(string) {},
		Settings: func() config.BandwidthSettings { return b.settings },
	})
	require.ErrorIs(t, err, panel.ErrKeepaliveRunning)

	b.stop()
	assert.False(t, b.files.Running())
}

// A panel polling beside the helper reads what it writes and probes nothing, and takes
// over when it stops (spec 085 R22).
func TestAPanelProbesNothingWhileTheHelperRunsAndTakesOverAfter(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.start()
	b.await("the helper's first check", func() bool { return b.checks.Load() > 0 })

	var probes atomic.Int64
	checkers := core.VPNCheckers{"ping": func(context.Context, core.VPNCheck) error {
		probes.Add(1)
		return nil
	}}
	source := panel.NewBandwidth(nil, noCounters)
	source.SetVPNSystems(b.systems, checkers, b.settings)

	for range 20 {
		source.ForgetVPNReadTime()
		_, err := source.Poll(context.Background())
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}
	assert.Zero(t, probes.Load())

	b.stop()
	// The store's last point is on a clock of the test's making, so it is no guide to
	// what is due: a fresh store is one with nothing checked.
	path := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "hayami", "vpn-health.jsonl")
	require.NoError(t, os.Remove(path))
	require.Eventually(t, func() bool {
		source.ForgetVPNReadTime()
		_, err := source.Poll(context.Background())
		return err == nil && probes.Load() > 0
	}, 20*time.Second, 10*time.Millisecond, "a panel takes over")
}

// A press reaches the helper through the file, and the disconnect stays down.
func TestADisconnectPressedInAPanelStaysDownUntilAConnectIsPressed(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.start()
	b.await("a check", func() bool { return b.checks.Load() > 0 })
	source := panel.NewBandwidth(nil, noCounters)
	source.SetVPNSystems(b.systems, nil, b.settings)

	require.NoError(t, source.ControlVPN(context.Background(), view.VPNDisconnect, "kept", nil))

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, []string{"disconnect kept"}, b.tunnels.said(), "not re-dialled")
	s := b.files.Load()
	assert.True(t, s.IsHeld("kept"))

	require.NoError(t, source.ControlVPN(context.Background(), view.VPNConnect, "kept", nil))
	assert.Equal(t, []string{"disconnect kept", "connect kept"}, b.tunnels.said())
	s = b.files.Load()
	assert.False(t, s.IsHeld("kept"))

	// And a drop after that is re-dialled again.
	b.await("the helper to see it up", func() bool { return b.checks.Load() > 1 })
	b.tunnels.drop("kept")
	b.await("the re-dial", func() bool { return len(b.tunnels.said()) == 3 })
}

// A press counts with no helper running, which a socket would not.
func TestAHoldIsWrittenWithNoHelperRunning(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	source := panel.NewBandwidth(nil, noCounters)
	source.SetVPNSystems(b.systems, nil, b.settings)

	require.NoError(t, source.ControlVPN(context.Background(), view.VPNDisconnect, "kept", nil))

	s := b.files.Load()
	assert.True(t, s.IsHeld("kept"))
}

// A bounce a person presses is not a drop to race, and when it is over a drop is one.
func TestABouncePressedHoldsOnlyForItsLength(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	source := panel.NewBandwidth(nil, noCounters)
	source.SetVPNSystems(b.systems, nil, b.settings)

	require.NoError(t, source.ControlVPN(context.Background(), view.VPNBounce, "kept", nil))

	s := b.files.Load()
	assert.False(t, s.IsHeld("kept"))
	assert.Equal(t, []string{"disconnect kept", "connect kept"}, b.tunnels.said())
}

// The helper reading a tunnel down while a person's connect is under way must not dial
// it as well (found on a real tunnel: NetworkManager refused the second activation).
func TestTheHelperDoesNotDialAgainstAConnectPressed(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true))
	b.start()
	b.await("a check", func() bool { return b.checks.Load() > 0 })
	source := panel.NewBandwidth(nil, noCounters)
	source.SetVPNSystems(b.systems, nil, b.settings)
	require.NoError(t, source.ControlVPN(context.Background(), view.VPNDisconnect, "kept", nil))

	slow := make(chan struct{})
	b.tunnels.mu.Lock()
	b.tunnels.slow = slow
	b.tunnels.mu.Unlock()
	pressed := make(chan error, 1)
	go func() { pressed <- source.ControlVPN(context.Background(), view.VPNConnect, "kept", nil) }()
	b.await("the connect", func() bool { return len(b.tunnels.said()) == 2 })

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, []string{"disconnect kept", "connect kept"}, b.tunnels.said(), "one dial, the person's")

	close(slow)
	require.NoError(t, <-pressed)
	s := b.files.Load()
	assert.False(t, s.IsHeld("kept"), "over when the press is")
}

// A disconnect that replaces a press under way means to stay down, whichever of the two
// finishes first.
func TestAPressReplacedLeavesTheHoldToTheOneThatReplacedIt(t *testing.T) {
	for _, action := range []string{view.VPNConnect, view.VPNBounce} {
		t.Run(action, func(t *testing.T) {
			b := newHelperBench(t, true, checkedVPN("kept", true))
			source := panel.NewBandwidth(nil, noCounters)
			source.SetVPNSystems(b.systems, nil, b.settings)
			_, err := source.Poll(context.Background())
			require.NoError(t, err)
			slow := make(chan struct{})
			b.tunnels.slow = slow

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pressed := make(chan struct{})
			go func() {
				_ = source.ControlVPN(ctx, action, "kept", nil)
				close(pressed)
			}()
			b.await("the connect", func() bool {
				return slices.Contains(b.tunnels.said(), "connect kept")
			})

			cancel()
			require.NoError(t, source.ControlVPN(context.Background(), view.VPNDisconnect, "kept", nil))
			close(slow)
			<-pressed

			s := b.files.Load()
			assert.True(t, s.IsHeld("kept"))
		})
	}
}

func TestTheDrawerSaysWhatTheHelperIsDoing(t *testing.T) {
	b := newHelperBench(t, true, checkedVPN("kept", true), checkedVPN("other", false))
	source := panel.NewBandwidth(nil, noCounters)
	source.SetVPNSystems(b.systems, nil, b.settings)
	_, err := source.Poll(context.Background())
	require.NoError(t, err)
	says := func() string {
		d, err := source.PollDetail(context.Background(), view.DetailVPN)
		require.NoError(t, err)
		return d.Parts[0].Text()
	}
	require.NotContains(t, func() string {
		d, err := source.PollDetail(context.Background(), view.DetailVPN)
		require.NoError(t, err)
		return d.Parts[1].Text()
	}(), "Keepalive", "not on one it is not switched on for")

	assert.Contains(t, says(), "helper not running")

	release, ok, err := b.files.HoldRunning()
	require.NoError(t, err)
	require.True(t, ok)
	defer release()
	assert.Regexp(t, `Keepalive\s+on`, says())

	require.NoError(t, b.files.Update(func(s *keepalive.State) { s.SetRetry("kept", 2, time.Now().Add(time.Minute)) }))
	assert.Contains(t, says(), "retry 3 in")

	require.NoError(t, b.files.Update(func(s *keepalive.State) { s.Hold("kept") }))
	assert.Contains(t, says(), "held down by you")

	require.NoError(t, b.files.Update(func(s *keepalive.State) { s.Disable("kept", "failed 3 checks in a row") }))
	assert.Contains(t, says(), "failed 3 checks in a row")

	require.NoError(t, source.ControlVPN(context.Background(), view.VPNReenable, "kept", nil))
	assert.False(t, strings.Contains(says(), "failed 3 checks in a row"))
}
