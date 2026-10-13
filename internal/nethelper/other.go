//go:build !windows

package nethelper

import "context"

// Probe finds nothing to help with.
func Probe() State { return State{} }

// Install is refused.
func Install(string, string) error { return ErrNotWindows }

// Remove is refused.
func Remove() error { return ErrNotWindows }

// Run is refused.
func Run(context.Context, string, string) error { return ErrNotWindows }

// Elevated reports that nothing here is.
func Elevated() bool { return false }

// CurrentUser has no answer.
func CurrentUser() (string, error) { return "", ErrNotWindows }

// RunElevated is refused.
func RunElevated(context.Context, string, ...string) error { return ErrNotWindows }
