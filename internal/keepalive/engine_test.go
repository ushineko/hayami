package keepalive_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/keepalive"
)

const nm, ov3 = "NetworkManager", "OpenVPN3"

var errDial = errors.New("did not connect")

// bench is an engine, a clock and a state, driving one VPN through a scene.
type bench struct {
	t      *testing.T
	e      *keepalive.Engine
	s      keepalive.State
	now    time.Time
	name   string
	system string
	o      keepalive.Options
}

func newBench(t *testing.T, system string) *bench {
	t.Helper()
	return &bench{
		t: t, e: keepalive.New(map[string]bool{nm: true, ov3: false}),
		now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC), name: "invented", system: system,
		o: keepalive.Options{Checked: true},
	}
}

var timing = keepalive.Timing{Every: 120 * time.Second, Grace: 15 * time.Second}

func (b *bench) read(state core.VPNState) []keepalive.Action {
	v := core.VPN{Name: b.name, State: state}
	if state == core.VPNUp {
		v.Backend = b.system
	}
	return b.e.Observe(b.now, v, b.o, timing, &b.s)
}

func (b *bench) pass(d time.Duration) { b.now = b.now.Add(d) }

func kinds(actions []keepalive.Action) []keepalive.Kind {
	out := []keepalive.Kind{}
	for _, a := range actions {
		out = append(out, a.Kind)
	}
	return out
}

// settled is a tunnel seen up, past its grace, at the point a check is asked for.
func (b *bench) settled() {
	b.t.Helper()
	require.Empty(b.t, b.read(core.VPNUp), "the grace period")
	b.pass(16 * time.Second)
	require.Equal(b.t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))
}

// failCheck ends the check under way as a failure, and asks for the next.
func (b *bench) failCheck() []keepalive.Kind {
	b.t.Helper()
	got := kinds(b.e.CheckDone(b.name, keepalive.Failed, &b.s))
	return got
}

func (b *bench) nextCheck() {
	b.t.Helper()
	b.pass(121 * time.Second)
	require.Equal(b.t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))
}

// R23: only a connected tunnel is checked, after its grace period.
func TestOnlyAConnectedTunnelIsCheckedAfterItsGrace(t *testing.T) {
	b := newBench(t, nm)

	assert.Empty(t, b.read(core.VPNDown), "down: nothing to check")
	assert.Empty(t, b.read(core.VPNConnecting), "coming up: not yet")
	assert.Empty(t, b.read(core.VPNUp), "just up: in its grace")
	b.pass(14 * time.Second)
	assert.Empty(t, b.read(core.VPNUp), "still in it")
	b.pass(2 * time.Second)
	assert.Equal(t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))
}

func TestAChecksIntervalIsKept(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	assert.Empty(t, b.e.CheckDone(b.name, keepalive.Passed, &b.s))

	b.pass(119 * time.Second)
	assert.Empty(t, b.read(core.VPNUp))
	b.pass(2 * time.Second)
	assert.Equal(t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))
}

func TestAVPNWithNoChecksIsNotChecked(t *testing.T) {
	b := newBench(t, nm)
	b.o.Checked = false
	b.read(core.VPNUp)
	b.pass(time.Hour)

	assert.Empty(t, b.read(core.VPNUp))
}

// R23: a failure below the threshold bounces.
func TestAFailureBelowTheThresholdBounces(t *testing.T) {
	b := newBench(t, nm)
	b.settled()

	assert.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	assert.False(t, b.s.IsDisabled(b.name))
}

// R23: a pass resets the failure count, so failures must be in a row.
func TestAPassResetsTheFailureCount(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	require.Empty(t, b.e.BounceDone(b.now, b.name, b.o, nil, &b.s))
	b.nextCheck()
	require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	require.Empty(t, b.e.BounceDone(b.now, b.name, b.o, nil, &b.s))
	b.nextCheck()
	require.Equal(t, 2, b.e.Failures(b.name))

	require.Empty(t, b.e.CheckDone(b.name, keepalive.Passed, &b.s))

	assert.Zero(t, b.e.Failures(b.name))
	b.nextCheck()
	assert.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck(), "the count started again")
}

