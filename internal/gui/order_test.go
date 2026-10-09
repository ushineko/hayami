package gui_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// counted is a source that counts its polls. It polls once and then waits an
// hour, so nothing draws while a test applies settings: under the test driver
// fyne.Do runs on the poller's own goroutine.
type counted struct {
	fixed
	polls atomic.Int32
}

func (c *counted) Interval() time.Duration { return time.Hour }
func (c *counted) Poll(context.Context) (bool, error) {
	c.polls.Add(1)
	return true, nil
}

// three is a window over a bandwidth, a cooler and a usage section, drawn.
func three(t *testing.T) (*gui.Panel, []*fixed) {
	t.Helper()
	a := test.NewTempApp(t)
	srcs := []*fixed{{interfaces()}, {view.Cooler(desk(true))}, {view.Usage(time.Now(), []view.UsageWindow{{Account: "CC", Name: "5h", Fraction: 0.1}}, time.Now())}}
	ps := make([]panel.Source, len(srcs))
	for i, s := range srcs {
		ps[i] = s
	}
	p := gui.New(a, gui.Options{Sources: ps, Title: "hayami"})
	for _, s := range srcs {
		p.Draw(s.sec.Key, s.sec, true)
	}
	return p, srcs
}

/*
Spec 052. A new order in the settings is the window's order at once: the
shown sections first, in the settings' order, the rest after. Before this the
window kept the order it was built with until the next start (issue #158).
*/
func TestTheWindowTakesTheSettingsOrderAtOnce(t *testing.T) {
	p, _ := three(t)
	require.Equal(t, []string{"bandwidth", "cooler", "usage"}, gui.CardOrder(p))

	p.Apply(config.Config{Sections: []string{"usage", "bandwidth", "cooler"}})
	assert.Equal(t, []string{"usage", "bandwidth", "cooler"}, gui.CardOrder(p))

	// Hidden and moved at once: the shown ones first, the hidden after.
	p.Apply(config.Config{Sections: []string{"cooler", "usage"}})
	assert.Equal(t, []string{"cooler", "usage", "bandwidth"}, gui.CardOrder(p))
	assert.False(t, gui.CardDrawn(p, "bandwidth"), "a hidden section is drawn")
	assert.True(t, gui.CardDrawn(p, "cooler"))
}

// A reorder moves the cards and rebuilds nothing: a row is the same object
// after it.
func TestAReorderRebuildsNoRow(t *testing.T) {
	p, _ := three(t)
	row := gui.RowOf(p, "cooler", "cpu")
	require.NotNil(t, row)
	p.Apply(config.Config{Sections: []string{"usage", "cooler", "bandwidth"}})
	assert.Same(t, row, gui.RowOf(p, "cooler", "cpu"))
}

// A section the settings do not show is not polled, and is polled at once
// when they show it (spec 052): a hidden section costs nothing. The test sets
// what is shown the way Apply does, without the window: under the test driver
// fyne.Do draws on the poller's goroutine, and a layout from the test's own
// would race it.
func TestAHiddenSectionIsNotPolledUntilShown(t *testing.T) {
	a := test.NewTempApp(t)
	shown := &counted{fixed: fixed{interfaces()}}
	hidden := &counted{fixed: fixed{view.Cooler(desk(true))}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{shown, hidden}, Title: "hayami"})
	gui.Show(p, []string{"bandwidth"})
	stop := gui.SerialDraws(p)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); stop() })
	p.Poll(ctx)
	require.Eventually(t, func() bool { return shown.polls.Load() > 0 }, 2*time.Second, 10*time.Millisecond,
		"the shown section was not polled")
	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, hidden.polls.Load(), "a hidden section was polled")

	gui.Show(p, []string{"bandwidth", "cooler"})
	require.Eventually(t, func() bool { return hidden.polls.Load() > 0 }, 2*time.Second, 10*time.Millisecond,
		"a section shown was not polled")
}

// Apply is what tells the pollers: a section the settings hide stops being
// polled, and one they show is polled again.
func TestApplySetsWhatIsPolled(t *testing.T) {
	p, _ := three(t)
	p.Apply(config.Config{Sections: []string{"cooler"}})
	assert.True(t, gui.Shown(p, "cooler"))
	assert.False(t, gui.Shown(p, "bandwidth"))
	assert.False(t, gui.Shown(p, "usage"))
	p.Apply(config.Config{Sections: []string{"usage", "bandwidth"}})
	assert.True(t, gui.Shown(p, "bandwidth"))
	assert.False(t, gui.Shown(p, "cooler"))
}
