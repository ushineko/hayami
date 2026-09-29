package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"github.com/ushineko/fynedesygn/glance"

	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

// ThemeWith is themeWith, for a test in the black-box package.
//
// The hook is the whole of how the panel and the preferences window share one
// application theme without wearing each other's face, and it is not worth
// widening the package's API for everybody else to be able to say so. The
// queue is named because the real one is fyne.Do, which needs an event loop a
// test does not have.
// WithCardOpacity is withCardOpacity, so a test can say which theme the fade
// belongs to and which it does not.
func WithCardOpacity(base fyne.Theme, percent int) fyne.Theme {
	return withCardOpacity(base, percent)
}

func ThemeWith(
	store *config.Store,
	notify func(config.Config),
	queue func(func()),
) func(fdtheme.Appearance) fyne.Theme {
	return themeWith(store, notify, queue)
}

// ShownCells is how many of a card's cells are currently drawn, so a test can
// say what the window shows without walking the object tree.
func ShownCells(p *Panel, key string) int {
	c, ok := p.cards[key]
	if !ok {
		return -1
	}
	n := 0
	for _, cell := range c.cells {
		if cell.Shown() {
			n++
		}
	}
	return n
}

// Restore is restore, so a test can exercise the decision a cold start makes
// without starting an application.
func Restore(
	sources []panel.Source,
	drawn map[string]bool,
	cached readings.Cache,
) map[string]view.Section {
	return restore(sources, drawn, cached)
}

// CellStale reports whether a card's nth cell is drawn as a last-known value.
func CellStale(p *Panel, key string, n int) bool {
	c, ok := p.cards[key]
	if !ok || n >= len(c.cells) {
		return false
	}
	return c.cells[n].Reading().Stale
}

// Seed is seed, and Cached is what the panel would write, so a test can say
// that a section which never reported keeps the reading it had.
func Seed(p *Panel, c readings.Cache) { p.seed(c) }

func Cached(p *Panel) readings.Cache {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	out := make(readings.Cache, len(p.cache))
	for k, v := range p.cache {
		out[k] = v
	}
	return out
}

// CardMarked reports whether a card's header carries the unavailable marker.
func CardMarked(p *Panel, key string) bool {
	c, ok := p.cards[key]
	if !ok {
		return false
	}
	for _, o := range test.LaidOutObjects(c.card.Object()) {
		if txt, is := o.(*canvas.Text); is && txt.Text == glance.GoneMarker {
			return txt.Visible()
		}
	}
	return false
}