// R23: a failure at the threshold disconnects and disables, and says why.
func TestAFailureAtTheThresholdDisconnectsAndDisables(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	for range keepalive.FailureThreshold - 1 {
		require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
		require.Empty(t, b.e.BounceDone(b.now, b.name, b.o, nil, &b.s))
		b.nextCheck()
	}

	got := b.e.CheckDone(b.name, keepalive.Failed, &b.s)

	assert.Equal(t, []keepalive.Kind{keepalive.DisconnectAndDisable, keepalive.Notify}, kinds(got))
	assert.True(t, b.s.IsDisabled(b.name))
	assert.Contains(t, b.s.Disabled[b.name], "3 checks")
}

// R23: the helper does nothing further until a person re-enables it.
func TestADisabledVPNIsLeftAloneUntilReenabled(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	require.Empty(t, b.e.BounceDone(b.now, b.name, b.o, nil, &b.s))
	b.s.Disable(b.name, "failed 3 checks in a row")

	assert.Empty(t, b.read(core.VPNDown), "not re-dialled")
	b.pass(time.Hour)
	assert.Empty(t, b.read(core.VPNUp), "not checked")

	b.s.Enable(b.name)
	assert.Equal(t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))
	assert.Zero(t, b.e.Failures(b.name), "its count starts again")
}

// R23's addition: a probe that could not run counts as no result.
func TestAProbeThatCouldNotRunCountsForNothing(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	require.Empty(t, b.e.BounceDone(b.now, b.name, b.o, nil, &b.s))
	b.nextCheck()

	assert.Empty(t, b.e.CheckDone(b.name, keepalive.NoResult, &b.s))

	assert.Equal(t, 1, b.e.Failures(b.name), "neither a failure nor a pass")
}

// R4: a system that would not answer is not a tunnel that dropped.
func TestASystemThatWouldNotAnswerIsNotADrop(t *testing.T) {
	b := newBench(t, nm)
	b.settled()

	assert.Empty(t, b.read(core.VPNUnknown))
	assert.Empty(t, b.e.Observe(b.now, core.VPN{Name: b.name, State: core.VPNDown, Stale: true}, b.o, timing, &b.s))
}

// R24: the helper re-dials a tunnel that dropped.
func TestATunnelThatDroppedIsRedialled(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)

	assert.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
	assert.Empty(t, b.read(core.VPNDown), "one at a time")
}

// R24: and never opens a new one.
func TestAVPNNotSeenConnectedIsLeftAlone(t *testing.T) {
	b := newBench(t, nm)

	assert.Empty(t, b.read(core.VPNDown))
	b.pass(time.Hour)
	assert.Empty(t, b.read(core.VPNDown))
}

// R24: an outage never disables a VPN, however long.
func TestAnOutageNeverDisables(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	for range 50 {
		require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
		b.e.RedialDone(b.now, b.name, errDial, &b.s)
		b.pass(keepalive.BackoffMax + time.Second)
	}

	assert.False(t, b.s.IsDisabled(b.name))
}

// R24: a failed re-dial backs off from 30 s, doubling, to 600 s, and says so.
func TestAFailedRedialBacksOffAndIsVisible(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	wants := []time.Duration{30, 60, 120, 240, 480, 600, 600}
	for i, want := range wants {
		want *= time.Second
		require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)), "attempt %d", i)
		said := b.e.RedialDone(b.now, b.name, errDial, &b.s)
		if i == 0 {
			require.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(said))
			assert.Contains(t, said[0].Text, "outside the tunnel", "the first failure of an outage is said")
		} else {
			require.Empty(t, said, "and only the first")
		}

		retry := b.s.Retries[b.name]
		assert.Equal(t, i+1, retry.Attempts, "each retry is visible")
		assert.Equal(t, b.now.Add(want), retry.At)

		b.pass(want - time.Second)
		assert.Empty(t, b.read(core.VPNDown), "attempt %d waits its %s", i, want)
		b.pass(time.Second)
	}
}

