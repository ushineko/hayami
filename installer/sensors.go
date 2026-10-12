package installer

import (
	"fmt"

	"github.com/ushineko/hayami/internal/core"
)

/*
Sensors is what the installer found of LibreHardwareMonitor on this machine:
the "custom check" the sensors page is built on. Everything in it is read,
and nothing is changed to learn it.
*/
type Sensors struct {
	// Applies is false off Windows, where the kernel gives the processor's
	// temperature without help and there is nothing to offer.
	Applies bool
	// Exe is LibreHardwareMonitor's program where winget installs it, or ""
	// when it is not there.
	Exe string
	// Host says whether it is running, starts with Windows, and has its
	// driver.
	Host core.LHMHost
	// URL is where hayami asks for its sensor tree.
	URL string
	// Reading is the processor's temperature as hayami read it there, such
	// as "Core (Tctl/Tdie) 48.1 °C", or "" when it could not.
	Reading string
	// Problem is why it could not, in the words the panel's own reason line
	// uses.
	Problem string
}

// Ready reports whether hayami already reads the temperature: there is
// nothing to set up.
func (s Sensors) Ready() bool { return s.Reading != "" }

// Installed reports whether LibreHardwareMonitor is on the machine: its
// program where winget puts it, or signs of one installed another way.
func (s Sensors) Installed() bool {
	return s.Exe != "" || s.Host.Running || s.Host.Task
}

// Found says what the check found, in a sentence or two.
func (s Sensors) Found() string {
	switch {
	case !s.Applies:
		return "This system gives hayami the processor's temperature itself. There is nothing to set up."
	case s.Ready():
		return fmt.Sprintf("LibreHardwareMonitor is set up: hayami reads the processor's temperature from it (%s). There is nothing to do.", s.Reading)
	case s.Installed():
		text := "LibreHardwareMonitor is installed, and hayami cannot read the processor's temperature from it yet."
		if s.Problem != "" {
			text += " " + s.Problem + "."
		}
		return text
	default:
		return "LibreHardwareMonitor is not installed. Without it the panel shows the processor's load and no temperature."
	}
}

// Offer is the question the page asks, or "" when there is nothing to
// offer.
func (s Sensors) Offer() string {
	switch {
	case !s.Applies || s.Ready():
		return ""
	case s.Exe != "":
		return "Configure LibreHardwareMonitor for hayami"
	default:
		return "Install LibreHardwareMonitor and configure it for hayami"
	}
}

// Why is why the page exists at all, said once above the steps.
const Why = "Windows keeps the processor's temperature behind a kernel driver, and hayami loads none. " +
	"LibreHardwareMonitor does, and hayami reads what it serves on this computer. Setting it up:"

// Caution is what a person should know before saying yes.
const Caution = "It is a second program: it runs as an administrator and loads a kernel driver (PawnIO). " +
	"The panel works without it, and you can set it up later by running this installer again."

// Steps are what setting it up does, in order. They are install_windows.ps1
// -WithSensors's, which the hook runs: each is done only if it is not done
// already.
func (s Sensors) Steps() []string {
	var steps []string
	if s.Exe == "" {
		steps = append(steps, "installs it with winget, which accepts winget's source and package agreements for that one package")
	}
	return append(steps,
		"turns its web server on, with no password, on the port hayami asks; Windows Firewall's default keeps it off your network",
		"makes it start minimized, and hide in the notification area when its window is closed",
		"registers it to start with Windows, as an administrator",
		"starts it; on its first start it offers its PawnIO driver, which is what reads the temperature",
		"Windows asks for an administrator once, for all of this",
	)
}
