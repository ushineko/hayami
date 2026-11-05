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
