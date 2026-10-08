//go:build !windows

package panel

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/sanshoku/hwmon"
)

// cpuSensorDetail names every sensor looked for, for the reason given when
// none reads: "no coretemp/Package id 0" on an AMD machine sent somebody
// looking for an Intel driver that was never going to be there.
func cpuSensorDetail() string {
	names := make([]string, 0, len(hwmon.CPU))
	for _, s := range hwmon.CPU {
		names = append(names, s.String())
	}
	return fmt.Sprintf("looked under %s for %s", hwmon.Root, strings.Join(names, ", "))
}

// gpuSensorDetail is every route to the card's temperature, for the reason
// given when none answers.
func gpuSensorDetail() string {
	names := make([]string, 0, len(hwmon.GPU)+1)
	for _, s := range hwmon.GPU {
		names = append(names, s.String())
	}
	return fmt.Sprintf("looked under %s and tried %s", hwmon.Root, strings.Join(append(names, "nvidia-smi"), ", "))
}

// cpuPackage is the first processor sensor this machine has, in hwmon.CPU's
// order.
func cpuPackage() (float64, error) {
	_, v, err := hwmon.First(hwmon.Root, hwmon.CPU)
	if err != nil {
		return 0, fmt.Errorf("reading the processor temperature: %w", err)
	}
	return v, nil
}

// cpuTemperature is the processor's temperature from the kernel's sensors.
// The LibreHardwareMonitor address is a Windows setting and is not used here.
func cpuTemperature(string) func(context.Context) (float64, error) {
	return func(context.Context) (float64, error) { return cpuPackage() }
}
