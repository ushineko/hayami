package core

import (
	"github.com/ushineko/sanshoku/hwmon"
)

// NewHost is Linux's table (spec 043): the kernel's sensors for the processor
// and the card, nvidia-smi behind them, /proc for the load and the counters,
// nl80211 for Wi-Fi, and the udev rule as the advice for a device that would
// not open. BlueZ and Apple's accessory protocol read Bluetooth batteries here
// because their descriptions say they read on Linux (spec 048).
func NewHost(HostConfig) Host {
	return Host{
		Platform:       "linux",
		CPUTemperature: linuxCPUTemperature(hwmon.Root),
		CPUMissing:     linuxCPUMissing(hwmon.Root),
		Graphics:       linuxGraphics(hwmon.Root, BusyGlob, PCIIDsPath, NvidiaSMI),
		GPUMissing:     linuxGPUMissing(hwmon.Root),
		CPULoad:        func() *CPULoad { return NewCPULoad(ProcStatPath) },
		CPUName:        func() string { return CPUName(CPUInfoPath) },
		Counters:       ReadCounters,
		Wireless:       ReadWireless,
		Permission:     PermissionAbsence,
	}
}
