package panel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

/*
Drill-downs: a detail reached from a row of another (spec 065, spec 066).

A drill is a detail whose ID carries the path to it, the detail it was
reached through first: `cpu/firefox`, `cpu/firefox/4411`,
`peers/eno2/fpsd/1534`. The path is in the ID and nowhere else, so a drill
is polled, closed and restored exactly as any detail is, and no shell needs
a notion of where a person has navigated to.

How deep the path goes is the section's own: the bandwidth breakdown has an
interface before the name and a peer beside the process, the cooler's lists
have neither. So a section says where a drill leads back to, and what is
here is only what every section's drills have in common.
*/

// DrillRoot is the detail a drill was reached through, and Steps the path
// below it. A detail that is not a drill has no steps.
func DrillRoot(id string) (root string, steps []string) {
	parts := strings.Split(id, "/")
	return parts[0], parts[1:]
}

// DrillUnder is the ID of a drill one step below another.
func DrillUnder(id string, step string) string { return id + "/" + step }

// DrillOut is the ID one step back, which for the first step is the detail
// the drill was reached through.
func DrillOut(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[:i]
	}
	return id
}

/*
Trail is the first line of a drill: where the page is.

The way out is a button in the heading, the same corner on every page, and
not a row in a list of readings. The crumbs say where the reader is; they
are not themselves the way back.
*/
func Trail(head, id string) []view.DetailLine {
	_, steps := DrillRoot(id)
	crumbs := []string{head}
	for _, s := range steps {
		crumbs = append(crumbs, strings.TrimPrefix(s, "@"))
	}
	return []view.DetailLine{{Name: strings.Join(crumbs, crumbSeparator), Heading: true}}
}

/*
crumbSeparator is ASCII, and deliberately plainer than the glyph the
heading's buttons carry.

Those are chosen by asking the face what it can draw (fynedesygn spec 073),
which this cannot do: a section builds its readings without a theme, and it
should not be given one for the sake of a decoration. A layer that cannot
ask the face has no business guessing, and guessing here is exactly what
drew a missing-glyph box on Windows (spec 066 gap 3).
*/
const crumbSeparator = " > "

/*
ProcessLines is one process: who it is, who started it, what it holds, and
the offer to end it.

The same page whatever list it was reached from. What a process is doing to
the processor, the card, the memory or the network is the line above these,
which the section puts there; everything below is the process itself.

A process that is no longer there says so and offers nothing: the kill is
carried as a line the shells read (view.ActKill) and draw as they choose,
and a page about a process that has gone does not carry it.
*/
func ProcessLines(pid int) []view.DetailLine {
	info := core.ProcessInfo(pid)
	if info.Gone {
		return nil
	}
	add := func(name, value string) view.DetailLine {
		return view.DetailLine{Name: name, Value: value}
	}
	out := []view.DetailLine{
		add("pid", strconv.Itoa(info.PID)),
		add("parent", info.Parent),
		add("state", info.State),
		add("threads", strconv.Itoa(info.Threads)),
		add("sockets", strconv.Itoa(info.Sockets)),
	}
	if children := core.ProcessChildren(pid); len(children) > 0 {
		out = append(out, view.DetailLine{Name: "children", Heading: true})
		for _, c := range children {
			out = append(out, view.DetailLine{Name: "  " + c})
		}
	}
	return append(out, view.DetailLine{
		Name: "kill", Value: core.EndAsk + ", then " + core.EndForce, Open: view.ActKill,
	})
}

/*
DrillPID is the process a drill is about, from the last step of its path:
`cpu/firefox/4411` and `peers/eno2/fpsd/1534` are both about a process, and
a name or a peer (`@host`) is not a number. A shell that has to act on a
process reads it here and not from one section's notion of its path.
*/
func DrillPID(id string) (int, bool) {
	_, steps := DrillRoot(id)
	if len(steps) == 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(steps[len(steps)-1])
	return pid, err == nil && pid > 0
}

// PIDLabel names a process's row in the list of processes behind a name.
func PIDLabel(pid int) string {
	if pid == 0 {
		return "not attributable"
	}
	return fmt.Sprintf("pid %d", pid)
}

/*
AsPage turns a drill's lines into the one part a drawer draws whole.

A detail of lines is cut to the drawer's height and never scrolls, and the
cut is a prefix: the rows at the end go first. On a drill that is the wrong
end. Beside a short card the page about a process kept "now" and "pid" and
dropped parent, state, threads, sockets and children -- the answers a person
opened the page to read -- and said nothing about having dropped them
(fynedesygn spec 072 gap 6, this spec's gap 4).

A detail of parts is drawn whole and scrolls. So a drill is one part: its
trail becomes the part's heading, which is where a heading belongs anyway,
and everything under it is the part's lines.
*/
func AsPage(d view.Detail) view.Detail {
	if len(d.Parts) > 0 || len(d.Lines) == 0 {
		return d
	}
	head, rest := d.Lines[0], d.Lines[1:]
	title := head.Name
	if !head.Heading {
		// No trail to promote: keep every line and let the part be headed
		// by the detail itself.
		title, rest = d.Title, d.Lines
	}
	d.Parts = []view.Detail{{
		Section: d.Section,
		ID:      d.ID,
		Title:   title,
		Lines:   rest,
	}}
	d.Lines = nil
	return d
}

// DrillLines are a drill's own lines, wherever it keeps them: a page made by
// AsPage holds them in its one part.
func DrillLines(d view.Detail) []view.DetailLine {
	if len(d.Parts) > 0 {
		return d.Parts[0].Lines
	}
	return d.Lines
}
