//go:build !windows

package desktop

import (
	"fmt"
	"sync"

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
	onMove  func(x, y int)
	watched bool
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
type geometry struct{ report func(x, y int32) }

// Report is the D-Bus method the KWin script calls.
//
// The width and height are taken and dropped. A glance panel is sized by its
// content and Panel.Resize owns that; remembering a size here would put the
// two in an argument the user would see as a window that will not stay the
// size they made it.
func (g geometry) Report(x, y, _, _ int32) *dbus.Error {
	g.report(x, y)
	return nil
}

/*
Watch asks KWin to report the panel's position whenever it changes, and calls
onMove with it.

The script stays loaded, which is what makes this cheap: a handler fires when
the window actually moves rather than a round trip every few seconds for an
answer that changes twice a day. A loaded script keeps its handlers -- "a
script runs once" is about the body -- which is verified in
kwin.WatchGeometryScript's own documentation.

onMove is called from the bus's goroutine, so a caller that touches the
interface queues it.
*/
func (p *Position) Watch(onMove func(x, y int)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.watched {
		return nil
	}

	conn, err := session()
	if err != nil {
		return err
	}
	p.conn, p.onMove = conn, onMove

	obj := geometry{report: func(x, y int32) {
		p.mu.Lock()
		fn := p.onMove
		p.mu.Unlock()
		if fn != nil {
			fn(int(x), int(y))
		}
	}}
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

	if err := p.load(watchName, kwin.WatchGeometryScript(
		p.target, geometryService, geometryPath, geometryIface, geometryMethod)); err != nil {
		return err
	}
	p.watched = true
	return nil
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
// somebody can run twice.
func (p *Position) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return
	}
	unload := kwin.UnloadScriptCall()
	for _, name := range []string{watchName, placeName} {
		_ = p.conn.Object(unload.Destination, dbus.ObjectPath(unload.Path)).
			Call(unload.Interface+"."+unload.Method, 0, name).Err
	}
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

	unload := kwin.UnloadScriptCall()
	_ = p.conn.Object(unload.Destination, dbus.ObjectPath(unload.Path)).
		Call(unload.Interface+"."+unload.Method, 0, name).Err

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
