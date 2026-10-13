//go:build !windows

package desktop

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/ushineko/fynedesygn/glance/kwin"
)

// DrawerAsksCompositor is whether the panel's drawer needs the compositor
// for the three things a Wayland client cannot do itself: read the room
// beside the window, grow leftwards, and give the keyboard back. True here,
// where the compositor is asked and a desktop without KWin goes without.
const DrawerAsksCompositor = true

// The object the compositor reports the room beside the panel to, and the
// names the drawer's scripts are loaded under.
const (
	roomPath   = "/Room"
	roomIface  = "io.ushineko.hayami.Room"
	roomMethod = "Report"

	holdName  = "hayami-hold-edge"
	roomName  = "hayami-room"
	yieldName = "hayami-yield"
)

// scriptWait bounds one exchange with the compositor. KWin answers in a
// millisecond or two; a compositor that has stopped answering costs this and
// the drawer opens without it.
const scriptWait = 500 * time.Millisecond

/*
Drawer is the compositor's half of the panel's drawer (spec 060 R18,
fynedesygn spec 068): the room beside the window, the right edge held so a
drawer can open on the left, and the keyboard handed back.

All three are KWin scripts, as the window's position is (Position), run over
the session bus. Each failure is quiet: without KWin the drawer opens on the
right and the keyboard stays where the click put it, which is what the
library does when nobody tells it otherwise.
*/
type Drawer struct {
	target kwin.Target

	mu   sync.Mutex
	conn *dbus.Conn
	room chan [2]int32
	held bool
}

// NewDrawer builds the helper for the window of the app with the title.
// Nothing is asked of the compositor until a method is called.
func NewDrawer(appID, title string) *Drawer {
	return &Drawer{target: kwin.Target{Class: appID, Caption: title}}
}

// roomReport is the object KWin calls with the room on each side.
type roomReport struct{ report func(left, right int32) }

// Report is the D-Bus method the KWin script calls.
func (r roomReport) Report(left, right int32) *dbus.Error {
	r.report(left, right)
	return nil
}

// connect opens the session bus and exports the object the room is reported
// to, once. The caller holds the lock.
func (d *Drawer) connect() error {
	if d.conn != nil {
		return nil
	}
	// A connection of its own. The shared one is the position watch's, which
	// closes it when the panel stops.
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoKWin, err)
	}
	d.room = make(chan [2]int32, 1)
	obj := roomReport{report: func(left, right int32) {
		select {
		case d.room <- [2]int32{left, right}:
		default:
		}
	}}
	if err := conn.Export(obj, dbus.ObjectPath(roomPath), roomIface); err != nil {
		return fmt.Errorf("%w: exporting the room object: %w", ErrNoKWin, err)
	}
	d.conn = conn
	return nil
}

// Asks reports whether the drawer needs this helper on this system
// (DrawerAsksCompositor).
func (*Drawer) Asks() bool { return DrawerAsksCompositor }

/*
Room is the room between the window and each edge of the area windows are
placed in, in the compositor's logical pixels, and whether the compositor
said. It is asked at each opening: the answer is one script loaded, run and
unloaded.
*/
func (d *Drawer) Room(ctx context.Context) (left, right float32, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.connect() != nil {
		return 0, 0, false
	}
	// The connection's own name: the script calls this process back, and the
	// well-known name belongs to whichever panel took it first.
	names := d.conn.Names()
	if len(names) == 0 {
		return 0, 0, false
	}
	select {
	case <-d.room: // an answer nobody waited for
	default:
	}
	ctx, cancel := context.WithTimeout(ctx, scriptWait)
	defer cancel()
	if err := d.run(ctx, roomName, kwin.ReportRoomScript(d.target, names[0], roomPath, roomIface, roomMethod), false); err != nil {
		return 0, 0, false
	}
	select {
	case got := <-d.room:
		return float32(got[0]), float32(got[1]), true
	case <-ctx.Done():
		return 0, 0, false
	}
}

/*
HoldEdge loads or unloads the script that keeps the window's right edge where
it is when its width changes, and reports whether the edge is held now. It is
loaded only while the drawer is on the left: loaded for one on the right it
would make that grow leftwards too.
*/
func (d *Drawer) HoldEdge(ctx context.Context, on bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if on == d.held {
		return d.held
	}
	if d.connect() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, scriptWait)
	defer cancel()
	if !on {
		d.unload(ctx, holdName)
		d.held = false
		return false
	}
	d.held = d.run(ctx, holdName, kwin.HoldRightEdgeScript(d.target), true) == nil
	return d.held
}

// Yield gives the keyboard back to the window a person was in before they
// clicked the panel. The script acts only when the panel is the active
// window.
func (d *Drawer) Yield(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.connect() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, scriptWait)
	defer cancel()
	_ = d.run(ctx, yieldName, kwin.YieldFocusScript(d.target), false)
}

// Stop unloads what was left loaded.
func (d *Drawer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), scriptWait)
	defer cancel()
	for _, name := range []string{holdName, roomName, yieldName} {
		d.unload(ctx, name)
	}
	_ = d.conn.Close()
	d.conn, d.held = nil, false
}

// run loads a script under a name and runs it, and unloads it again unless
// it is one that stays. A script already loaded under the name is unloaded
// first, for the reason Position.load gives. The caller holds the lock.
func (d *Drawer) run(ctx context.Context, name, body string, stay bool) error {
	path, err := writeScript(body)
	if err != nil {
		return err
	}
	defer remove(path)

	d.unload(ctx, name)
	call := kwin.LoadScriptCall()
	var id int
	if err := d.conn.Object(call.Destination, dbus.ObjectPath(call.Path)).
		CallWithContext(ctx, call.Interface+"."+call.Method, 0, path, name).Store(&id); err != nil {
		return fmt.Errorf("%w: loading the %s script: %w", ErrNoKWin, name, err)
	}
	run := kwin.RunCall(id)
	if err := d.conn.Object(run.Destination, dbus.ObjectPath(run.Path)).
		CallWithContext(ctx, run.Interface+"."+run.Method, 0).Err; err != nil {
		return fmt.Errorf("%w: running the %s script: %w", ErrNoKWin, name, err)
	}
	if !stay {
		d.unload(ctx, name)
	}
	return nil
}

// unload takes a script out of the compositor by name. One that is not
// loaded is answered false and nothing else.
func (d *Drawer) unload(ctx context.Context, name string) {
	unload := kwin.UnloadScriptCall()
	_ = d.conn.Object(unload.Destination, dbus.ObjectPath(unload.Path)).
		CallWithContext(ctx, unload.Interface+"."+unload.Method, 0, name).Err
}
