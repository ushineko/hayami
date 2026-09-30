package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku/hwmon"
)

// ProcStatPath is where the kernel keeps the processor's time counters.
const ProcStatPath = "/proc/stat"

/*
CPULoad is the processor's utilisation, from /proc/stat. hotaru's
readings.CPU, which is the source of record for this reading.

The kernel counts jiffies since boot, so utilisation is a rate and needs two
samples to exist at all. Reporting the since-boot average instead would be a
number that is always true and never useful: a machine up for a week reads
4 % while it compiles.

**The first call takes both samples**, CPUWarmup apart. Without it the first
poll has no load, and a command that polls once -- `--once`, `doctor`,
`readings` -- would never say one. 200 ms is twenty jiffies at the usual
100 Hz, enough for a figure, and short enough that a one-shot command does not
feel slower for it. Later calls difference against the previous one.
*/
type CPULoad struct {
	mu                 sync.Mutex
	path               string
	lastBusy, lastIdle float64
	seen               bool

	// wait is the pause between the first call's two samples. A test
	// replaces it to change the file between them rather than sleep.
	wait func()
}

// CPUWarmup is how far apart the first call's two samples are.
const CPUWarmup = 200 * time.Millisecond

// NewCPULoad reads the counters at path: ProcStatPath, or a test's file.
func NewCPULoad(path string) *CPULoad {
	return &CPULoad{path: path, wait: func() { time.Sleep(CPUWarmup) }}
}

// Load is the percentage busy since the previous call, and whether there is
// one. Between two cooler polls that is a mean over five seconds, which is
// what a panel read at a glance wants: an instant flickers between 3 and 100
// on an idle machine. The first call's is over CPUWarmup.
func (c *CPULoad) Load() (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.seen {
		busy, idle, err := procStat(c.path)
		if err != nil {
			return 0, false
		}
		c.lastBusy, c.lastIdle, c.seen = busy, idle, true
		c.wait()
	}

	busy, idle, err := procStat(c.path)
	if err != nil {
		return 0, false
	}
	was, wasIdle := c.lastBusy, c.lastIdle
	c.lastBusy, c.lastIdle = busy, idle
	total := (busy - was) + (idle - wasIdle)
	if total <= 0 {
		// Two samples inside one jiffy. Nothing to say, and not an error.
		return 0, false
	}
	return 100 * (busy - was) / total, true
}

