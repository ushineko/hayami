package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	fynetheme "fyne.io/fyne/v2/theme"

	"github.com/ushineko/fynedesygn/glance"

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

The room is the filled drawer's own width (Drawer.Available), less the
pinned name column and the row's gaps. A reading changes no figure's
width, so it changes neither how many are shown nor where they stand.
*/

/*
cellsShown is how many of a detail's columns the drawer's width fits, while
the row is filled; none while it is not.

A drawer at its usual width has room for the count beside a name and a
figure, and the first build showed it there. But the drawer at its usual
width is the one a person has learnt to read, and a list that changed shape
on it would be a change nobody asked for (R7): the columns are what filling
gives, and the width says how many.
*/
func (p *Panel) cellsShown(dr *glance.Drawer, d view.Detail) int {
	if len(d.Columns) == 0 || !p.win.Panel().Filled() {
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
