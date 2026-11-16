package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The drawer in the pane (spec 098): what is behind a card, as the window's
drawer shows it, read-only. A card is chosen with Tab and opened with Enter;
inside, the list's keys switch lists, f filters, the arrows mark a line,
Enter drills into it, space pauses the reading and Esc goes back a step.

The detail is the same view.Detail from the same source, read at the same
interval, and let go when it is no longer shown.
*/

// paneDrawer is an open drawer's state.
type paneDrawer struct {
	section string
	// root is the list opened or switched to, and id what is shown: the
	// list, or a drill page under it.
	root, id string
	detail   view.Detail

	// gen counts what the drawer has shown, so a reading of something it
	// has since left is dropped.
	gen   int
	reads int

	paused    bool
	filter    string
	filtering bool
	marked    int
	scroll    int
}

// detailRead carries a detail's reading back to the model.
type detailRead struct {
	gen    int
	detail view.Detail
}

// detailTick asks for the next reading of the shown detail.
type detailTick struct{ gen int }

// DrawerMinWidth is the narrowest drawer the pane draws: the window's usual
// drawer, 32 ems, at about 0.6 em a monospace character. A list without
// columns, the bandwidth's two rates, is laid out to fit it.
const DrawerMinWidth = 52

// CardsMinWidth is the narrowest the cards are drawn beside a drawer; a pane
// with less room gives the drawer all of it until Esc.
const CardsMinWidth = 56

// detailer is a section's source as a Detailer, where it is one with
// something to offer.
func (m Model) detailer(key string) (panel.Detailer, bool) {
	d, ok := m.source(key).(panel.Detailer)
	if !ok || len(d.Details()) == 0 {
		return nil, false
	}
	return d, true
}

// choosable are the cards a person can open, in the order they are drawn.
func (m Model) choosable() []string {
	var out []string
	for _, s := range m.Sections() {
		if _, ok := m.detailer(s.Key); ok {
			out = append(out, s.Key)
		}
	}
	return out
}

// choose moves the chosen card by step among the choosable ones.
func (m *Model) choose(step int) {
	keys := m.choosable()
	if len(keys) == 0 {
		return
	}
	at := -1
	for i, k := range keys {
		if k == m.chosen {
			at = i
		}
	}
	if at < 0 {
		if step > 0 {
			at = -1
		} else {
			at = 0
		}
	}
	m.chosen = keys[(at+step+len(keys))%len(keys)]
}

// choices are the lists a person switches between where id is one of them,
// as the window's switch offers them: those with a label.
func choices(d panel.Detailer, id string) []view.DetailInfo {
	var out []view.DetailInfo
	among := false
	for _, info := range d.Details() {
		if info.Label == "" {
			continue
		}
		out = append(out, info)
		among = among || info.ID == id
	}
	if !among {
		return nil
	}
	return out
}

// opensOn is the list a card opens on: the one it last showed, the one its
// section names, or the first with a label (spec 062).
func (m Model) opensOn(key string, d panel.Detailer) string {
	has := func(id string) bool {
		for _, info := range d.Details() {
			if info.ID == id && id != "" {
				return true
			}
		}
		return false
	}
	if last := m.last[key]; has(last) {
		return last
	}
	if opens := m.source(key).Section().Opens; has(opens) {
		return opens
	}
	for _, info := range d.Details() {
		if info.Label != "" {
			return info.ID
		}
	}
	return d.Details()[0].ID
}

// open shows a detail of a section in the drawer, letting go of what it
// showed, and asks for its first reading.
func (m *Model) open(key, root, id string) tea.Cmd {
	d, ok := m.detailer(key)
	if !ok {
		return nil
	}
	gen, filter := 1, ""
	if m.dr != nil {
		gen = m.dr.gen + 1
		// A drill and the way back out of it keep the list's filter: the
		// list a person comes back to is the one they left.
		if m.dr.section == key && m.dr.root == root {
			filter = m.dr.filter
		}
		m.release()
	}
	m.dr = &paneDrawer{section: key, root: root, id: id, gen: gen, marked: -1, filter: filter,
		detail: view.Detail{Title: title(d, id)}}
	m.chosen, m.last[key] = key, root
	return readDetail(d, id, gen)
}

// title is the heading a detail is asked under, before its first reading.
func title(d panel.Detailer, id string) string {
	for _, info := range d.Details() {
		if info.ID == id {
			return info.Title
		}
	}
	return ""
}

// release lets go of what the drawer shows.
func (m *Model) release() {
	if m.dr == nil {
		return
	}
	if d, ok := m.source(m.dr.section).(panel.Detailer); ok {
		d.CloseDetail(m.dr.id)
	}
}

// closeDrawer lets go of what the drawer shows and closes it.
func (m *Model) closeDrawer() {
	m.release()
	m.dr = nil
}

// readDetail reads a detail once.
func readDetail(d panel.Detailer, id string, gen int) tea.Cmd {
	return func() tea.Msg {
		detail, _ := d.PollDetail(context.Background(), id)
		return detailRead{gen: gen, detail: detail}
	}
}

// read takes a detail's reading, and asks for the next at its interval.
func (m *Model) read(msg detailRead) tea.Cmd {
	if m.dr == nil || msg.gen != m.dr.gen {
		return nil
	}
	d, ok := m.detailer(m.dr.section)
	if !ok {
		return nil
	}
	// The heading a failed read carries is the one it was asked under.
	if msg.detail.Title == "" {
		msg.detail.Title = m.dr.detail.Title
	}
	m.dr.detail = msg.detail
	m.dr.reads++
	gen := msg.gen
	return tea.Tick(d.DetailInterval(m.dr.id), func(time.Time) tea.Msg { return detailTick{gen: gen} })
}

