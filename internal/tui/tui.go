/*
Package tui is the terminal panel.

It arranges the same sections the window does, in whichever arrangement the
settings or the command line name. It is not a lesser view of the window: the
sections are the same objects, and the only thing that differs is how they are
laid out.
*/
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// Options are what the terminal panel needs to start.
type Options struct {
	// Sources are the sections, in the order to draw them.
	Sources []panel.Source

	// Arrangement is how to lay them out.
	Arrangement view.Arrangement

	// Once draws one frame and exits, for a prompt or a status line. The
	// widget this replaces calls it --line.
	Once bool

	// Shown are the sections to draw, by key, in order (spec 052). Sources
	// may hold more: one not shown is not polled. Nil shows every source, in
	// the sources' order.
	Shown []string

	// Watch is asked every WatchInterval for the shown sections and the
	// arrangement the settings now give, changed false when they are as they
	// were. It is how a change made in the window's preferences reaches a
	// pane. Nil watches nothing.
	Watch func() (shown []string, arrangement view.Arrangement, changed bool)
}

// WatchInterval is how often the terminal panel looks at its settings.
const WatchInterval = 2 * time.Second

// Model is the panel's state.
type Model struct {
	opts          Options
	width, height int

	// chosen is the card Tab has chosen, dr the drawer open beside or over
	// the cards, nil when none is, and last the list each card last showed
	// (spec 098).
	chosen string
	dr     *paneDrawer
	last   map[string]string

	// scroll is how far each section's run is scrolled, by key (spec 105):
	// the first of its lines drawn.
	scroll map[string]int

	// paint is how a status becomes colour. Nil in a terminal that has none,
	// and nil is also what Render expects when nothing should be painted.
	paint view.Painter

	// drawn records which sources have something to say, by key. A source
	// that has never answered is not drawn at all, so a machine without the
	// hardware looks like a program built without the section.
	drawn map[string]bool

	// reported records which sources have finished their first poll, which is
	// a different question from whether they have anything to say: a source
	// that answers "nothing" has still answered. Only --once reads it, and it
	// is what that flag waits for.
	reported map[string]bool

	// shown is the sections drawn, and order the order of every source
	// (spec 052): the shown ones first, as the settings give them.
	shown map[string]bool
	order []string
}

// New builds the model.
func New(o Options) Model {
	m := Model{
		opts:     o,
		width:    80,
		height:   24,
		last:     map[string]string{},
		scroll:   map[string]int{},
		drawn:    map[string]bool{},
		reported: map[string]bool{},
		paint:    Painter(),
	}
	shown := o.Shown
	if shown == nil {
		for _, s := range o.Sources {
			shown = append(shown, s.Key())
		}
	}
	m.show(shown)
	return m
}

// show sets the sections drawn and their order: the shown ones first, in the
// order given, then every other source (spec 052).
func (m *Model) show(keys []string) {
	m.shown = make(map[string]bool, len(keys))
	for _, k := range keys {
		m.shown[k] = true
	}
	m.order = make([]string, 0, len(m.opts.Sources))
	seen := map[string]bool{}
	for _, k := range keys {
		if m.source(k) != nil && !seen[k] {
			m.order = append(m.order, k)
			seen[k] = true
		}
	}
	for _, s := range m.opts.Sources {
		if !seen[s.Key()] {
			m.order = append(m.order, s.Key())
		}
	}
}

// OnceDeadline bounds the single frame.
//
// --once is for a prompt or a status line, and a prompt that hangs is worse
// than a prompt that is missing a reading. Every source is waited for, but not
// past this: what has arrived is drawn and the rest are left out, which is the
// same thing the panel shows for a source that has nothing to say.
//
// Five seconds is above what the slow sources actually take — the cooler
// answers well inside a second, a silent peripheral costs its retries, a
// cached usage read is immediate — and below what anybody would sit through.
const OnceDeadline = 5 * time.Second

// giveUp ends the single frame whether or not every source has answered.
type giveUp struct{}

// tick asks for the next poll of one source.
type tick struct{ key string }

// polled carries a source's answer back to the model.
type polled struct {
	key   string
	drawn bool
}

// Init polls every source at once, so the first frame is the real one rather
// than an empty panel that fills in.
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.opts.Sources)+2)
	for _, s := range m.opts.Sources {
		if m.shown[s.Key()] {
			cmds = append(cmds, poll(s))
		}
	}
	if m.opts.Once {
		cmds = append(cmds, tea.Tick(OnceDeadline, func(time.Time) tea.Msg { return giveUp{} }))
	} else if m.opts.Watch != nil {
		cmds = append(cmds, watchAfter())
	}
	return tea.Batch(cmds...)
}

