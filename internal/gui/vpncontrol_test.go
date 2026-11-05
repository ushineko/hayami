package gui_test

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

// vpnSource is a section with one VPN part in the drawer, recording the presses that
// reach it. A press does what the test's answer says.
type vpnSource struct {
	*detailSource

	mu      sync.Mutex
	presses []string
	answer  func(ctx context.Context, action string, signIn core.SignIn) error
}

func (v *vpnSource) ControlVPN(ctx context.Context, action, name string, signIn core.SignIn) error {
	v.mu.Lock()
	v.presses = append(v.presses, action+" "+name)
	answer := v.answer
	v.mu.Unlock()
	if answer != nil {
		return answer(ctx, action, signIn)
	}
	return nil
}

func (v *vpnSource) pressed() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.presses...)
}

// vpnDrawer is a panel with its drawer open on two VPNs, the first in state and the
// second up, so a press can be seen to land on the one it was meant for.
func vpnDrawer(t *testing.T, state view.VPNState) (*gui.Panel, *vpnSource, func(string, func() bool)) {
	t.Helper()
	detail := view.VPNDetail("net", []view.VPNDetailReading{
		{VPNReading: view.VPNReading{Name: "invented", State: state}},
		{VPNReading: view.VPNReading{Name: "other", State: view.VPNUp}},
	})
	src := &vpnSource{detailSource: newDetailSource("net", view.Section{Title: "Network"}, map[string]view.Detail{view.DetailVPN: detail})}
	src.section.Opens = view.DetailVPN
	p, await := drawerPanel(t, src)
	gui.OpenURLWith(p, func(*url.URL) error { t.Error("no test opens a page unless it says so"); return nil })
	gui.Tap(p, "net", "", -1)
	await("the drawer's parts", func() bool { return len(gui.DrawerPartHeads(p)) == 2 })
	return p, src, await
}

func partSays(p *gui.Panel, part int, words string) bool {
	return strings.Contains(strings.Join(gui.DrawerPartLines(p, part), "\n"), words)
}

func TestAVPNOffersWhatItsStateAllows(t *testing.T) {
	for state, want := range map[view.VPNState][]string{
		view.VPNDown:       {"Connect"},
		view.VPNConnecting: {"Disconnect"},
		view.VPNUp:         {"Disconnect", "Bounce"},
		view.VPNUnknown:    {},
	} {
		p, _, _ := vpnDrawer(t, state)
		for _, label := range []string{"Connect", "Disconnect", "Bounce"} {
			offered := false
			for _, line := range gui.DrawerPartLines(p, 0) {
				offered = offered || strings.Contains(line, label)
			}
			assert.Equal(t, slices.Contains(want, label), offered, "state %v offering %s", state, label)
		}
	}
}

// Only a row that does something is marked: nothing else says it can be pressed.
func TestARowToPressIsMarkedAndAReadingIsNot(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNDown)
	src.answer = func(context.Context, string, core.SignIn) error { return core.ErrVPNDenied }
	require.True(t, gui.PressVPNRow(p, 0, "Connect"))
	await("the refusal", func() bool { return partSays(p, 0, "Last press") })

	marker := strings.TrimSuffix(gui.VPNPressMarker(p), " ")
	require.NotEmpty(t, marker)
	for _, line := range gui.DrawerPartLines(p, 0) {
		assert.Equal(t, strings.Contains(line, "Connect"), strings.HasPrefix(line, marker), line)
	}
}

func TestConnectTakesOnePress(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNDown)

	require.True(t, gui.PressVPNRow(p, 0, "Connect"))

	await("the press", func() bool { return len(src.pressed()) == 1 })
	assert.Equal(t, []string{"connect invented"}, src.pressed())
}

func TestDisconnectAndBounceTakeTwoPresses(t *testing.T) {
	for label, action := range map[string]string{"Disconnect": "disconnect", "Bounce": "bounce"} {
		t.Run(label, func(t *testing.T) {
			p, src, await := vpnDrawer(t, view.VPNUp)

			require.True(t, gui.PressVPNRow(p, 0, label))

			assert.Empty(t, src.pressed(), "the first press only asks")
			assert.True(t, partSays(p, 0, "Confirm?"), "and the row says so")
			assert.False(t, partSays(p, 1, "Confirm?"), "on that VPN and not the next")

			require.True(t, gui.PressVPNRow(p, 0, label))

			await("the press", func() bool { return len(src.pressed()) == 1 })
			assert.Equal(t, []string{action + " invented"}, src.pressed())
			assert.False(t, partSays(p, 0, "Confirm?"))
		})
	}
}

func TestASecondPressOnAnotherRowAnswersNothing(t *testing.T) {
	p, src, _ := vpnDrawer(t, view.VPNUp)

	require.True(t, gui.PressVPNRow(p, 0, "Disconnect"))
	require.True(t, gui.PressVPNRow(p, 1, "Disconnect"))

	assert.True(t, partSays(p, 1, "Confirm?"), "the other VPN's row asked its own question")
	assert.False(t, partSays(p, 0, "Confirm?"), "and the first one's is gone")
	assert.Empty(t, src.pressed())
}

func TestAQuestionNotAnsweredLapses(t *testing.T) {
	p, src, _ := vpnDrawer(t, view.VPNUp)

	require.True(t, gui.PressVPNRow(p, 0, "Disconnect"))
	gui.LapseVPNQuestion(p)
	require.True(t, gui.PressVPNRow(p, 0, "Disconnect"))

	assert.Empty(t, src.pressed(), "a click long after the question is a new first press")
	assert.True(t, partSays(p, 0, "Confirm?"))
}

