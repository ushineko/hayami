package core

import (
	"fmt"
	"sort"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
One process for a drill-down, on Windows (spec 065 R31).

Everything the page says is read without a handle to the process and without
an administrator: the parent, the threads and their states and the start time
from the process table NtQuerySystemInformation gives (processTable, which the
lists already read), and the sockets from iphlpapi's owner tables. So a
process this user could not open has a page like any other, and there is no
helper to ask, unlike the network by process (spec 061).

**A parent ID is not a parent.** Windows records the ID of the process that
started this one and never updates it: when the parent exits, its number can
be given to a later process, which is then nobody's parent. A parent is
therefore only a parent if it started before the child did, which is the test
Task Manager and Process Explorer make.
*/

// EndAsk and EndForce are the two ways a process is ended, as the page names
// them: asking its windows to close, and TerminateProcess.
const (
	EndAsk   = "close"
	EndForce = "end"
)

// winProc is one row of the process table, as much of it as a drill reads.
type winProc struct {
	PID, PPID int
	Name      string
	Created   int64
	Threads   int
	State     string
}

// ProcessInfo is one process for a drill-down, and Gone for one that has
// exited or was never there.
func ProcessInfo(pid int) ProcInfo {
	info := ProcInfo{PID: pid}
	procs, err := winProcesses()
	self, ok := procs[pid]
	if err != nil || !ok {
		info.Gone = true
		return info
	}
	info.Name, info.State, info.Threads = self.Name, self.State, self.Threads
	if parent, ok := winParent(procs, self); ok {
		info.PPID = parent.PID
		info.Parent = fmt.Sprintf("%s (%d)", parent.Name, parent.PID)
	}
	info.Sockets = socketsOwnedBy(pid)
	return info
}

// ProcessChildren are the processes a process started, newest last.
func ProcessChildren(pid int) Children {
	procs, err := winProcesses()
	if err != nil {
		return nil
	}
	var out Children
	for _, c := range winChildren(procs, pid) {
		out = append(out, fmt.Sprintf("%s (%d)", c.Name, c.PID))
	}
	return out
}

// ProcessTree is a process and everything it started, children before their
// parents, as ProcDescendants is on Linux.
func ProcessTree(pid int) []int {
	procs, err := winProcesses()
	if err != nil {
		return []int{pid}
	}
	return winTree(procs, pid)
}

// winParent is the process that started p, if it is still there: the one
// with p's parent ID that started before p.
func winParent(procs map[int]winProc, p winProc) (winProc, bool) {
	parent, ok := procs[p.PPID]
	if !ok || parent.PID == p.PID || parent.Created > p.Created {
		return winProc{}, false
	}
	return parent, true
}

// winChildren are the processes pid started, oldest first.
func winChildren(procs map[int]winProc, pid int) []winProc {
	var out []winProc
	for _, p := range procs {
		if parent, ok := winParent(procs, p); ok && parent.PID == pid {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Created != out[j].Created {
			return out[i].Created < out[j].Created
		}
		return out[i].PID < out[j].PID
	})
	return out
}

// winTree is pid and its descendants, breadth first and then reversed, so
// the deepest come first and pid itself last.
func winTree(procs map[int]winProc, pid int) []int {
	order := []int{pid}
	seen := map[int]bool{pid: true}
	for i := 0; i < len(order); i++ {
		for _, c := range winChildren(procs, order[i]) {
			if !seen[c.PID] {
				seen[c.PID] = true
				order = append(order, c.PID)
			}
		}
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// winProcesses is the process table by ID. The idle process is left out: it
// is not a process anyone started or can end.
func winProcesses() (map[int]winProc, error) {
	buf, err := processTable()
	if err != nil {
		return nil, err
	}
	out := map[int]winProc{}
	eachProcess(buf, func(p *windows.SYSTEM_PROCESS_INFORMATION) {
		pid := int(p.UniqueProcessID)
		if pid == 0 {
			return
		}
		out[pid] = winProc{
			PID:     pid,
			PPID:    int(p.InheritedFromUniqueProcessID),
			Name:    programName(p.ImageName.String()),
			Created: p.CreateTime,
			Threads: int(p.NumberOfThreads),
			State:   winState(processThreads(p)),
		}
	})
	return out, nil
}

// systemThreadInformation is SYSTEM_THREAD_INFORMATION. The process table
// carries NumberOfThreads of them straight after each process's entry.
type systemThreadInformation struct {
	KernelTime      int64
	UserTime        int64
	CreateTime      int64
	WaitTime        uint32
	StartAddress    uintptr
	UniqueProcess   uintptr
	UniqueThread    uintptr
	Priority        int32
	BasePriority    int32
	ContextSwitches uint32
	ThreadState     uint32
	WaitReason      uint32
}

// processThreads are the thread entries that follow a process's entry.
func processThreads(p *windows.SYSTEM_PROCESS_INFORMATION) []systemThreadInformation {
	if p.NumberOfThreads == 0 {
		return nil
	}
	first := (*systemThreadInformation)(unsafe.Add(unsafe.Pointer(p), unsafe.Sizeof(*p)))
	return unsafe.Slice(first, p.NumberOfThreads)
}

// The KTHREAD_STATE and KWAIT_REASON values winState reads.
const (
	threadReady         = 1
	threadRunning       = 2
	threadStandby       = 3
	threadWaiting       = 5
	threadDeferredReady = 7
	waitSuspended       = 5
)

/*
winState is what a process is doing, from its threads: Windows keeps a state
for each thread and none for the process.

Running if any thread is running or ready to; suspended if every thread is
waiting because it was suspended, which is how Task Manager says it of a
store app put away; otherwise waiting. A process with no threads left is
exiting: it is a table entry kept for whoever still holds a handle to it.
*/
func winState(threads []systemThreadInformation) string {
	if len(threads) == 0 {
		return "exiting"
	}
	suspended := true
	for _, t := range threads {
		switch t.ThreadState {
		case threadReady, threadRunning, threadStandby, threadDeferredReady:
			return "running"
		}
		if t.ThreadState != threadWaiting || t.WaitReason != waitSuspended {
			suspended = false
		}
	}
	if suspended {
		return "suspended"
	}
	return "waiting"
}

var (
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = iphlpapi.NewProc("GetExtendedUdpTable")
)

// The table classes asked for: every TCP socket and every UDP socket, each
// with the ID of the process that holds it.
const (
	tcpTableOwnerPIDAll = 5
	udpTableOwnerPID    = 1
)

// ownerTable is one of the four tables a process's sockets are counted from,
// and where in a row the owner's ID is.
type ownerTable struct {
	proc      *windows.LazyProc
	family    uint32
	class     uint32
	rowSize   uintptr
	pidOffset uintptr
}

// The rows are MIB_TCPROW_OWNER_PID, MIB_TCP6ROW_OWNER_PID,
// MIB_UDPROW_OWNER_PID and MIB_UDP6ROW_OWNER_PID, each with the owner last.
var ownerTables = []ownerTable{
	{procGetExtendedTcpTable, windows.AF_INET, tcpTableOwnerPIDAll, 24, 20},
	{procGetExtendedTcpTable, windows.AF_INET6, tcpTableOwnerPIDAll, 56, 52},
	{procGetExtendedUdpTable, windows.AF_INET, udpTableOwnerPID, 12, 8},
	{procGetExtendedUdpTable, windows.AF_INET6, udpTableOwnerPID, 28, 24},
}

/*
socketsOwnedBy is how many TCP and UDP sockets a process holds.

Not every socket, as the Linux count is: /proc lists a process's Unix sockets
with the rest, and Windows keeps no table of those that this user can read.
A table that could not be read counts nothing rather than failing the page.
*/
func socketsOwnedBy(pid int) int {
	n := 0
	for _, t := range ownerTables {
		buf := t.read()
		if len(buf) < 4 {
			continue
		}
		rows := uintptr(*(*uint32)(unsafe.Pointer(&buf[0])))
		for i := uintptr(0); i < rows; i++ {
			at := 4 + i*t.rowSize + t.pidOffset
			if at+4 > uintptr(len(buf)) {
				break
			}
			if int(*(*uint32)(unsafe.Pointer(&buf[at]))) == pid {
				n++
			}
		}
	}
	return n
}

// read is the table, asked for its size and asked again if it grew.
func (t ownerTable) read() []byte {
	size := uint32(16 << 10)
	for range 4 {
		buf := make([]byte, size)
		r, _, _ := t.proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)),
			0, uintptr(t.family), uintptr(t.class), 0)
		switch windows.Errno(r) {
		case 0:
			return buf
		case windows.ERROR_INSUFFICIENT_BUFFER:
			size += 4 << 10
		default:
			return nil
		}
	}
	return nil
}
