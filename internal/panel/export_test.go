package panel

import (
	"context"
	"time"

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
}