func TestARefusedPressIsSaidUnderThatVPN(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNDown)
	src.answer = func(context.Context, string, core.SignIn) error { return core.ErrVPNDenied }

	require.True(t, gui.PressVPNRow(p, 0, "Connect"))

	await("the refusal", func() bool { return partSays(p, 0, core.ErrVPNDenied.Error()) })
	assert.False(t, partSays(p, 1, core.ErrVPNDenied.Error()))
	assert.True(t, partSays(p, 0, "Connect"), "it was not connected, and can be pressed again")
}

func TestAPressThatWorkedLeavesNoWord(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNDown)
	release := make(chan struct{})
	src.answer = func(context.Context, string, core.SignIn) error { <-release; return nil }

	require.True(t, gui.PressVPNRow(p, 0, "Connect"))
	assert.True(t, partSays(p, 0, "connecting"), "it says it is under way")

	close(release)
	await("the word to go", func() bool { return !partSays(p, 0, "Last press") })
}

// The address goes to the browser, and the row never draws it.
func TestASignInGoesToTheBrowserAndIsNotDrawn(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNDown)
	var opened []string
	var mu sync.Mutex
	gui.OpenURLWith(p, func(u *url.URL) error {
		mu.Lock()
		defer mu.Unlock()
		opened = append(opened, u.String())
		return nil
	})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	src.answer = func(_ context.Context, _ string, signIn core.SignIn) error {
		signIn(&url.URL{Scheme: "https", Host: "sign-in.invalid", Path: "/page"})
		<-release
		return nil
	}

	require.True(t, gui.PressVPNRow(p, 0, "Connect"))

	await("the row to say so", func() bool { return partSays(p, 0, "sign in in your browser") })
	mu.Lock()
	assert.Equal(t, []string{"https://sign-in.invalid/page"}, opened)
	mu.Unlock()
	assert.False(t, partSays(p, 0, "sign-in.invalid"))
}

// A second connect would close the session waiting at its sign-in page.
func TestPressingConnectAgainDoesNotStartAgain(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNDown)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	src.answer = func(context.Context, string, core.SignIn) error { <-release; return nil }

	require.True(t, gui.PressVPNRow(p, 0, "Connect"))
	await("the press", func() bool { return len(src.pressed()) == 1 })
	require.True(t, gui.PressVPNRow(p, 0, "Connect"))

	assert.Len(t, src.pressed(), 1)
}

// Disconnecting a connect that is taking too long cancels it, and what the
// cancelled connect then says does not stand over the disconnect.
func TestADisconnectReplacesAConnectUnderWay(t *testing.T) {
	p, src, await := vpnDrawer(t, view.VPNConnecting)
	cancelled := make(chan struct{})
	src.answer = func(ctx context.Context, action string, _ core.SignIn) error {
		if action == view.VPNBounce {
			<-ctx.Done()
			close(cancelled)
			return core.ErrVPNFailed
		}
		return nil
	}
	gui.StartVPNPress(p, view.VPNBounce, "invented")
	await("the bounce", func() bool { return len(src.pressed()) == 1 })

	require.True(t, gui.PressVPNRow(p, 0, "Disconnect"))
	require.True(t, gui.PressVPNRow(p, 0, "Disconnect"))

	<-cancelled
	await("the word to go", func() bool { return !partSays(p, 0, "Last press") })
	assert.False(t, partSays(p, 0, core.ErrVPNFailed.Error()))
}

// Re-enabling takes nothing down, so it takes one press.
func TestReenablingTakesOnePress(t *testing.T) {
	detail := view.VPNDetail("net", []view.VPNDetailReading{{
		VPNReading: view.VPNReading{Name: "invented", State: view.VPNDown},
		Keepalive:  "failed 3 checks in a row", KeepaliveDisabled: true,
	}})
	src := &vpnSource{detailSource: newDetailSource("net", view.Section{Title: "Network"}, map[string]view.Detail{view.DetailVPN: detail})}
	src.section.Opens = view.DetailVPN
	p, await := drawerPanel(t, src)
	gui.Tap(p, "net", "", -1)
	await("the drawer's parts", func() bool { return len(gui.DrawerPartHeads(p)) == 1 })

	require.True(t, gui.PressVPNRow(p, 0, "Re-enable"))

	await("the press", func() bool { return len(src.pressed()) == 1 })
	assert.Equal(t, []string{"reenable invented"}, src.pressed())
}

// The drawer's plot moves as a card's does (spec 086): it drifts over the time between
// checks, and a failure that has left the window leaves the plot.
func TestTheDrawersPlotDriftsAndLosesATraceThatLeft(t *testing.T) {
	reading := func(failed bool) view.Detail {
		return view.VPNDetail("net", []view.VPNDetailReading{{
			VPNReading: view.VPNReading{Name: "invented", State: view.VPNUp},
			Samples:    []float64{10, 20, 30}, Failed: []bool{false, failed, false}, CheckEvery: time.Minute,
		}})
	}
	src := &vpnSource{detailSource: newDetailSource("net", view.Section{Title: "Network"}, map[string]view.Detail{view.DetailVPN: reading(true)})}
	src.section.Opens = view.DetailVPN
	p, await := drawerPanel(t, src)
	gui.Tap(p, "net", "", -1)
	await("the failure on the plot", func() bool {
		if len(gui.DrawerPartHeads(p)) != 1 {
			return false
		}
		_, failed, ok := gui.DrawerPlot(p, 0, "failed")
		return ok && len(failed) == 3
	})

	every, latency, ok := gui.DrawerPlot(p, 0, "latency")
	require.True(t, ok)
	assert.Equal(t, time.Minute, every)
	assert.Equal(t, []float64{10, 20, 30}, latency)

	src.set(view.DetailVPN, reading(false))
	await("the failure gone from the plot", func() bool {
		_, failed, _ := gui.DrawerPlot(p, 0, "failed")
		return len(failed) == 0
	})
}
