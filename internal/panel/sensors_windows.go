package panel

import (
	"context"

	"github.com/ushineko/hayami/internal/core"
)

// cpuSensorDetail says why a Windows machine has no processor temperature,
// where the source did not say which way it is missing. The kernel keeps it
// behind a driver: the ACPI thermal zone, the one route without one, is
// unsupported on most desktop boards and reads a fixed value on others (spec
// 034).
func cpuSensorDetail() string {
	return "Windows offers no CPU temperature without a kernel driver"
}

// cpuTemperature is the processor's temperature from LibreHardwareMonitor at
// lhm, or at its default address (spec 036). It loads the driver hayami does
// not, and publishes what it reads.
func cpuTemperature(lhm string) func(context.Context) (float64, error) {
	return core.NewLHM(lhm).CPUTemperature
}

// gpuSensorDetail is every route to the card's temperature, for the reason
// given when none answers.
func gpuSensorDetail() string {
	return "asked D3DKMT and the GPU Engine counters, and tried nvidia-smi"
}
