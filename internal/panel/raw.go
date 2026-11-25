package panel

import (
	"context"
	"time"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

/*
RawDetail is one reading of a detail as the plain values it is drawn from
(spec 113 R14): the processes and their use, the peers, an account's limits,
a VPN's checks, a day's changes. Names are whole and figures are in their
units; nothing here has been cut, padded or formatted for a column.

**One read serves every reader.** A source reads a detail once, into this;
the shells draw their view.Detail from it (DrawDetail), and the service maps
the same value to its contract. Drawing reads nothing, so a detail drawn
twice was read once.

Exactly one of the kinds below is set for a detail that was read; none for
one that could not be, which carries Unread instead.
*/
type RawDetail struct {
	// Section, ID and Title are the detail's, as view.Detail's are.
	Section string
	ID      string
	Title   string

	// At is when it was read.
	At time.Time

	// Unread is why the detail could not be read; nil for one that was.
	Unread error

	Processes *RawProcesses
	Process   *RawProcess
	Net       *RawNet
	VPN       *RawVPN
	Account   *view.UsageBreakdown
	Overview  *RawOverview
	Day       *RawDay
	Change    *RawChange
}

/*
RawDetailer is a Detailer whose details are read as plain values and drawn
from them. PollDetail is DrawDetail of ReadDetail, so a shell that asks for
the drawing gets what it always got.
*/
type RawDetailer interface {
	Detailer

	// ReadDetail reads one detail. A detail that cannot be read is returned
	// with Unread set, and the error is for a log; an ID the source does not
	// offer is the zero RawDetail and an error.
	ReadDetail(ctx context.Context, id string) (RawDetail, error)

	// DrawDetail is a reading as the shells show it. It reads nothing.
	DrawDetail(raw RawDetail) view.Detail
}

// pollRaw is PollDetail for a source that reads raw: the reading, drawn.
func pollRaw(ctx context.Context, s RawDetailer, id string) (view.Detail, error) {
	raw, err := s.ReadDetail(ctx, id)
	return s.DrawDetail(raw), err
}

// unreadRaw is a detail that could not be read: its heading and the reason.
func unreadRaw(section, id, title string, err error) RawDetail {
	return RawDetail{Section: section, ID: id, Title: title, At: time.Now(), Unread: err}
}

// drawUnread is the drawing of a detail that could not be read, and ok is
// false for one that was.
func drawUnread(raw RawDetail) (view.Detail, bool) {
	if raw.Unread == nil {
		return view.Detail{}, false
	}
	return unread(raw.Section, raw.ID, raw.Title, raw.Unread), true
}

// RawUse is one process's figures in one list, with its PID, which a list of
// names drops and a drill keeps.
type RawUse struct {
	PID int
	view.ProcessUse
}

/*
RawProcesses is one of the cooler's lists, or the processes of one name in
it (a drill, cpu/<name>).

Each figure is the list's own: a share of every processor (cpu), a card's
busiest engine (gpu), bytes held (memory), bytes read and written a second
(io). The summary over the list is the one its list has: CPU, GPU, IO or
Memory, and nil for the others and for a drill.
*/
type RawProcesses struct {
	// List is the list's ID: cpu, gpu, memory or io.
	List string

	// Name is a drill's process name; empty for the whole list.
	Name string

	// Uses are the processes, one per PID. A use with PID 0 and no name is
	// what the disks did that no readable process accounts for (io).
	Uses []RawUse

	// Note is what the figures are, in words: "share of every processor
	// together".
	Note string

	// InUse is the machine's memory in use and its total, which the memory
	// list's note says; nil for another list.
	InUse *MemoryInUse

	CPU    *view.CPUFigures
	GPU    *view.GPUFigures
	IO     *view.IOFigures
	Memory *view.MemoryFigures
}

// MemoryInUse is the memory in use, in bytes, of the whole.
type MemoryInUse struct {
	Used, Total uint64
}

/*
RawProc is one process as a page about it says: who it is, who started it,
and what it holds. Never its command line, its path or its owner (spec 060
R6): a process is its name.
*/
type RawProc struct {
	PID        int
	Name       string
	PPID       int
	ParentName string
	State      string
	Threads    int
	Sockets    int
	Children   []core.ProcRef

	// Gone is a process that is no longer there.
	Gone bool
}

// RawProcess is a drill to one process: cpu/<name>/<pid>.
type RawProcess struct {
	// List is the list it was reached through, and Name the process's name
	// in it.
	List, Name string

	// Now is its figure in that list this reading; nil where it moved
	// nothing.
	Now *RawUse

	// Idle is a reading in which no process of its name moved anything.
	Idle bool

	// Note is what the figure is, as the list's note says.
	Note string

	Proc RawProc
}

/*
RawNet is the network by process, for the bandwidth section's lists and
their drills: the reader's reading, the interfaces it is a share of, and the
whole names of the peers in it. A drill's ID says which part of it the page
is about (bandwidthdrill.go).
*/
type RawNet struct {
	Use core.NetUse

	// Interfaces are the section's interfaces at the reading.
	Interfaces []core.Rates

	// Names are the peers' whole names, by address, where a name is known.
	Names map[string]string

	// Proc is the process a page is about (peers/<iface>/<name>/<pid>); nil
	// for every other page.
	Proc *RawProc
}

// RawVPN is every chosen VPN as the drawer says it.
type RawVPN struct {
	VPNs []view.VPNDetailReading
}

// RawOverview is the usage section's overview: each account's breakdown and
// each forge's fortnight (spec 062 R9, spec 102).
type RawOverview struct {
	Accounts []RawAccount
	Forges   []view.DevReading
}

// RawAccount is one account in the overview: its breakdown, or why it could
// not be read.
type RawAccount struct {
	Label     string
	Breakdown *view.UsageBreakdown
	Err       error
}

// RawDay is one day of a forge's fortnight (spec 102 R6).
type RawDay struct {
	Forge view.DevReading
	Date  string
}

// The states a change's page can be in.
const (
	// ChangeWaiting is a page whose forge has not been read yet.
	ChangeWaiting = "waiting"
	// ChangeGone is a page whose change the forge's last reading no longer
	// lists.
	ChangeGone = "gone"
	// ChangeFound is a page whose change was found, read or being read.
	ChangeFound = "found"
)

// RawChange is a change's page (spec 111): what the day's line said, and the
// change once read.
type RawChange struct {
	// State is one of the Change states.
	State string

	// Forge is the forge's name, "GitHub".
	Forge string

	// Open is the kind of page (view.DevOpenPR, ...), Repo and Ref the
	// change's, and Line the day's line for it.
	Open, Repo, Ref, Line string

	Change  *view.DevChange
	Reading bool
	Failed  *view.Reason
}
