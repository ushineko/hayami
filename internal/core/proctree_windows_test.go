package core

import (
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The thread entries are found by stepping past the process entry, so both
// must be the size the kernel writes or every state read is garbage.
func TestProcessTableLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the sizes below are the 64-bit ones")
	}
	if got := unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}); got != 256 {
		t.Errorf("SYSTEM_PROCESS_INFORMATION is %d bytes, the kernel writes 256", got)
	}
	if got := unsafe.Sizeof(systemThreadInformation{}); got != 80 {
		t.Errorf("SYSTEM_THREAD_INFORMATION is %d bytes, the kernel writes 80", got)
	}
}

func TestWinState(t *testing.T) {
	th := func(state, reason uint32) systemThreadInformation {
		return systemThreadInformation{ThreadState: state, WaitReason: reason}
	}
	cases := []struct {
		name    string
		threads []systemThreadInformation
		want    string
	}{
		{"no threads", nil, "exiting"},
		{"one running", []systemThreadInformation{th(threadWaiting, 6), th(threadRunning, 0)}, "running"},
		{"one ready", []systemThreadInformation{th(threadReady, 0)}, "running"},
		{"all waiting", []systemThreadInformation{th(threadWaiting, 6), th(threadWaiting, 13)}, "waiting"},
		{"all suspended", []systemThreadInformation{th(threadWaiting, waitSuspended), th(threadWaiting, waitSuspended)}, "suspended"},
		{"some suspended", []systemThreadInformation{th(threadWaiting, waitSuspended), th(threadWaiting, 6)}, "waiting"},
	}
	for _, c := range cases {
		if got := winState(c.threads); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A parent ID that a later process has been given is not that process's
// child: the parent must have started first.
func TestWinTreeIgnoresAReusedParentID(t *testing.T) {
	procs := map[int]winProc{
		10: {PID: 10, PPID: 1, Name: "shell", Created: 100},
		11: {PID: 11, PPID: 10, Name: "child", Created: 200},
		12: {PID: 12, PPID: 11, Name: "grandchild", Created: 300},
		// 20's parent was a 10 that exited; this 10 started after it.
		20: {PID: 20, PPID: 10, Name: "orphan", Created: 50},
	}
	if got, want := winTree(procs, 10), []int{12, 11, 10}; !slices.Equal(got, want) {
		t.Errorf("tree %v, want %v", got, want)
	}
	if _, ok := winParent(procs, procs[20]); ok {
		t.Error("an orphan was given the later process with its parent's ID")
	}
	if p, ok := winParent(procs, procs[11]); !ok || p.PID != 10 {
		t.Errorf("parent of 11 is %v, %v", p, ok)
	}
}

// TestHelperSleep is not a test: it is the child the live tests start.
func TestHelperSleep(t *testing.T) {
	if os.Getenv("HAYAMI_HELPER_SLEEP") == "" {
		t.Skip("only run as a child")
	}
	time.Sleep(time.Minute)
}

// The real table: a child this test starts is on its page, its parent is
// this test, and the tree ends it before its parent.
func TestProcessInfoLive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperSleep$")
	cmd.Env = append(os.Environ(), "HAYAMI_HELPER_SLEEP=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	child, self := cmd.Process.Pid, os.Getpid()

	info := ProcessInfo(child)
	if info.Gone {
		t.Fatal("the child is not in the table")
	}
	if info.PPID != self || !strings.HasSuffix(info.Parent, "("+strconv.Itoa(self)+")") {
		t.Errorf("parent %q (%d), want this test (%d)", info.Parent, info.PPID, self)
	}
	if info.Threads < 1 || info.State == "" {
		t.Errorf("threads %d, state %q", info.Threads, info.State)
	}
	if mine := ProcessInfo(self); mine.Sockets < 1 {
		t.Errorf("this test holds a listener and counts %d sockets", mine.Sockets)
	}
	if !slices.ContainsFunc(ProcessChildren(self), func(c string) bool {
		return strings.HasSuffix(c, "("+strconv.Itoa(child)+")")
	}) {
		t.Errorf("children %v do not include %d", ProcessChildren(self), child)
	}
	tree := ProcessTree(self)
	if i, j := slices.Index(tree, child), slices.Index(tree, self); i < 0 || j != len(tree)-1 || i > j {
		t.Errorf("tree %v: want the child before this test, and this test last", tree)
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if !ProcessInfo(child).Gone {
		t.Error("an ended child is still reported")
	}
}
