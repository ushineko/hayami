package panel

import (
	"context"
	"sort"
	"strconv"

	"github.com/ushineko/hayami/internal/view"
)

/*
The cooler's drill-downs (spec 066).

The same shape as the bandwidth breakdown's, one level shallower: its lists
have no interface above a process and no peer beside it, so a name leads to
the processes behind it and a process to itself.

	cpu/<name>          the processes of one name
	cpu/<name>/<pid>    one process

and the same for `gpu` and `memory`. What each level is measured in is the
list's own -- a share of the processors, a card's busiest engine, bytes held
-- and the figure on a row is read from the same place the list reads it.
*/

// Drills reports whether an ID is one of the cooler's drill-downs.
func (c *Cooler) Drills(id string) bool {
	root, steps := DrillRoot(id)
	if len(steps) == 0 || len(steps) > 2 {
		return false
	}
	_, ok := coolerDetails[root]
	return ok
}

// DrillUp is where the way out leads: a process to its name, a name to the
// list it is in.
func (c *Cooler) DrillUp(id string) string { return DrillOut(id) }

// coolerDrill is a drill-down of one of the cooler's lists.
func (c *Cooler) coolerDrill(ctx context.Context, id string) (view.Detail, error) {
	root, steps := DrillRoot(id)
	name := steps[0]
	d := view.Detail{Section: c.Key(), ID: id, Title: view.ProcessTitle(root)}
	d.Lines = Trail(view.ProcessTitle(root), id)

	all, note, err := coolerDetails[root](c, ctx)
	if err != nil {
		return unread(c.Key(), id, view.ProcessTitle(root), err), err
	}
	if root == view.DetailMemory {
		// The list's note is the whole machine's; a page of one program's
		// processes says what each figure is.
		note = "resident set"
	}
	d.Note = note

	var mine []pidUse
	for _, p := range all {
		if p.Name == name {
			mine = append(mine, p)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].Value > mine[j].Value })

	// A process that has gone quiet is not a process that has gone: the
	// page it was opened from is still the page the reader wants.
	if len(mine) == 0 {
		d.Lines = append(d.Lines, view.DetailLine{Name: "idle", Heading: true})
	}

	format := func(name string, indent int, v float64) view.DetailLine {
		return view.DrillLine(root, name, indent, view.ProcessUse{Value: v})
	}
	if len(steps) == 1 {
		// The list's columns but the count: each of these is one process
		// (spec 092 R2).
		uses := make([]view.ProcessUse, len(mine))
		for i, p := range mine {
			uses[i] = p.ProcessUse
		}
		d.Columns = view.OfferColumns(root, uses, false)
		withCells := func(name string, indent int, u view.ProcessUse) view.DetailLine {
			line := view.DrillLine(root, name, indent, u)
			line.Cells = view.Cells(d.Columns, u)
			return line
		}
		d.Lines = append(d.Lines, withCells("all", 0, view.Sum(uses)))
		for _, p := range mine {
			line := withCells(PIDLabel(p.PID), 1, p.ProcessUse)
			if p.PID != 0 {
				line.Open = DrillUnder(id, strconv.Itoa(p.PID))
			}
			d.Lines = append(d.Lines, line)
		}
		return AsPage(d), nil
	}

	pid, _ := strconv.Atoi(steps[1])
	for _, p := range mine {
		if p.PID == pid {
			d.Lines = append(d.Lines, format("now", 0, p.Value))
		}
	}
	d.Lines = append(d.Lines, ProcessLines(pid)...)
	return AsPage(d), nil
}
