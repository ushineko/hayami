/*
Package desktop is what the compositor grants that the toolkit cannot ask for.

A glance window is frameless, above other windows and shown through. Only the
first of those is here, and the reason is worth stating because the obvious
assumption is wrong in both directions:

  - **Translucency is the toolkit's** and is done natively. GLFW grants a
    framebuffer with an alpha channel, the design system makes the window's
    background transparent, and the cards are faded by a theme. None of that
    needs a compositor, so it works on any desktop and not only on Plasma.
  - **The titlebar is the compositor's.** Fyne asks GLFW for an undecorated
    window and KWin decorates it anyway, and nothing in the process can
    overrule that. It is the one thing a rule is needed for.
  - **Always on top** is asked for by the toolkit and granted here, and the
    rule asks again. A rule survives a compositor restart and a toolkit
    request is one the window manager may decline, so the second ask is cheap
    insurance rather than a duplicate.

The rule is a write into the user's own kwinrulesrc, which holds every window
rule they have, so it happens when they ask and never on a first run.

Nothing here imports a toolkit. Both front ends use it.
*/
package desktop

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/ushineko/fynedesygn/glance/kwin"
)

// PanelTitle is the panel window's title, and the second thing the rule
// matches on.
//
// **The app ID is not enough.** Every window in the program carries it —
// Fyne takes the Wayland app_id from the app's unique ID — so a rule keyed on
// it alone strips the titlebar off the preferences window too, which is a
// window a person needs to be able to move and close. The design system's
// Rule.Title exists for exactly this case and says so.
const PanelTitle = "hayami"

// Description is how the rule identifies itself in System Settings.
//
// A rule the user cannot recognise is a rule they cannot remove, and this one
// is going into a file beside every other rule they have.
const Description = "hayami — frameless, on top, translucent"

// DefaultOpacity is how opaque the panel's cards are, as a percentage.
//
// Ninety-five, which is what the program this replaces has run at for its
// whole 1.x life. It is a measurement of habit rather than of anything, and it
// is the default because it is the familiar one.
//
// It is applied by the toolkit, not by KWin: the window's background is
// already fully transparent and this is the card on top of it. It lives here
// rather than in the GUI package because the settings and the command line
// both need it and neither imports a toolkit.
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
}

// ruleFor builds the rule for an app ID.
//
// Forced, because a glance window the user can accidentally push behind
// something is not one.
//
// **No opacity.** The rule used to carry one and does not need to: the panel
// fades its own cards through the theme, which works on any desktop rather
// than only on Plasma, and a rule that also faded the window would fade it
// twice.
func ruleFor(appID string) kwin.Rule {
	return kwin.Rule{
		AppID:       appID,
		Title:       PanelTitle,
		Description: Description,
		AlwaysOnTop: true,
		NoBorder:    true,
		SkipTaskbar: true,
		SkipPager:   true,
	}
}

// Install puts the rule in place and asks KWin to reload.
//
// Installing twice updates the rule rather than adding a second, which is the
// design system's behaviour and is asserted rather than assumed.
func Install(appID string) error {
	// Anything written by an older version first, or installing leaves the
	// two side by side and the older one wins on the windows it matches.
	if err := removeAll(appID); err != nil {
		return err
	}
	if err := kwin.Install(ruleFor(appID)); err != nil {
		return fmt.Errorf("installing the window rule: %w", err)
	}
	return reconfigure()
}

// Remove takes the rule out and asks KWin to reload, giving the titlebar back.
//
// Removing one that is not there is not an error: the user's intent is that
// there be no rule, and there is none.
func Remove(appID string) error {
	if err := removeAll(appID); err != nil {
		return err
	}
	return reconfigure()
}

/*
removeAll takes out every rule this program has ever written for the app ID.

**Both the titled rule and an untitled one.** The rule used to match on the app
ID alone, which stripped the titlebar off the preferences window too, because
every window in the program carries the same Wayland app_id. Adding the title
fixed that and created a worse problem: a remove keyed on the new match cannot
see a rule written under the old one, so the old rule stayed in the user's
kwinrulesrc, kept stripping both windows, and made the preferences checkbox
look like it did nothing — it was removing a rule while another one held the
panel frameless.

A program that changes what its rule matches on has to clean up after the
version of itself that matched differently. There is no third form to worry
about, and if there ever is, it belongs in this list rather than in a comment.
*/
func removeAll(appID string) error {
	for _, title := range []string{PanelTitle, ""} {
		if _, err := kwin.RemoveTitled(appID, title); err != nil {
			return fmt.Errorf("removing the window rule: %w", err)
		}
	}
	return nil
}

// Current reports whether the rule is installed and what opacity it carries.
func Current(appID string) (Rule, error) {
	// Either form counts as installed. A panel held frameless by a rule an
	// older version wrote is a panel that is frameless, and a checkbox that
	// said otherwise would be lying about what is on screen.
	for _, title := range []string{PanelTitle, ""} {
		_, found, err := kwin.LookupTitled(appID, title)
		if err != nil {
			return Rule{}, fmt.Errorf("reading the window rules: %w", err)
		}
		if found {
			return Rule{Installed: true}, nil
		}
	}
	return Rule{}, nil
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
