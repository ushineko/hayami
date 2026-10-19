package gui

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
Ending a process, where there are no signals to ask with (spec 065 R30).

The ask is WM_CLOSE to the process's windows, which is what closing them
does and what taskkill without /F sends: a program with a window is given
the chance to save and go. A process with no window has nothing to ask
through and is neither sent to nor refused; it is ended with the rest after
the grace. The force is TerminateProcess.

The counts mean what they mean on Linux. A process that has already gone is
neither sent to nor refused, because that is the outcome that was wanted. A
refusal is what Windows answers for anything this user may not touch: a
process of another account or a higher integrity, which cannot be opened to
be ended, and whose windows will not take a message from this one.
*/
func endAll(pids []int, force bool) (sent, refused int, err error) {
	if force {
		for _, pid := range pids {
			switch terminate(pid) {
			case outcomeSent:
				sent++
			case outcomeRefused:
				refused++
			}
		}
		return sent, refused, nil
	}
	windowsOf := topWindows()
	for _, pid := range pids {
		if !isAlive(pid) {
			continue
		}
		switch closeWindows(windowsOf[uint32(pid)]) { //nolint:gosec // a process ID fits a DWORD
		case outcomeSent:
			sent++
		case outcomeRefused:
			refused++
		}
	}
	return sent, refused, nil
}

// alive is which of the processes are still there.
func alive(pids []int) []int {
	var out []int
	for _, pid := range pids {
		if isAlive(pid) {
			out = append(out, pid)
		}
	}
	return out
}

// outcome is what became of asking one process.
type outcome int

const (
	outcomeNone    outcome = iota // gone, or nothing to ask through
	outcomeSent                   // asked, or ended
	outcomeRefused                // Windows would not let this user
)

// terminate ends one process with TerminateProcess.
func terminate(pid int) outcome {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(pid)) //nolint:gosec // a process ID fits a DWORD
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return outcomeNone // no process has that ID
		}
		return outcomeRefused
	}
	defer windows.CloseHandle(h) //nolint:errcheck // nothing to do if closing fails
	if exited(h) {
		return outcomeNone
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		// A process that was already ending answers access denied, which
		// is not a refusal: it is going.
		if exited(h) {
			return outcomeNone
		}
		return outcomeRefused
	}
	return outcomeSent
}

/*
isAlive reports whether a process is still there.

One that cannot be opened even to be asked is counted as there: with no way
to ask, the honest answer is not to claim it has gone. Only an ID no process
holds, or a process that has exited, is gone.
*/
func isAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false, uint32(pid)) //nolint:gosec // a process ID fits a DWORD
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(h) //nolint:errcheck // nothing to do if closing fails
	return !exited(h)
}

// exited reports whether the process behind a handle has finished. A handle
// keeps an exited process's entry; it does not keep the process.
func exited(h windows.Handle) bool {
	ev, err := windows.WaitForSingleObject(h, 0)
	if err == nil {
		return ev == windows.WAIT_OBJECT_0
	}
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code != stillActive
}

// stillActive is STILL_ACTIVE, the exit code of a process that has not
// exited.
const stillActive = 259

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindow                = user32.NewProc("GetWindow")
	procPostMessage              = user32.NewProc("PostMessageW")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
)

const (
	gwOwner = 4      // GW_OWNER
	wmClose = 0x0010 // WM_CLOSE
)

/*
topWindows is each process's top-level windows: the visible ones that no
other window owns, which are the ones a person would close. A dialog is
owned by its window and closes with it; a hidden window is not one anyone
can see to close.
*/
func topWindows() map[uint32][]uintptr {
	out := map[uint32][]uintptr{}
	cb := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
			return 1
		}
		if owner, _, _ := procGetWindow.Call(hwnd, gwOwner); owner != 0 {
			return 1
		}
		var pid uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid))) //nolint:errcheck // a zero pid matches nothing
		if pid != 0 {
			out[pid] = append(out[pid], hwnd)
		}
		return 1
	})
	procEnumWindows.Call(cb, 0) //nolint:errcheck // an empty map is the answer to a failed walk
	return out
}

// closeWindows posts WM_CLOSE to a process's windows. A window of an
// elevated program refuses a message from this one (UIPI), which is a
// refusal; a process with no window is not asked.
func closeWindows(hwnds []uintptr) outcome {
	got := outcomeNone
	for _, hwnd := range hwnds {
		if r, _, _ := procPostMessage.Call(hwnd, wmClose, 0, 0); r != 0 {
			return outcomeSent
		}
		got = outcomeRefused
	}
	return got
}
