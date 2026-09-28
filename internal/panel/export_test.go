package panel

import (
	"context"
	"time"

	"github.com/ushineko/hayami/internal/cooler"
	"github.com/ushineko/hayami/internal/peripherals"
)

// SetPeripheralSources replaces the two sources and the clock.
//
// It is in an export_test.go so it exists only when the test binary is built:
// the sources are swapped so the suite touches neither a real device nor a
// real subprocess, and that is not a reason to widen the package's API for
// everybody else.
func SetPeripheralSources(
	p *Peripherals,
	logitech func() ([]peripherals.Battery, error),
	headsets func(context.Context) ([]peripherals.Battery, error),
	now func() time.Time,
) {
	p.logitech, p.headsets, p.now = logitech, headsets, now
	p.bluetooth = func() ([]peripherals.Battery, error) { return nil, peripherals.ErrNoBluez }
}

// SetPeripheralBluetooth replaces just the Bluetooth source.
func SetPeripheralBluetooth(p *Peripherals, bluetooth func() ([]peripherals.Battery, error)) {
	p.bluetooth = bluetooth
}

// SetCoolerSources replaces the two sources, so the suite touches neither the
// real hwmon tree nor a real subprocess.
func SetCoolerSources(
	c *Cooler,
	sensor func() (float64, error),
	liquid func(context.Context) (cooler.Liquid, error),
) {
	c.sensor, c.liquid = sensor, liquid
}
