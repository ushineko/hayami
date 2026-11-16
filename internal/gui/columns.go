package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	fynetheme "fyne.io/fyne/v2/theme"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The columns of a list, as many as the drawer's width fits (spec 092).

A detail says which columns it has and in what order they go; the window
decides how many of them it shows, and the width is all it decides by. Each
cell is a fixed number of monospace characters, so a column fits whole or not
at all, and a column after one that does not fit is not shown either: the
columns are read left to right in the order the detail gave them, and a gap
would say one was missing.

The room is the drawer's width as shown (Drawer.Available), less the pinned
name column and the row's gaps. The drawer opens as wide as its list wants,
room allowing (spec 093), and a reading changes no figure's width, so it
changes neither how many are shown nor where they stand.
*/

/*
cellsShown is how many of a detail's columns the drawer's width fits, filled
or not (spec 093 R6): the drawer opens as wide as its list wants, room
allowing (sizeDrawer), and the width says how many.
*/
func (p *Panel) cellsShown(dr *glance.Drawer, d view.Detail) int {
	if len(d.Columns) == 0 {
		return 0
	}
	figure := view.FigureWidth(d.Lines)
	if figure == 0 {
		return 0
	}
	th, _ := p.theme()
	// The row is a pinned name, a spacer and the value, with the theme's
	// padding between each of them.
	room := dr.Available().Width - p.kit.name - 2*th.Size(fynetheme.SizeNamePadding)
	n := 0
	for n < len(d.Columns) && monoWidth(th, view.ColumnsWidth(figure, d.Columns, n+1)) <= room {
		n++
	}
	return n
}

/*
wantEms is the drawer width, in ems, that shows every column of the lists a
drawer's switch offers beside id (spec 093 R1, R2), worked out as a row is
laid out: the drawer's margins, the pinned name column, the row's paddings,
and the widest list's figure and columns in the monospace face. It is the
lists' kinds', so no reading changes it, and choosing another list on the
switch does not resize the drawer. A drawer whose lists have no columns
wants the usual width.
*/
func (p *Panel) wantEms(d *drawer, src panel.Detailer, id string) float32 {
	root := detailRoot(src, id)
	chars := view.ColumnsWant(root)
	for _, c := range choices(src, root) {
		chars = max(chars, view.ColumnsWant(c.ID))
	}
	if chars == 0 {
		return glance.DrawerWidthInEms
	}
	th, _ := p.theme()
	text := th.Size(fynetheme.SizeNameText)
	// The margins are what the drawer's own width has that its Available
	// does not; a drawer in a filled row is drawn at another width, so its
	// margins are the ones last read.
	if !d.dr.Open() || !p.win.Panel().Filled() {
		p.kit.margins = d.dr.WidthInEms()*text - d.dr.Available().Width
	}
	w := p.kit.margins + p.kit.name + 2*th.Size(fynetheme.SizeNamePadding) + monoWidth(th, chars)
	// A whole pixel over, so that the last column is not lost to rounding.
	return (w + 1) / text
}

/*
sizeDrawer sets a drawer's width before it is shown a detail (spec 093 R3,
R4): its want, as far as the room beside the window takes it; a list wanting
more than the usual width is cut no narrower than that. A closed drawer takes
the room the screen has
for it; an open one, the outermost, its own width and the room beyond; any
other keeps what it has. Where nothing says the room, a closed drawer opens
at the usual width.

It is called when a drawer opens or is given a new subject, never on a
reading.
*/
func (p *Panel) sizeDrawer(d *drawer, src panel.Detailer, id string) {
	pn := p.win.Panel()
	room, known := p.kit.room, p.kit.roomKnown
	p.kit.roomKnown = false
	if !known {
		room, known = pn.RoomHere()
	}
	limit := float32(glance.DrawerWidthInEms)
	switch {
	case !d.dr.Open() && known:
		limit = room
	case d.dr.Open() && d.dr == pn.Outermost() && known:
		limit = d.dr.WidthInEms() + room
	case d.dr.Open():
		limit = d.dr.WidthInEms()
	}
	// A list narrower than the usual width is as wide as it wants; a wider
	// one is cut to the room, but not below the usual width.
	want := p.wantEms(d, src, id)
	d.dr.SetWidthInEms(min(want, max(limit, glance.DrawerWidthInEms)))
}

// monoWidth is how wide n characters of a value are in the theme's monospace
// face, which every row's value is drawn in.
func monoWidth(th fyne.Theme, n int) float32 {
	style := fyne.TextStyle{Monospace: true}
	t := canvas.NewText(strings.Repeat("0", n), nil)
	t.TextSize = th.Size(fynetheme.SizeNameText)
	t.TextStyle = style
	t.FontSource = th.Font(style)
	return t.MinSize().Width
}

// setHeads names the columns shown over the list, or hides the line where
// none is: a drawer at its usual width keeps the list it had (R6, R7).
func (p *Panel) setHeads(r *glance.Row, d view.Detail, n int) {
	if n == 0 {
		r.SetShown(false)
		return
	}
	r.SetLabel("")
	r.Set(p.value(view.Row{Value: view.ColumnHeads(view.FigureWidth(d.Lines), d.Columns, n)}))
	r.SetShown(true)
}
