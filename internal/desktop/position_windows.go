package desktop

import (
	"errors"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	enumWindows              = user32.NewProc("EnumWindows")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	getWindowTextW           = user32.NewProc("GetWindowTextW")
	isWindowVisible          = user32.NewProc("IsWindowVisible")
	getWindowRect            = user32.NewProc("GetWindowRect")
	setWindowPos             = user32.NewProc("SetWindowPos")
	monitorFromRect          = user32.NewProc("MonitorFromRect")
)

const (
	swpNoSize            = 0x0001
	swpNoZOrder          = 0x0004
	swpNoActivate        = 0x0010
	monitorDefaultToNull = 0
)

// pollEvery is how often the panel's place is looked at. Windows raises no
// event another thread can wait on for a window it does not own the loop of,
// and a glance panel moves a few times a day; one call a second is nothing.
const pollEvery = time.Second

// appearWait bounds how long Restore waits for the window to exist, and
// appearPoll is how often it looks. Restore is asked for before the window is
// shown, as on Linux, where the KWin script waits for it.
const (
	appearWait = 10 * time.Second
	appearPoll = 50 * time.Millisecond
)

type rect struct{ Left, Top, Right, Bottom int32 }

/*
Position watches where the panel is and puts it back, on Windows.

Here the program can do both itself, which Wayland will not let it: the window
is found among this process's own top-level windows by its title, read with
GetWindowRect and placed with SetWindowPos. The window's own thread is not
needed for either.

It matters more here than on Linux. A frameless window on Windows has no
Alt-drag behind it, so a panel restored somewhere that is no longer a screen
could not be brought back; a remembered place that no monitor covers is not
restored, and the panel opens where it opens.
*/
type Position struct {
	title string

	mu   sync.Mutex
	stop chan struct{}
}

// NewPosition builds the watcher for the window titled title. Nothing happens
// until Watch is called. The app ID is Linux's matching key and unused here:
// the title alone tells the panel from the preferences window.
func NewPosition(_, title string) *Position { return &Position{title: title} }

// Watch reports each place the panel is moved to. The place it has when the
// watch starts is not reported: that is where it was put, not where it went.
func (p *Position) Watch(onMove func(x, y int)) error {
	stop := p.started()
	if stop == nil {
		return nil
	}
	go func() {
		tick := time.NewTicker(pollEvery)
		defer tick.Stop()
		var last rect
		seen := false
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			r, ok := windowRect(findWindow(p.title))
			if !ok {
				continue
			}
			if seen && (r.Left != last.Left || r.Top != last.Top) {
				onMove(int(r.Left), int(r.Top))
			}
			last, seen = r, true
		}
	}()
	return nil
}

// Restore puts the panel at x, y once its window exists, unless no monitor
// would show it there. It returns at once; the wait is bounded by appearWait
// and ended by Stop.
func (p *Position) Restore(x, y int) error {
	p.mu.Lock()
	stop := p.stop
	p.mu.Unlock()
	if stop == nil {
		return errors.New("restoring before watching")
	}
	go func() {
		deadline := time.After(appearWait)
		tick := time.NewTicker(appearPoll)
		defer tick.Stop()
		for {
			if hwnd := findWindow(p.title); hwnd != 0 {
				place(hwnd, x, y)
				return
			}
			select {
			case <-stop:
				return
			case <-deadline:
				return
			case <-tick.C:
			}
		}
	}()
	return nil
}

// Stop ends the watch and any wait to restore.
func (p *Position) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}
}

// started makes the stop channel, or returns nil if the watch already runs.
func (p *Position) started() chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil {
		return nil
	}
	p.stop = make(chan struct{})
	return p.stop
}

// place moves hwnd to x, y keeping its size, if a monitor covers the result.
func place(hwnd uintptr, x, y int) {
	r, ok := windowRect(hwnd)
	if !ok {
		return
	}
	w, h := r.Right-r.Left, r.Bottom-r.Top
	target := rect{int32(x), int32(y), int32(x) + w, int32(y) + h}
	if m, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&target)), monitorDefaultToNull); m == 0 {
		return
	}
	_, _, _ = setWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0,
		swpNoSize|swpNoZOrder|swpNoActivate)
}

func windowRect(hwnd uintptr) (rect, bool) {
	var r rect
	if hwnd == 0 {
		return r, false
	}
	ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r, ok != 0
}

// The EnumWindows callback is made once, because syscall.NewCallback's slots
// are never released; what it looks for is passed through these, under enumMu.
var (
	enumOnce     sync.Once
	enumCallback uintptr
	enumMu       sync.Mutex
	enumTitle    string
	enumFound    uintptr
)

// findWindow is this process's visible top-level window titled title, or 0.
func findWindow(title string) uintptr {
	enumOnce.Do(func() {
		pid := uint32(os.Getpid())
		enumCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
			var owner uint32
			_, _, _ = getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
			if owner != pid {
				return 1
			}
			if v, _, _ := isWindowVisible.Call(hwnd); v == 0 {
				return 1
			}
			var buf [256]uint16
			n, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if syscall.UTF16ToString(buf[:n]) != enumTitle {
				return 1
			}
			enumFound = hwnd
			return 0
		})
	})
	enumMu.Lock()
	defer enumMu.Unlock()
	enumTitle, enumFound = title, 0
	_, _, _ = enumWindows.Call(enumCallback, 0)
	return enumFound
}
