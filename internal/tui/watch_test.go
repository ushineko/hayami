package tui_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// countedSource answers at once and counts how often it was asked.
type countedSource struct {
	slowSource
	polls atomic.Int32
}

func (s *countedSource) Poll(ctx context.Context) (bool, error) {
	s.polls.Add(1)
	return s.slowSource.Poll(ctx)
}

func counted(keys ...string) ([]*countedSource, []panel.Source) {
	cs := make([]*countedSource, len(keys))
	ps := make([]panel.Source, len(keys))
	for i, k := range keys {
		cs[i] = &countedSource{slowSource: slowSource{key: k, says: true}}
		ps[i] = cs[i]
	}
	return cs, ps
}

// start runs a batch's commands in the background, as bubbletea does, and
// drops their messages: these tests ask what was polled, not what it said.
// A tick in the batch sleeps out its interval and is never waited for.
func start(cmd tea.Cmd) {
	for _, c := range expand(cmd) {
		go c()
	}
}

func keys(secs []view.Section) []string {
	out := make([]string, len(secs))
	for i, s := range secs {
		out[i] = s.Key
	}
	return out
}

// Spec 052. The terminal panel follows its settings as the window does: a
// change to the sections shown, their order or the arrangement is drawn at
// the next look, a section newly shown is polled at once, and one no longer
// shown is not drawn.
func TestTheTerminalPanelFollowsItsSettings(t *testing.T) {
	cs, ps := counted("bandwidth", "cooler", "usage")
	looks := 0
	m := tui.New(tui.Options{
		Sources:     ps,
		Shown:       []string{"bandwidth", "cooler"},
		Arrangement: view.ArrangeStack,
		Watch: func() ([]string, view.Arrangement, bool) {
			looks++
			if looks == 1 {
				return nil, view.ArrangeStack, false
			}
			return []string{"usage", "bandwidth"}, view.ArrangeGrid, true
		},
	})

	start(m.Init())
	require.Eventually(t, func() bool { return cs[0].polls.Load() > 0 && cs[1].polls.Load() > 0 },
		2*time.Second, 10*time.Millisecond, "a shown section was not polled")
	assert.Zero(t, cs[2].polls.Load(), "a section the settings do not show was polled")

	updated, _ := m.Update(tui.Drawn(ps))
	m = updated.(tui.Model)
	assert.Equal(t, []string{"bandwidth", "cooler"}, keys(m.Sections()))

	// The first look finds nothing new and changes nothing.
	updated, _ = m.Update(tui.Watched())
	m = updated.(tui.Model)
	assert.Equal(t, []string{"bandwidth", "cooler"}, keys(m.Sections()))

	updated, cmd := m.Update(tui.Watched())
	m = updated.(tui.Model)
	assert.Equal(t, []string{"usage", "bandwidth"}, keys(m.Sections()), "the new order or set was not drawn")
	assert.Equal(t, view.ArrangeGrid, tui.ArrangementOf(m), "the new arrangement was not taken")
	start(cmd)
	require.Eventually(t, func() bool { return cs[2].polls.Load() > 0 },
		2*time.Second, 10*time.Millisecond, "a section newly shown was not polled at once")
}

// With no settings given, every source is drawn in its own order, as before.
func TestWithNoSettingsEverySourceIsShown(t *testing.T) {
	_, ps := counted("bandwidth", "cooler")
	m := tui.New(tui.Options{Sources: ps})
	updated, _ := m.Update(tui.Drawn(ps))
	assert.Equal(t, []string{"bandwidth", "cooler"}, keys(updated.(tui.Model).Sections()))
}
