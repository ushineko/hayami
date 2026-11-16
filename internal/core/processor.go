package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
Processor is the processor as the CPU drawer's summary says it (spec 095): each
logical processor's load, the clock, the load average or the run queue, and
how many processes and threads there are. Each part says whether it was read,
and a reading with some of them is a reading.

Cores has one place for every processor the machine could have, so an offline
one keeps its place: its load is NoCore, as is every core's on the first
reading of a reader that has not yet taken two samples.
*/
type Processor struct {
	Cores    []float64 `json:"cores,omitempty"`
	HasCores bool      `json:"has_cores,omitempty"`

	// Clock is the online cores' average current clock and MaxClock the
	// highest a core may reach, in MHz; MaxClock is zero where it is not
	// known (Windows gives a base, which is not a maximum).
	Clock    float64 `json:"clock,omitempty"`
	MaxClock float64 `json:"max_clock,omitempty"`
	HasClock bool    `json:"has_clock,omitempty"`

	// LoadAvg is Linux's, over 1, 5 and 15 minutes; Queue is Windows's
	// processor queue length, which it has instead.
	LoadAvg    [3]float64 `json:"load_avg"`
	HasLoadAvg bool       `json:"has_load_avg,omitempty"`
	Queue      float64    `json:"queue,omitempty"`
	HasQueue   bool       `json:"has_queue,omitempty"`

	// Queues says the machine has a queue and not a load average, whether
	// or not it was read, so its line keeps its name.
	Queues bool `json:"queues,omitempty"`

	Processes int  `json:"processes,omitempty"`
	Threads   int  `json:"threads,omitempty"`
	HasTasks  bool `json:"has_tasks,omitempty"`
}

// NoCore is a core's load where there is none: offline, or not yet two
// samples. JSON has no NaN.
const NoCore = -1.0

// ProcessorTimes is one logical processor's busy and idle time since boot, in the
// source's unit, and whether it is online to be counted.
type ProcessorTimes struct {
	Busy, Idle float64
	Online     bool
}

/*
ProcessorReader reads the processor for the summary, differencing each core's
times against its previous call as CPULoad does the whole processor's: the
first call takes two samples, CPUWarmup apart, so a command that reads once
has a figure.
*/
type ProcessorReader struct {
	mu    sync.Mutex
	cores func() ([]ProcessorTimes, error)
	rest  func(context.Context) Processor
	last  []ProcessorTimes
	seen  bool
	wait  func()
}

// NewProcessorReader reads the cores through cores and everything else
// through rest; either may be nil, and its part is then not read.
func NewProcessorReader(cores func() ([]ProcessorTimes, error), rest func(context.Context) Processor) *ProcessorReader {
	return &ProcessorReader{cores: cores, rest: rest, wait: func() { time.Sleep(CPUWarmup) }}
}

// Read is the processor now; each core's load since the previous call.
func (r *ProcessorReader) Read(ctx context.Context) Processor {
	var p Processor
	if r.rest != nil {
		p = r.rest(ctx)
	}
	if r.cores == nil {
		return p
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.seen {
		if first, err := r.cores(); err == nil {
			r.last, r.seen = first, true
			r.wait()
		}
	}
	now, err := r.cores()
	if err != nil {
		return p
	}
	p.Cores, p.HasCores = LoadsBetween(r.last, now), true
	r.last, r.seen = now, true
	return p
}

// LoadsBetween is each core's load between two samples, in percent, NoCore for a
// core offline in either or with no time between them.
func LoadsBetween(was, now []ProcessorTimes) []float64 {
	out := make([]float64, len(now))
	for i, c := range now {
		out[i] = NoCore
		if i >= len(was) || !c.Online || !was[i].Online {
			continue
		}
		busy, idle := c.Busy-was[i].Busy, c.Idle-was[i].Idle
		if total := busy + idle; total > 0 && busy >= 0 && idle >= 0 {
			out[i] = 100 * busy / total
		}
	}
	return out
}

// The files Linux's processor summary reads (spec 095 R4).
const (
	CPUPossiblePath = "/sys/devices/system/cpu/possible"
	CPUDir          = "/sys/devices/system/cpu"
	LoadAvgPath     = "/proc/loadavg"
)

// statTimes is one /proc/stat cpu line's busy and idle, by procStat's rule:
// idle is idle plus iowait, guest time is already in user and nice.
func statTimes(fields []string) (busy, idle float64, err error) {
	for i, field := range fields {
		v, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("a cpu line is not what it was: %w", err)
		}
		switch {
		case i == 3 || i == 4: // idle, iowait
			idle += v
		case i >= 8: // guest, guest_nice: already counted
		default:
			busy += v
		}
	}
	return busy, idle, nil
}

