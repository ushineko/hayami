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
)

/*
The graphics card by process (spec 060 R10). Windows has the GPU Engine
counters (engines.go); Linux has DRM fdinfo for the drivers that publish it,
and nvidia-smi for the one that does not. The readers here are files and
text, with no build tag, so a test on any system reads what it wrote.
*/

/*
FDInfoGPU reads each process's use of a card from DRM fdinfo: a file a
process holds open on /dev/dri has, in /proc/<pid>/fdinfo, a line per engine
with the time that client has had on it.

	drm-driver:     i915
	drm-client-id:  42
	drm-engine-render:      123456789 ns

It is a time, not a share, so a figure needs two readings; the first call
takes both, CPUWarmup apart, as CPULoad's does. A client is one open of the
device however many descriptors and processes hold it, so it is counted once,
for the first process found with it.

Only this user's processes can be read: another user's fd directory is
closed. They are left out, not reported as idle.
*/
type FDInfoGPU struct {
	// Root is /proc, or a test's tree.
	Root string

	mu     sync.Mutex
	last   map[engineKey]uint64
	lastAt time.Time

	now  func() time.Time
	wait func()
}

// NewFDInfoGPU reads the tree at root.
func NewFDInfoGPU(root string) *FDInfoGPU {
	return &FDInfoGPU{Root: root, now: time.Now, wait: func() { time.Sleep(CPUWarmup) }}
}

// engineKey is one engine of one client of one driver.
type engineKey struct {
	driver, client, engine string
}

// driDevices is where a DRM device node lives. A descriptor open on anything
// else has no fdinfo worth reading, and a process has thousands.
const driDevices = "/dev/dri/"

// Read is each process's busiest engine since the previous call, in percent.
// It is errNothing where no process this user may read holds a card whose
// driver publishes its engines.
func (f *FDInfoGPU) Read(ctx context.Context) ([]GPUProcess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.last == nil {
		first, _, err := f.sample(ctx)
		if err != nil {
			return nil, err
		}
		f.last, f.lastAt = first, f.now()
		f.wait()
	}
	now, owners, err := f.sample(ctx)
	if err != nil {
		return nil, err
	}
	at := f.now()
	elapsed := float64(at.Sub(f.lastAt).Nanoseconds())

	// Each process's engines, summed over the clients it owns.
	byProcess := map[int]map[string]float64{}
	for key, ns := range now {
		pid := owners[key.driver+"/"+key.client]
		if byProcess[pid] == nil {
			byProcess[pid] = map[string]float64{}
		}
		if was, ok := f.last[key]; ok && elapsed > 0 && ns >= was {
			byProcess[pid][key.driver+"/"+key.engine] += 100 * float64(ns-was) / elapsed
		}
	}
	f.last, f.lastAt = now, at

	if len(byProcess) == 0 {
		return nil, errNothing
	}
	out := make([]GPUProcess, 0, len(byProcess))
	for pid, engines := range byProcess {
		busiest := 0.0
		for _, v := range engines {
			busiest = max(busiest, min(v, 100))
		}
		out = append(out, GPUProcess{PID: pid, Load: busiest})
	}
	return out, nil
}

// sample is every engine's time so far, and which process each client is
// counted for.
func (f *FDInfoGPU) sample(ctx context.Context) (map[engineKey]uint64, map[string]int, error) {
	entries, err := os.ReadDir(f.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("listing %s: %w", f.Root, err)
	}
	engines := map[engineKey]uint64{}
	owners := map[string]int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err //nolint:wrapcheck // the context's own message is the whole story
		}
		fds, err := os.ReadDir(filepath.Join(f.Root, e.Name(), "fd"))
		if err != nil {
			continue // another user's process, or one that has gone
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(f.Root, e.Name(), "fd", fd.Name()))
			if err != nil || !strings.HasPrefix(filepath.ToSlash(target), driDevices) {
				continue
			}
			body, err := os.ReadFile(filepath.Join(f.Root, e.Name(), "fdinfo", fd.Name())) //nolint:gosec // procfs, or a test's tree
			if err != nil {
				continue
			}
			driver, client, times := ParseFDInfo(string(body))
			if client == "" || len(times) == 0 {
				continue
			}
			id := driver + "/" + client
			if _, seen := owners[id]; seen {
				continue
			}
			owners[id] = pid
			for engine, ns := range times {
				engines[engineKey{driver, client, engine}] = ns
			}
		}
	}
	return engines, owners, nil
}

