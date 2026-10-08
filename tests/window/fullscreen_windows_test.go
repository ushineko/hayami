package window_test

import (
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

var (
	fsUser32           = windows.NewLazySystemDLL("user32.dll")
	fsCreateWindowEx   = fsUser32.NewProc("CreateWindowExW")
	fsDestroyWindow    = fsUser32.NewProc("DestroyWindow")
	fsPeekMessage      = fsUser32.NewProc("PeekMessageW")
	fsTranslateMessage = fsUser32.NewProc("TranslateMessage")
	fsDispatchMessage  = fsUser32.NewProc("DispatchMessageW")
	fsForeground       = fsUser32.NewProc("GetForegroundWindow")
	fsSetForeground    = fsUser32.NewProc("SetForegroundWindow")
	fsBringToTop       = fsUser32.NewProc("BringWindowToTop")
	fsAttachInput      = fsUser32.NewProc("AttachThreadInput")
	fsThreadOf         = fsUser32.NewProc("GetWindowThreadProcessId")
	fsIsVisible        = fsUser32.NewProc("IsWindowVisible")
	fsWindowRect       = fsUser32.NewProc("GetWindowRect")
	fsMonitorFromWin   = fsUser32.NewProc("MonitorFromWindow")
	fsMonitorInfo      = fsUser32.NewProc("GetMonitorInfoW")
)

type fsRect struct{ L, T, R, B int32 }

/*
Spec 051. The panel stands aside while a full-screen app is in front on its
monitor, and comes back, in the same place and without the focus, when it
goes.

The full-screen app is a plain borderless popup this test opens over the
panel's monitor: the shape of a browser in full screen or a game in windowed
full screen (fynedesygn spec 059 has the detection). Two sizes, exactly the
monitor and 8 px past every side, because a full-screen window is often a
little larger than its monitor.

It takes the foreground for a few seconds, and skips when a full-screen window
is already in front: a game or a film someone is in the middle of.
*/
func TestThePanelStandsAsideForAFullScreenApp(t *testing.T) {
	skipOverFullScreen(t)
	hwnd := start(t, panelSettings{Sections: []string{"cooler"}, HideForFullscreen: true, Settle: coolerSettle})
	before := fsWindowRectOf(hwnd)
	mon := fsMonitorOf(hwnd)

	for _, c := range []struct {
		name string
		r    fsRect
	}{
		{"exactly the monitor", mon},
		{"8 px past every side", fsRect{mon.L - 8, mon.T - 8, mon.R + 8, mon.B + 8}},
	} {
		began := time.Now()
		closeCover := cover(t, c.r)
		require.True(t, fsWaitVisible(hwnd, false, 3*time.Second), "%s: the panel did not hide", c.name)
		t.Logf("%s: hidden after %v", c.name, time.Since(began).Round(10*time.Millisecond))

		began = time.Now()
		closeCover()
		require.True(t, fsWaitVisible(hwnd, true, 3*time.Second), "%s: the panel did not come back", c.name)
		t.Logf("%s: back after %v", c.name, time.Since(began).Round(10*time.Millisecond))
		after := fsWindowRectOf(hwnd)
		require.Equal(t, [2]int32{before.L, before.T}, [2]int32{after.L, after.T}, "%s: the panel came back somewhere else", c.name)
		fg, _, _ := fsForeground.Call()
		require.NotEqual(t, hwnd, fg, "%s: coming back took the focus", c.name)
	}
}

// With the setting off, a full-screen app in front leaves the panel shown.
func TestThePanelWithTheSettingOffStaysForAFullScreenApp(t *testing.T) {
	skipOverFullScreen(t)
	hwnd := start(t, panelSettings{Sections: []string{"cooler"}, HideForFullscreen: false, Settle: coolerSettle})
	closeCover := cover(t, fsMonitorOf(hwnd))
	defer closeCover()
	require.False(t, fsWaitVisible(hwnd, false, 2500*time.Millisecond), "the panel hid with the setting off")
}

// skipOverFullScreen skips when what is in front already covers its monitor.
func skipOverFullScreen(t *testing.T) {
	t.Helper()
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	fg, _, _ := fsForeground.Call()
	if fg == 0 {
		return
	}
	r, m := fsWindowRectOf(fg), fsMonitorOf(fg)
	if r.L <= m.L && r.T <= m.T && r.R >= m.R && r.B >= m.B {
		t.Skip("a full-screen window is in front (a game or a film); not taking it over")
	}
}

func fsWindowRectOf(hwnd uintptr) fsRect {
	var r fsRect
	_, _, _ = fsWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func fsMonitorOf(hwnd uintptr) fsRect {
	mon, _, _ := fsMonitorFromWin.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST
	info := struct {
		Size          uint32
		Monitor, Work fsRect
		Flags         uint32
	}{}
	info.Size = uint32(unsafe.Sizeof(info))
	_, _, _ = fsMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&info)))
	return info.Monitor
}

func fsWaitVisible(hwnd uintptr, visible bool, d time.Duration) bool {
	for deadline := time.Now().Add(d); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		v, _, _ := fsIsVisible.Call(hwnd)
		if (v != 0) == visible {
			return true
		}
	}
	return false
}

/*
cover opens a borderless popup over r, brings it to the front, and returns
what closes it. The popup has a thread of its own that pumps its messages; it
is brought to the front by joining the input of the thread in front for a
moment, not by pressing a key, which would type into whatever was there.
*/
func cover(t *testing.T, r fsRect) func() {
	t.Helper()
	const (
		wsPopup   = 0x80000000
		wsVisible = 0x10000000
	)
	made := make(chan uintptr, 1)
	stop, gone := make(chan struct{}), make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(gone)
		class, _ := syscall.UTF16PtrFromString("STATIC")
		h, _, _ := fsCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), 0, wsPopup|wsVisible,
			uintptr(r.L), uintptr(r.T), uintptr(r.R-r.L), uintptr(r.B-r.T), 0, 0, 0, 0)
		made <- h
		if h == 0 {
			return
		}
		fg, _, _ := fsForeground.Call()
		other, _, _ := fsThreadOf.Call(fg, 0)
		self := uintptr(windows.GetCurrentThreadId())
		if other != 0 && other != self {
			_, _, _ = fsAttachInput.Call(self, other, 1)
		}
		_, _, _ = fsBringToTop.Call(h)
		_, _, _ = fsSetForeground.Call(h)
		if other != 0 && other != self {
			_, _, _ = fsAttachInput.Call(self, other, 0)
		}
		var msg [48]byte
		for {
			select {
			case <-stop:
				_, _, _ = fsDestroyWindow.Call(h)
				return
			default:
			}
			for {
				got, _, _ := fsPeekMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 1) // PM_REMOVE
				if got == 0 {
					break
				}
				_, _, _ = fsTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
				_, _, _ = fsDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	h := <-made
	require.NotZero(t, h, "the full-screen popup was not made")
	done := false
	closeIt := func() {
		if done {
			return
		}
		done = true
		close(stop)
		<-gone
	}
	t.Cleanup(closeIt)
	return closeIt
}