// tickDetail reads the shown detail again, or, paused, waits another
// interval. The first two readings are taken whether or not it is paused, as
// the window's are: a rate is a difference, and a page held on its first
// reading would show a busy process as idle.
func (m *Model) tickDetail(msg detailTick) tea.Cmd {
	if m.dr == nil || msg.gen != m.dr.gen {
		return nil
	}
	d, ok := m.detailer(m.dr.section)
	if !ok {
		return nil
	}
	if m.dr.paused && m.dr.reads >= 2 {
		gen := msg.gen
		return tea.Tick(d.DetailInterval(m.dr.id), func(time.Time) tea.Msg { return detailTick{gen: gen} })
	}
	return readDetail(d, m.dr.id, m.dr.gen)
}

// visible is the detail as the drawer draws it: filtered, without actions.
func (m Model) visible() view.Detail {
	return view.PaneDetail(m.dr.detail, m.dr.filter)
}

// drawerKey handles a key while the drawer is open. handled is false for a
// key the drawer leaves to the pane.
func (m *Model) drawerKey(msg tea.KeyMsg) (cmd tea.Cmd, handled bool) {
	dr := m.dr
	if dr.filtering {
		switch msg.Type {
		case tea.KeyEnter:
			dr.filtering = false
		case tea.KeyEsc:
			dr.filter, dr.filtering = "", false
		case tea.KeyBackspace:
			if r := []rune(dr.filter); len(r) > 0 {
				dr.filter = string(r[:len(r)-1])
			}
		case tea.KeyRunes, tea.KeySpace:
			dr.filter += string(msg.Runes)
		case tea.KeyCtrlC:
			return tea.Quit, true
		}
		dr.marked = -1
		return nil, true
	}

	src, _ := m.detailer(dr.section)
	switch msg.String() {
	case "esc":
		return m.back(src), true
	case "tab":
		m.choose(1)
		return nil, true
	case "shift+tab":
		m.choose(-1)
		return nil, true
	case "enter":
		if m.chosen != "" && m.chosen != dr.section {
			if d, ok := m.detailer(m.chosen); ok {
				root := m.opensOn(m.chosen, d)
				return m.open(m.chosen, root, root), true
			}
		}
		lines := view.Selectable(m.visible())
		if dr.marked >= 0 && dr.marked < len(lines) && lines[dr.marked].Open != "" {
			return m.open(dr.section, dr.root, lines[dr.marked].Open), true
		}
		return nil, true
	case "down", "j":
		m.mark(1)
		return nil, true
	case "up", "k":
		m.mark(-1)
		return nil, true
	case "pgdown":
		dr.scroll += max(m.height-4, 1)
		return nil, true
	case "pgup":
		dr.scroll = max(dr.scroll-max(m.height-4, 1), 0)
		return nil, true
	case " ":
		dr.paused = !dr.paused
		if !dr.paused && src != nil {
			// Resuming reads at once rather than at the next tick.
			dr.gen++
			dr.reads = max(dr.reads, 2)
			return readDetail(src, dr.id, dr.gen), true
		}
		return nil, true
	case "f":
		if dr.detail.Filterable {
			dr.filtering = true
			return nil, true
		}
	}
	// A list's key switches to it, as the window's switch does.
	if src != nil && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		for _, info := range choices(src, dr.root) {
			if strings.EqualFold(info.Key, string(msg.Runes)) {
				if info.ID != dr.root || dr.id != dr.root {
					return m.open(dr.section, info.ID, info.ID), true
				}
				return nil, true
			}
		}
	}
	return nil, false
}

// back is Esc: up from a drill page, then out of the list's filter, then the
// drawer closes.
func (m *Model) back(src panel.Detailer) tea.Cmd {
	dr := m.dr
	switch {
	case dr.id != dr.root:
		up := dr.root
		if drl, ok := src.(panel.Driller); ok && drl.Drills(dr.id) {
			up = drl.DrillUp(dr.id)
		}
		return m.open(dr.section, dr.root, up)
	case dr.filter != "":
		dr.filter, dr.marked = "", -1
		return nil
	default:
		m.closeDrawer()
		return nil
	}
}

// mark moves the marked line by step, and scrolls a detail of parts so it
// stays in sight.
func (m *Model) mark(step int) {
	n := len(view.Selectable(m.visible()))
	if n == 0 {
		m.dr.marked = -1
		return
	}
	m.dr.marked = min(max(m.dr.marked+step, 0), n-1)
	if len(m.dr.detail.Parts) > 0 {
		// A line a part's headings put further down is near enough: the
		// page scrolls by the marked line's place among the lines.
		m.dr.scroll = max(m.dr.marked-(m.height-6), 0)
	}
}

// drawerWidth is the width the drawer asks for: the widest list its switch
// offers, as the window's does (spec 093), and never under DrawerMinWidth.
func (m Model) drawerWidth() int {
	want := view.ColumnsWant(m.dr.root)
	if d, ok := m.detailer(m.dr.section); ok {
		for _, info := range choices(d, m.dr.root) {
			want = max(want, view.ColumnsWant(info.ID))
		}
	}
	return max(DrawerMinWidth, 2+view.DetailNameWidth+2+want)
}

// drawerLines are the drawer at a width and a height.
func (m Model) drawerLines(width, height int) []string {
	shape := view.DrawerShape{
		Shown: m.dr.root, Filter: m.dr.filter, Filtering: m.dr.filtering,
		Marked: m.dr.marked, Paused: m.dr.paused, Scroll: m.dr.scroll,
	}
	if d, ok := m.detailer(m.dr.section); ok {
		shape.Choices = choices(d, m.dr.root)
	}
	return view.RenderDetail(m.visible(), width, height, shape)
}
