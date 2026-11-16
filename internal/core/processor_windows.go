package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsProcessor is Windows's processor reader (spec 095 R5): each core from
// NtQuerySystemInformation, the clock and the run queue from the performance
// counters, and the processes and threads from K32GetPerformanceInfo. None
// needs a right the panel does not have.
func windowsProcessor() *ProcessorReader {
	counters := &processorCounters{}
	return NewProcessorReader(processorTimes, func(context.Context) Processor {
		p := Processor{Queues: true}
		if clock, queue, ok := counters.read(); ok {
			p.Clock, p.HasClock = clock, clock > 0
			p.Queue, p.HasQueue = queue, true
		}
		var pi performanceInformation
		pi.Cb = uint32(unsafe.Sizeof(pi))
		if r, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), uintptr(pi.Cb)); r != 0 {
			p.Processes, p.Threads, p.HasTasks = int(pi.ProcessCount), int(pi.ThreadCount), true
		}
		return p
	})
}

// processorTimes is every processor's times, from
// NtQuerySystemInformation(SystemProcessorPerformanceInformation).
func processorTimes() ([]ProcessorTimes, error) {
	size := uint32(48 * 64)
	for range 4 {
		b := make([]byte, size)
		var need uint32
		err := windows.NtQuerySystemInformation(windows.SystemProcessorPerformanceInformation,
			unsafe.Pointer(&b[0]), size, &need)
		if err == nil {
			return ParseProcessorTimes(b[:min(need, size)])
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) && !errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) {
			return nil, fmt.Errorf("NtQuerySystemInformation: %w", err)
		}
		size = max(need, size) * 2
	}
	return nil, errors.New("NtQuerySystemInformation: the processor times kept growing")
}

// The counters the clock and the run queue come from. The clock is the
// processor's frequency times its performance, which is what Task Manager
// shows as the speed: Processor Frequency alone is the base, and does not
// move.
const (
	counterPerformance = `\Processor Information(_Total)\% Processor Performance`
	counterFrequency   = `\Processor Information(_Total)\Processor Frequency`
	counterQueue       = `\System\Processor Queue Length`
)

var procPdhFormattedValue = pdh.NewProc("PdhGetFormattedCounterValue")

// pdhValue is PDH_FMT_COUNTERVALUE with a double value.
type pdhValue struct {
	Status uint32
	_      uint32
	Value  float64
}

/*
processorCounters holds a query open across polls. Performance is a rate, so
the first read collects twice, CPUWarmup apart, as engineLoad does; a machine
without the counters fails once and has no clock and no queue.
*/
type processorCounters struct {
	mu       sync.Mutex
	query    uintptr
	counters [3]uintptr
	last     time.Time
	failed   bool
}

func (c *processorCounters) read() (clock, queue float64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed {
		return 0, 0, false
	}
	if c.query == 0 && !c.open() {
		c.failed = true
		return 0, 0, false
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
		return 0, 0, false
	}
	var v [3]float64
	for i, counter := range c.counters {
		var out pdhValue
		r, _, _ := procPdhFormattedValue.Call(counter, pdhFmtDouble|pdhFmtNoCap100, 0, uintptr(unsafe.Pointer(&out)))
		if r != 0 || out.Status > 1 {
			return 0, 0, false
		}
		v[i] = out.Value
	}
	return v[1] * v[0] / 100, v[2], true
}

func (c *processorCounters) open() bool {
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&c.query))); r != 0 {
		return false
	}
	for i, name := range []string{counterPerformance, counterFrequency, counterQueue} {
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