/*
ProcStatCores reads each logical processor's times from /proc/stat's cpuN
lines, one place for each of possible processors: the kernel has no line for
an offline core, which keeps its place, offline. A line past possible widens
the list rather than being dropped.
*/
func ProcStatCores(statPath string, possible int) ([]ProcessorTimes, error) {
	body, err := os.ReadFile(statPath) //nolint:gosec // procfs, or a test's file
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", statPath, err)
	}
	out := make([]ProcessorTimes, max(possible, 0))
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") || fields[0] == "cpu" {
			continue
		}
		n, err := strconv.Atoi(fields[0][3:])
		if err != nil || n < 0 || n >= maxCores {
			continue
		}
		busy, idle, err := statTimes(fields[1:])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", statPath, err)
		}
		for len(out) <= n {
			out = append(out, ProcessorTimes{})
		}
		out[n] = ProcessorTimes{Busy: busy, Idle: idle, Online: true}
	}
	return out, nil
}

// maxCores bounds a core's number: more than any machine has, fewer than a
// file could ask to be allocated.
const maxCores = 4096

/*
ParseCPUList is how many processors a kernel CPU list names, "0-31" or
"0-7,10-31", as its highest number plus one: the places the cores take.
*/
func ParseCPUList(s string) (int, error) {
	highest := -1
	for part := range strings.SplitSeq(strings.TrimSpace(s), ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		if !isRange {
			hi = lo
		}
		a, err1 := strconv.Atoi(lo)
		b, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || a < 0 || b < a || b >= maxCores {
			return 0, errCPUList
		}
		highest = max(highest, b)
	}
	return highest + 1, nil
}

var errCPUList = errors.New("not a CPU list")

// possibleCores is how many places the cores take, from the kernel's list, or
// zero where it cannot be read and the stat file's lines are all there is.
func possibleCores(path string) int {
	body, err := os.ReadFile(path) //nolint:gosec // sysfs, or a test's file
	if err != nil {
		return 0
	}
	n, err := ParseCPUList(string(body))
	if err != nil {
		return 0
	}
	return n
}

/*
CPUFreq is the online cores' average current clock and their highest
maximum, in MHz, from cpufreq under dir: scaling_cur_freq and
cpuinfo_max_freq, in kHz. A core without cpufreq leaves the clock to the
rest; none at all is no clock.
*/
func CPUFreq(dir string, cores []ProcessorTimes) (clock, maxClock float64, ok bool) {
	var sum float64
	n := 0
	for i, c := range cores {
		if !c.Online {
			continue
		}
		base := filepath.Join(dir, "cpu"+strconv.Itoa(i), "cpufreq")
		cur, err := readKHz(filepath.Join(base, "scaling_cur_freq"))
		if err != nil {
			continue
		}
		sum += cur
		n++
		if m, err := readKHz(filepath.Join(base, "cpuinfo_max_freq")); err == nil {
			maxClock = max(maxClock, m)
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return sum / float64(n), maxClock, true
}

// readKHz is a cpufreq file's kHz as MHz.
func readKHz(path string) (float64, error) {
	body, err := os.ReadFile(path) //nolint:gosec // sysfs, or a test's file
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", path, err)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("%s: not a clock", path)
	}
	return v / 1000, nil
}

/*
ParseLoadAvg reads /proc/loadavg, "3.88 4.38 3.74 5/10878 2521036": the load
average over 1, 5 and 15 minutes and, after the slash, how many scheduling
entities there are, which is the threads.
*/
func ParseLoadAvg(s string) (avg [3]float64, threads int, err error) {
	f := strings.Fields(s)
	if len(f) < 4 {
		return avg, 0, errLoadAvg
	}
	for i := range 3 {
		if avg[i], err = strconv.ParseFloat(f[i], 64); err != nil || avg[i] < 0 {
			return avg, 0, errLoadAvg
		}
	}
	_, total, ok := strings.Cut(f[3], "/")
	if threads, err = strconv.Atoi(total); !ok || err != nil || threads < 0 {
		return avg, 0, errLoadAvg
	}
	return avg, threads, nil
}

var errLoadAvg = errors.New("not a load average")

// countProcesses is how many processes there are: the numbered entries under
// proc.
func countProcesses(proc string) (int, error) {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", proc, err)
	}
	n := 0
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			n++
		}
	}
	return n, nil
}

