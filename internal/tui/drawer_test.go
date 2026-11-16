package tui_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// listSource is a section with process lists behind it: CPU and IO, each a
// few programs, and a page of each program's processes.
type listSource struct {
	mu      sync.Mutex
	details map[string]view.Detail
	polls   map[string]int
	closed  map[string]int
}

func newListSource() *listSource {
	cpu := view.Processes("cooler", view.DetailCPU, []view.ProcessUse{
		{Name: "editor", Value: 30, Count: 2}, {Name: "browser", Value: 20}, {Name: "shell", Value: 1},
	})
	for i, l := range cpu.Lines {
		cpu.Lines[i].Open = "cpu/" + l.Name
	}
	io := view.Processes("cooler", view.DetailIO, []view.ProcessUse{{Name: "copy", Value: 1 << 20}})
	page := view.Detail{Section: "cooler", ID: "cpu/editor", Title: "CPU by process", Parts: []view.Detail{{
		Lines: []view.DetailLine{
			{Name: "CPU by process > editor", Heading: true},
			{Name: "editor 101", Value: "  20.0", Unit: "%"},
			{Name: "end all", Open: view.ActKill},
		},
	}}}
	return &listSource{
		details: map[string]view.Detail{view.DetailCPU: cpu, view.DetailIO: io, "cpu/editor": page},
		polls:   map[string]int{}, closed: map[string]int{},
	}
}

func (s *listSource) Key() string                        { return "cooler" }
func (s *listSource) Poll(context.Context) (bool, error) { return true, nil }
func (s *listSource) Interval() time.Duration            { return time.Hour }
func (s *listSource) Data() any                          { return nil }
func (s *listSource) Section() view.Section {
	return view.Section{Key: "cooler", Title: "Hardware", Opens: view.DetailCPU,
		Rows: []view.Row{{Label: "CPU", Value: "12 %"}}}
}

func (s *listSource) Details() []view.DetailInfo {
	return []view.DetailInfo{view.ProcessInfo(view.DetailCPU), view.ProcessInfo(view.DetailIO)}
}

func (s *listSource) PollDetail(_ context.Context, id string) (view.Detail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polls[id]++
	return s.details[id], nil
}

func (s *listSource) DetailInterval(string) time.Duration { return time.Hour }

func (s *listSource) CloseDetail(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed[id]++
}

func (s *listSource) Drills(id string) bool { return strings.Contains(id, "/") }

func (s *listSource) DrillUp(id string) string { return panel.DrillOut(id) }

func (s *listSource) count(id string) (polls, closed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.polls[id], s.closed[id]
}

func (s *listSource) set(id string, d view.Detail) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.details[id] = d
}

// plainSource is a section with nothing behind it.
type plainSource struct{}

func (plainSource) Key() string                        { return "plain" }
func (plainSource) Poll(context.Context) (bool, error) { return true, nil }
func (plainSource) Interval() time.Duration            { return time.Hour }
func (plainSource) Data() any                          { return nil }
func (plainSource) Section() view.Section {
	return view.Section{Key: "plain", Title: "Plain", Rows: []view.Row{{Label: "a", Value: "1"}}}
}

// pane is a model over the sources, drawn, at width and height.
func pane(t *testing.T, arr view.Arrangement, width, height int, sources ...panel.Source) tui.Model {
	t.Helper()
	m := tui.New(tui.Options{Sources: sources, Arrangement: arr})
	next, _ := m.Update(tui.Drawn(sources))
	next, _ = next.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(tui.Model)
}

