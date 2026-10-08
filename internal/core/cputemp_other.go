//go:build !windows

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/sanshoku/hwmon"
)

// CPUSensorDetail names every sensor looked for, for the reason given when
// none reads: "no coretemp/Package id 0" on an AMD machine sent somebody
// looking for an Intel driver that was never going to be there.
func CPUSensorDetail() string {
	names := make([]string, 0, len(hwmon.CPU))
	for _, s := range hwmon.CPU {
		names = append(names, s.String())
	}
	return fmt.Sprintf("looked under %s for %s", hwmon.Root, strings.Join(names, ", "))
}

// HostCPUTemperature is the processor's temperature from the kernel's sensors,
// the first in hwmon.CPU's order, or an Absence naming every sensor looked
// for. The LibreHardwareMonitor address is a Windows setting and is not used
// here.
func HostCPUTemperature(string) func(context.Context) (float64, error) {
	return func(context.Context) (float64, error) {
		_, v, err := hwmon.First(hwmon.Root, hwmon.CPU)
		if err != nil {
			return 0, &Absence{Code: AbsenceCPUSensor, Detail: CPUSensorDetail(),
				Err: fmt.Errorf("reading the processor temperature: %w", err)}
		}
		return v, nil
	}
}
