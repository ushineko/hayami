package core

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var procGetSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")

// NewHost is Windows' table (spec 043): LibreHardwareMonitor for the
// processor's temperature (spec 036), D3DKMT and the GPU Engine counters for
// the card with nvidia-smi behind them (spec 034), GetSystemTimes and the
// registry for the load and the name, the interface table and the WLAN
// service for the network (specs 033, 037), Windows' own refusal as the advice
// for a device that would not open, and no Bluetooth batteries: sanshoku's
// readers are BlueZ and an L2CAP socket, neither of which Windows gives a
// program (spec 035).
func NewHost(cfg HostConfig) Host {
	return Host{
		Platform:       "windows",
		CPUTemperature: windowsCPUTemperature(NewLHM(cfg.LHM).CPUTemperature),
		CPUMissing:     windowsCPUMissing,
		Graphics:       windowsGraphics(nativeGraphics(), NvidiaSMI),
		GPUMissing:     windowsGPUMissing,
		CPULoad:        systemTimesLoad,
		CPUName:        registryCPUName,
		Counters:       ReadCounters,
		Wireless:       ReadWireless,
		Permission:     PermissionAbsence,
	}
}

// systemTimesLoad is this machine's processor load: GetSystemTimes, which is
// Windows' /proc/stat. It needs no privilege.
func systemTimesLoad() *CPULoad { return newCPULoad(systemTimes) }

/*
systemTimes is the busy and idle time of every processor since boot, in
100-nanosecond ticks.

**Kernel time includes idle time.** GetSystemTimes counts the idle thread as
kernel work, so busy is kernel plus user less idle. Counting kernel time as
busy outright reads a sleeping machine as fully loaded.

It is the time-based figure, as /proc/stat's is, and so it is Task Manager's
"% Processor Time" and not its "% Processor Utility": on a processor that is
boosting, Task Manager can read higher than this.
*/
func systemTimes() (busy, idle float64, err error) {
	var i, k, u windows.Filetime
	r, _, callErr := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u)))
	if r == 0 {
		return 0, 0, fmt.Errorf("GetSystemTimes: %w", callErr)
	}
	ticks := func(t windows.Filetime) float64 {
		return float64(uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime))
	}
	idle = ticks(i)
	return ticks(k) + ticks(u) - idle, idle, nil
}

// cpuKey is where Windows describes the first processor. Every core of a
// desktop part carries the same name, as /proc/cpuinfo's do.
const cpuKey = `HARDWARE\DESCRIPTION\System\CentralProcessor\0`

// registryCPUName is the processor's model as Windows gives it, "AMD Ryzen 5
// 2600X Six-Core Processor", or empty where the key cannot be read. Windows
// pads the value with trailing spaces on some parts, which are not the name.
func registryCPUName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, cpuKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	name, _, err := k.GetStringValue("ProcessorNameString")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}