// press sends keys, running what each asks for the way the program would,
// except what waits: a reading's next tick is an hour away.
func press(t *testing.T, m tui.Model, keys ...tea.KeyMsg) (tui.Model, bool) {
	t.Helper()
	quit := false
	var run func(next tea.Model, cmd tea.Cmd) tea.Model
	run = func(next tea.Model, cmd tea.Cmd) tea.Model {
		if cmd == nil {
			return next
		}
		msgs := make(chan tea.Msg, 1)
		go func() { msgs <- cmd() }()
		select {
		case msg := <-msgs:
			if msg == nil {
				return next
			}
			if _, ok := msg.(tea.QuitMsg); ok {
				quit = true
				return next
			}
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, c := range batch {
					next = run(next, c)
				}
				return next
			}
			updated, more := next.Update(msg)
			return run(updated, more)
		case <-time.After(200 * time.Millisecond):
			return next
		}
	}
	var next tea.Model = m
	for _, k := range keys {
		updated, cmd := next.Update(k)
		next = run(updated, cmd)
	}
	return next.(tui.Model), quit
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyList(names ...string) []tea.KeyMsg {
	out := make([]tea.KeyMsg, len(names))
	for i, n := range names {
		out[i] = key(n)
	}
	return out
}

func text(m tui.Model) string { return strings.Join(m.Lines(), "\n") }

// Spec 098 R1, R2. Tab chooses among the cards with something behind them,
// Enter opens the chosen card's list, Esc closes it and lets the list go;
// Esc with nothing open quits, as it always has.
func TestTabChoosesAndEnterOpens(t *testing.T) {
	src := newListSource()
	m := pane(t, view.ArrangeStack, 80, 40, plainSource{}, src)

	m, _ = press(t, m, key("tab"))
	assert.Contains(t, text(m), "▸ Hardware")
	assert.NotContains(t, text(m), "▸ Plain", "a card with nothing behind it is not chosen")

	m, _ = press(t, m, key("enter"))
	require.Contains(t, text(m), "CPU by process")
	polls, _ := src.count(view.DetailCPU)
	assert.Equal(t, 1, polls)

	m, quit := press(t, m, key("esc"))
	assert.False(t, quit)
	assert.NotContains(t, text(m), "CPU by process")
	_, closed := src.count(view.DetailCPU)
	assert.Equal(t, 1, closed)

	_, quit = press(t, m, key("esc"))
	assert.True(t, quit)
}

// R3, R4. A list's key switches to it, letting the other go; f filters by
// name, Enter keeps the filter and Esc takes it away.
func TestTheSwitchAndTheFilter(t *testing.T) {
	src := newListSource()
	m := pane(t, view.ArrangeStack, 80, 40, src)
	m, _ = press(t, m, keyList("tab", "enter")...)
	assert.Contains(t, text(m), "[CPU]")

	m, _ = press(t, m, key("i"))
	assert.Contains(t, text(m), "IO by process")
	assert.Contains(t, text(m), "[IO]")
	_, closed := src.count(view.DetailCPU)
	assert.Equal(t, 1, closed, "the CPU list let go")

	m, _ = press(t, m, keyList("c", "f", "e", "d")...)
	assert.Contains(t, text(m), "/ed▏")
	assert.Contains(t, text(m), "editor")
	assert.NotContains(t, text(m), "browser")

	m, quit := press(t, m, key("q"))
	assert.False(t, quit, "q is typed into the filter")
	m, _ = press(t, m, key("backspace"), key("enter"))
	assert.Contains(t, text(m), "/ed")
	assert.NotContains(t, text(m), "▏")

	m, _ = press(t, m, key("esc"))
	assert.Contains(t, text(m), "browser", "Esc took the filter away")
	assert.Contains(t, text(m), "CPU by process", "and left the drawer open")
}

// R5, R10. A marked line drills into its program's processes, without the
// line that would end them; Esc comes back to the list.
func TestADrillAndBack(t *testing.T) {
	src := newListSource()
	m := pane(t, view.ArrangeStack, 80, 40, src)
	m, _ = press(t, m, keyList("tab", "enter", "down")...)
	assert.Contains(t, text(m), "› editor")

	m, _ = press(t, m, key("enter"))
	polls, _ := src.count("cpu/editor")
	assert.Equal(t, 1, polls)
	assert.Contains(t, text(m), "editor 101")
	assert.NotContains(t, text(m), "end all", "the pane does not act")

	m, _ = press(t, m, key("esc"))
	assert.Contains(t, text(m), "browser")
	_, closed := src.count("cpu/editor")
	assert.Equal(t, 1, closed)
}

