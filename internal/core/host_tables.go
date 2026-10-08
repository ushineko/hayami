package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/sanshoku/hwmon"
)

/*
The platforms' chains and their accounts of absence, as plain functions with no
build tag (spec 043): host_linux.go and host_windows.go assemble them, and a
test on any system checks every platform's order and wording.
*/

// linuxCPUTemperature is the kernel's processor sensors under root, by label
// in hwmon.CPU's order: a provider per sensor, so the reason on a machine with
// none names every one looked for.
func linuxCPUTemperature(root string) Chain[float64] {
	return Chain[float64]{Providers: hwmonSensors(root, hwmon.CPU)}
}

// linuxCPUMissing is Linux's account of no processor temperature: every
// sensor looked for, and where.
func linuxCPUMissing(root string) func([]string, error) *Absence {
	return func(tried []string, err error) *Absence {
		if err != nil {
			err = fmt.Errorf("reading the processor temperature: %w", err)
		}
		return &Absence{Code: AbsenceCPUSensor,
			Detail: fmt.Sprintf("looked under %s for %s", root, strings.Join(tried, ", ")), Err: err}
	}
}

// linuxGraphics is the card through the kernel: hwmon's GPU sensors, AMD's
// busy file, then nvidia-smi for a card on NVIDIA's own driver, which
// registers neither (spec 026).
func linuxGraphics(root, busy, pciIDs string, smi func(context.Context) (string, error)) *GraphicsReader {
	return &GraphicsReader{Root: root, Busy: busy, SMI: smi, PCIIDs: pciIDs}
}

// linuxGPUMissing is Linux's account of no card temperature: where it looked
// and every route tried.
func linuxGPUMissing(root string) func([]string) string {
	return func(tried []string) string {
		return fmt.Sprintf("looked under %s and tried %s", root, strings.Join(tried, ", "))
	}
}

// nativeName is what Windows' own report of the card is called in a reason.
const nativeName = "D3DKMT and the GPU Engine counters"

// windowsCPUTemperature is LibreHardwareMonitor at its address (spec 036),
// read through read: it loads the driver hayami does not.
func windowsCPUTemperature(read func(context.Context) (float64, error)) Chain[float64] {
	return Chain[float64]{Providers: []Provider[float64]{ProviderFunc[float64]{N: "LibreHardwareMonitor", F: read}}}
}

// windowsCPUMissing is Windows' account where LibreHardwareMonitor did not say
// which way it is missing (a cancelled poll): the kernel keeps the temperature
// behind a driver, and the ACPI thermal zone, the one route without one, is
// unsupported on most desktop boards and reads a fixed value on others
// (spec 034).
func windowsCPUMissing(_ []string, err error) *Absence {
	return &Absence{Code: AbsenceCPUSensor, Detail: "Windows offers no CPU temperature without a kernel driver", Err: err}
}

// windowsGraphics is the card through D3DKMT and the GPU Engine counters,
// then nvidia-smi for what they left out (spec 034). No hwmon, no busy file.
func windowsGraphics(native func(context.Context) Graphics, smi func(context.Context) (string, error)) *GraphicsReader {
	return &GraphicsReader{Native: native, NativeName: nativeName, SMI: smi}
}

// windowsGPUMissing is Windows' account of no card temperature: the first
// route asked, then the ones tried after it.
func windowsGPUMissing(tried []string) string {
	switch len(tried) {
	case 0:
		return "no route to the card's temperature"
	case 1:
		return "asked " + tried[0]
	default:
		return "asked " + tried[0] + ", and tried " + strings.Join(tried[1:], ", ")
	}
}

// otherCPUMissing and otherGPUMissing are a platform with no reader at all.
func otherCPUMissing(_ []string, err error) *Absence {
	return &Absence{Code: AbsenceCPUSensor, Detail: "this build reads no processor temperature on this system", Err: err}
}

func otherGPUMissing([]string) string { return "this build reads no graphics card on this system" }

// hwmonSensors are the sensors as providers, in order, each read under root.
func hwmonSensors(root string, sensors []hwmon.Sensor) []Provider[float64] {
	out := make([]Provider[float64], 0, len(sensors))
	for _, s := range sensors {
		out = append(out, ProviderFunc[float64]{N: s.String(), F: func(context.Context) (float64, error) {
			v, err := s.Read(root)
			if err != nil {
				return 0, fmt.Errorf("reading %s: %w", s, err)
			}
			return v, nil
		}})
	}
	return out
}
