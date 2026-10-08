package testenv

import (
	"context"
	"runtime"
	"testing"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"

	"github.com/ushineko/hayami/internal/panel"
)

/*
NoDevices takes the desk away from the real device sections, so a test that
polls them opens no device.

On Linux an empty hidraw tree does it, and the drivers stay real: a test can
write a node into the tree and have a real driver find it. Where hidraw reads
the system's own device list (Windows, spec 035) no directory stands in for
it, and without the empty scan the suite asked the real mouse and keyboard --
a unit suite has no business writing to somebody's hardware, and `go test
./...` runs packages in parallel, racing any other reader of the same
receiver.
*/
func NoDevices(t testing.TB) {
	t.Helper()
	sys, dev := hidraw.SysRoot, hidraw.DevRoot
	hidraw.SysRoot, hidraw.DevRoot = t.TempDir(), t.TempDir()
	t.Cleanup(func() { hidraw.SysRoot, hidraw.DevRoot = sys, dev })

	if runtime.GOOS == "linux" {
		return
	}
	scan := panel.DeviceScan
	panel.DeviceScan = func(context.Context, ...sanshoku.Driver) ([]sanshoku.Candidate, error) {
		return nil, nil
	}
	t.Cleanup(func() { panel.DeviceScan = scan })
}