// poll reads one source. Each keeps its own cadence: a byte counter and a
// thermal probe share a pane and nothing else.
func poll(s panel.Source) tea.Cmd {
	return func() tea.Msg {
		// The error is not what decides whether anything is drawn. A source
		// that failed says so in its section's reasons, and a section with a
		// reason is a section to draw -- the alternative is the silence this
		// whole spec is about (issue #54).
		drawn, _ := s.Poll(context.Background())
		return polled{key: s.Key(), drawn: drawn || len(s.Section().Reasons) > 0}
	}
}

// after schedules the next poll of one source.
func after(s panel.Source) tea.Cmd {
	return tea.Tick(s.Interval(), func(time.Time) tea.Msg { return tick{key: s.Key()} })
}

// Update handles a resize, a keypress and the polls.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// A pane with no size is a pane whose size is not known yet: a pty
		// without a window size reports zero, and rendering at zero is a
		// column of ellipses. The default stands until something real
		// arrives.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)

	case tea.MouseMsg:
		return m.mouse(msg), nil

	case detailRead:
		return m, m.read(msg)

	case detailTick:
		return m, m.tickDetail(msg)

	case polled:
		m.drawn[msg.key] = msg.drawn
		if m.opts.Once {
			// Every source, not the first one. Init batches a poll per source
			// and they answer in whatever order they finish; quitting on the
			// first message drew whichever won the race and dropped the rest,
			// which with the default sections is bandwidth answering "nothing
			// yet" and a frame with no sections in it at all.
			m.reported[msg.key] = true
			if len(m.reported) >= len(m.shown) {
				return m, tea.Quit
			}
			return m, nil
		}
		if s := m.source(msg.key); s != nil {
			return m, after(s)
		}
		return m, nil

	case giveUp:
		return m, tea.Quit

	case drawnAll:
		for _, s := range msg.sources {
			m.drawn[s.Key()] = true
		}
		return m, nil

	case tick:
		// A section no longer shown stops here; showing it again polls it.
		if s := m.source(msg.key); s != nil && m.shown[msg.key] {
			return m, poll(s)
		}
		return m, nil

	case watch:
		shown, arr, changed := m.opts.Watch()
		if !changed {
			return m, watchAfter()
		}
		was := m.shown
		m.show(shown)
		m.opts.Arrangement = arr
		cmds := []tea.Cmd{watchAfter()}
		for _, k := range m.order {
			if m.shown[k] && !was[k] {
				cmds = append(cmds, poll(m.source(k)))
			}
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// key handles a keypress: the drawer's first where one is open, then the
// pane's own. q, Ctrl+C, and Esc with nothing open quit, as they always have.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Keys typed faster than the terminal is read arrive as one message of
	// several runes: "fhay" is f, then h, a and y into the filter it opened.
	// Each is taken as it would have been alone; a filter takes them whole.
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && (m.dr == nil || !m.dr.filtering) {
		var cmds []tea.Cmd
		var next tea.Model = m
		for i, r := range msg.Runes {
			if mm := next.(Model); mm.dr != nil && mm.dr.filtering {
				rest := tea.KeyMsg{Type: tea.KeyRunes, Runes: msg.Runes[i:]}
				next, cmd := mm.key(rest)
				return next, tea.Batch(append(cmds, cmd)...)
			}
			var cmd tea.Cmd
			next, cmd = next.(Model).key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			cmds = append(cmds, cmd)
		}
		return next, tea.Batch(cmds...)
	}
	if m.dr != nil {
		if cmd, handled := m.drawerKey(msg); handled {
			return m, cmd
		}
	}
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		m.closeDrawer()
		return m, tea.Quit
	case "tab":
		m.choose(1)
	case "shift+tab":
		m.choose(-1)
	case "down", "j":
		// The chosen section's run first, as a list in a list scrolls before
		// the page does; past its end, the next section (spec 105).
		if !m.scrollBy(m.chosen, 1) {
			m.choose(1)
		}
	case "up", "k":
		if !m.scrollBy(m.chosen, -1) {
			m.choose(-1)
		}
	case "enter":
		if m.chosen == "" {
			m.choose(1)
		}
		if d, ok := m.detailer(m.chosen); ok {
			root := m.opensOn(m.chosen, d)
			return m, m.open(m.chosen, root, root)
		}
	}
	return m, nil
}

// source finds a source by key.
func (m Model) source(key string) panel.Source {
	for _, s := range m.opts.Sources {
		if s.Key() == key {
			return s
		}
	}
	return nil
}

// Sections are what the model would draw, which is what a test asks for
// rather than parsing the rendered string.
func (m Model) Sections() []view.Section {
	out := make([]view.Section, 0, len(m.order))
	for _, k := range m.order {
		if !m.shown[k] || !m.drawn[k] {
			continue
		}
		sec := m.source(k).Section()
		sec.ScrollFirst = max(0, min(m.scroll[k], sec.MostFirst(m.opts.Arrangement)))
		out = append(out, sec)
	}
	return out
}

