/*
Package cooler reads what the machine says about its own temperature.

Two sources, because one is not enough. The kernel exposes the processor in a
file; it does not expose this cooler at all, because the Kraken here is not one
the nzxt-kraken3 driver matches, so no hwmon node exists for it. liquidctl is
the only source of a coolant temperature and a pump speed, and it is the one
subprocess this package is allowed.

The approach is hotaru's, not its code. That project's internal/cooler reads
sensors by label and its comment says what it bought: OpenLinkHub stopped being
a dependency, because the number the Python asked a daemon for over HTTP is in
a file. hotaru's package also speaks HID to the device it writes to, which
hayami has no business opening, so what is copied is the rule and its reason.
*/
package cooler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// HwmonRoot is where the kernel puts its sensors.
const HwmonRoot = "/sys/class/hwmon"

// Sensor is one labelled temperature the kernel already exposes.
//
// **Read by label, never by hwmon index.** `coretemp` was hwmon10 when this
// was written and will be something else after a reboot; a program that
// remembers the number reports another chip's temperature rather than failing,
// which is the worst way to be wrong.
type Sensor struct {
	// Chip is the hwmon name: "coretemp", "nct6798", "amdgpu".
	Chip string

	// Label is the sensor within it: "Package id 0". Empty means the chip's
	// first temperature, for a chip that labels nothing.
	Label string
}

/*
CPUSensors are the processor temperatures this build knows, most wanted first.

**One constant was one vendor.** It named `coretemp`, which is Intel's driver,
so a Ryzen read nothing at all and said "no coretemp/Package id 0" -- precise
about what it looked for and silent about only ever looking for one thing
(issue #72).

The order is not alphabetical and not arbitrary:

  - `coretemp` / `Package id 0` is Intel's package temperature.
  - `k10temp` / `Tdie` is AMD's die temperature, where a chip exposes it.
  - `k10temp` / `Tctl` is the fallback, and **it is not a temperature.** It is
    a control value carrying a per-model offset the firmware uses for fan
    curves, and on some chips it reads several degrees above the die. It is
    last among AMD's because it is the one to take when there is nothing
    better; a 2600X exposes only this. Do not "tidy" it above Tdie.
  - `zenpower` / `Tdie` is the out-of-tree AMD driver some people run instead
    of k10temp, which exposes a die temperature where k10temp gives only Tctl.

The first that reads wins, so a machine with both k10temp and zenpower loaded
gets the in-tree one, which is the one its fan curves are built on.
*/
var CPUSensors = []Sensor{
	{Chip: "coretemp", Label: "Package id 0"},
	{Chip: "k10temp", Label: "Tdie"},
	{Chip: "k10temp", Label: "Tctl"},
	{Chip: "zenpower", Label: "Tdie"},
}

// CPUPackage is the first CPU sensor this machine has, or the last one tried
// where it has none -- so the error names something real rather than nothing.
func CPUPackage() (float64, error) { return ReadCPU(HwmonRoot) }

/*
ReadCPU is CPUPackage against a hwmon root the caller names, so a test can
build its own tree rather than depending on the machine it runs on.

A machine with none of them gets an error naming every sensor looked for, not
just the last: "no coretemp/Package id 0" on an AMD box sent somebody looking
for an Intel driver that was never going to be there.
*/
func ReadCPU(root string) (float64, error) {
	for _, s := range CPUSensors {
		if v, err := s.Read(root); err == nil {
			return v, nil
		}
	}
	return 0, fmt.Errorf("%w: looked for %s", ErrNoSensor, CPUSensorNames())
}

// CPUSensorNames lists the sensors, for an error and for the panel's reason.
func CPUSensorNames() string {
	names := make([]string, 0, len(CPUSensors))
	for _, s := range CPUSensors {
		names = append(names, s.Chip+"/"+s.Label)
	}
	return strings.Join(names, ", ")
}

// ErrNoSensor is a sensor this machine does not have. It is a reading that is
// absent, not a failure: a panel on a machine with a different processor
// should draw the rest.
var ErrNoSensor = errors.New("no such sensor on this machine")

// Temperature is the sensor's reading in degrees.
func (s Sensor) Temperature() (float64, error) { return s.Read(HwmonRoot) }

// Read is Temperature against a hwmon root the caller names, so a test can
// build its own tree rather than depending on the machine it runs on.
//
// It walks the trees looking for the chip, then for the label inside it.
func (s Sensor) Read(root string) (float64, error) {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return 0, ErrNoSensor
	}

	for _, d := range dirs {
		dir := filepath.Join(root, d.Name())
		name, err := text(filepath.Join(dir, "name"))
		if err != nil || name != s.Chip {
			continue
		}
		if v, err := s.readChip(dir); err == nil {
			return v, nil
		}
	}
	return 0, ErrNoSensor
}

// readChip finds the labelled temperature inside one chip's directory.
func (s Sensor) readChip(dir string) (float64, error) {
	if s.Label == "" {
		return milli(filepath.Join(dir, "temp1_input"))
	}

	labels, err := filepath.Glob(filepath.Join(dir, "temp*_label"))
	if err != nil {
		return 0, ErrNoSensor
	}
	for _, path := range labels {
		got, err := text(path)
		if err != nil || got != s.Label {
			continue
		}
		return milli(strings.TrimSuffix(path, "_label") + "_input")
	}
	return 0, ErrNoSensor
}

// text is a one-line sysfs file, trimmed.
func text(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a path under the hwmon root
	if err != nil {
		return "", err //nolint:wrapcheck // the caller turns any failure into ErrNoSensor
	}
	return strings.TrimSpace(string(b)), nil
}

// milli reads a sysfs temperature, which the kernel writes in thousandths.
func milli(path string) (float64, error) {
	s, err := text(path)
	if err != nil {
		return 0, ErrNoSensor
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, ErrNoSensor
	}
	return v / 1000, nil
}
