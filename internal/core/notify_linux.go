package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"
)

// UserServiceActive asks the person's own service manager, over the session bus,
// whether a service is running. A service it has never heard of is not.
func UserServiceActive(ctx context.Context, service string) (bool, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false, fmt.Errorf("connecting to the session bus: %w", err)
	}
	const name = "org.freedesktop.systemd1"
	var unit dbus.ObjectPath
	err = conn.Object(name, "/org/freedesktop/systemd1").CallWithContext(ctx, name+".Manager.LoadUnit", 0, service).Store(&unit)
	if err != nil {
		return false, fmt.Errorf("asking the service manager about %s: %w", service, err)
	}
	var state dbus.Variant
	err = conn.Object(name, unit).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, name+".Unit", "ActiveState").Store(&state)
	if err != nil {
		return false, fmt.Errorf("asking the service manager about %s: %w", service, err)
	}
	return state.Value() == "active", nil
}

// notifyTimeout is -1: the notification server's own default for how long to show one.
const notifyTimeout = int32(-1)

// notifyBody is text as a notification's body. A VPN's name is free text, and a server
// offering markup reads the body as markup, so a name could hide what is being said of
// it (spec 085's adversarial review).
func notifyBody(text string, markup bool) string {
	if !markup {
		return text
	}
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

// Notify puts a message on the desktop through the session's notification service
// (spec 085 R25). A desktop with none is an error for the journal, and nothing more.
func Notify(ctx context.Context, summary, body string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connecting to the session bus: %w", err)
	}
	const name = "org.freedesktop.Notifications"
	server := conn.Object(name, "/org/freedesktop/Notifications")
	var capabilities []string
	// Not knowing is taken as markup: an ampersand shown as text costs less than a
	// warning a VPN's name has hidden.
	markup := server.CallWithContext(ctx, name+".GetCapabilities", 0).Store(&capabilities) != nil ||
		slices.Contains(capabilities, "body-markup")
	var id uint32
	err = server.CallWithContext(ctx, name+".Notify", 0,
		"hayami", uint32(0), "", summary, notifyBody(body, markup), []string{}, map[string]dbus.Variant{}, notifyTimeout).Store(&id)
	if err != nil {
		return fmt.Errorf("asking for a notification: %w", err)
	}
	return nil
}
