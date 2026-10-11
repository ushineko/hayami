package installer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/scripts"
)

// sensorsPackage is LibreHardwareMonitor's winget package, whose directory
// under the person's WinGet packages holds its program.
const sensorsPackage = "LibreHardwareMonitor.LibreHardwareMonitor"

// probeTimeout bounds the one request the check makes. LibreHardwareMonitor
// answers on loopback in milliseconds; a port nothing listens on refuses at
// once.
const probeTimeout = 3 * time.Second

/*
probeSensors is the check: where LibreHardwareMonitor is, whether it runs
and starts with Windows, and whether hayami can read the processor's
temperature from it now. It reads and changes nothing, and needs no
administrator: the same questions the panel asks (core.ProbeLHMHost), and
one GET of data.json.
*/
func probeSensors(ctx context.Context) Sensors {
	s := Sensors{Applies: true, Exe: findSensors(), Host: core.ProbeLHMHost(), URL: sensorsURL()}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	root, err := core.FetchLHM(ctx, &http.Client{Timeout: probeTimeout}, s.URL)
	switch {
	case err == nil:
		if v, label, ok := core.LHMCPUTemperature(root); ok {
			s.Reading = fmt.Sprintf("%s %.1f °C", label, v)
		} else {
			s.Problem = "It answers, and has no processor temperature: its PawnIO driver may not be installed"
		}
	case errors.Is(err, core.ErrLHMAuth):
		s.Problem = "Its web server asks for a password, which hayami does not send"
	case s.Host.Running:
		s.Problem = "It is running, and its web server does not answer at " + s.URL
	case s.Host.Task:
		s.Problem = "It is not running, though it is registered to start with Windows"
	default:
		s.Problem = "It is not running"
	}
	return s
}

// findSensors is LibreHardwareMonitor's program where winget's portable
// install puts it, or "".
func findSensors() string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	found, _ := filepath.Glob(filepath.Join(local, "Microsoft", "WinGet", "Packages", sensorsPackage+"*", "LibreHardwareMonitor.exe"))
	if len(found) == 0 {
		return ""
	}
	return found[0]
}

// sensorsURL is where hayami asks: the cooler section's setting when the
// person has one, or LibreHardwareMonitor's default.
func sensorsURL() string {
	path, err := config.Path()
	if err != nil {
		return core.LHMURL
	}
	if _, err := os.Stat(path); err != nil {
		return core.LHMURL
	}
	store, err := config.Open(path)
	if err != nil {
		return core.LHMURL
	}
	defer func() { _ = store.Close() }()
	if url := strings.TrimSpace(config.Cooler.Get(store.Config()).LHM); url != "" {
		return url
	}
	return core.LHMURL
}

// sensorsScripts are the scripts the set-up runs, from the same files the
// checkout's installer uses.
var sensorsScripts = []string{"install_windows.ps1", "lhm_settings.ps1"}

/*
setUpSensors installs and configures LibreHardwareMonitor by running
install_windows.ps1 -SensorsOnly: the steps install_windows.ps1
-WithSensors has always taken, from the same file, so there is one
description of them and one set of tests (spec 042). The scripts are carried
in the installer and written to a temporary directory for the run.

The script asks Windows for an administrator once, for the one step that
needs it; that prompt is the person's to answer. Its lines go to the
installer's log.
*/
func setUpSensors(ctx context.Context, c *extension.Context) error {
	work, err := os.MkdirTemp("", "hayami-sensors-*")
	if err != nil {
		return fmt.Errorf("making a directory for the scripts: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	dir := filepath.Join(work, "scripts")
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("making a directory for the scripts: %w", err)
	}
	for _, name := range sensorsScripts {
		b, err := fs.ReadFile(scripts.Windows, name)
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	// Windows PowerShell writes a pipe in the console's code page, in which
	// the degree sign of a temperature is another character: it is told to
	// write UTF-8 first. The path is quoted for PowerShell, where a quote
	// in a quoted string is written twice.
	script := strings.ReplaceAll(filepath.Join(dir, sensorsScripts[0]), "'", "''")
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", //nolint:gosec // a script this installer carries, in a directory it made
		"[Console]::OutputEncoding = [Text.Encoding]::UTF8; & '"+script+"' -SensorsOnly; exit $LASTEXITCODE")
	// No console of its own: the wizard is a window, and one opening beside
	// it for a minute would look like a fault.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("starting the set-up: %w", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the set-up: %w", err)
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), " \t\r"); strings.TrimSpace(line) != "" {
			c.Log(extension.Info, line)
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("the set-up script: %w", err)
	}
	return nil
}

// ours are the programs an install holds that may be running.
var ours = []string{"hayami.exe", "hayami-tui.exe"}

// stopWait is how long a stopped program is given to end before the
// uninstaller goes on to its files.
const stopWait = 5 * time.Second

/*
stopPrograms ends every hayami.exe and hayami-tui.exe that runs from root,
and returns the names of those it stopped. A program of the same name from
somewhere else, such as a build in a checkout, is not this install's and is
left alone.
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
		if stop(entry.ProcessID, filepath.Join(root, name)) {
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

// stop ends the process pid when its program is the file at path, and
// reports whether it did.
func stop(pid uint32, path string) bool {
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
	if !strings.EqualFold(filepath.Clean(windows.UTF16ToString(buf[:n])), filepath.Clean(path)) {
		return false
	}
	if err := windows.TerminateProcess(h, 0); err != nil {
		return false
	}
	_, _ = windows.WaitForSingleObject(h, uint32(stopWait/time.Millisecond)) //nolint:gosec // five seconds
	return true
}
