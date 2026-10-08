package panel

import (
	"context"
	"time"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

// NewPeripheralsOver builds the peripherals source over a test's scan and
// clock.
//
// It is in an export_test.go so it exists only when the test binary is built:
// the scan is swapped so the suite opens no device, and that is not a reason
// to widen the package's API for everybody else.
func NewPeripheralsOver(scan Scan, now func() time.Time) *Peripherals {
	return newPeripherals(scan, now)
}

// NewCoolerOver builds the cooler source over a test's scan and processor
// sensor, so the suite touches neither the real hwmon tree nor a real device.
func NewCoolerOver(scan Scan, sensor func() (float64, error)) *Cooler {
	return newCooler(scan, func(context.Context) (float64, error) { return sensor() })
}

// SensorDetail is what the reason for a missing processor temperature says
// for err.
func SensorDetail(err error) string {
	return reason(view.Reason{Detail: platform().CPUSensorDetail()}, err).Detail
}

// PermissionDetail is the detail a device that may not be opened is given.
const PermissionDetail = core.PermissionDetail

// GPUSensorDetail is what the reason for a missing graphics card says, every
// route tried.
func GPUSensorDetail() string { return platform().GPUSensorDetail() }

// BluetoothAbsent is the Bluetooth vendor's line on a desk with nothing on
// it, where this platform's drivers read Bluetooth; nothing where none does
// (spec 035, spec 048).
func BluetoothAbsent() []string {
	for _, v := range vendors(platform().Platform) {
		if v.name == "Bluetooth" {
			return []string{v.absent()}
		}
	}
	return nil
}

// Vendor is one vendor as the peripherals section asks it, for the tests
// that pin the list (spec 048).
type Vendor struct {
	Name, Absent string
	Quiet        bool
	Drivers      []string
}

// Vendors is the vendor list for platform.
func Vendors(platform string) []Vendor {
	var out []Vendor
	for _, v := range vendors(platform) {
		names := make([]string, 0, len(v.drivers))
		for _, d := range v.drivers {
			names = append(names, d.Name())
		}
		out = append(out, Vendor{Name: v.name, Absent: v.absent(), Quiet: v.quiet, Drivers: names})
	}
	return out
}

// SetUsageRead replaces the gather, so a test can drive the usage section's
// reasons without a cache directory or a credential store.
func SetUsageRead(u *Usage, read func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error)) {
	u.read = read
}

// SetProcessors replaces the processor's load and the graphics card's reader,
// so a test drives both without /proc/stat, the card or nvidia-smi.
func SetProcessors(c *Cooler, load func() (float64, bool), graphics func(context.Context) core.Graphics) {
	c.load = load
	c.graphics = func(ctx context.Context) (core.Graphics, error) { return graphics(ctx), nil }
}

// SetGraphics replaces the graphics card's reader with one that also says why
// it has no temperature, as a host's chain does.
func SetGraphics(c *Cooler, graphics func(context.Context) (core.Graphics, error)) {
	c.graphics = graphics
}

// NewCoolerOn builds the cooler source over a test's host and scan, which is
// NewCooler with nothing of the machine's in it.
func NewCoolerOn(h *core.Host, scan Scan) *Cooler { return NewCooler(h, scan) }

// NewPeripheralsOn builds the peripherals source over a test's host, scan and
// clock.
func NewPeripheralsOn(h *core.Host, scan Scan, now func() time.Time) *Peripherals {
	return newPeripheralsOn(h, scan, now)
}

// SetCPUName replaces where the processor's model is read from, so a test
// names it without /proc/cpuinfo.
func SetCPUName(c *Cooler, name func() string) { c.cpuName = name }

// RememberIn keeps a source's memory of devices in a test's file, and loads
// what is already there (spec 032).
func RememberIn(p *Peripherals, path string) { p.remember(path) }
