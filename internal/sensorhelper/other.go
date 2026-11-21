//go:build !windows

package sensorhelper

import (
	"context"

	"github.com/ushineko/hayami/internal/nethelper"
)

// Probe finds nothing to help with.
func Probe() nethelper.State { return nethelper.State{} }

// Install is refused.
func Install(string, string) error { return ErrNotWindows }

// Remove is refused.
func Remove() error { return ErrNotWindows }

// Run is refused.
func Run(context.Context, string, string) error { return ErrNotWindows }

// What the command line says of the helper here.
const (
	About        = "There is no sensor helper on this system: the kernel gives the processor's temperature."
	InstallSteps = About
)
