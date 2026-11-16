package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
Disks is the machine's disk IO as the IO row and the IO drawer's summary say it
(spec 097): the disks' read and write rates, each disk's busy share, the IO
pressure or the disks' queue, and the rate swapped in and out. Each part says
whether it was read.
*/
type Disks struct {
	// Read and Write are bytes per second across the disks, between the
	// block layer and the hardware: iotop's Current DISK READ and WRITE.
	Read     float64 `json:"read"`
	Write    float64 `json:"write"`
	HasRates bool    `json:"has_rates,omitempty"`

	// Names are the disks and Busy each one's share of the time it was doing
	// IO, from 0 to 1, NoCore where it has no figure yet.
	Names   []string  `json:"names,omitempty"`
	Busy    []float64 `json:"busy,omitempty"`
	HasBusy bool      `json:"has_busy,omitempty"`

	// Pressure is Linux's IO pressure, the share of the last ten seconds in
	// which some task waited on IO, in percent; Queue is Windows's average
	// disk queue length, which it has instead. Queues says which the machine
	// has, whether or not it was read.
	Pressure    float64 `json:"pressure,omitempty"`
	HasPressure bool    `json:"has_pressure,omitempty"`
	Queue       float64 `json:"queue,omitempty"`
	HasQueue    bool    `json:"has_queue,omitempty"`
	Queues      bool    `json:"queues,omitempty"`

	// SwapIn and SwapOut are bytes per second.
	SwapIn  float64 `json:"swap_in,omitempty"`
	SwapOut float64 `json:"swap_out,omitempty"`
	HasSwap bool    `json:"has_swap,omitempty"`

	// AllIO says a process's IO here is every transfer, files, network and
	// devices, and not the disk's alone (Windows, spec 097 D2), so the list
	// is not a share of these rates.
	AllIO bool `json:"all_io,omitempty"`
}

// DiskCounter is one disk's counts since boot: sectors read and written, and
// the milliseconds it spent doing IO.
type DiskCounter struct {
	Name                        string
	SectorsRead, SectorsWritten float64
	IOMillis                    float64
}

// diskSample is one reading of every counter a rate is made from.
type diskSample struct {
	disks           []DiskCounter
	swapIn, swapOut float64
	hasSwap         bool
	at              time.Time
}

// DiskReader differences the disks' counters against its previous call, as
// CPULoad does the processor's: the first call takes two samples, CPUWarmup
// apart.
type DiskReader struct {
	mu     sync.Mutex
	sample func() (diskSample, error)
	rest   func(context.Context) Disks
	last   diskSample
	seen   bool
	now    func() time.Time
	wait   func()
}

