//go:build !windows

package core

// ProcessInfo is one process for a drill-down, from this platform's table.
func ProcessInfo(pid int) ProcInfo { return ReadProc(ProcPath, pid) }

// ProcessChildren are the processes a process started.
func ProcessChildren(pid int) Children { return ProcChildren(ProcPath, pid) }

// ProcessTree is a process and everything it started, children first.
func ProcessTree(pid int) []int { return ProcDescendants(ProcPath, pid) }

// EndAsk and EndForce are the two ways a process is ended, as the page names
// them: the signal that asks, and the one that does not.
const (
	EndAsk   = "TERM"
	EndForce = "KILL"
)
