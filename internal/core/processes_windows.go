package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// filetimeSecond is how many 100-nanosecond ticks make a second.
const filetimeSecond = 1e7

/*
systemProcesses reads every process from
NtQuerySystemInformation(SystemProcessInformation): the name, the user and
kernel time and the private working set, in one call and with no handle to
any process, so a process this user could not open is listed like the rest.

The table is asked for its size and then read, and asked again if it grew in
between: processes come and go.

The idle process is left out. Its "kernel time" is the time nothing ran, and
it would head every list.
*/
func systemProcesses(context.Context) ([]ProcessSample, error) {
	size := uint32(1 << 20)
	var buf []byte
	for range 4 {
		b := make([]byte, size)
		var need uint32
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation,
			unsafe.Pointer(&b[0]), size, &need)
		if err == nil {
			buf = b
			break
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) && !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
			return nil, fmt.Errorf("NtQuerySystemInformation: %w", err)
		}
		size = max(need, size) + 1<<16
	}
	if buf == nil {
		return nil, errors.New("NtQuerySystemInformation: the process table kept growing")
	}

	var out []ProcessSample
	for offset := uintptr(0); ; {
		p := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[offset]))
		if pid := int(p.UniqueProcessID); pid != 0 {
			out = append(out, ProcessSample{
				PID:        pid,
				Name:       programName(p.ImageName.String()),
				CPUSeconds: float64(p.UserTime+p.KernelTime) / filetimeSecond,
				Memory:     uint64(p.WorkingSetPrivateSize), //nolint:gosec // a size is not negative
			})
		}
		if p.NextEntryOffset == 0 {
			break
		}
		offset += uintptr(p.NextEntryOffset)
	}
	return out, nil
}

// programName is an image name as a person says it: "chrome" for
// "chrome.exe". The kernel has no image and is "System".
func programName(image string) string {
	if image == "" {
		return "System"
	}
	if len(image) > 4 && strings.EqualFold(image[len(image)-4:], ".exe") {
		return image[:len(image)-4]
	}
	return image
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryStatusEx is MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// globalMemory is the physical memory in use, from GlobalMemoryStatusEx: the
// total less what is available, which is what Task Manager calls "In use".
func globalMemory(context.Context) (Memory, error) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return Memory{}, fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}
	if m.AvailPhys > m.TotalPhys {
		return Memory{}, errors.New("GlobalMemoryStatusEx: more memory available than there is")
	}
	return Memory{Used: m.TotalPhys - m.AvailPhys, Total: m.TotalPhys}, nil
}