// R2, R4, R5. A drill from a filtered list comes back to it filtered; Esc
// goes up from the page first, then takes the filter away.
func TestADrillKeepsTheFilter(t *testing.T) {
	m := pane(t, view.ArrangeStack, 80, 40, newListSource())
	m, _ = press(t, m, keyList("tab", "enter", "f", "e", "d", "enter", "down", "enter")...)
	assert.Contains(t, text(m), "editor 101")

	m, _ = press(t, m, key("esc"))
	assert.Contains(t, text(m), "/ed")
	assert.NotContains(t, text(m), "browser")

	m, _ = press(t, m, key("esc"))
	assert.Contains(t, text(m), "browser")
}

// R6. Space pauses the reading, and again reads at once.
func TestSpacePausesAndResumes(t *testing.T) {
	src := newListSource()
	m := pane(t, view.ArrangeStack, 80, 40, src)
	m, _ = press(t, m, keyList("tab", "enter", "space")...)
	assert.Contains(t, m.Lines()[0], "paused")
	before, _ := src.count(view.DetailCPU)

	m, _ = press(t, m, key("space"))
	assert.NotContains(t, m.Lines()[0], "paused")
	after, _ := src.count(view.DetailCPU)
	assert.Equal(t, before+1, after, "resuming reads at once")
}

// R8. Wide, the drawer stands beside the cards; narrow, or in row, it takes
// the pane. A reading with other figures moves nothing.
func TestTheDrawerIsLaidOutByWidth(t *testing.T) {
	wide := pane(t, view.ArrangeStack, 160, 30, newListSource())
	wide, _ = press(t, wide, keyList("tab", "enter")...)
	assert.True(t, wide.Side())
	assert.Contains(t, wide.Lines()[0], "Hardware")
	assert.Contains(t, wide.Lines()[0], " │ CPU by process")

	narrow := pane(t, view.ArrangeStack, 70, 30, newListSource())
	narrow, _ = press(t, narrow, keyList("tab", "enter")...)
	assert.False(t, narrow.Side())
	assert.True(t, strings.HasPrefix(narrow.Lines()[0], "CPU by process"))

	row := pane(t, view.ArrangeRow, 160, 30, newListSource())
	row, _ = press(t, row, keyList("tab", "enter")...)
	assert.False(t, row.Side())

	src := newListSource()
	m := pane(t, view.ArrangeStack, 160, 30, src)
	m, _ = press(t, m, keyList("tab", "enter")...)
	split := strings.Index(m.Lines()[0], "│")
	src.set(view.DetailCPU, view.Processes("cooler", view.DetailCPU, []view.ProcessUse{{Name: "a-much-longer-name", Value: 99.9, Count: 999}}))
	m, _ = press(t, m, key("space"), key("space"))
	assert.Contains(t, text(m), "a-much-longer-n")
	assert.Equal(t, split, strings.Index(m.Lines()[0], "│"), "a reading moved the split")
}

// --once has no keys, and draws no drawer.
func TestOnceDrawsNoDrawer(t *testing.T) {
	src := newListSource()
	m := tui.New(tui.Options{Sources: []panel.Source{src}, Arrangement: view.ArrangeStack, Once: true})
	next, _ := m.Update(tui.Drawn([]panel.Source{src}))
	assert.NotContains(t, strings.Join(next.(tui.Model).Lines(), "\n"), "CPU by process")
}

// R4. Keys typed faster than the terminal is read arrive as one message;
// "fed" is the filter opened and "ed" typed into it, as three keys would be.
func TestKeysInOneBurstAreEachTaken(t *testing.T) {
	m := pane(t, view.ArrangeStack, 80, 40, newListSource())
	m, _ = press(t, m, keyList("tab", "enter")...)

	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fed")})
	assert.Contains(t, text(m), "/ed▏")
	assert.NotContains(t, text(m), "browser")

	m, _ = press(t, m, key("esc"), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ic")})
	assert.Contains(t, text(m), "[CPU]", "i then c: the IO list, then the CPU list")
}
