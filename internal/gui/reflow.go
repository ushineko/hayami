package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	fynetheme "fyne.io/fyne/v2/theme"
	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/view"
)

/*
flowProse is a breakdown with its prose lines -- a change's title and
description (spec 112) -- flowed to the drawer's width, measured in the face
the rows draw in, so a paragraph fills the drawer however wide it is. The
view leaves them whole: only the window knows its pixels.

A breakdown with no prose is returned as it is. The last one flowed is
kept, keyed by the page and the width, since a page is drawn every poll.
*/
func (p *Panel) flowProse(d *drawer, detail view.Detail) view.Detail {
	chars := 0
	for _, part := range detail.Parts {
		for _, l := range part.Lines {
			if l.Prose {
				chars += len(l.Name)
			}
		}
	}
	if chars == 0 {
		return detail
	}
	th, _ := p.theme()
	text := th.Size(fynetheme.SizeNameText)
	pad := th.Size(fynetheme.SizeNamePadding)
	room := d.dr.Available().Width - 2*pad
	if room < 4*text {
		// Not laid out yet: the drawer's own width less its margins.
		room = glance.DrawerWidthInEms*text - 2*glance.CardPadH()
	}
	key := fmt.Sprintf("%s|%s|%d|%.1f|%.1f", detail.Section, detail.ID, chars, room, text)
	if d.flowKey == key {
		return d.flowed
	}
	measure := canvas.NewText("", nil)
	measure.TextSize = text
	measure.FontSource = th.Font(fyne.TextStyle{})
	fits := func(s string, indent int) bool {
		measure.Text = s
		return measure.MinSize().Width+float32(indent)*glance.IndentStep*text <= room
	}
	out := detail
	out.Parts = make([]view.Detail, len(detail.Parts))
	for i, part := range detail.Parts {
		part.Lines = view.Reflow(part.Lines, fits)
		out.Parts[i] = part
	}
	d.flowKey, d.flowed = key, out
	return out
}
