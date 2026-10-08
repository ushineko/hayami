package core

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// lhmProcess is LibreHardwareMonitor's executable, as a process list shows it.
const lhmProcess = "librehardwaremonitor.exe"

// pawnIOService is the PawnIO driver's service name.
const pawnIOService = "PawnIO"

/*
ProbeLHMHost says whether PawnIO is installed and LibreHardwareMonitor is
running, for the reason given when its web server does not answer.

Both are asked read-only, and neither needs administrator rights: the service
manager is opened to connect and the service to query its status, which any
user may; the process list names every process, elevated ones included.
*/
func ProbeLHMHost() LHMHost {
	return LHMHost{PawnIO: serviceInstalled(pawnIOService), Running: processRunning(lhmProcess)}
}

func serviceInstalled(name string) bool {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	s, err := windows.OpenService(m, n, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	_ = windows.CloseServiceHandle(s)
	return true
}

func processRunning(exe string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			return true
		}
	}
	return false
}