// R24: a successful re-dial resets the backoff and starts a new grace period.
func TestASuccessfulRedialResetsTheBackoffAndTheGrace(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	b.pass(time.Hour)
	require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
	b.e.RedialDone(b.now, b.name, errDial, &b.s)
	b.pass(31 * time.Second)
	require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))

	require.Empty(t, b.e.RedialDone(b.now, b.name, nil, &b.s))

	assert.Empty(t, b.s.Retries[b.name])
	assert.Empty(t, b.read(core.VPNUp), "a new grace period")
	b.pass(16 * time.Second)
	assert.Equal(t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))

	// The next outage starts from the first wait.
	require.Empty(t, b.e.CheckDone(b.name, keepalive.Passed, &b.s))
	require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
	b.e.RedialDone(b.now, b.name, errDial, &b.s)
	assert.Equal(t, b.now.Add(30*time.Second), b.s.Retries[b.name].At)
}

// R24: a bounce whose reconnect failed joins the re-dials.
func TestABounceThatLeftItDownJoinsTheRedials(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	assert.Empty(t, b.read(core.VPNDown), "the bounce has it")

	said := b.e.BounceDone(b.now, b.name, b.o, errDial, &b.s)

	require.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(said), "a bounce that left traffic outside the tunnel is said")
	assert.Equal(t, b.now.Add(30*time.Second), b.s.Retries[b.name].At)
	b.pass(31 * time.Second)
	assert.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
}

// R24: re-dialling can be switched off per VPN.
func TestRedialCanBeSwitchedOffForAVPN(t *testing.T) {
	b := newBench(t, nm)
	b.o.NoRedial = true
	b.settled()

	assert.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(b.read(core.VPNDown)), "never a re-dial, but said")
	assert.Empty(t, b.read(core.VPNDown), "once")
	b.e.CheckDone(b.name, keepalive.NoResult, &b.s)
	b.read(core.VPNUp)

	// And a bounce that left it down stays down, and is said.
	b.pass(time.Hour)
	require.Equal(t, []keepalive.Kind{keepalive.RunChecks}, kinds(b.read(core.VPNUp)))
	require.Equal(t, []keepalive.Kind{keepalive.Bounce}, b.failCheck())
	assert.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(b.e.BounceDone(b.now, b.name, b.o, errDial, &b.s)))
	assert.Empty(t, b.s.Retries[b.name])
}

// An outage that ends and another that starts are two things to say.
func TestEachOutageIsSaidOnce(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	outage := func() {
		t.Helper()
		require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
		require.Len(t, b.e.RedialDone(b.now, b.name, errDial, &b.s), 1)
		b.pass(31 * time.Second)
		require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
		require.Empty(t, b.e.RedialDone(b.now, b.name, errDial, &b.s))
	}
	outage()
	b.read(core.VPNUp)
	outage()
}

// A person's own disconnect is no outage to be told of.
func TestAFailedDialOnAHeldTunnelIsNotSaid(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
	b.s.Hold(b.name)
	assert.Empty(t, b.e.RedialDone(b.now, b.name, errDial, &b.s))
}

// R24: a disconnect pressed in hayami suppresses re-dialling.
func TestAPressedDisconnectStaysDown(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	b.s.Hold(b.name)

	assert.Empty(t, b.read(core.VPNUp), "the press is on its way: the hold stands while it is read up")
	require.True(t, b.s.IsHeld(b.name))
	assert.Empty(t, b.read(core.VPNDown))
	b.pass(time.Hour)
	assert.Empty(t, b.read(core.VPNDown))
	assert.True(t, b.s.IsHeld(b.name))
}

// R24: and cancels any retry waiting.
func TestAPressedDisconnectCancelsARetryWaiting(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
	b.e.RedialDone(b.now, b.name, errDial, &b.s)
	require.NotEmpty(t, b.s.Retries[b.name])

	b.s.Hold(b.name)
	b.pass(time.Hour)

	assert.Empty(t, b.read(core.VPNDown))
	assert.Empty(t, b.s.Retries[b.name])
}

