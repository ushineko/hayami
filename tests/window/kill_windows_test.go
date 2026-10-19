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

// drawer is the drawer's columns of a picture, as a picture of their own:
// the cards beside it have lines and words at the same heights.
func drawer(p *picture, from, to int) *picture {
	b := p.img.Bounds()
	crop := image.NewNRGBA(image.Rect(0, 0, to-from, b.Dy()))
	for y := range b.Dy() {
		copy(crop.Pix[y*crop.Stride:y*crop.Stride+(to-from)*4], p.img.Pix[y*p.img.Stride+from*4:y*p.img.Stride+to*4])
	}
	return &picture{img: crop, path: p.path, text: textLines(crop)}
}

// figured are a drawer's rows that carry a figure at their right: the list's
// rows, and not the heading, the switch, the filter or the note under them.
func figured(d *picture) []span {
	var out []span
	for i, line := range d.text {
		if i == 0 {
			continue // the heading, whose right holds buttons
		}
		ws := d.words(line)
		if len(ws) > 1 && ws[len(ws)-1].From > d.img.Bounds().Dx()*2/3 {
			out = append(out, line)
		}
	}
	return out
}

/*
Spec 065 R24-R27, spec 066, on Windows (R30, R31). From the cooler's
processor list: the spinner's name, then its process, then Kill and
Confirm in the heading, left of Pause; the spinner and the child it started are both
gone. A cooler page's ID is not a bandwidth one, and the kill once read it
as one and ended nothing.

The clicks are found from the pictures: the list is sorted by share and the
spinner is busier than anything else on a desk at rest, so it is the first
row; the process is the second row of its page, under "all"; the kill is
the heading's last word but one, Pause being the last, and the confirm
takes its place.
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

	// step clicks the drawer's i-th row with a figure, from the left of its
	// text.
	step := func(name string, i int) {
		t.Helper()
		shot := shoot(t, hwnd, name+".png")
		rows := figured(drawer(shot, from, to))
		require.Greater(t, len(rows), i, "%s: the drawer has %d rows (picture %s)", name, len(rows), shot.path)
		tap(t, hwnd, from+(to-from)/3, (rows[i].From+rows[i].To)/2)
		time.Sleep(listSettle)
	}
	// The spinner is the busiest name, so the first row.
	step("kill-list", 0)
	// "all", then the spinner's own process, which is the busiest of its
	// name: this test is a process of the same name, idle.
	step("kill-name", 1)

	page := shoot(t, hwnd, "kill-process.png")
	d := drawer(page, from, to)
	require.NotEmpty(t, d.text, "picture %s", page.path)
	heading := d.text[0]
	ws := d.words(heading)
	// The heading ends in its buttons: Kill, then Pause.
	require.GreaterOrEqual(t, len(ws), 3, "picture %s", page.path)
	kill := ws[len(ws)-2]
	kill.From, kill.To = kill.From+from, kill.To+from
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

	// What the page says once a reading has come since: for a person to look
	// at, not asserted.
	time.Sleep(listSettle)
	shoot(t, hwnd, "kill-after.png")
}