// ParseFDInfo reads a DRM client out of an fdinfo file: its driver, its
// client ID, and the nanoseconds each engine has given it. A file with no
// client ID is not a DRM client.
func ParseFDInfo(body string) (driver, client string, engines map[string]uint64) {
	engines = map[string]uint64{}
	for line := range strings.SplitSeq(body, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch {
		case key == "drm-driver":
			driver = value
		case key == "drm-client-id":
			client = value
		case strings.HasPrefix(key, "drm-engine-") && !strings.HasPrefix(key, "drm-engine-capacity-"):
			number, unit, _ := strings.Cut(value, " ")
			if ns, err := strconv.ParseUint(number, 10, 64); err == nil && unit == "ns" {
				engines[strings.TrimPrefix(key, "drm-engine-")] = ns
			}
		}
	}
	return driver, client, engines
}

// pmonName is what the per-process form of nvidia-smi is called in a reason.
const pmonName = "nvidia-smi pmon"

// fdinfoName is what DRM fdinfo is called in a reason.
const fdinfoName = "DRM fdinfo"

// NvidiaPMON asks nvidia-smi for one sample of each process's use of the
// card, within SMITimeout. It is the second use of the one subprocess rule 9
// allows, and is run only while the list is shown.
func NvidiaPMON(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, SMITimeout)
	defer cancel()
	//nolint:gosec // every argument is a constant; nothing here is input
	out, err := exec.CommandContext(ctx, "nvidia-smi", "pmon", "-c", "1", "-s", "u").Output()
	if err != nil {
		return "", fmt.Errorf("running nvidia-smi pmon: %w", err)
	}
	return string(out), nil
}

/*
ParsePMON reads nvidia-smi pmon's table:

	# gpu         pid   type     sm    mem    enc    dec    jpg    ofa    command
	# Idx           #    C/G      %      %      %      %      %      %    name
	    0       1234     G      3      1      -      -      -      -    Xorg

The fourth column is the share of the card's compute the process had, and "-"
is a process that holds the card and did nothing in the sample. The command
is its name; a process is listed once per card and its figures summed here.
*/
func ParsePMON(out string) []GPUProcess {
	by := map[int]*GPUProcess{}
	var order []int
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue // "-": a card with no process on it
		}
		load, _ := strconv.ParseFloat(fields[3], 64)
		p, ok := by[pid]
		if !ok {
			p = &GPUProcess{PID: pid, Name: fields[len(fields)-1]}
			by[pid] = p
			order = append(order, pid)
		}
		p.Load = min(p.Load+load, 100)
	}
	result := make([]GPUProcess, 0, len(order))
	for _, pid := range order {
		result = append(result, *by[pid])
	}
	return result
}

// pmonProcesses is NvidiaPMON read into processes, or errNothing where it
// listed none.
func pmonProcesses(run func(context.Context) (string, error)) func(context.Context) ([]GPUProcess, error) {
	return func(ctx context.Context) ([]GPUProcess, error) {
		text, err := run(ctx)
		if err != nil {
			return nil, err
		}
		found := ParsePMON(text)
		if len(found) == 0 {
			return nil, errNothing
		}
		return found, nil
	}
}

/*
gpuProcesses is a platform's graphics card by process: a chain of providers
chosen for the card the row is about, and the process table to name them by.

A provider knows a process by its number. The name is the process table's, so
every list names a program the same way, and a process the table does not
have keeps the name its provider gave or is left out.
*/
type gpuProcesses struct {
	chain   func(via string) Chain[[]GPUProcess]
	samples func(context.Context) ([]ProcessSample, error)
}

// Read is the card's processes, named, or the absence naming every provider
// tried.
func (g *gpuProcesses) Read(ctx context.Context, via string) ([]GPUProcess, error) {
	chain := g.chain(via)
	o := chain.Read(ctx)
	if !o.Answered {
		if o.Absence != nil {
			return nil, o.Absence
		}
		return nil, &Absence{Code: AbsenceGPUProcesses, Text: "not available",
			Detail: gpuProcessesMissing(o.Tried), Err: o.Err}
	}
	names := map[int]string{}
	if samples, err := g.samples(ctx); err == nil {
		for _, s := range samples {
			names[s.PID] = s.Name
		}
	}
	out := make([]GPUProcess, 0, len(o.Value))
	for _, p := range o.Value {
		if name, ok := names[p.PID]; ok {
			p.Name = name
		}
		if p.Name == "" {
			continue // gone between the two readings
		}
		out = append(out, p)
	}
	return out, nil
}

// gpuProcessesMissing is the account of no list: every provider tried.
func gpuProcessesMissing(tried []string) string {
	if len(tried) == 0 {
		return "this build reads no graphics card by process on this system"
	}
	return "tried " + strings.Join(tried, ", ") + "; none listed a process"
}
