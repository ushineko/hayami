package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcInfo is what is known of one process for a drill-down: who it is, who
// started it, and how much of the machine it is holding.
type ProcInfo struct {
	PID     int
	PPID    int
	Name    string
	Parent  string
	Cmdline string
	State   string
	Threads int
	Sockets int
	Started string
	User    string
	Gone    bool
}

// Children are the processes a process started, as "name (pid)".
type Children []string

/*
ReadProc reads one process, and reports Gone for one that has exited.

A drill-down outlives the reading that opened it: a person stops the list on
a process and reads about it, and the process may be gone by the time they
do. That is a state the page says plainly rather than an error.
*/
func ReadProc(root string, pid int) ProcInfo {
	info := ProcInfo{PID: pid}
	dir := filepath.Join(root, strconv.Itoa(pid))
	status, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		info.Gone = true
		return info
	}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "Name":
			info.Name = value
		case "PPid":
			info.PPID, _ = strconv.Atoi(value)
		case "State":
			info.State = value
		case "Threads":
			info.Threads, _ = strconv.Atoi(value)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		info.Cmdline = strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", " "))
	}
	if fds, err := os.ReadDir(filepath.Join(dir, "fd")); err == nil {
		for _, fd := range fds {
			if link, err := os.Readlink(filepath.Join(dir, "fd", fd.Name())); err == nil &&
				strings.HasPrefix(link, "socket:") {
				info.Sockets++
			}
		}
	}
	if info.PPID > 0 {
		if b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(info.PPID), "comm")); err == nil {
			info.Parent = fmt.Sprintf("%s (%d)", strings.TrimSpace(string(b)), info.PPID)
		}
	}
	return info
}

// ProcChildren are the processes a process started, newest last. It walks
// every process once: /proc has no index from a parent to its children.
func ProcChildren(root string, pid int) Children {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out Children
	for _, e := range entries {
		child, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "status"))
		if err != nil {
			continue
		}
		name, ppid := "", 0
		for _, line := range strings.Split(string(b), "\n") {
			switch {
			case strings.HasPrefix(line, "Name:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			case strings.HasPrefix(line, "PPid:"):
				ppid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
			}
		}
		if ppid == pid {
			out = append(out, fmt.Sprintf("%s (%d)", name, child))
		}
	}
	return out
}

/*
ProcDescendants is a process and everything it started, children before
their parents.

A program is often a tree and not a process: a browser is a parent and forty
children, and ending the parent alone leaves the forty running, reparented
to init, still holding their sockets. Ending them in this order means a
child is signalled while its parent is still there to have been its parent,
which is how a person reads what they asked for.

The whole table is read once and the tree built from it, because /proc has
no index from a parent to its children and asking per process would walk it
once per generation.
*/
func ProcDescendants(root string, pid int) []int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []int{pid}
	}
	kids := map[int][]int{}
	for _, e := range entries {
		child, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "status"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if !strings.HasPrefix(line, "PPid:") {
				continue
			}
			if ppid, err := strconv.Atoi(strings.TrimSpace(
				strings.TrimPrefix(line, "PPid:"))); err == nil {
				kids[ppid] = append(kids[ppid], child)
			}
			break
		}
	}
	// Breadth first for the generations, then reversed, so the deepest are
	// signalled first and the process asked about is signalled last.
	order := []int{pid}
	for i := 0; i < len(order); i++ {
		order = append(order, kids[order[i]]...)
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}
