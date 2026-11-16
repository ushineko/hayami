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
	buf, err := processTable()
	if err != nil {
		return nil, err
	}
	var out []ProcessSample
	eachProcess(buf, func(p *windows.SYSTEM_PROCESS_INFORMATION) {
		if pid := int(p.UniqueProcessID); pid != 0 {
			out = append(out, ProcessSample{
				PID:        pid,
				Name:       programName(p.ImageName.String()),
				CPUSeconds: float64(p.UserTime+p.KernelTime) / filetimeSecond,
				Memory:     uint64(p.WorkingSetPrivateSize), //nolint:gosec // a size is not negative
			})
		}
	})
	return out, nil
}

// processTable is NtQuerySystemInformation(SystemProcessInformation) as it
// stood when it was read.
func processTable() ([]byte, error) {
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
	return buf, nil
}

// eachProcess calls fn for every entry of a process table.
func eachProcess(buf []byte, fn func(*windows.SYSTEM_PROCESS_INFORMATION)) {
	for offset := uintptr(0); ; {
		p := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[offset]))
		fn(p)
		if p.NextEntryOffset == 0 {
			return
		}
		offset += uintptr(p.NextEntryOffset)
	}
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
	out := Memory{Used: m.TotalPhys - m.AvailPhys, Total: m.TotalPhys, SwapName: "page file"}
	// The cache and the page file are the summary's (spec 094 R7). Either
	// failing costs its own line, not the memory in use.
	var pi performanceInformation
	pi.Cb = uint32(unsafe.Sizeof(pi))
	if r, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.Cb)); r != 0 && pi.PageSize > 0 {
		out.Cache, out.HasCache = uint64(pi.SystemCache)*uint64(pi.PageSize), true
		if buf, err := pageFileTable(); err == nil {
			if used, total, err := pageFiles(buf, uint64(pi.PageSize)); err == nil {
				out.SwapUsed, out.SwapTotal, out.HasSwap = used, total, true
			}
		}
	}
	return out, nil
}

var procGetPerformanceInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetPerformanceInfo")

// performanceInformation is PERFORMANCE_INFORMATION; the sizes and counts in
// pages are SIZE_T.
type performanceInformation struct {
	Cb                uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

// pageFileTable is NtQuerySystemInformation(SystemPageFileInformation): the
// page files, as long as the call says, and empty for none.
func pageFileTable() ([]byte, error) {
	size := uint32(4096)
	for range 4 {
		b := make([]byte, size)
		var need uint32
		err := windows.NtQuerySystemInformation(windows.SystemPageFileInformation,
			unsafe.Pointer(&b[0]), size, &need)
		if err == nil {
			return b[:min(need, size)], nil
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) && !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
			return nil, fmt.Errorf("NtQuerySystemInformation: %w", err)
		}
		size = max(need, size) * 2
	}
	return nil, errors.New("NtQuerySystemInformation: the page file information kept growing")
}
