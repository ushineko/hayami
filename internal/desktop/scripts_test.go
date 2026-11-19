//go:build linux

package desktop_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/desktop"
)

/*
These tests are spec 100's: what a panel does with the scripts it leaves in
the compositor when another panel is about, and when they are taken away.

They run against a bus of their own, started for each test, with a fake KWin
on it, and never against the session's. A panel's scripts loaded into the
compositor of the person at the desk is what spec 060 learned not to do (the
Desk interface's comment); deskBus refuses to go on unless the program's
session bus is the test's.
*/

// panelName is the bus name a panel of record holds (Position.Watch).
const panelName = "io.ushineko.hayami"

// The names the panel loads its scripts under.
const (
	holdScript  = "hayami-hold-edge"
	watchScript = "hayami-geometry"
	placeScript = "hayami-place"
)

// fakeKWin answers the scripting calls the panel makes, and keeps which
// names are loaded.
type fakeKWin struct {
	conn *dbus.Conn

	mu     sync.Mutex
	loaded map[string]bool
	loads  map[string]int
	next   int
}

func (k *fakeKWin) LoadScript(_, name string) (int32, *dbus.Error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.loaded[name] {
		return -1, nil // KWin's answer to a name already loaded
	}
	id := k.next
	k.next++
	k.loaded[name] = true
	k.loads[name]++
	err := k.conn.ExportWithMap(script{}, map[string]string{"Run": "run"},
		dbus.ObjectPath(fmt.Sprintf("/Scripting/Script%d", id)), "org.kde.kwin.Script")
	if err != nil {
		return -1, dbus.MakeFailedError(err)
	}
	return int32(id), nil
}

func (k *fakeKWin) UnloadScript(name string) (bool, *dbus.Error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	was := k.loaded[name]
	delete(k.loaded, name)
	return was, nil
}

func (k *fakeKWin) IsScriptLoaded(name string) (bool, *dbus.Error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.loaded[name], nil
}

// has is whether a script is loaded under the name.
func (k *fakeKWin) has(name string) bool {
	is, _ := k.IsScriptLoaded(name)
	return is
}

// load puts a script under a name as another panel would have.
func (k *fakeKWin) load(name string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.loaded[name] = true
}

// unload takes a script away as another panel quitting would have.
func (k *fakeKWin) unload(name string) { _, _ = k.UnloadScript(name) }

func (k *fakeKWin) loadsOf(name string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.loads[name]
}

type script struct{}

func (script) Run() *dbus.Error { return nil }