func TestAPressedDisconnectCancelsARedialUnderWay(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	require.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))

	b.s.Hold(b.name)

	assert.Equal(t, []keepalive.Kind{keepalive.CancelRedial}, kinds(b.read(core.VPNDown)))
	assert.Empty(t, b.e.RedialDone(b.now, b.name, errDial, &b.s))
	assert.Empty(t, b.s.Retries[b.name], "a cancelled re-dial is not retried")
}

// A press on connect releases the hold, so a later drop is re-dialled again.
func TestAConnectAfterAPressedDisconnectBringsBackRedialling(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	b.s.Hold(b.name)
	require.Empty(t, b.read(core.VPNDown))

	b.s.Release(b.name)
	require.Empty(t, b.read(core.VPNUp))

	assert.Equal(t, []keepalive.Kind{keepalive.Redial}, kinds(b.read(core.VPNDown)))
}

// Brought back with no press, by nmcli or autoconnect: the person's hold is over, as
// vpn-toggle's is on seeing it connected.
func TestATunnelBroughtBackWithNoPressEndsItsHold(t *testing.T) {
	b := newBench(t, nm)
	b.read(core.VPNUp)
	b.s.Hold(b.name)
	require.Empty(t, b.read(core.VPNDown))

	require.Empty(t, b.read(core.VPNUp))

	assert.False(t, b.s.IsHeld(b.name))
}

// A failing check on a tunnel a person is taking down says nothing of the tunnel.
func TestAFailedCheckOnAHeldTunnelIsNotCounted(t *testing.T) {
	b := newBench(t, nm)
	b.settled()
	b.s.Hold(b.name)

	assert.Empty(t, b.failCheck())
	assert.Zero(t, b.e.Failures(b.name))
}

// R25: an OpenVPN3 tunnel is checked and notified on, and never acted on.
func TestASignInTunnelIsNotifiedOnAndNeverActedOn(t *testing.T) {
	b := newBench(t, ov3)
	b.settled()

	failed := b.e.CheckDone(b.name, keepalive.Failed, &b.s)
	require.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(failed), "a failed check")
	assert.Contains(t, failed[0].Text, b.name)
	assert.Zero(t, b.e.Failures(b.name), "nothing counts toward a disconnect")

	b.nextCheck()
	assert.Empty(t, b.e.CheckDone(b.name, keepalive.Failed, &b.s), "once, until it passes")
	b.nextCheck()
	require.Empty(t, b.e.CheckDone(b.name, keepalive.Passed, &b.s))
	b.nextCheck()
	assert.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(b.e.CheckDone(b.name, keepalive.Failed, &b.s)))

	dropped := b.read(core.VPNDown)
	require.Equal(t, []keepalive.Kind{keepalive.Notify}, kinds(dropped), "a drop")
	b.pass(time.Hour)
	assert.Empty(t, b.read(core.VPNDown), "once, until it comes back")
	assert.False(t, b.s.IsDisabled(b.name))
}

func TestAStateFileLastsAndAStrangeOneIsEmpty(t *testing.T) {
	dir := t.TempDir()
	files := keepalive.At(dir)
	empty := files.Load()
	assert.False(t, empty.IsHeld("a"), "no file")

	require.NoError(t, files.Update(func(s *keepalive.State) { s.Hold("a") }))
	require.NoError(t, files.Update(func(s *keepalive.State) { s.Disable("b", "why") }))

	got := files.Load()
	assert.True(t, got.IsHeld("a"), "one writer does not lose the other's")
	assert.Equal(t, "why", got.Disabled["b"])

	require.NoError(t, os.WriteFile(filepath.Join(dir, "vpn-keepalive.json"), []byte("{not json"), 0o600))
	strange := files.Load()
	assert.False(t, strange.IsHeld("a"))
}

func TestOnlyOneHelperRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no lock here to find a helper by, and no VPN systems for one to run over (spec 085 R3)")
	}
	files := keepalive.At(t.TempDir())
	assert.False(t, files.Running())

	release, ok, err := files.HoldRunning()
	require.NoError(t, err)
	require.True(t, ok)
	defer release()

	assert.True(t, files.Running())
	_, again, err := files.HoldRunning()
	require.NoError(t, err)
	assert.False(t, again)
}