// section is the shown section with the key, as Sections gives it.
func (m Model) section(key string) (view.Section, bool) {
	for _, s := range m.Sections() {
		if s.Key == key {
			return s, true
		}
	}
	return view.Section{}, false
}

// scrollBy moves a section's run by step lines, and reports whether it moved:
// false for a section with no run, or one already at that end.
func (m *Model) scrollBy(key string, step int) bool {
	s, ok := m.section(key)
	if !ok || s.ScrollLines == 0 {
		return false
	}
	next := max(0, min(s.ScrollFirst+step, s.MostFirst(m.opts.Arrangement)))
	if next == s.ScrollFirst {
		return false
	}
	m.scroll[key] = next
	return true
}

// mouse scrolls the run under the pointer with the wheel (spec 105). The
// wheel anywhere else, and every other press, does nothing.
func (m Model) mouse(msg tea.MouseMsg) Model {
	step := 0
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		step = 1
	case tea.MouseButtonWheelUp:
		step = -1
	default:
		return m
	}
	for _, r := range m.Regions() {
		if r.InRun(msg.X, msg.Y) {
			m.scrollBy(r.Key, step)
			break
		}
	}
	return m
}

/*
Regions are where each section's card is drawn, as Lines draws them, for the
pointer: none while a drawer stands in the cards' place.
*/
func (m Model) Regions() []view.Region {
	if m.dr != nil && !m.Side() {
		return nil
	}
	width := m.width
	if m.dr != nil {
		width = m.width - m.drawerWidth() - 3
	}
	_, regions := view.RenderRegions(m.Sections(), m.opts.Arrangement, width, nil)
	return regions
}

// View draws the panel.
func (m Model) View() string {
	return strings.Join(m.Lines(), "\n")
}

/*
Lines are the panel as drawn (spec 098): the cards, the chosen one's title
marked, and an open drawer beside them where the pane is wide enough for both
and in their place where it is not, or in row. The split is decided by the
width alone, so no reading moves it.
*/
func (m Model) Lines() []string {
	sections := m.Sections()
	for i, s := range sections {
		if s.Key == m.chosen {
			sections[i].Title = "▸ " + s.Title
		}
	}
	if m.dr == nil {
		return view.RenderWith(sections, m.opts.Arrangement, m.width, m.paint)
	}
	drawer := m.drawerWidth()
	cards := m.width - drawer - 3
	if m.opts.Arrangement == view.ArrangeRow || cards < CardsMinWidth {
		return m.drawerLines(m.width, m.height)
	}
	left := view.RenderWith(sections, m.opts.Arrangement, cards, m.paint)
	right := m.drawerLines(drawer, m.height)
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = view.PadVisible(l, cards) + " │ " + r
	}
	return out
}

// Side reports whether an open drawer stands beside the cards rather than in
// their place, for a test.
func (m Model) Side() bool {
	return m.dr != nil && m.opts.Arrangement != view.ArrangeRow && m.width-m.drawerWidth()-3 >= CardsMinWidth
}

// Start runs the panel until it is told to stop.
func Start(o Options) error {
	m := New(o)
	opts := []tea.ProgramOption{}
	if o.Once {
		// One frame to stdout and no alternate screen: the output is meant to
		// be captured, piped or left in the scrollback.
		opts = append(opts, tea.WithInput(nil))
	} else {
		// The wheel scrolls a run under the pointer (spec 105). A terminal
		// reporting the mouse keeps a drag for itself only with Shift held,
		// which the README says.
		opts = append(opts, tea.WithAltScreen(), tea.WithMouseCellMotion())
	}
	if _, err := tea.NewProgram(m, opts...).Run(); err != nil {
		return fmt.Errorf("running the terminal panel: %w", err)
	}
	return nil
}

// Drawn is every source reporting that it has something to say, as the
// messages Update expects. It is exported for the parity test, which drives
// the model the way the program does rather than reaching inside it.
func Drawn(sources []panel.Source) tea.Msg {
	return drawnAll{sources: sources}
}

// drawnAll marks every source drawn at once.
type drawnAll struct{ sources []panel.Source }

// watch asks Options.Watch whether the settings changed.
type watch struct{}

// watchAfter schedules the next look at the settings.
func watchAfter() tea.Cmd {
	return tea.Tick(WatchInterval, func(time.Time) tea.Msg { return watch{} })
}

// Watched is the message the panel acts on when it looks at its settings,
// exported so a test can drive a change without waiting for the interval.
func Watched() tea.Msg { return watch{} }