// Read is the disks now: the rates and busy shares since the previous call,
// and what rest reads as it stands.
func (r *DiskReader) Read(ctx context.Context) Disks {
	var d Disks
	if r.rest != nil {
		d = r.rest(ctx)
	}
	if r.sample == nil {
		return d
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.seen {
		if first, err := r.sample(); err == nil {
			first.at = r.now()
			r.last, r.seen = first, true
			r.wait()
		}
	}
	now, err := r.sample()
	if err != nil {
		return d
	}
	now.at = r.now()
	diskRates(&d, r.last, now)
	r.last, r.seen = now, true
	return d
}

// sectorBytes is the unit diskstats counts in, whatever the disk's own
// sector: the kernel's 512.
const sectorBytes = 512

// pageBytes is the unit vmstat's swap counts are in.
var pageBytes = float64(os.Getpagesize())

// diskRates fills d's rates and busy shares from two samples. A disk new in
// the second has no share yet; a counter that went backwards is another disk
// under an old name, and has none either.
func diskRates(d *Disks, was, now diskSample) {
	seconds := now.at.Sub(was.at).Seconds()
	if seconds <= 0 {
		return
	}
	before := make(map[string]DiskCounter, len(was.disks))
	for _, c := range was.disks {
		before[c.Name] = c
	}
	d.Names, d.Busy = make([]string, len(now.disks)), make([]float64, len(now.disks))
	for i, c := range now.disks {
		d.Names[i], d.Busy[i] = c.Name, NoCore
		b, ok := before[c.Name]
		if !ok || c.SectorsRead < b.SectorsRead || c.SectorsWritten < b.SectorsWritten || c.IOMillis < b.IOMillis {
			continue
		}
		d.Read += (c.SectorsRead - b.SectorsRead) * sectorBytes / seconds
		d.Write += (c.SectorsWritten - b.SectorsWritten) * sectorBytes / seconds
		d.Busy[i] = min((c.IOMillis-b.IOMillis)/(seconds*1000), 1)
	}
	d.HasRates, d.HasBusy = true, true
	if now.hasSwap && was.hasSwap && now.swapIn >= was.swapIn && now.swapOut >= was.swapOut {
		d.SwapIn = (now.swapIn - was.swapIn) * pageBytes / seconds
		d.SwapOut = (now.swapOut - was.swapOut) * pageBytes / seconds
		d.HasSwap = true
	}
}

// The files Linux's disk reader reads (spec 097 R5).
const (
	DiskStatsPath  = "/proc/diskstats"
	VMStatPath     = "/proc/vmstat"
	IOPressurePath = "/proc/pressure/io"
	SysBlockPath   = "/sys/block"
)

// virtualDisks are the block devices that are not a disk, or that count
// another disk's IO again: loop files, RAM disks, compressed swap in RAM,
// device-mapper and software RAID over disks already counted, optical
// drives.
var virtualDisks = []string{"loop", "ram", "zram", "dm-", "md", "sr", "fd", "nbd"}

/*
ParseDiskStats reads /proc/diskstats for the whole disks: those whole is a
name under sysBlock says (a partition is not), less the virtual ones. Each
line is

	major minor name reads merged sectors ms writes merged sectors ms in_flight io_ms ...

so the sectors read are the sixth field, the sectors written the tenth and the
milliseconds doing IO the thirteenth.
*/
func ParseDiskStats(body string, whole func(name string) bool) []DiskCounter {
	var out []DiskCounter
	for line := range strings.SplitSeq(body, "\n") {
		f := strings.Fields(line)
		if len(f) < 13 {
			continue
		}
		name := f[2]
		if isVirtualDisk(name) || !whole(name) {
			continue
		}
		read, err1 := strconv.ParseFloat(f[5], 64)
		written, err2 := strconv.ParseFloat(f[9], 64)
		io, err3 := strconv.ParseFloat(f[12], 64)
		if err1 != nil || err2 != nil || err3 != nil || read < 0 || written < 0 || io < 0 {
			continue
		}
		out = append(out, DiskCounter{Name: name, SectorsRead: read, SectorsWritten: written, IOMillis: io})
	}
	return out
}

func isVirtualDisk(name string) bool {
	for _, prefix := range virtualDisks {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ParseVMStatSwap reads vmstat's pswpin and pswpout, pages swapped in and out
// since boot.
func ParseVMStatSwap(body string) (in, out float64, ok bool) {
	var gotIn, gotOut bool
	for line := range strings.SplitSeq(body, "\n") {
		key, value, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || v < 0 {
			continue
		}
		switch key {
		case "pswpin":
			in, gotIn = v, true
		case "pswpout":
			out, gotOut = v, true
		}
	}
	return in, out, gotIn && gotOut
}

/*
ParsePressure reads a PSI file's "some" line for its ten-second average, in
percent:

	some avg10=1.23 avg60=0.50 avg300=0.10 total=1294206172
*/
func ParsePressure(body string) (float64, error) {
	for line := range strings.SplitSeq(body, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "some" {
			continue
		}
		v, found := strings.CutPrefix(f[1], "avg10=")
		if !found {
			break
		}
		p, err := strconv.ParseFloat(v, 64)
		if err != nil || p < 0 || p > 100 {
			break
		}
		return p, nil
	}
	return 0, errPressure
}

var errPressure = errors.New("no IO pressure")

// DiskPaths are where Linux's disk reader reads, so a test can point them at
// its own files.
type DiskPaths struct {
	DiskStats, VMStat, Pressure, SysBlock string
}

// LinuxDisks is Linux's disk reader over paths (spec 097 R5).
func LinuxDisks(paths DiskPaths) *DiskReader {
	whole := func(name string) bool {
		_, err := os.Stat(filepath.Join(paths.SysBlock, name))
		return err == nil
	}
	sample := func() (diskSample, error) {
		body, err := os.ReadFile(paths.DiskStats)
		if err != nil {
			return diskSample{}, fmt.Errorf("reading %s: %w", paths.DiskStats, err)
		}
		s := diskSample{disks: ParseDiskStats(string(body), whole)}
		if vm, err := os.ReadFile(paths.VMStat); err == nil {
			s.swapIn, s.swapOut, s.hasSwap = ParseVMStatSwap(string(vm))
		}
		return s, nil
	}
	rest := func(context.Context) Disks {
		var d Disks
		if body, err := os.ReadFile(paths.Pressure); err == nil {
			if p, err := ParsePressure(string(body)); err == nil {
				d.Pressure, d.HasPressure = p, true
			}
		}
		return d
	}
	return &DiskReader{sample: sample, rest: rest, now: time.Now, wait: func() { time.Sleep(CPUWarmup) }}
}

// pdhTotal is the instance the PhysicalDisk counters sum every disk under.
const pdhTotal = "_Total"

/*
WindowsDisks is the disks from the PhysicalDisk and Memory counters' values,
by instance (spec 097 R6): the rates and the queue from _Total; each disk's
busy share from its idle time, 100 less it being Task Manager's active time,
in the instances' name order; the pages swapped in and out as bytes. It is
plain arithmetic, so it runs and is tested everywhere.
*/
func WindowsDisks(read, write, idle, queue, pagesIn, pagesOut map[string]float64, pageSize float64) Disks {
	d := Disks{Queues: true, AllIO: true}
	r, okR := read[pdhTotal]
	w, okW := write[pdhTotal]
	if okR && okW && r >= 0 && w >= 0 {
		d.Read, d.Write, d.HasRates = r, w, true
	}
	if q, ok := queue[pdhTotal]; ok && q >= 0 {
		d.Queue, d.HasQueue = q, true
	}
	for name := range idle {
		if name != pdhTotal {
			d.Names = append(d.Names, name)
		}
	}
	slices.Sort(d.Names)
	d.Busy = make([]float64, len(d.Names))
	for i, name := range d.Names {
		d.Busy[i] = min(max(1-idle[name]/100, 0), 1)
	}
	d.HasBusy = len(d.Names) > 0
	in, okIn := single(pagesIn)
	out, okOut := single(pagesOut)
	if okIn && okOut && in >= 0 && out >= 0 {
		d.SwapIn, d.SwapOut, d.HasSwap = in*pageSize, out*pageSize, true
	}
	return d
}

// single is a single-instance counter's value, and whether it has one.
func single(m map[string]float64) (float64, bool) {
	for _, v := range m {
		return v, true
	}
	return 0, false
}

// NewDiskReader is a reader whose every figure rest reads as it stands, with
// no counters of its own to difference: Windows's, whose counters are rates
// already, or a test's.
func NewDiskReader(rest func(context.Context) Disks) *DiskReader {
	return &DiskReader{rest: rest}
}
