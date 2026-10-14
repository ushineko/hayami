package installer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// panel is the name of the desktop panel's program, which is also what the
// kernel calls its process.
const panel = "hayami"

// stopWait is how long a panel that was asked to stop is given before it is
// made to. A variable, so a test does not wait for it.
var stopWait = 5 * time.Second

/*
stopPrograms stops the desktop panel that runs from the install directory,
or from where install.sh put it, and returns the names of what it stopped.

Linux removes the file of a running program without complaint, so nothing
has to stop for the files' sake as on Windows. The panel stops because
nothing keeps two from running: the installer starts the panel when it is
done, and an install over a running one would leave both on the desk, the
old one reading settings the new one writes.

Only the desktop panel is stopped. A terminal panel is a pane somebody has
open, any number of them may run, and it keeps running from the program it
was started from. A hayami from anywhere else, a checkout's build, is left
alone, and so is another person's.
*/
func stopPrograms(root string) []string {
	if root == "" {
		return nil
	}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	home, _ := os.UserHomeDir()

	var asked []int
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil || pid == os.Getpid() || !runsFrom(pid, root, home) {
			continue
		}
		if syscall.Kill(pid, syscall.SIGTERM) == nil {
			asked = append(asked, pid)
		}
	}

	// The new panel starts after this, so it waits until the old one is
	// gone: two that overlap for a moment are two windows for that moment.
	deadline := time.Now().Add(stopWait)
	for _, pid := range asked {
		for running(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if running(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	stopped := make([]string, len(asked))
	for i := range stopped {
		stopped[i] = panel
	}
	return stopped
}

// stopEarlier stops the panel an earlier install left running, before the
// installer starts the new one.
func stopEarlier(root string) []string { return stopPrograms(root) }

/*
runsFrom reports whether pid is a panel that this install replaces: its
program is inside root, where an installer put it, or is ~/.local/bin/hayami,
where install.sh did.

The kernel's name for the process is what says it is the panel, because the
file may no longer have one: an install that replaced it leaves the process
running from a file that was moved aside or deleted. Another person's
process cannot be read, and is passed over.
*/
func runsFrom(pid int, root, home string) bool {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	comm, err := os.ReadFile(filepath.Join(dir, "comm"))
	if err != nil || strings.TrimSpace(string(comm)) != panel {
		return false
	}
	exe, err := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil {
		return false
	}
	exe = strings.TrimSuffix(exe, " (deleted)")
	if home != "" && exe == filepath.Join(home, ".local", "bin", panel) {
		return true
	}
	rel, err := filepath.Rel(root, exe)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// running reports whether pid still runs a program. A process that has
// ended and not been collected has none.
func running(pid int) bool {
	_, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	return err == nil
}
