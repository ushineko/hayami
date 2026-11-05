package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// Against the session's real service manager, where there is one: what doctor's warning
// about vpn-toggle is read through (spec 085 R27).
func TestARealUserServiceIsReadActiveOrNot(t *testing.T) {
	conn, err := dbus.SessionBus()
	if err != nil {
		t.Skip("no session bus")
	}
	if conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, "org.freedesktop.systemd1").Err != nil {
		t.Skip("no user service manager on this bus")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	active, err := core.UserServiceActive(ctx, "default.target")
	require.NoError(t, err)
	assert.True(t, active, "a running session has reached its default target")

	active, err = core.UserServiceActive(ctx, "hayami-no-such-service-anywhere.service")
	require.NoError(t, err)
	assert.False(t, active, "a service it has never heard of is not running")
}
