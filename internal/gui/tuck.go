package gui

import (
	"context"
	"time"

	"github.com/ushineko/fynedesygn/glance"
)

/*
The panel tucks into a corner (spec 091, fynedesygn spec 081).

The glyph over the cards' top-right corner tucks the panel into a square
with hayami's icon at the cards' bottom corner on the screen-edge side; a
click on the square brings it back. While tucked nothing is read, the cards'
sections or the drawers' details: a tucked panel costs what a hidden section
costs. The drawers come back as they were.

On Plasma the compositor holds the corner (kwin.HoldCornerScript, loaded by
desktop.Drawer.HoldCorner in place of the edge script), and is asked for the
room to choose it. On Windows the window does both. Elsewhere nothing can
hold the corner, and the glyph is not offered.
*/

// edgeReturns is how long after the panel unfolds the edge script goes back
// in place of the corner's: long enough for the compositor to have applied
// the resize with the corner held.
var edgeReturns = 300 * time.Millisecond

// buildTuck offers the glyph where the corner can be held, and stops and
// starts the readings as the panel tucks and unfolds.
func (p *Panel) buildTuck() {
	pn := p.win.Panel()
	pn.SetOnTuck(p.tuckChanged)
	pn.SetTuckable(pn.CanTuck() || p.kit.desk.Asks(), p.askTuck)
}

// askTuck tucks the panel into the corner the room gives. On Plasma the room
// is asked and the corner script loaded off the UI thread, then the panel
// tucks on it.
func (p *Panel) askTuck() {
	pn := p.win.Panel()
	if _, known := pn.TuckHere(); known || !p.kit.desk.Asks() {
		return
	}
	desk := p.kit.desk
	go func() {
		ctx := context.Background()
		left, right, _ := desk.Room(ctx)
		corner := make(chan glance.Side, 1)
		p.do(func() { corner <- pn.TuckCorner(left, right) })
		c := <-corner
		held := desk.HoldCorner(ctx, c == glance.Right)
		p.do(func() {
			pn.SetCornerHeld(held)
			pn.Tuck(c)
		})
	}()
}

// tuckChanged stops every reading when the panel tucks and starts them again
// when it unfolds, at once.
func (p *Panel) tuckChanged(tucked bool) {
	p.tucked.Store(tucked)
	if tucked {
		p.win.Panel().SetFillable(false, nil)
		for _, d := range p.openDrawers() {
			p.stopDetail(d)
		}
		return
	}
	p.wakeShown()
	for _, d := range p.openDrawers() {
		if det, ok := p.detailer(d.section); ok {
			p.renderOne(d)
			p.pollDetail(d, det, d.id)
		}
	}
	p.offerFill()
	p.holdEdgeAgain()
}

// holdEdgeAgain puts the edge script back in place of the corner's once the
// panel has unfolded, for the drawers' side.
func (p *Panel) holdEdgeAgain() {
	if !p.kit.desk.Asks() {
		return
	}
	pn := p.win.Panel()
	desk, wait := p.kit.desk, edgeReturns
	left := pn.Drawer().Side() == glance.Left
	go func() {
		time.Sleep(wait)
		held := desk.HoldEdge(context.Background(), left)
		p.do(func() {
			pn.SetCornerHeld(false)
			pn.Drawer().SetEdgeHeld(held)
		})
	}()
}

// wakeShown wakes the poll of every section shown, so each reads at once.
func (p *Panel) wakeShown() {
	p.shownMu.Lock()
	keys := make([]string, 0, len(p.shown))
	for k := range p.shown {
		keys = append(keys, k)
	}
	p.shownMu.Unlock()
	for _, k := range keys {
		select {
		case p.wake[k] <- struct{}{}:
		default:
		}
	}
}
