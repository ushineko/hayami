package panel

import (
	"context"
	"time"

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
	return newCooler(scan, sensor)
}

// UdevDetail is the detail a device that may not be opened is given.
const UdevDetail = udevDetail

// SetUsageRead replaces the gather, so a test can drive the usage section's
// reasons without a cache directory or a credential store.
func SetUsageRead(u *Usage, read func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error)) {
	u.read = read
}