/*
procStat reads the aggregate line.

	cpu  user nice system idle iowait irq softirq steal guest guest_nice

Idle is idle plus iowait: a processor waiting on a disk is not working, and
counting iowait as busy reads 100 % through a large copy on a sleeping machine.
Guest time is already included in user and nice, so it is not added again.
*/
func procStat(path string) (busy, idle float64, err error) {
	body, err := os.ReadFile(path) //nolint:gosec // procfs, or a test's file
	if err != nil {
		return 0, 0, fmt.Errorf("reading %s: %w", path, err)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		for i, field := range fields[1:] {
			v, err := strconv.ParseFloat(field, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("%s is not what it was: %w", path, err)
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
	return 0, 0, fmt.Errorf("no aggregate line in %s", path)
}

// BusyGlob is where AMD's driver puts the card's utilisation. A pattern and
// not a card index, because the numbers move between boots.
const BusyGlob = "/sys/class/drm/card*/device/gpu_busy_percent"

// SMIFlag is what nvidia-smi is asked for, in the order ParseSMI expects. One
// constant flag rather than a query put together: an argument the subprocess
// checker can see is not anybody's input.
const SMIFlag = "--query-gpu=temperature.gpu,utilization.gpu"

// SMITimeout bounds the nvidia-smi call. A sensor read that hangs must not
// hold up a poll, let alone the shutdown it would be blocking.
const SMITimeout = 2 * time.Second

// Graphics is what the graphics card said. Either number can be missing, and
// a machine with no card this build can read reports neither.
type Graphics struct {
	Temperature    float64 `json:"temperature"`
	HasTemperature bool    `json:"has_temperature"`
	Load           float64 `json:"load"`
	HasLoad        bool    `json:"has_load"`
}

/*
GraphicsReader reads the card by whichever route the machine has: hwmon and
the AMD busy file where the kernel has a driver, nvidia-smi where it does not.

**nvidia-smi is the one subprocess the direct-access rule allows here.**
Devices are read directly, through sanshoku, and no subprocess reads one; but
NVIDIA's proprietary driver registers no hwmon and no busy file, and what it
does offer is NVML, a vendor library and not a kernel node. Reaching it from
Go is cgo, which the terminal panel is built without. hotaru has made the same
call at a two-second cadence for months; here it is every five.

Each field is replaceable, so a test reads a tree it built and runs no tool.
*/
type GraphicsReader struct {
	// Root is the hwmon tree: hwmon.Root, or a test's.
	Root string
	// Busy is the pattern the AMD busy file is looked for under.
	Busy string
	// SMI runs nvidia-smi and returns what it printed.
	SMI func(context.Context) (string, error)
}

// NewGraphicsReader reads this machine's card.
func NewGraphicsReader() *GraphicsReader {
	return &GraphicsReader{Root: hwmon.Root, Busy: BusyGlob, SMI: NvidiaSMI}
}

// Read is the card's temperature and load. The kernel first; nvidia-smi only
// for what the kernel did not say, and once for both numbers. Nothing here is
// an error: a card that cannot be read is absent, and absence is the answer.
func (g *GraphicsReader) Read(ctx context.Context) Graphics {
	var out Graphics
	if _, t, err := hwmon.First(g.Root, hwmon.GPU); err == nil {
		out.Temperature, out.HasTemperature = t, true
	}
	if load, ok := busyIn(g.Busy); ok {
		out.Load, out.HasLoad = load, true
	}
	if (out.HasTemperature && out.HasLoad) || g.SMI == nil {
		return out
	}

	text, err := g.SMI(ctx)
	if err != nil {
		return out
	}
	t, load, gotTemp, gotLoad := ParseSMI(text)
	if !out.HasTemperature && gotTemp {
		out.Temperature, out.HasTemperature = t, true
	}
	if !out.HasLoad && gotLoad {
		out.Load, out.HasLoad = load, true
	}
	return out
}

// NvidiaSMI asks nvidia-smi for the temperature and the utilisation, within
// SMITimeout. A machine without it is an error from exec, which Read takes as
// absence.
func NvidiaSMI(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, SMITimeout)
	defer cancel()
	//nolint:gosec // every argument is a constant; nothing here is input
	out, err := exec.CommandContext(ctx, "nvidia-smi", SMIFlag, "--format=csv,noheader,nounits").Output()
	if err != nil {
		return "", fmt.Errorf("running nvidia-smi: %w", err)
	}
	return string(out), nil
}

// busyIn is the first readable busy file matching pattern.
func busyIn(pattern string) (float64, bool) {
	if pattern == "" {
		return 0, false
	}
	found, err := filepath.Glob(pattern)
	if err != nil {
		return 0, false
	}
	for _, path := range found {
		body, err := os.ReadFile(path) //nolint:gosec // sysfs, or a test's file
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

/*
ParseSMI reads the first line of nvidia-smi's CSV. hotaru's, which is the
source of record.

	41, 4

Temperature first, utilisation second, in the order SMIFlag asks. A unit, when
one travels with the number (" 4 %" without nounits), is dropped. A field that
does not parse is absence rather than an error: a format is not an API, and
this one has changed before.
*/
func ParseSMI(out string) (temperature, load float64, gotTemp, gotLoad bool) {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	parts := strings.Split(line, ",")
	if v, err := strconv.ParseFloat(leading(parts[0]), 64); err == nil {
		temperature, gotTemp = v, true
	}
	if len(parts) > 1 {
		if v, err := strconv.ParseFloat(leading(parts[1]), 64); err == nil {
			load, gotLoad = v, true
		}
	}
	return temperature, load, gotTemp, gotLoad
}

// leading is the first figure in a field like " 3 %".
func leading(field string) string {
	fields := strings.Fields(field)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
