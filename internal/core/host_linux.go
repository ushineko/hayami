package core

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/apple"
	"github.com/ushineko/sanshoku/bluez"
	"github.com/ushineko/sanshoku/hwmon"
)

// NewHost is Linux's table (spec 043): the kernel's sensors for the processor
// and the card, nvidia-smi behind them, /proc for the load and the counters,
// nl80211 for Wi-Fi, the udev rule as the advice for a device that would not
// open, and BlueZ and Apple's accessory protocol for Bluetooth batteries.
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
		Bluetooth:      []sanshoku.Driver{apple.Driver{}, bluez.Driver{}},
	}
}
