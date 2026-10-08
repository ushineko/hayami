//go:build !linux && !windows

package core

import (
	"context"
	"errors"
	"fmt"
)

// NewHost is a platform this build has no readers for (spec 043): every chain
// is empty and says so, the load and the counters report their absence, and
// no Linux path is assumed. Its device drivers are the HID ones sanshoku finds
// nothing with here.
func NewHost(HostConfig) Host {
	return Host{
		Platform:   "other",
		CPUMissing: otherCPUMissing,
		Graphics:   &GraphicsReader{},
		GPUMissing: otherGPUMissing,
		CPULoad: func() *CPULoad {
			return newCPULoad(func() (float64, float64, error) {
				return 0, 0, fmt.Errorf("processor load: %w", errors.ErrUnsupported)
			})
		},
		CPUName:    func() string { return "" },
		Counters:   ReadCounters,
		Wireless:   func(context.Context) (map[string]Wireless, error) { return nil, nil },
		Permission: PermissionAbsence,
	}
}
