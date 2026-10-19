package gui

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperSleep is not a test: it is the child the kill tests end.
func TestHelperSleep(t *testing.T) {
	if os.Getenv("HAYAMI_HELPER_SLEEP") == "" {
		t.Skip("only run as a child")
	}
	time.Sleep(time.Minute)
}

func startChild(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start %s: %v", cmd.Path, err)
	}
	// The handle this keeps is a handle and not the process: an exited
	// child is gone to alive whether or not it has been waited for.
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid
}

func waitGone(t *testing.T, pid int, within time.Duration) {
	t.Helper()
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if len(alive([]int{pid})) == 0 {
			return
		}
	}
	t.Fatalf("pid %d still there after %s", pid, within)
}

// A process with no window has nothing to ask through: the ask counts
// nothing either way, the force ends it, and a second force finds it gone
// rather than refusing.
func TestEndAllWindowless(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperSleep$")
	cmd.Env = append(os.Environ(), "HAYAMI_HELPER_SLEEP=1")
	pid := startChild(t, cmd)

	if sent, refused, err := endAll([]int{pid}, false); err != nil || sent != 0 || refused != 0 {
		t.Fatalf("ask: sent %d, refused %d, %v; want nothing", sent, refused, err)
	}
	if len(alive([]int{pid})) != 1 {
		t.Fatal("asking ended a process it had no way to ask")
	}
	if sent, refused, _ := endAll([]int{pid}, true); sent != 1 || refused != 0 {
		t.Fatalf("force: sent %d, refused %d", sent, refused)
	}
	waitGone(t, pid, 5*time.Second)
	if sent, refused, _ := endAll([]int{pid}, true); sent != 0 || refused != 0 {
		t.Errorf("gone: sent %d, refused %d; gone is neither", sent, refused)
	}
}

/*
A process with a window is asked through it, and closes.

Two waits, and only the second is the test. The first is for the form to be
shown, which the form says itself from its Shown event: a cold runner can
spend a long time loading PowerShell and Windows Forms, and a session with
no desktop to draw on never shows it at all. That is the machine, not the
kill, and the test says so and skips. Once the form is shown, a window is
there to be found, and not asking it is a failure.
*/
func TestEndAllAsksAWindowToClose(t *testing.T) {
	if testing.Short() {
		t.Skip("starts PowerShell")
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
		"Add-Type -AssemblyName System.Windows.Forms; "+
			"$f = New-Object Windows.Forms.Form; $f.Text = 'hayami kill test'; "+
			"$f.Add_Shown({ [Console]::Out.WriteLine('shown'); [Console]::Out.Flush() }); "+
			"[Windows.Forms.Application]::Run($f)")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	pid := startChild(t, cmd)

	shown := make(chan struct{})
	go func() {
		if line, _ := bufio.NewReader(out).ReadString('\n'); strings.TrimSpace(line) == "shown" {
			close(shown)
		}
	}()
	select {
	case <-shown:
	case <-time.After(formShown):
		t.Skipf("PowerShell did not show its form in %s: this session may have no desktop to draw on", formShown)
	}

	var sent int
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end) && sent == 0; {
		sent, _, _ = endAll([]int{pid}, false)
		if sent == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if sent != 1 {
		t.Fatal("the form was shown and its window was never asked to close")
	}
	waitGone(t, pid, 10*time.Second)
}

// formShown is how long PowerShell is given to load Windows Forms and show
// a form: seconds on a desk, and longer on a cold runner.
const formShown = 90 * time.Second

// The kernel's own process is not this user's to end.
func TestEndAllRefusesSystem(t *testing.T) {
	const system = 4
	if sent, refused, _ := endAll([]int{system}, true); sent != 0 || refused != 1 {
		t.Errorf("System: sent %d, refused %d; want a refusal", sent, refused)
	}
	if len(alive([]int{system})) != 1 {
		t.Error("System reported gone")
	}
}
