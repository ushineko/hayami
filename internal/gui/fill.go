package gui

import (
	"context"
	"time"
)

/*
The drawers fill the room (spec 090, fynedesygn spec 080).

A row of drawers at its cap stops short of the screen's edge by up to a
drawer. The outermost drawer offers Fill, which widens every open drawer by
the same amount until the window reaches the edge; a second press puts them
back.

The cap is the room on the screen, which is the window's to read on Windows
and the compositor's on Plasma. Where nothing can say it there is no edge to
fill to and Fill is not offered.
*/

// roomSettles is how long after the row changes the compositor is asked for
// the room beside it. Asked at once, the room is the window's before the
// resize the change asked for: one drawer more room than there is, and Fill
// was not offered at the cap.
var roomSettles = 250 * time.Millisecond

// offerFill shows Fill on the outermost drawer while the row is at its cap,
// and hides it otherwise. It runs on the UI thread after the row changes.
func (p *Panel) offerFill() {
	pn := p.win.Panel()
	p.kit.fillGen++
	p.noteRowWidth(pn.DrawersWidth())
	if pn.Opened() == 0 || p.tucked.Load() {
		pn.SetFillable(false, nil)
		return
	}
	// A filled row is at its cap: there is no room left to read.
	if pn.Filled() {
		pn.SetFillable(true, p.toggleFill)
		return
	}
	fits, known := pn.FitsHere()
	if known {
		pn.SetFillable(!fits, p.toggleFill)
		return
	}
	pn.SetFillable(false, nil)
	if !p.kit.desk.Asks() {
		return
	}
	gen, desk, settle := p.kit.fillGen, p.kit.desk, roomSettles
	go func() {
		time.Sleep(settle)
		left, right, ok := desk.Room(context.Background())
		p.do(func() {
			if gen != p.kit.fillGen || !ok || pn.Opened() == 0 {
				return
			}
			pn.SetFillable(!pn.Fits(left, right), p.toggleFill)
		})
	}()
}

// rowWidthsKept is how many of the row's recent widths drawerShift knows.
const rowWidthsKept = 8

// noteRowWidth remembers a width the open drawers added to the window.
func (p *Panel) noteRowWidth(w float32) {
	if w <= 0 {
		return
	}
	ws := p.kit.rowWidths
	if n := len(ws); n > 0 && ws[n-1] == w {
		return
	}
	ws = append(ws, w)
	if len(ws) > rowWidthsKept {
		ws = ws[len(ws)-rowWidthsKept:]
	}
	p.kit.rowWidths = ws
}

// toggleFill fills the row or puts it back, and draws the drawers again at
// their new width (spec 090 R4).
func (p *Panel) toggleFill() {
	pn := p.win.Panel()
	if pn.Filled() {
		pn.Unfill()
		p.renderOpen()
		return
	}
	if _, known := pn.FillHere(); known {
		p.renderOpen()
		return
	}
	if !p.kit.desk.Asks() {
		return
	}
	desk := p.kit.desk
	go func() {
		left, right, ok := desk.Room(context.Background())
		if !ok {
			return
		}
		p.do(func() {
			if pn.Fill(left, right) {
				p.renderOpen()
			}
		})
	}()
}

// renderOpen draws every open drawer.
func (p *Panel) renderOpen() {
	for _, d := range p.openDrawers() {
		p.renderOne(d)
	}
}
