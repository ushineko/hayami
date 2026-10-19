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

/*
coolerPIDs is each list's processes with their PIDs, which the lists
themselves throw away: a list is of names, because a browser is forty
processes and a person asks about the browser (spec 060 R6). A drill is the
other question -- which of the forty -- so it reads the same source and
keeps what the list dropped.
*/
func (c *Cooler) coolerPIDs(ctx context.Context, root string) ([]pidUse, string, error) {
	switch root {
	case view.DetailCPU:
		if c.processes == nil {
			return nil, "", errNoProcesses
		}
		processes, err := c.processes.Read(ctx)
		if err != nil {
			return nil, "", err //nolint:wrapcheck // the reader's own account
		}
		var out []pidUse
		for _, p := range processes {
			if p.HasCPU {
				out = append(out, pidUse{PID: p.PID, Name: p.Name, Value: p.CPU})
			}
		}
		return out, "share of every processor together", nil

	case view.DetailMemory:
		if c.processes == nil {
			return nil, "", errNoProcesses
		}
		processes, err := c.processes.Read(ctx)
		if err != nil {
			return nil, "", err //nolint:wrapcheck // the reader's own account
		}
		var out []pidUse
		for _, p := range processes {
			out = append(out, pidUse{PID: p.PID, Name: p.Name, Value: float64(p.Memory)})
		}
		return out, "resident set", nil

	case view.DetailGPU:
		if c.gpuProcesses == nil {
			return nil, "", errNoProcesses
		}
		c.mu.Lock()
		via := c.gpuVia
		c.mu.Unlock()
		processes, err := c.gpuProcesses.Read(ctx, via)
		if err != nil {
			return nil, "", err //nolint:wrapcheck // the reader's own account
		}
		var out []pidUse
		for _, p := range processes {
			out = append(out, pidUse{PID: p.PID, Name: p.Name, Value: p.Load})
		}
		return out, "each program's busiest engine", nil
	}
	return nil, "", errNoProcesses
}

// pidUse is one process's figure in whichever list it was read from.
type pidUse struct {
	PID   int
	Name  string
	Value float64
}

// coolerDrill is a drill-down of one of the cooler's lists.
func (c *Cooler) coolerDrill(ctx context.Context, id string) (view.Detail, error) {
	root, steps := DrillRoot(id)
	name := steps[0]
	d := view.Detail{Section: c.Key(), ID: id, Title: view.ProcessTitle(root)}
	d.Lines = Trail(view.ProcessTitle(root), id)

	all, note, err := c.coolerPIDs(ctx, root)
	if err != nil {
		return unread(c.Key(), id, view.ProcessTitle(root), err), err
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
		var total float64
		for _, p := range mine {
			total += p.Value
		}
		d.Lines = append(d.Lines, format("all", 0, total))
		for _, p := range mine {
			line := format(PIDLabel(p.PID), 1, p.Value)
			if p.PID != 0 {
				line.Open = DrillUnder(id, strconv.Itoa(p.PID))
			}
			d.Lines = append(d.Lines, line)
		}
		return d, nil
	}

	pid, _ := strconv.Atoi(steps[1])
	for _, p := range mine {
		if p.PID == pid {
			d.Lines = append(d.Lines, format("now", 0, p.Value))
		}
	}
	d.Lines = append(d.Lines, ProcessLines(pid)...)
	return d, nil
}
