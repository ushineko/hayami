package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	fd "github.com/ushineko/fynedesygn"
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

// CardMinSize is the smallest a card can be drawn, which is what decides
// whether a change to it reflows the window.
func CardMinSize(p *Panel, key string) fyne.Size {
	c, ok := p.cards[key]
	if !ok {
		return fyne.Size{}
	}
	return c.card.Object().MinSize()
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

// CardDrawn reports whether a card is on screen, so a test can say what the
// window shows without walking the object tree.
func CardDrawn(p *Panel, key string) bool {
	c, ok := p.cards[key]
	if !ok {
		return false
	}
	return c.card.Drawn()
}

// CardTip is a card's hover text.
func CardTip(p *Panel, key string) string {
	c, ok := p.cards[key]
	if !ok {
		return ""
	}
	return c.card.Tip()
}

// CardRows is every piece of text a card's shown rows draw.
//
// Read off the objects rather than from the row's own fields, because
// glance.Row exposes its reading and not its label -- a gap logged in spec
// 015. Walking the object is a test's business and not a reason to widen the
// library's API from here.
func CardRows(p *Panel, key string) []string {
	c, ok := p.cards[key]
	if !ok {
		return nil
	}
	var out []string
	for _, r := range c.rows {
		if !r.Shown() {
			continue
		}
		out = append(out, texts(r.Object())...)
	}
	return out
}

// texts is every non-empty string drawn in an object tree.
func texts(o fyne.CanvasObject) []string {
	var out []string
	switch v := o.(type) {
	case *canvas.Text:
		if v.Text != "" {
			out = append(out, v.Text)
		}
	case *fyne.Container:
		for _, child := range v.Objects {
			out = append(out, texts(child)...)
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(v).Objects() {
			out = append(out, texts(child)...)
		}
	}
	return out
}

// SparkSamples are what a card's plot holds for one trace: nil when the card
// has no plot or the plot has no such trace, and an empty slice for a trace
// that is declared and has nothing yet.
func SparkSamples(p *Panel, key, trace string) []float64 {
	c, ok := p.cards[key]
	if !ok || c.spark == nil {
		return nil
	}
	return c.spark.Samples(trace)
}

// SparkScale is how a card's plot fits its traces, and false when the card
// has no plot.
func SparkScale(p *Panel, key string) (glance.Scale, bool) {
	c, ok := p.cards[key]
	if !ok || c.spark == nil {
		return 0, false
	}
	return c.spark.Scale(), true
}

// TrailColour is trailColour, and PanelTheme the theme it resolves in, so a
// test can say which colour a trace is drawn in.
func TrailColour(p *Panel, t view.Trail, sec view.Section) color.Color {
	return p.trailColour(t, sec)
}

func PanelTheme(p *Panel) fyne.Theme { return p.win.Panel().Theme() }

// CellBar is the fill of a card's nth cell's bar and its colour, and false
// when the cell draws no bar.
//
// glance.Cell exposes SetBar and ClearBar and not what they set, so this lays
// the cell out and reads the rectangles it draws, as the library's own tests
// do: a track and a fill when there is a bar, nothing when there is not.
func CellBar(p *Panel, key string, n int, width float32) (fraction float32, fill color.Color, ok bool) {
	c, found := p.cards[key]
	if !found || n >= len(c.cells) {
		return 0, nil, false
	}
	o := c.cells[n].Object()
	o.Resize(fyne.NewSize(width, o.MinSize().Height))
	var rects []*canvas.Rectangle
	for _, child := range o.(*fyne.Container).Objects {
		if !child.Visible() {
			continue
		}
		for _, d := range test.LaidOutObjects(child) {
			if r, is := d.(*canvas.Rectangle); is {
				rects = append(rects, r)
			}
		}
	}
	if len(rects) != 2 {
		return 0, nil, false
	}
	return rects[1].Size().Width / rects[0].Size().Width, rects[1].FillColor, true
}

// PanelSize is the size the panel asks its window for.
func PanelSize(p *Panel) fyne.Size { return p.win.Panel().Size() }

// RowColour is the explicit colour a card's nth shown row's value was given,
// nil when it takes its colour from its status.
func RowColour(p *Panel, key string, n int) color.Color {
	c, ok := p.cards[key]
	if !ok || n >= len(c.rows) {
		return nil
	}
	return c.rows[n].Reading().Colour
}

// Status is the design-system status a card's nth row's value carries, by the
// word cli uses for it.
func Status(p *Panel, key string, n int) string {
	c, ok := p.cards[key]
	if !ok || n >= len(c.rows) {
		return ""
	}
	switch c.rows[n].Reading().Status {
	case fd.StatusWarn:
		return "warn"
	case fd.StatusBad:
		return "bad"
	case fd.StatusGood:
		return "good"
	default:
		return "info"
	}
}
