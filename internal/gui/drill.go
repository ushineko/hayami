package gui

import (
	"fmt"
	"time"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The drawer's navigation (spec 065): pause, drill and the kill.

A detail line says what a click on it asks for (view.DetailLine.Open): a
detail to open, or one of the named actions. The drawer reports which row
was clicked and this turns that into the ask. Nothing here knows what a
bandwidth peer is, so the same wiring serves any section whose lines carry
an Open.
*/

// rowAsk is what the row under a click asks for, by the line it was drawn
// from. Rows and lines are drawn in step, so the index is the line.
func (p *Panel) rowAsk(r *glance.Row) string { return r.ID() }

/*
tappedRow does what a clicked row asks: it pauses, it goes back, it opens a
drill-down, or it works the kill's two steps.

A kill is two clicks and not one. The first turns the row into a question
and the second answers it, and the question is drawn where the row was, so
the pointer is already over the answer and a person who meant to read is not
one click from ending a process.
*/
func (p *Panel) tappedRow(r *glance.Row) {
	ask := p.rowAsk(r)
	if ask == "" {
		return
	}
	d := p.drawer
	src, ok := p.source(d.section)
	if !ok {
		return
	}
	det, isDetailer := src.(panel.Detailer)
	if !isDetailer {
		return
	}
	switch ask {
	case view.ActPause:
		p.setPaused(!d.paused.Load())
	case view.ActBack:
		d.confirming = false
		p.showDetail(det, d.section, p.drillUp(d.id))
	case view.ActKill:
		d.confirming = true
		p.renderDrawer()
	case view.ActCancel:
		d.confirming = false
		p.renderDrawer()
	case view.ActConfirm:
		d.confirming = false
		p.kill(d.id)
	default:
		d.confirming = false
		// A drill is read while it is read: opening one stops the list
		// moving under the reader, which is what they asked for by
		// stopping on it.
		was := d.paused.Load()
		p.showDetail(det, d.section, ask)
		p.setPaused(was)
	}
}

// setPaused stops or restarts the reading of the open detail. The cards
// behind the drawer are not touched: pause is a property of what is being
// read, not of the panel.
func (p *Panel) setPaused(on bool) {
	d := p.drawer
	if d.paused.Load() == on {
		return
	}
	d.paused.Store(on)
	p.renderDrawer()
}

/*
setControls draws the drawer's own state: the button that stops and starts
the reading, and whether the row under the pointer is lit.

The highlight is on only while the reading is stopped. A list that is moving
is not one a person is picking a row out of, and lighting rows as they slide
under a still pointer would be the panel twitching at them.
*/
func (p *Panel) setControls() {
	d := p.drawer
	dr := p.win.Panel().Drawer()
	if !p.drills(d.section) {
		dr.SetButtons()
		dr.SetLeadButton(nil)
		dr.SetHighlight(false)
		return
	}
	if up := p.drillUp(d.id); up != d.id {
		src, _ := p.source(d.section)
		det, _ := src.(panel.Detailer)
		dr.SetLeadButton(&glance.DrawerSpec{
			Label: backLabel,
			Words: []string{backLabel},
			On: func() {
				d.confirming = false
				p.showDetail(det, d.section, up)
			},
		})
	} else {
		dr.SetLeadButton(nil)
	}

	pause := glance.DrawerSpec{
		Label: pauseLabel,
		On:    func() { p.setPaused(true) },
		Words: []string{pauseLabel, resumeLabel},
	}
	if d.paused.Load() {
		pause.Label = resumeLabel
		pause.On = func() { p.setPaused(false) }
		pause.Active = true
	}
	// The rows answer to a click whether or not the reading is stopped, so
	// they light under the pointer whether or not it is: a surface that
	// responds but does not say so is a surface a person pokes at.
	dr.SetHighlight(true)

	// The kill is on the page about one process and nowhere else, and only
	// while there is a process there to end.
	if !p.killable() {
		d.confirming = false
		dr.SetButtons(pause)
		return
	}
	if d.confirming {
		dr.SetButtons(
			glance.DrawerSpec{
				Label: cancelLabel,
				On:    func() { d.confirming = false; p.renderDrawer() },
				Words: []string{cancelLabel},
			},
			glance.DrawerSpec{
				Label: confirmLabel,
				On: func() {
					d.confirming = false
					p.kill(d.id)
				},
				Words: []string{killLabel, confirmLabel},
			},
			pause)
		return
	}
	dr.SetButtons(
		glance.DrawerSpec{
			Label: killLabel,
			On:    func() { d.confirming = true; p.renderDrawer() },
			Words: []string{killLabel, confirmLabel},
		},
		pause)
}

// drillUp asks the section where its way out leads, and stays put for a
// section that has no drills.
func (p *Panel) drillUp(id string) string {
	src, ok := p.source(p.drawer.section)
	if !ok {
		return id
	}
	dr, ok := src.(panel.Driller)
	if !ok {
		return id
	}
	return dr.DrillUp(id)
}

// killable reports whether the open page is about a process that is there to
// be ended. The section says so by offering the action; this does not go
// looking for processes of its own.
func (p *Panel) killable() bool {
	for _, l := range p.drawer.detail.Lines {
		if l.Open == view.ActKill {
			return true
		}
	}
	return false
}

/*
kill ends the process a drill-down is about, and everything it started:
SIGTERM to the tree, and SIGKILL after killGrace to whatever is still there.

The tree and not the process alone. A browser is a parent and forty
children; ending the parent leaves the forty running, reparented to init and
still holding their sockets, which to the person who pressed the button
looks as though nothing happened at all.

The outcome is said on the page, with counts, because some of a tree can be
ended and some refused: anything this user did not start answers with a
refusal, and a panel that said nothing would look as though it had worked.
*/
func (p *Panel) kill(id string) {
	path, ok := panel.ParseDrillPath(id)
	if !ok || path.PID == 0 {
		return
	}
	pids := core.ProcDescendants(core.ProcPath, path.PID)
	sent, refused, err := endAll(pids, false)
	if err != nil {
		// A machine where this is not done yet says so, rather than
		// reporting nought sent as though the processes had refused.
		p.drawer.killed = err.Error()
		p.renderDrawer()
		return
	}
	p.drawer.killed = fmt.Sprintf("TERM sent to %d", sent)
	if refused > 0 {
		p.drawer.killed += fmt.Sprintf(", %d refused", refused)
	}
	p.renderDrawer()

	go func() {
		time.Sleep(killGrace)
		left := alive(pids)
		if len(left) == 0 {
			p.do(func() {
				p.drawer.killed = fmt.Sprintf("gone (%d ended)", len(pids))
				p.renderDrawer()
			})
			return
		}
		killed, denied, _ := endAll(left, true)
		p.do(func() {
			p.drawer.killed = fmt.Sprintf("KILL sent to %d", killed)
			if denied > 0 {
				p.drawer.killed += fmt.Sprintf(", %d refused", denied)
			}
			p.renderDrawer()
		})
	}()
}

// killGrace is how long a process is given to end on a TERM before it is
// sent a KILL.
const killGrace = 5 * time.Second

/*
decorate adds to a reading what the drawer knows and the section does not:
whether the reading is stopped, and where a kill has got to.

The section reads the machine; it has no business knowing that a person has
stopped the list or is being asked to confirm something. So the state lives
here and is drawn on top, which also means every section gets the pause from
the one place.
*/
func (p *Panel) decorate(d view.Detail) view.Detail {
	drawer := p.drawer
	_ = drawer
	if !p.drills(d.Section) {
		return d
	}
	decorated := func(lines []view.DetailLine) []view.DetailLine {
		out := make([]view.DetailLine, 0, len(lines)+2)
		for _, l := range lines {
			// The kill is a button in the heading. The section still says
			// it is available by carrying the line, which killable reads;
			// drawing it as a row as well would offer it twice.
			if l.Open == view.ActKill {
				continue
			}
			out = append(out, l)
		}
		if drawer.killed != "" {
			out = append(out, view.DetailLine{Name: "  " + drawer.killed, Heading: true})
		}
		return out
	}

	if len(d.Parts) > 0 {
		parts := make([]view.Detail, len(d.Parts))
		copy(parts, d.Parts)
		// The pause belongs to the drawer, not to each part of it, so it
		// goes on the first and the rest are left alone.
		if len(parts) > 0 {
			parts[0].Lines = decorated(parts[0].Lines)
		}
		d.Parts = parts
		return d
	}
	d.Lines = decorated(d.Lines)
	return d
}

// drills reports whether a section's details carry the navigation: the
// bandwidth breakdown and the cooler's three lists do.
func (p *Panel) drills(section string) bool {
	src, ok := p.source(section)
	if !ok {
		return false
	}
	dr, ok := src.(panel.Driller)
	return ok && dr.Drills(p.drawer.id)
}

// The button's two words, with a glyph in front of each. These are the
// text-presentation forms: U+25B6 and U+23F8 are drawn from a colour emoji
// font here, which beside a word reads as a picture stuck to it.
const (
	pauseLabel  = "\u2016 Pause"
	resumeLabel = "\u25b8 Resume"
	// U+25C2, the mirror of the U+25B8 used elsewhere and in the trail.
	// U+2190 is not in the panel's face and was drawn as a missing-glyph
	// box; a glyph that is already known to render is the safe one.
	backLabel    = "\u25c2 Back"
	killLabel    = "Kill"
	confirmLabel = "Confirm?"
	cancelLabel  = "Cancel"
)
