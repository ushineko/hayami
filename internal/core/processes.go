package core

import (
	"context"
	"runtime"
	"sync"
	"time"
)

/*
ProcessSample is one process as the system counts it: its ID, its name, the
processor time it has had since it started and the memory it holds now
(spec 060).

**The name and nothing else about it.** No command line, no path to the
program and no owner is read: the panel says which program, and a command
line carries file names, addresses and now and then a password.
*/
type ProcessSample struct {
	PID  int
	Name string

	// CPUSeconds is the user and kernel time the process has had, in seconds.
	CPUSeconds float64

	// Memory is what the process holds in physical memory, in bytes: its
	// resident set on Linux, its private working set on Windows.
	Memory uint64
}

// Process is one process's use of the machine since the previous reading.
type Process struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`

	// CPU is the processor time the process had since the previous reading,
	// as a percentage of the whole machine: every processor busy is 100.
	// HasCPU is false for a process first seen in this reading.
	CPU    float64 `json:"cpu"`
	HasCPU bool    `json:"has_cpu"`

	Memory uint64 `json:"memory"`
}

/*
ProcessList reads the processes and differences their processor time against
the previous reading, as CPULoad does for the machine's.

**The first call takes both samples**, CPUWarmup apart, for the reason
CPULoad's does: a command that reads once would otherwise never say a share.
Later calls difference against the previous call's sample, so between two
polls the share is a mean over the interval.
*/
type ProcessList struct {
	mu sync.Mutex

	// sample reads the processes: this platform's, or a test's.
	sample func(context.Context) ([]ProcessSample, error)

	last   map[int]float64
	lastAt time.Time

	// cpus is how many processors a second of processor time is shared
	// between; now and wait are the clock and the first call's pause, which
	// a test replaces.
	cpus int
	now  func() time.Time
	wait func()
}

// NewProcessList differences whatever sample reads.
func NewProcessList(sample func(context.Context) ([]ProcessSample, error)) *ProcessList {
	return &ProcessList{
		sample: sample,
		cpus:   runtime.NumCPU(),
		now:    time.Now,
		wait:   func() { time.Sleep(CPUWarmup) },
	}
}

// Read is every process the system would name, each with its share of the
// processors since the previous call and the memory it holds.
func (l *ProcessList) Read(ctx context.Context) ([]Process, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.last == nil {
		first, err := l.sample(ctx)
		if err != nil {
			return nil, err
		}
		l.remember(first)
		l.wait()
	}

	samples, err := l.sample(ctx)
	if err != nil {
		return nil, err
	}
	now := l.now()
	elapsed := now.Sub(l.lastAt).Seconds() * float64(max(l.cpus, 1))

	out := make([]Process, 0, len(samples))
	for _, s := range samples {
		p := Process{PID: s.PID, Name: s.Name, Memory: s.Memory}
		// A process ID is reused. One whose time went backwards is another
		// process under an old number, and has no share yet.
		if was, ok := l.last[s.PID]; ok && elapsed > 0 && s.CPUSeconds >= was {
			p.CPU, p.HasCPU = 100*(s.CPUSeconds-was)/elapsed, true
		}
		out = append(out, p)
	}
	l.rememberAt(samples, now)
	return out, nil
}

func (l *ProcessList) remember(samples []ProcessSample) { l.rememberAt(samples, l.now()) }

func (l *ProcessList) rememberAt(samples []ProcessSample, at time.Time) {
	l.last = make(map[int]float64, len(samples))
	for _, s := range samples {
		l.last[s.PID] = s.CPUSeconds
	}
	l.lastAt = at
}

// Names is each process's name by its ID, for a reader that knows a process
// only by number: the graphics card's counters, a socket's owner.
func Names(processes []Process) map[int]string {
	out := make(map[int]string, len(processes))
	for _, p := range processes {
		out[p.PID] = p.Name
	}
	return out
}

// Memory is the machine's physical memory: how much is in use and how much
// there is, in bytes.
type Memory struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// GPUProcess is one process's use of the graphics card since the previous
// reading: its busiest engine, as the card's own figure is the busiest engine
// (BusiestEngine).
type GPUProcess struct {
	PID  int     `json:"pid"`
	Name string  `json:"name,omitempty"`
	Load float64 `json:"load"`
}

// NetProcess is one process's use of the network since the previous reading,
// in bytes per second. A PID of zero is what was counted and could not be
// put to a process this program may read.
type NetProcess struct {
	PID  int     `json:"pid"`
	Name string  `json:"name,omitempty"`
	Rx   float64 `json:"rx"`
	Tx   float64 `json:"tx"`
}

// NetUse is the network by process, and what the list counts: its own
// account of being partial, which the panel says under it (spec 060 R11).
type NetUse struct {
	Processes []NetProcess `json:"processes"`

	// HasRate is false for the first reading, which has nothing to
	// difference against.
	HasRate bool `json:"has_rate"`

	// Counts is what the list is made of, in a few words: "TCP only".
	Counts string `json:"counts"`
}

// NetReader reads the network by process. It keeps what it needs between
// readings, and Close lets go of anything it holds open.
type NetReader interface {
	Read(ctx context.Context) (NetUse, error)
	Close()
}

// GPUProcessReader reads the graphics card by process. via is the provider
// the card's own reading came from (Graphics.Via), so the list is of the card
// the row is for.
type GPUProcessReader interface {
	Read(ctx context.Context, via string) ([]GPUProcess, error)
}