/*
deskBus starts a bus of the test's own with a fake KWin on it, points the
program's session bus at it, and returns the fake and the bus's address.
*/
func deskBus(t *testing.T) (*fakeKWin, string) {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("dbus-daemon is not installed: these tests need a bus of their own")
		}
		t.Skip("no dbus-daemon")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "bus.conf")
	require.NoError(t, os.WriteFile(conf, []byte(`<busconfig>
  <type>session</type>
  <listen>unix:dir=`+dir+`</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`), 0o600))
	cmd := exec.CommandContext(t.Context(), daemon, "--config-file="+conf, "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	addr, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	addr = strings.TrimSpace(addr)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)

	conn, err := dbus.Connect(addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	k := &fakeKWin{conn: conn, loaded: map[string]bool{}, loads: map[string]int{}}
	require.NoError(t, conn.ExportWithMap(k, map[string]string{
		"LoadScript":     "loadScript",
		"UnloadScript":   "unloadScript",
		"IsScriptLoaded": "isScriptLoaded",
	}, "/Scripting", "org.kde.kwin.Scripting"))
	reply, err := conn.RequestName("org.kde.KWin", dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	require.Equal(t, dbus.RequestNameReplyPrimaryOwner, reply)

	// The program's shared session connection may be one an earlier test
	// made, on another bus. It is closed so the next use connects here, and
	// the test stops unless it does.
	marker := fmt.Sprintf("io.example.hayamitest.p%d", os.Getpid())
	_, err = conn.RequestName(marker, dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	onTestBus := func() bool {
		shared, err := dbus.SessionBus()
		if err != nil {
			return false
		}
		var has bool
		if shared.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, marker).Store(&has) != nil || !has {
			_ = shared.Close()
			return false
		}
		return true
	}
	if !onTestBus() {
		require.True(t, onTestBus(), "the program's session bus is not the test's; refusing to go on")
	}
	return k, addr
}

/*
anotherPanelOfRecord holds the panel's bus name from another process, as the
panel on the desk does while a second one runs, until the test ends.
*/
func anotherPanelOfRecord(t *testing.T, addr string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHelperHoldsThePanelName$")
	cmd.Env = append(os.Environ(), "HAYAMI_TEST_HOLD_NAME="+addr)
	in, err := cmd.StdinPipe()
	require.NoError(t, err)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "held", strings.TrimSpace(line))
}

// TestHelperHoldsThePanelName is anotherPanelOfRecord's other process, and
// skipped when run any other way. It holds the name until its input closes.
func TestHelperHoldsThePanelName(t *testing.T) {
	addr := os.Getenv("HAYAMI_TEST_HOLD_NAME")
	if addr == "" {
		t.Skip("run by anotherPanelOfRecord")
	}
	conn, err := dbus.Connect(addr)
	require.NoError(t, err)
	reply, err := conn.RequestName(panelName, dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	require.Equal(t, dbus.RequestNameReplyPrimaryOwner, reply)
	fmt.Println("held")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func nothing(int, int, int) {}

// R1. A second panel quitting leaves the scripts of the panel of record
// loaded: the hold, the watch and the place.
func TestASecondPanelLeavesThePanelOfRecordsScripts(t *testing.T) {
	k, addr := deskBus(t)
	anotherPanelOfRecord(t, addr)
	for _, name := range []string{holdScript, watchScript, placeScript} {
		k.load(name)
	}

	d := desktop.NewDrawer(appID, "example")
	pos := desktop.NewPosition(appID, "example")
	require.NoError(t, pos.Watch(nothing))
	assert.True(t, d.HoldEdge(context.Background(), true))
	d.Stop()
	pos.Stop()

	for _, name := range []string{holdScript, watchScript, placeScript} {
		assert.True(t, k.has(name), "%s should still be loaded for the panel of record", name)
	}
}

// R1. The panel of record still takes its scripts out when it quits.
func TestThePanelOfRecordTakesItsScriptsOut(t *testing.T) {
	k, _ := deskBus(t)

	d := desktop.NewDrawer(appID, "example")
	pos := desktop.NewPosition(appID, "example")
	require.NoError(t, pos.Watch(nothing))
	require.NoError(t, pos.Restore(10, 20))
	assert.True(t, d.HoldEdge(context.Background(), true))
	require.True(t, k.has(holdScript) && k.has(watchScript) && k.has(placeScript))
	d.Stop()
	pos.Stop()

	for _, name := range []string{holdScript, watchScript, placeScript} {
		assert.False(t, k.has(name), "%s should be taken out", name)
	}
}

// R2. A hold that was taken away is loaded again at the next asking, for
// the same edge as before: the panel does not take it as still there.
func TestAHoldThatWasTakenAwayIsLoadedAgain(t *testing.T) {
	k, _ := deskBus(t)
	d := desktop.NewDrawer(appID, "example")
	t.Cleanup(d.Stop)

	require.True(t, d.HoldEdge(context.Background(), true))
	k.unload(holdScript)

	assert.True(t, d.HoldEdge(context.Background(), true))
	assert.True(t, k.has(holdScript))
	assert.Equal(t, 2, k.loadsOf(holdScript))
}

// R2. The corner's hold too.
func TestACornerHoldThatWasTakenAwayIsLoadedAgain(t *testing.T) {
	k, _ := deskBus(t)
	d := desktop.NewDrawer(appID, "example")
	t.Cleanup(d.Stop)

	require.True(t, d.HoldCorner(context.Background(), true))
	k.unload(holdScript)

	assert.True(t, d.HoldCorner(context.Background(), true))
	assert.True(t, k.has(holdScript))
}

// R3. The panel of record puts its watch back when it has been taken away.
func TestAWatchThatWasTakenAwayIsPutBack(t *testing.T) {
	t.Cleanup(desktop.SetKeepEvery(20 * time.Millisecond))
	k, _ := deskBus(t)
	pos := desktop.NewPosition(appID, "example")
	require.NoError(t, pos.Watch(nothing))
	t.Cleanup(pos.Stop)

	k.unload(watchScript)

	assert.Eventually(t, func() bool { return k.has(watchScript) }, 2*time.Second, 10*time.Millisecond)
}
