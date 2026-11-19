//go:build !windows

package desktop

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/ushineko/fynedesygn/glance/kwin"
)

// The bus name, object and interface the compositor calls back on.
//
// The name is the app id, which is what the rest of this program identifies
// the panel by. A second copy of hayami cannot take the name, which is what
// stops two panels fighting over one remembered position.
const (
	geometryService = "io.ushineko.hayami"
	geometryPath    = "/Geometry"
	geometryIface   = "io.ushineko.hayami.Geometry"
	geometryMethod  = "Report"
)

// watchName and placeName are what the two scripts that stay loaded are
// loaded under, so each can be unloaded by name without remembering an id.
const (
	watchName = "hayami-geometry"
	placeName = "hayami-place"
)

// keepEvery is how often the panel of record asks KWin whether its watch is
// still loaded (spec 100). A variable, so a test does not wait for it.
var keepEvery = 30 * time.Second

/*
Position watches where the panel is and puts it back.

Neither half is the toolkit's to do. A Wayland client cannot place itself --
measured, on Plasma 6, as a move of exactly nothing -- and cannot read where
it is, because the move belongs to the compositor and raises no event a
toolkit sees. Both go through KWin's scripting API, which is the same route
the window rule already takes.

It is quiet where there is no KWin. A machine running another compositor gets
a panel that opens where it opens, which is what it does today.
*/
type Position struct {
	target kwin.Target

	mu      sync.Mutex
	conn    *dbus.Conn
	settle  *settler
	watched bool
	// done stops the keeper.
	done chan struct{}
}

// NewPosition builds the watcher for the window of the app with the title.
// Nothing happens until Watch is called.
//
// The title, because the app ID is not a window: on Wayland the preferences
// window carries it too, and a drag of that window was saved as the panel's
// position (issue #107). The panel's title is the program's to set and it
// is set before the window is mapped, so the compositor's first report of
// the window already carries it.
func NewPosition(appID, title string) *Position {
	return &Position{target: kwin.Target{Class: appID, Caption: title}}
}

// geometry is the object KWin calls. Its one method takes what the script
// sends: the frame's x, y, width and height.
type geometry struct{ settle *settler }

// Report is the D-Bus method the KWin script calls.
//
// The sender and the message are the bus library's to fill in and are not
// part of what the script sends: they are how reports are put back in order
// (settler). The width is passed on, because a panel with a drawer out is
// wider than its cards and its corner is not theirs. It is never remembered
// as a size: a glance panel is sized by its content and Panel.Resize owns
// that. The height is dropped.
func (g geometry) Report(sender dbus.Sender, msg dbus.Message, x, y, width, _ int32) *dbus.Error {
	g.settle.take(string(sender), msg.Serial(), int(x), int(y), int(width))
	return nil
}

/*
Watch asks KWin to report the panel's position whenever it changes, and calls
onMove with the place it came to rest at and the width it has there, in the
compositor's logical pixels. A panel is wider than its cards while its drawer
is out, and by how much is the caller's to know.

The script stays loaded, which is what makes this cheap: a handler fires when
the window actually moves rather than a round trip every few seconds for an
answer that changes twice a day. A loaded script keeps its handlers -- "a
script runs once" is about the body -- which is verified in
kwin.WatchGeometryScript's own documentation.

onMove is called from a goroutine of the watch's own, SettleWait after the
last change, so a caller that touches the interface queues it.
*/
func (p *Position) Watch(onMove func(x, y, width int)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.watched {
		return nil
	}

	conn, err := session()
	if err != nil {
		return err
	}
	p.conn, p.settle = conn, &settler{wait: SettleWait, out: onMove}

	obj := geometry{settle: p.settle}
	if err := conn.Export(obj, dbus.ObjectPath(geometryPath), geometryIface); err != nil {
		return fmt.Errorf("%w: exporting the geometry object: %w", ErrNoKWin, err)
	}
	// Introspection is not needed by KWin, which calls the method directly,
	// but a bus object with none is invisible to every tool anybody would
	// debug this with.
	node := &introspect.Node{
		Name: geometryPath,
		Interfaces: []introspect.Interface{{
			Name: geometryIface,
			Methods: []introspect.Method{{
				Name: geometryMethod,
				Args: []introspect.Arg{
					{Name: "x", Type: "i", Direction: "in"},
					{Name: "y", Type: "i", Direction: "in"},
					{Name: "width", Type: "i", Direction: "in"},
					{Name: "height", Type: "i", Direction: "in"},
				},
			}},
		}},
	}
	if err := conn.Export(introspect.NewIntrospectable(node),
		dbus.ObjectPath(geometryPath), "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("%w: exporting introspection: %w", ErrNoKWin, err)
	}

	// The name is requested rather than demanded: a second panel is not an
	// error, it simply does not get to remember the position.
	if _, err := conn.RequestName(geometryService, dbus.NameFlagDoNotQueue); err != nil {
		return fmt.Errorf("%w: taking the bus name: %w", ErrNoKWin, err)
	}

	if err := p.load(watchName, p.watchScript()); err != nil {
		return err
	}
	p.watched = true
	p.done = make(chan struct{})
	go p.keep(p.done)
	return nil
}

