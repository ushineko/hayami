/*
Package desktop is what the compositor grants that the toolkit cannot ask for.

A glance window is frameless, above other windows and shown through, and on
Plasma none of those three is the toolkit's to give. Fyne asks GLFW for an
undecorated window and KWin decorates it anyway; Fyne's desktop backend never
requests a transparent framebuffer, so nothing drawn in this process can be
see-through; and this panel is a native Wayland client, so the X11 property
that would set its own opacity is not there to write.

All three are KWin's, and KWin offers them two different ways:

  - A **rule** is persistent. It survives a restart and a compositor restart
    and is the only route to frameless and on-top. It is also a write into the
    user's own kwinrulesrc, so it happens when they ask.
  - A **script** over D-Bus is live and leaves nothing behind. It is what a
    menu item does, because a menu is for trying a value.

Nothing here imports a toolkit. Both front ends use it.
*/
package desktop

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/ushineko/fynedesygn/glance/kwin"
)

// Description is how the rule identifies itself in System Settings.
//
// A rule the user cannot recognise is a rule they cannot remove, and this one
// is going into a file beside every other rule they have.
const Description = "hayami — frameless, on top, translucent"

// DefaultOpacity is the panel's opacity as a percentage.
//
// Ninety-five, which is what the program this replaces has run at for its
// whole 1.x life. It is a measurement of habit rather than of anything, and it
// is the default because it is the familiar one.
const DefaultOpacity = 95

// ErrNoKWin is Plasma not being there: another compositor, no session bus, or
// a call it refused.
//
// Not a failure of the panel. A desktop that is not Plasma gets a window with
// a titlebar, which is what every desktop gave before this package existed.
var ErrNoKWin = errors.New("kwin is not answering")

// Rule is the state of hayami's window rule.
type Rule struct {
	// Installed is whether the rule is in the user's kwinrulesrc at all.
	Installed bool

	// Opacity is the percentage the installed rule carries. Meaningless when
	// Installed is false.
	Opacity int
}

// ruleFor builds the rule for an app ID at an opacity.
//
// `above` and `noborder` are forced, because a glance window that the user can
// accidentally push behind something is not one. The opacity is applied
// initially rather than forced, so the window menu can still override it —
// which is the reference's choice and the reason a person can experiment
// without editing anything.
func ruleFor(appID string, opacity int) kwin.Rule {
	return kwin.Rule{
		AppID:       appID,
		Description: Description,
		AlwaysOnTop: true,
		NoBorder:    true,
		Opacity:     opacity,
		SkipTaskbar: true,
		SkipPager:   true,
	}
}

// Install puts the rule in place and asks KWin to reload.
//
// Installing twice updates the rule rather than adding a second, which is the
// design system's behaviour and is asserted rather than assumed.
func Install(appID string, opacity int) error {
	if opacity < 0 || opacity > 100 {
		return fmt.Errorf("an opacity of %d is not a percentage", opacity)
	}
	if err := kwin.Install(ruleFor(appID, opacity)); err != nil {
		return fmt.Errorf("installing the window rule: %w", err)
	}
	return reconfigure()
}

// Remove takes the rule out and asks KWin to reload, giving the titlebar back.
//
// Removing one that is not there is not an error: the user's intent is that
// there be no rule, and there is none.
func Remove(appID string) error {
	if _, err := kwin.Remove(appID); err != nil {
		return fmt.Errorf("removing the window rule: %w", err)
	}
	return reconfigure()
}

// Current reports whether the rule is installed and what opacity it carries.
func Current(appID string) (Rule, error) {
	r, found, err := kwin.Lookup(appID)
	if err != nil {
		return Rule{}, fmt.Errorf("reading the window rules: %w", err)
	}
	if !found {
		return Rule{}, nil
	}
	return Rule{Installed: true, Opacity: r.Opacity}, nil
}

/*
SetOpacity changes the running window's opacity now, without writing anything.

This is the menu's mechanism. It loads a script into KWin, runs it and unloads
it; the script walks the window list and sets the opacity of the windows whose
resource class is this app's. Nothing survives a restart, which is the point:
the persistent value lives in the rule and is set from the preferences.

A desktop that is not Plasma answers ErrNoKWin, and the caller says so rather
than failing.
*/
func SetOpacity(appID string, opacity int) error {
	if opacity < 0 || opacity > 100 {
		return fmt.Errorf("an opacity of %d is not a percentage", opacity)
	}

	conn, err := session()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	path, err := writeScript(kwin.OpacityScript(appID, float32(opacity)/100))
	if err != nil {
		return err
	}
	defer remove(path)

	load := kwin.LoadScriptCall()
	var id int
	err = conn.Object(load.Destination, dbus.ObjectPath(load.Path)).
		Call(load.Interface+"."+load.Method, 0, path, Description).Store(&id)
	if err != nil {
		return fmt.Errorf("%w: loading the opacity script: %w", ErrNoKWin, err)
	}

	run := kwin.RunCall(id)
	if err := conn.Object(run.Destination, dbus.ObjectPath(run.Path)).
		Call(run.Interface+"."+run.Method, 0).Err; err != nil {
		return fmt.Errorf("%w: running the opacity script: %w", ErrNoKWin, err)
	}

	unload := kwin.UnloadScriptCall()
	// The unload is best effort. The opacity is already applied, and a script
	// left loaded is untidy rather than broken.
	_ = conn.Object(unload.Destination, dbus.ObjectPath(unload.Path)).
		Call(unload.Interface+"."+unload.Method, 0, Description).Err
	return nil
}

// reconfigure asks KWin to read its rules again, which is what makes a written
// rule take effect without a logout.
func reconfigure() error {
	conn, err := session()
	if err != nil {
		// The rule is written. A compositor that is not there to be told is
		// not a failed install: the rule applies the next time one starts.
		return err
	}
	defer func() { _ = conn.Close() }()

	call := kwin.ReconfigureCall()
	if err := conn.Object(call.Destination, dbus.ObjectPath(call.Path)).
		Call(call.Interface+"."+call.Method, 0).Err; err != nil {
		return fmt.Errorf("%w: asking kwin to reload: %w", ErrNoKWin, err)
	}
	return nil
}

// session opens the session bus, which is where KWin listens.
func session() (*dbus.Conn, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoKWin, err)
	}
	return conn, nil
}
