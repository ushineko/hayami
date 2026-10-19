package window_test

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

/*
TestHelperSpin is not a test: it is the process the kill test ends. It keeps
four processors busy, so it heads the processor list, and starts one idle
child of itself, so ending it is ending a tree. It is this test binary run
again because a program built for the occasion is an unknown one, and a desk
with Application Control will not run it.
*/
func TestHelperSpin(t *testing.T) {
	switch os.Getenv("HAYAMI_HELPER") {
	case "spin":
		child := exec.Command(os.Args[0], "-test.run=^TestHelperSpin$")
		child.Env = append(os.Environ(), "HAYAMI_HELPER=idle")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(child.Process.Pid)
		for range 4 {
			go func() {
				for {
				}
			}()
		}
		time.Sleep(2 * time.Minute)
	case "idle":
		time.Sleep(2 * time.Minute)
	default:
		t.Skip("only run as a child")
	}
}

// startSpinner runs the spinner, and returns its ID and its child's.
func startSpinner(t *testing.T) (parent, child int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperSpin$")
	// Not a window run: the child would build the panel all over again.
	cmd.Env = append(os.Environ(), "HAYAMI_HELPER=spin", "HAYAMI_WINDOW_TEST=0")
	pipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(pipe).ReadString('\n')
	require.NoError(t, err)
	child, err = strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	t.Cleanup(func() {
		if p, err := os.FindProcess(child); err == nil {
			_ = p.Kill()
		}
	})
	return cmd.Process.Pid, child
}

// stillRunning reports whether a process is still there.
func stillRunning(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // a process ID fits a DWORD
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h) //nolint:errcheck // a test's handle
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// drawerText is the picture's lines of text inside the drawer's columns.
func drawerText(p *picture, from, to int) []span {
	b := p.img.Bounds()
	crop := image.NewNRGBA(image.Rect(0, 0, to-from, b.Dy()))
	for y := range b.Dy() {
		copy(crop.Pix[y*crop.Stride:y*crop.Stride+(to-from)*4], p.img.Pix[y*p.img.Stride+from*4:y*p.img.Stride+to*4])
	}
	return textLines(crop)
}

/*
Spec 065 R24-R27, spec 066, on Windows (R30, R31). From the cooler's
processor list: the spinner's name, then its process, then Kill and
Confirm in the heading; the spinner and the child it started are both
gone. A cooler page's ID is not a bandwidth one, and the kill once read it
as one and ended nothing.

The clicks are found from the pictures: the list is sorted by share and the
spinner is busier than anything else on a desk at rest, so it is the first
row; the process is the second row of its page, under "all"; the kill is
the heading's last word and the confirm takes its place.
*/
func TestAKillFromTheCoolerEndsTheTree(t *testing.T) {
	cpuLabel, _ := processorRows(t)
	parent, child := startSpinner(t)
	s := panelSettings{Sections: []string{"cooler"}, LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle}
	hwnd := start(t, s)
	c := cardOf(t, "cooler", s)
	p := shoot(t, hwnd, "kill-closed.png")
	closed := place(t, hwnd)
	width := int(closed.Right - closed.Left)
	cardRow := p.row(t, c, cpuLabel)

	tap(t, hwnd, width/2, (cardRow.From+cardRow.To)/2)
	time.Sleep(drawerSettle)
	open := place(t, hwnd)
	shift := opened(t, closed, open, "the cooler's")
	from, to := width, int(open.Right-open.Left)
	if shift > 0 {
		from, to = 0, shift
	}
	time.Sleep(listSettle)

	// step clicks the drawer's line at index i, from the left of its text.
	step := func(name string, i int) *picture {
		t.Helper()
		shot := shoot(t, hwnd, name+".png")
		lines := drawerText(shot, from, to)
		require.Greater(t, len(lines), i, "%s: the drawer has %d lines (picture %s)", name, len(lines), shot.path)
		tap(t, hwnd, from+(to-from)/3, (lines[i].From+lines[i].To)/2)
		time.Sleep(listSettle)
		return shot
	}
	// The heading, then the first row: the spinner.
	step("kill-list", 1)
	// The trail, "all", then the one process.
	step("kill-name", 2)

	page := shoot(t, hwnd, "kill-process.png")
	lines := drawerText(page, from, to)
	require.NotEmpty(t, lines, "picture %s", page.path)
	heading := lines[0]
	words := words(page.img, heading, page.gap())
	require.NotEmpty(t, words, "picture %s", page.path)
	kill := words[len(words)-1]
	require.Greater(t, kill.From, from+(to-from)/2,
		"the heading's last word is not at its right: no Kill button (picture %s)", page.path)
	y := (heading.From + heading.To) / 2
	tap(t, hwnd, (kill.From+kill.To)/2, y)
	time.Sleep(drawerSettle)
	asked := shoot(t, hwnd, "kill-confirm.png")
	require.True(t, stillRunning(parent), "a single press ended the process (picture %s)", asked.path)

	// The confirm takes the button's place.
	tap(t, hwnd, (kill.From+kill.To)/2, y)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && (stillRunning(parent) || stillRunning(child)) {
		time.Sleep(200 * time.Millisecond)
	}
	done := shoot(t, hwnd, "kill-done.png")
	require.False(t, stillRunning(parent), "the process is still there (picture %s)", done.path)
	require.False(t, stillRunning(child), fmt.Sprintf("the process's child is still there (picture %s)", done.path))
}