func (p *Position) watchScript() string {
	return kwin.WatchGeometryScript(p.target, geometryService, geometryPath, geometryIface, geometryMethod)
}

/*
keep puts the watch back when it has gone from the compositor while this is
the panel of record (spec 100). Another panel that quit took it: before spec
100 every panel unloaded the names on its way out, and a build from before
then still does. Without the watch the panel stops remembering where it is
put, and nothing says so.
*/
func (p *Position) keep(done chan struct{}) {
	t := time.NewTicker(keepEvery)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		p.mu.Lock()
		if p.conn != nil && !anotherPanel(p.conn) && !loaded(p.conn, watchName) {
			_ = p.load(watchName, p.watchScript())
		}
		p.mu.Unlock()
	}
}

/*
Restore puts the panel back where it was, as soon as there is a panel.

The script stays loaded and waits for the window (issue #106). The first
version ran kwin.PositionScript once, after a fixed delay, and that script
moves a window that is on screen at that moment: a cold start took 2.15 s to
put one there, the script found nothing, and the panel opened where the
compositor placed it. kwin.PlaceScript places the window when the compositor
adds it, which is the one party that knows the moment, and places the
window with the panel's title, once.

Watch first: the bus connection is the watch's.
*/
func (p *Position) Restore(x, y int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return fmt.Errorf("%w: restoring before watching", ErrNoKWin)
	}
	return p.load(placeName, kwin.PlaceScript(p.target, x, y))
}

// Stop unloads the watch script and releases the bus name.
//
// A script left loaded is untidy rather than broken -- it reports to a bus
// name nobody answers on -- but a program that tidies up after itself is one
// somebody can run twice. A second panel leaves the scripts alone: the names
// are the compositor's, one for the desk, and what is loaded under them is
// the panel of record's (spec 100).
func (p *Position) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return
	}
	if p.done != nil {
		close(p.done)
		p.done = nil
	}
	if !anotherPanel(p.conn) {
		unloadAll(p.conn, watchName, placeName)
	}
	p.settle.stop()
	_, _ = p.conn.ReleaseName(geometryService)
	_ = p.conn.Close()
	p.conn, p.watched = nil, false
}

// load puts a script in place under a name and leaves it loaded, for one
// that keeps handlers. The caller holds the lock.
//
// A script already loaded under the name is unloaded first. KWin answers a
// second load of a name with -1 and no error, and the run that follows fails
// on a path that does not exist; the name is left by a panel that did not
// get to Stop -- killed, crashed, or any panel before this one handled a
// signal -- and its script goes on reporting to the bus name this panel now
// owns, so the watch looks alive while the restore never happens (issue
// #106). Unloading a name that is not loaded is answered false and nothing
// else.
func (p *Position) load(name, body string) error {
	path, err := writeScript(body)
	if err != nil {
		return err
	}
	defer remove(path)

	unloadAll(p.conn, name)

	call := kwin.LoadScriptCall()
	var id int
	if err := p.conn.Object(call.Destination, dbus.ObjectPath(call.Path)).
		Call(call.Interface+"."+call.Method, 0, path, name).Store(&id); err != nil {
		return fmt.Errorf("%w: loading the %s script: %w", ErrNoKWin, name, err)
	}

	run := kwin.RunCall(id)
	if err := p.conn.Object(run.Destination, dbus.ObjectPath(run.Path)).
		Call(run.Interface+"."+run.Method, 0).Err; err != nil {
		return fmt.Errorf("%w: running the %s script: %w", ErrNoKWin, name, err)
	}
	return nil
}

/*
anotherPanel reports whether the panel's bus name belongs to another process:
a panel of record that is not this one (spec 100).

KWin's script names are global, and every panel loads the same ones, so a
second panel -- a checkout's build, a screenshot run -- that unloaded them on
its way out took the panel of record's hold and watch with it, and that panel
went on as if they were there. The name is the one Watch takes, which a
second panel cannot. A name nobody holds is nobody's: a panel without a
watch still tidies up.
*/
func anotherPanel(conn *dbus.Conn) bool {
	var pid uint32
	err := conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0, geometryService).Store(&pid)
	return err == nil && int(pid) != os.Getpid()
}

// loaded reports whether KWin has a script loaded under the name. A
// compositor that does not answer is taken to have it: nothing is loaded
// again into a compositor that is not listening.
func loaded(conn *dbus.Conn, name string) bool {
	call := kwin.LoadScriptCall()
	var is bool
	if err := conn.Object(call.Destination, dbus.ObjectPath(call.Path)).
		Call(call.Interface+".isScriptLoaded", 0, name).Store(&is); err != nil {
		return true
	}
	return is
}

// unloadAll takes scripts out of the compositor by name. One that is not
// loaded is answered false and nothing else.
func unloadAll(conn *dbus.Conn, names ...string) {
	unload := kwin.UnloadScriptCall()
	for _, name := range names {
		_ = conn.Object(unload.Destination, dbus.ObjectPath(unload.Path)).
			Call(unload.Interface+"."+unload.Method, 0, name).Err
	}
}
