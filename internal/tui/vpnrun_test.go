package tui_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// tunnelSource is a Network section with an interface and five tunnels up,
// fifteen lines of run in a stack.
type tunnelSource struct{}

func (tunnelSource) Key() string                        { return "bandwidth" }
func (tunnelSource) Poll(context.Context) (bool, error) { return true, nil }
func (tunnelSource) Interval() time.Duration            { return time.Hour }
func (tunnelSource) Data() any                          { return nil }
func (tunnelSource) Section() view.Section {
	sec := view.Bandwidth([]view.BandwidthReading{{Name: "eno2", HasRate: true, RxRate: 1 << 20}})
	vpns := make([]view.VPNReading, 5)
	for i := range vpns {
		vpns[i] = view.VPNReading{
			Name: fmt.Sprintf("wg%d", i), State: view.VPNUp,
			Traffic: &view.BandwidthReading{Name: fmt.Sprintf("wg%d", i), HasRate: true, HasTotal: true},
		}
	}
	sec.Rows = append(sec.Rows, view.VPNRows(vpns)...)
	sec.ScrollLines = view.VPNScrollLines
	sec.Opens = view.DetailVPN
	return sec
}

// The network opens the VPN drawer, as the real one does where VPNs are
// chosen; a section that opens nothing cannot be chosen.
func (tunnelSource) Details() []view.DetailInfo { return []view.DetailInfo{view.VPNDetailInfo()} }
func (tunnelSource) PollDetail(context.Context, string) (view.Detail, error) {
	return view.Detail{Section: "bandwidth", ID: view.DetailVPN, Title: "VPNs"}, nil
}
func (tunnelSource) DetailInterval(string) time.Duration { return time.Hour }
func (tunnelSource) CloseDetail(string)                  {}

// firstTunnel is the first tunnel the pane draws.
func firstTunnel(t *testing.T, m tui.Model) string {
	t.Helper()
	for _, l := range m.Lines() {
		if strings.HasPrefix(l, "wg") {
			return l[:3]
		}
	}
	require.Fail(t, "no tunnel drawn")
	return ""
}

// runRegion is where the network's run is drawn.
func runRegion(t *testing.T, m tui.Model) view.Region {
	t.Helper()
	for _, r := range m.Regions() {
		if r.Key == "bandwidth" {
			require.Positive(t, r.RunH)
			return r
		}
	}
	require.Fail(t, "no network region")
	return view.Region{}
}

// wheel turns the wheel at a cell.
func wheel(m tui.Model, x, y int, down bool) tui.Model {
	b := tea.MouseButtonWheelUp
	if down {
		b = tea.MouseButtonWheelDown
	}
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress})
	return next.(tui.Model)
}

// Spec 105 R7. The wheel over the run scrolls it a line a notch; over the
// interface above it, it does not.
func TestTheWheelOverTheRunScrollsIt(t *testing.T) {
	m := pane(t, view.ArrangeStack, 80, 40, tunnelSource{}, newListSource())
	r := runRegion(t, m)
	assert.Equal(t, 9, r.RunH)
	assert.Equal(t, "wg0", firstTunnel(t, m))

	for range 3 {
		m = wheel(m, 10, r.RunY+1, true)
	}
	assert.Equal(t, "wg1", firstTunnel(t, m), "three notches did not scroll one tunnel")

	m = wheel(m, 10, r.RunY-1, true)
	assert.Equal(t, "wg1", firstTunnel(t, m), "the wheel over the interface scrolled the run")

	for range 3 {
		m = wheel(m, 10, r.RunY, false)
	}
	assert.Equal(t, "wg0", firstTunnel(t, m))
}

// Spec 105 R7. With the network chosen, the arrows scroll its run to the end
// and only then move on to the next section; Tab moves on at once.
func TestTheArrowsScrollTheChosenRunBeforeMovingOn(t *testing.T) {
	m := pane(t, view.ArrangeStack, 80, 40, tunnelSource{}, newListSource())
	m, _ = press(t, m, key("tab"))
	require.Contains(t, text(m), "▸ Network")

	downs := make([]tea.KeyMsg, 6)
	for i := range downs {
		downs[i] = key("down")
	}
	m, _ = press(t, m, downs...)
	assert.Contains(t, text(m), "▸ Network", "the arrows left the run before its end")
	assert.Equal(t, "wg2", firstTunnel(t, m), "six lines down is two tunnels down")

	m, _ = press(t, m, key("down"))
	assert.Contains(t, text(m), "▸ Hardware", "the arrow at the run's end did not move on")

	m, _ = press(t, m, key("shift+tab"))
	m, _ = press(t, m, key("k"), key("k"), key("k"))
	assert.Contains(t, text(m), "▸ Network")
	assert.Equal(t, "wg1", firstTunnel(t, m), "three lines up is not a tunnel back")

	m, _ = press(t, m, key("tab"))
	assert.Contains(t, text(m), "▸ Hardware", "tab did not move on at once")
}
