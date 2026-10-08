package core

import "context"

// CPUSensorDetail says why a Windows machine has no processor temperature,
// where LibreHardwareMonitor did not say which way it is missing. The kernel
// keeps it behind a driver: the ACPI thermal zone, the one route without one,
// is unsupported on most desktop boards and reads a fixed value on others
// (spec 034).
func CPUSensorDetail() string {
	return "Windows offers no CPU temperature without a kernel driver"
}

// HostCPUTemperature is the processor's temperature from LibreHardwareMonitor
// at lhm, or at its default address (spec 036). It loads the driver hayami
// does not, and publishes what it reads. A failure it does not account for
// itself -- a cancelled poll -- is the platform's absence.
func HostCPUTemperature(lhm string) func(context.Context) (float64, error) {
	read := NewLHM(lhm).CPUTemperature
	return func(ctx context.Context) (float64, error) {
		v, err := read(ctx)
		if err == nil {
			return v, nil
		}
		if _, ok := AbsenceOf(err); ok {
			return 0, err
		}
		return 0, &Absence{Code: AbsenceCPUSensor, Detail: CPUSensorDetail(), Err: err}
	}
}
