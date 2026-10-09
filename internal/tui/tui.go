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
	opts  Options
	width int

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
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
		return m, nil

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
		out = append(out, m.source(k).Section())
	}
	return out
}

// View draws the panel.
func (m Model) View() string {
	lines := view.RenderWith(m.Sections(), m.opts.Arrangement, m.width, m.paint)
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
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
		opts = append(opts, tea.WithAltScreen())
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
