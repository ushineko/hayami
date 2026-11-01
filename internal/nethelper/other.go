//go:build !windows && !linux

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

// What the command line says of the helper here.
const (
	About        = "There is no helper on this system."
	InstallSteps = About
	InstallNeeds = "not on this system"
)

// ProgramName is the terminal program's file name.
const ProgramName = "hayami-tui"

// RemoveFromCopy has nothing to remove here.
const RemoveFromCopy = false

// The installer's Network page has nothing to offer here.
const (
	OfferWhy     = About
	OfferWithout = About
	OfferCaution = About
)

// OfferSteps is empty: there is nothing to install.
func OfferSteps() []string { return nil }
