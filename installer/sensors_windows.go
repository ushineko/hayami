package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/nethelper"
	"github.com/ushineko/hayami/internal/sensorhelper"
)

/*
probeSensors is the check: whether PawnIO's driver is installed, what there
is of the sensor helper, and whether LibreHardwareMonitor is running. It
reads and changes nothing, and needs no administrator: the service manager
and the process list, as the panel asks them.
*/
func probeSensors(_ context.Context, c *extension.Context) Sensors {
	host := core.ProbeLHMHost()
	return Sensors{Applies: true, PawnIO: host.PawnIO, Helper: sensorhelper.Probe(), Version: c.App().Version, LHM: host.Running}
}

/*
setUpSensors installs PawnIO with winget where it is missing, as the person
running the installer, and then runs the installed hayami-tui's own
`sensor-helper install` as an administrator. The account that may read the
helper is the one running this installer, named to the elevated program
because an administrator's consent may be given by another account.
*/
func setUpSensors(ctx context.Context, c *extension.Context) error {
	if err := sensorhelper.InstallPawnIO(ctx); err != nil {
		return err //nolint:wrapcheck // says which step
	}
	sid, err := nethelper.CurrentUser()
	if err != nil {
		return err //nolint:wrapcheck // says what it was asking
	}
	if err := nethelper.RunElevated(ctx, helperProgram(c), "sensor-helper", "install", "--allow", sid); err != nil {
		return err //nolint:wrapcheck // says what Windows answered
	}
	if !sensorhelper.Probe().Installed {
		return errors.New("the elevated step ended and the sensor helper is not installed") //nolint:err113 // said plainly
	}
	return nil
}

// ours are the programs an install holds that may be running.
var ours = []string{"hayami.exe", "hayami-tui.exe"}

// stopWait is how long a stopped program is given to end before the
// uninstaller goes on to its files.
const stopWait = 5 * time.Second

/*
stopPrograms ends every hayami.exe and hayami-tui.exe whose file is inside
root, wherever in it, and returns the names of those it stopped. A program
of the same name from somewhere else, such as a build in a checkout, is not
this install's and is left alone.

Inside root and not at a fixed place in it: the first version of this looked
for the programs beside the uninstaller, they are in bin, and an uninstall
with the panel running left hayami.exe behind.
*/
func stopPrograms(root string) []string {
	if root == "" {
		return nil
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	var stopped []string
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))} //nolint:gosec // the structure's own size
	for err := windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if !isOurs(name) || entry.ProcessID == uint32(os.Getpid()) { //nolint:gosec // a process ID
			continue
		}
		if stop(entry.ProcessID, root) {
			stopped = append(stopped, name)
		}
	}
	return stopped
}

func isOurs(name string) bool {
	for _, o := range ours {
		if strings.EqualFold(name, o) {
			return true
		}
	}
	return false
}

// stop ends the process pid when its program's file is inside root, and
// reports whether it did.
func stop(pid uint32, root string) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf)) //nolint:gosec // a fixed buffer
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return false
	}
	if !inside(root, windows.UTF16ToString(buf[:n])) {
		return false
	}
	if err := windows.TerminateProcess(h, 0); err != nil {
		return false
	}
	_, _ = windows.WaitForSingleObject(h, uint32(stopWait/time.Millisecond)) //nolint:gosec // five seconds
	return true
}

// inside reports whether the file at path is in the directory root or
// below it, as Windows compares paths: without regard to case.
func inside(root, path string) bool {
	rel, err := filepath.Rel(strings.ToLower(filepath.Clean(root)), strings.ToLower(filepath.Clean(path)))
	return err == nil && rel != "." && filepath.IsLocal(rel)
}
