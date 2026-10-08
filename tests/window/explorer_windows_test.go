package window_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

/*
The panel stays up when Explorer starts it.

A Start menu entry, a shortcut on the desktop and the login shortcut that
-Autostart writes are all started by Explorer, and cobra treats a program whose
parent is explorer.exe as a console tool somebody double-clicked: it prints a
note for them, waits five seconds and exits 1 (its "mousetrap"). The panel is
linked windowed, so the note goes nowhere and the person sees an hourglass and
then nothing. Started from a shell, its parent is the shell and none of it
happens, which is why every other test passed.

So the panel is started here the way a click starts it: a shortcut, opened by
explorer.exe. Its settings are this test's file and it draws the cooler alone,
so it fetches no usage; but Explorer hands it Explorer's environment, not the
test's, so the per-user folders it could write to are the real ones. It is
closed long before its first cache write (CacheInterval), and the test checks
that those folders are as it found them.
*/
func TestThePanelStaysUpWhenExplorerStartsIt(t *testing.T) {
	if !enabled() {
		t.Skip("drives a real window; set HAYAMI_WINDOW_TEST=1 to run it")
	}
	explorer, err := exec.LookPath("explorer.exe")
	if err != nil {
		t.Skip("no Explorer to start the panel")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell to write the shortcut")
	}

	dir := t.TempDir()
	s := panelSettings{Sections: []string{"cooler"}}
	s.x, s.y = awayFromPointer()
	settings := filepath.Join(dir, "settings.yaml")
	require.NoError(t, os.WriteFile(settings, []byte(s.yaml()), 0o600))

	// The real per-user folders the panel could touch, as they are now.
	watched := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "hayami"),
		filepath.Join(os.Getenv("APPDATA"), "hayami"),
	}
	before := snapshot(watched)

	lnk := filepath.Join(dir, "hayami.lnk")
	write := "$s = (New-Object -ComObject WScript.Shell).CreateShortcut('" + lnk + "'); " +
		"$s.TargetPath = '" + binary + "'; $s.Arguments = '--settings \"" + settings + "\"'; " +
		"$s.WorkingDirectory = '" + filepath.Dir(binary) + "'; $s.Save()"
	out, err := exec.CommandContext(t.Context(), powershell, "-NoProfile", "-Command", write).CombinedOutput()
	require.NoError(t, err, "%s", out)

	others := running(binary)
	require.NoError(t, exec.CommandContext(t.Context(), explorer, lnk).Start())

	var pid uint32
	for deadline := time.Now().Add(10 * time.Second); pid == 0 && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		for p := range running(binary) {
			if !others[p] {
				pid = p
			}
		}
	}
	require.NotZero(t, pid, "Explorer did not start the panel")
	proc, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, pid)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = windows.TerminateProcess(proc, 0)
		_, _ = windows.WaitForSingleObject(proc, 5000)
		_ = windows.CloseHandle(proc)
	})

	// cobra's wait is five seconds; past it with room to spare.
	if ev, _ := windows.WaitForSingleObject(proc, 8000); ev == windows.WAIT_OBJECT_0 {
		var code uint32
		_ = windows.GetExitCodeProcess(proc, &code)
		require.Failf(t, "the panel exited", "started by Explorer, it exited with %d within 8 s", code)
	}
	_, err = waitWindow(int(pid), "hayami", 10*time.Second)
	require.NoError(t, err, "started by Explorer, the panel is running but has no window")

	_ = windows.TerminateProcess(proc, 0)
	_, _ = windows.WaitForSingleObject(proc, 5000)
	// A panel the person is running writes those folders itself every
	// CacheInterval, so the check means something only when this test's panel
	// is the only one.
	if others := otherPanels(binary); others > 0 {
		t.Logf("%d other hayami panel(s) running: the per-user folders are theirs to write, not checked", others)
		return
	}
	require.Equal(t, before, snapshot(watched), "the panel started by Explorer changed a real per-user folder")
}

// otherPanels counts hayami.exe processes that are not the one at path.
func otherPanels(path string) int {
	n := 0
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	mine := running(path)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "hayami.exe") && !mine[e.ProcessID] {
			n++
		}
	}
	return n
}

// running is the processes whose image is the file at path.
func running(path string) map[uint32]bool {
	out := map[uint32]bool{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), filepath.Base(path)) {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil &&
			strings.EqualFold(windows.UTF16ToString(buf[:n]), path) {
			out[e.ProcessID] = true
		}
		_ = windows.CloseHandle(h)
	}
	return out
}

// snapshot is every file under the folders, with its size and time.
func snapshot(dirs []string) map[string]string {
	out := map[string]string{}
	for _, d := range dirs {
		_ = filepath.Walk(d, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				out[p] = fmt.Sprint(info.ModTime(), " ", info.Size())
			}
			return nil
		})
	}
	return out
}
