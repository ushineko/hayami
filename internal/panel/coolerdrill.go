package panel

import (
	"context"
	"sort"
	"strconv"
	"time"

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

// readDrill reads a drill-down of one of the cooler's lists: the processes
// of one name, or one of them.
func (c *Cooler) readDrill(ctx context.Context, id string) (RawDetail, error) {
	root, steps := DrillRoot(id)
	name := steps[0]
	title := view.ProcessTitle(root)

	all, note, err := coolerDetails[root](c, ctx)
	if err != nil {
		return unreadRaw(c.Key(), id, title, err), err
	}
	if root == view.DetailMemory {
		// The list's note is the whole machine's; a page of one program's
		// processes says what each figure is.
		note = "resident set"
	}

	var mine []RawUse
	for _, p := range all {
		if p.Name == name {
			mine = append(mine, p)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].Value > mine[j].Value })

	raw := RawDetail{Section: c.Key(), ID: id, Title: title, At: time.Now()}
	if len(steps) == 1 {
		raw.Processes = &RawProcesses{List: root, Name: name, Uses: mine, Note: note}
		return raw, nil
	}
	pid, _ := strconv.Atoi(steps[1])
	p := &RawProcess{List: root, Name: name, Idle: len(mine) == 0, Note: note, Proc: readProc(pid)}
	for i := range mine {
		if mine[i].PID == pid {
			p.Now = &mine[i]
		}
	}
	raw.Process = p
	return raw, nil
}

// coolerPage is a drill's page before its lines: its heading, its trail, and
// that its name is idle where it is.
func coolerPage(raw RawDetail, list, note string, idle bool) view.Detail {
	d := view.Detail{Section: raw.Section, ID: raw.ID, Title: raw.Title, Note: note}
	d.Lines = Trail(view.ProcessTitle(list), raw.ID)
	// A process that has gone quiet is not a process that has gone: the
	// page it was opened from is still the page the reader wants.
	if idle {
		d.Lines = append(d.Lines, view.DetailLine{Name: "idle", Heading: true})
	}
	return d
}

// drawCoolerName is the processes of one name: their sum, then each, leading
// to its own page.
func drawCoolerName(raw RawDetail) view.Detail {
	list := raw.Processes
	root := list.List
	d := coolerPage(raw, root, list.Note, len(list.Uses) == 0)
	// The list's columns but the count: each of these is one process
	// (spec 092 R2).
	uses := make([]view.ProcessUse, len(list.Uses))
	for i, p := range list.Uses {
		uses[i] = p.ProcessUse
	}
	d.Columns = view.OfferColumns(root, uses, false)
	withCells := func(name string, indent int, u view.ProcessUse) view.DetailLine {
		line := view.DrillLine(root, name, indent, u)
		line.Cells = view.Cells(d.Columns, u)
		return line
	}
	d.Lines = append(d.Lines, withCells("all", 0, view.Sum(uses)))
	for _, p := range list.Uses {
		line := withCells(PIDLabel(p.PID), 1, p.ProcessUse)
		if p.PID != 0 {
			line.Open = DrillUnder(raw.ID, strconv.Itoa(p.PID))
		}
		d.Lines = append(d.Lines, line)
	}
	return AsPage(d)
}

// drawCoolerProcess is one process: its figure now, then the process itself.
func drawCoolerProcess(raw RawDetail) view.Detail {
	p := raw.Process
	d := coolerPage(raw, p.List, p.Note, p.Idle)
	if p.Now != nil {
		d.Lines = append(d.Lines, view.DrillLine(p.List, "now", 0, view.ProcessUse{Value: p.Now.Value}))
	}
	d.Lines = append(d.Lines, ProcessLines(p.Proc)...)
	return AsPage(d)
}
