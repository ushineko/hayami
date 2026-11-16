package core

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The counters Windows's disk reader collects (spec 097 R6), in the order
// read uses them: each physical disk's rates, idle time and queue, and the
// pages swapped in and out. They are rates already, over the time between two
// collections.
var diskCounterNames = []string{
	`\PhysicalDisk(*)\Disk Read Bytes/sec`,
	`\PhysicalDisk(*)\Disk Write Bytes/sec`,
	`\PhysicalDisk(*)\% Idle Time`,
	`\PhysicalDisk(*)\Avg. Disk Queue Length`,
	`\Memory\Pages Input/sec`,
	`\Memory\Pages Output/sec`,
}

// windowsDisks is Windows's disk reader: the PhysicalDisk and Memory
// counters, which need no right the panel does not have.
func windowsDisks() *DiskReader {
	c := &diskCounters{}
	return NewDiskReader(c.read)
}

/*
diskCounters holds the query open across polls. The first read collects
twice, CPUWarmup apart, as engineLoad does; a machine without the counters
fails once and reads no disk.
*/
type diskCounters struct {
	mu       sync.Mutex
	query    uintptr
	counters []uintptr
	last     time.Time
	failed   bool
}

func (c *diskCounters) read(context.Context) Disks {
	none := Disks{Queues: true, AllIO: true}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed {
		return none
	}
	if c.query == 0 && !c.open() {
		c.failed = true
		return none
	}
	if c.last.IsZero() {
		_, _, _ = procPdhCollect.Call(c.query)
		c.last = time.Now()
	}
	if soon := CPUWarmup - time.Since(c.last); soon > 0 {
		time.Sleep(soon)
	}
	r, _, _ := procPdhCollect.Call(c.query)
	c.last = time.Now()
	if r != 0 {
		return none
	}
	v := make([]map[string]float64, len(c.counters))
	for i, counter := range c.counters {
		v[i] = pdhArray(counter)
	}
	return WindowsDisks(v[0], v[1], v[2], v[3], v[4], v[5], float64(windows.Getpagesize()))
}

func (c *diskCounters) open() bool {
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&c.query))); r != 0 {
		return false
	}
	c.counters = make([]uintptr, len(diskCounterNames))
	for i, name := range diskCounterNames {
		path, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return false
		}
		if r, _, _ := procPdhAddCounter.Call(c.query, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&c.counters[i]))); r != 0 {
			return false
		}
	}
	return true
}
