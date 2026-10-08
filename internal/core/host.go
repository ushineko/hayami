package core

import (
	"context"

	"github.com/ushineko/sanshoku"
)

/*
Host is what this platform offers the panel, in one table (spec 043,
docs/architecture.md rule 5): the chains a reading is asked through, the
readers with no alternative, the advice for a device it would not open, and
the device drivers it can use beyond the HID ones every platform has.

Each platform declares its own in host_linux.go, host_windows.go and
host_other.go, where _other is honest absence: no Linux path is assumed
anywhere else. The panel takes a Host through its Env and has no build tags;
a test builds one of its own.
*/
type Host struct {
	// Platform names the table, for a test and a log line: "linux",
	// "windows", or "other".
	Platform string

	// CPUTemperature is the processor's temperature, in degrees. CPUMissing
	// is the account given when no provider answered and none said why
	// itself; it is given the providers tried, and their first error.
	CPUTemperature Chain[float64]
	CPUMissing     func(tried []string, err error) *Absence

	// Graphics is the card, read as one chain. GPUMissing is the detail given
	// when the chain found no temperature, from the providers it tried.
	Graphics   *GraphicsReader
	GPUMissing func(tried []string) string

	// CPULoad builds a load reader, which keeps its own previous sample, and
	// CPUName is the processor's model, empty where it is not known.
	CPULoad func() *CPULoad
	CPUName func() string

	// Counters reads the interface table; Wireless describes the Wi-Fi
	// interfaces, nothing on a machine with none.
	Counters func() (map[string]Counters, error)
	Wireless func(context.Context) (map[string]Wireless, error)

	// Permission is an Open refused for want of permission, as the absence a
	// reader can act on, with this platform's advice.
	Permission func(error) (*Absence, bool)

	// Bluetooth are the drivers that read Bluetooth batteries here; none
	// where the platform gives hayami no way to (spec 035).
	Bluetooth []sanshoku.Driver
}

// HostConfig is what a Host is built with from the settings.
type HostConfig struct {
	// LHM is where LibreHardwareMonitor serves its sensor tree, for the
	// processor's temperature on Windows (spec 036); empty is its default
	// address. Other platforms ignore it.
	LHM string
}

// ReadCPUTemperature is the processor's temperature from the chain, or the
// absence: a provider's own account where one gave it, else the platform's,
// naming everything tried.
func (h *Host) ReadCPUTemperature(ctx context.Context) (float64, error) {
	o := h.CPUTemperature.Read(ctx)
	if o.Answered {
		return o.Value, nil
	}
	if o.Absence != nil {
		return 0, o.Absence
	}
	return 0, h.CPUMissing(o.Tried, o.Err)
}

// CPUSensorDetail is the account of a missing processor temperature when
// every provider was tried and none said why: what doctor shows on a machine
// with none.
func (h *Host) CPUSensorDetail() string {
	return h.CPUMissing(h.CPUTemperature.Names(), nil).Detail
}

// ReadGraphics is the card, and an absence when the chain found no
// temperature, which names every route it tried.
func (h *Host) ReadGraphics(ctx context.Context) (Graphics, error) {
	o := h.Graphics.ReadOutcome(ctx)
	if o.Value.HasTemperature {
		return o.Value, nil
	}
	return o.Value, &Absence{Code: AbsenceGPUSensor, Detail: h.GPUMissing(o.Tried), Err: o.Err}
}

// GPUSensorDetail is the detail of a missing card temperature when every
// route was tried.
func (h *Host) GPUSensorDetail() string { return h.GPUMissing(h.Graphics.Chain().Names()) }

// NewGraphicsReader reads this machine's card by this platform's routes.
func NewGraphicsReader() *GraphicsReader { return NewHost(HostConfig{}).Graphics }

// HostCPUName is this machine's processor model.
func HostCPUName() string { return NewHost(HostConfig{}).CPUName() }
