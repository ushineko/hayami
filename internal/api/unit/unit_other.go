//go:build !linux && !windows && !darwin

package unit

import "context"

// Offered is false: the service on Windows and macOS comes in its own phases
// of spec 113. A variable, as on Linux.
var Offered = func() bool { return false }

// Installed is never, here.
func Installed() bool { return false }

// Install refuses: there is no service manager this package writes for here
// yet.
func Install(context.Context, string) error { return ErrNotHere }

// Remove has nothing to remove.
func Remove(context.Context) error { return nil }

// Status is nothing installed.
func Status(context.Context) State { return State{} }

// InstallTCP refuses, as Install does.
func InstallTCP(context.Context, string, int, bool) error { return ErrNotHere }

// RemoveTCP has nothing to remove.
func RemoveTCP(context.Context, string) error { return nil }

// InstalledProgram is nothing: nothing is installed here.
func InstalledProgram() (string, error) { return "", ErrNotHere }
