package core

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// notifyTimeout is -1: the notification server's own default for how long to show one.
const notifyTimeout = int32(-1)

// Notify puts a message on the desktop through the session's notification service
// (spec 085 R25). A desktop with none is an error for the journal, and nothing more.
func Notify(ctx context.Context, summary, body string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connecting to the session bus: %w", err)
	}
	const name = "org.freedesktop.Notifications"
	var id uint32
	err = conn.Object(name, "/org/freedesktop/Notifications").CallWithContext(ctx, name+".Notify", 0,
		"hayami", uint32(0), "", summary, body, []string{}, map[string]dbus.Variant{}, notifyTimeout).Store(&id)
	if err != nil {
		return fmt.Errorf("asking for a notification: %w", err)
	}
	return nil
}
