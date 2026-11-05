//go:build !linux

package core

import (
	"context"
	"errors"
)

// Notify has nothing to say it with here: the keepalive helper is Linux's (spec 085 R3).
func Notify(context.Context, string, string) error {
	return errors.New("notifications are not done on this platform") //nolint:err113 // said plainly
}

// UserServiceActive has no service manager to ask here.
func UserServiceActive(context.Context, string) (bool, error) {
	return false, errors.New("there is no user service manager on this platform") //nolint:err113 // said plainly
}
