package installer

import (
	"fmt"

	"github.com/ushineko/hayami/internal/nethelper"
)

/*
Sensors is what the installer found on this machine for the processor's
temperature (spec 104): PawnIO's driver and the sensor helper (spec 103).
Everything in it is read, and nothing is changed to learn it.
*/
type Sensors struct {
	// Applies is false off Windows, where the kernel gives the processor's
	// temperature without help and there is nothing to offer.
	Applies bool
	// PawnIO reports whether PawnIO's driver is installed.
	PawnIO bool
	// Helper is what there is of the sensor helper.
	Helper nethelper.State
	// Version is the hayami being installed.
	Version string
	// LHM reports whether LibreHardwareMonitor is running, which the panel
	// reads where there is no helper.
	LHM bool
}

func (s Sensors) applies() bool   { return s.Applies }
func (s Sensors) why() string     { return Why }
func (s Sensors) caution() string { return Caution }

// Ready reports whether the helper is installed at this version: there is
// nothing to set up.
func (s Sensors) Ready() bool {
	return s.Helper.Installed && s.Helper.Version == s.Version
}

// Found says what the check found, in a sentence or two.
func (s Sensors) Found() string {
	switch {
	case !s.Applies:
		return "This system gives hayami the processor's temperature itself. There is nothing to set up."
	case s.Ready():
		return "The sensor helper is installed: hayami reads the processor's temperature through it. There is nothing to do."
	case s.Helper.Installed:
		version := s.Helper.Version
		if version == "" {
			version = "an unknown version"
		}
		return fmt.Sprintf("The sensor helper from hayami %s is installed. It goes on working; this version can take its place.", version)
	case s.LHM:
		return "hayami reads the processor's temperature from LibreHardwareMonitor. The sensor helper reads it without LibreHardwareMonitor."
	default:
		return "The sensor helper is not installed. Without it the panel shows the processor's load and no temperature."
	}
}

// Offer is the question the page asks, or "" when there is nothing to
// offer.
func (s Sensors) Offer() string {
	switch {
	case !s.Applies || s.Ready():
		return ""
	case s.Helper.Installed:
		return "Bring the sensor helper up to date"
	case !s.PawnIO:
		return "Install PawnIO and the sensor helper"
	default:
		return "Install the sensor helper"
	}
}

// Why is why the page exists at all, said once above the steps.
const Why = "Windows keeps the processor's temperature behind a kernel driver. PawnIO is that driver, " +
	"signed, and used by LibreHardwareMonitor and OpenRGB; it serves only an administrator, and the panel runs as you. " +
	"The sensor helper is a small Windows service that reads the temperature through it and hands it to the panel. Setting it up:"

// Caution is what a person should know before saying yes.
const Caution = "PawnIO is a kernel driver, and the helper runs as LocalSystem while a panel reads it. " +
	"Uninstalling hayami removes the helper and leaves PawnIO. " +
	"The panel works without them, and you can set them up later by running this installer again."

// Steps are what setting it up does, in order: each is done only if it is
// not done already.
func (s Sensors) Steps() []string {
	var steps []string
	asks := "Windows asks for an administrator once, for all of this"
	if !s.PawnIO {
		steps = append(steps, "installs PawnIO with winget (namazso.PawnIO), which accepts winget's source and package agreements for that one package")
		asks = "Windows asks for an administrator twice: once for PawnIO's installer, once for the helper"
	}
	return append(steps,
		"copies hayami's terminal program and two signed PawnIO modules to Program Files, where only an administrator can change them",
		"registers the service hayami-sensors to run that copy as LocalSystem, which PawnIO requires",
		"lets your account, and no other, start the service and read what it serves: the temperature and nothing else",
		"starts nothing: the service runs while a panel reads the temperature and stops when it is closed",
		asks,
	)
}