// ProcessorPaths are where Linux's processor summary reads, so a test can
// point them at its own files.
type ProcessorPaths struct {
	Stat, Possible, CPUDir, LoadAvg, Proc string
}

// LinuxProcessor is Linux's processor reader over paths (spec 095 R4).
func LinuxProcessor(paths ProcessorPaths) *ProcessorReader {
	var mu sync.Mutex
	var cores []ProcessorTimes
	read := func() ([]ProcessorTimes, error) {
		c, err := ProcStatCores(paths.Stat, possibleCores(paths.Possible))
		if err == nil {
			mu.Lock()
			cores = c
			mu.Unlock()
		}
		return c, err
	}
	rest := func(context.Context) Processor {
		var p Processor
		// The clock is asked of the cores the last sample found online; the
		// first reading takes a sample of its own.
		mu.Lock()
		online := cores
		mu.Unlock()
		if online == nil {
			online, _ = ProcStatCores(paths.Stat, possibleCores(paths.Possible))
		}
		p.Clock, p.MaxClock, p.HasClock = CPUFreq(paths.CPUDir, online)
		if body, err := os.ReadFile(paths.LoadAvg); err == nil {
			if avg, threads, err := ParseLoadAvg(string(body)); err == nil {
				p.LoadAvg, p.HasLoadAvg = avg, true
				if procs, err := countProcesses(paths.Proc); err == nil {
					p.Processes, p.Threads, p.HasTasks = procs, threads, true
				}
			}
		}
		return p
	}
	return NewProcessorReader(read, rest)
}

/*
ParseProcessorTimes reads NtQuerySystemInformation's
SystemProcessorPerformanceInformation: one 48-byte entry per processor, its
idle, kernel and user time first, in 100 ns ticks. Kernel time includes
idle, as GetSystemTimes's does, so busy is kernel plus user less idle. It
reads the bytes so that it runs, and is tested and fuzzed, everywhere.
*/
func ParseProcessorTimes(buf []byte) ([]ProcessorTimes, error) {
	const entry = 48
	if len(buf)%entry != 0 || len(buf)/entry > maxCores {
		return nil, errProcessorTimes
	}
	out := make([]ProcessorTimes, 0, len(buf)/entry)
	for at := 0; at < len(buf); at += entry {
		idle := float64(int64(binary.LittleEndian.Uint64(buf[at:])))     //nolint:gosec // LARGE_INTEGER
		kernel := float64(int64(binary.LittleEndian.Uint64(buf[at+8:]))) //nolint:gosec // LARGE_INTEGER
		user := float64(int64(binary.LittleEndian.Uint64(buf[at+16:])))  //nolint:gosec // LARGE_INTEGER
		if idle < 0 || kernel < idle || user < 0 {
			return nil, errProcessorTimes
		}
		out = append(out, ProcessorTimes{Busy: kernel + user - idle, Idle: idle, Online: true})
	}
	return out, nil
}

var errProcessorTimes = errors.New("the processor times do not add up")
