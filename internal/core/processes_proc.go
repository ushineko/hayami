package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/*
The Linux process and memory readers, over a /proc a test can build. They have
no build tag: they read files, and a test on any system reads a tree it wrote.
*/

// ProcPath is where the kernel lists its processes.
const ProcPath = "/proc"

// userHZ is the unit /proc/<pid>/stat counts processor time in. It is the
// kernel's USER_HZ, which is 100 on every architecture Linux runs on, and is
// not the kernel's own tick rate.
const userHZ = 100

/*
ProcProcesses reads every process under root: /proc/<pid>/stat for the name
and the processor time, /proc/<pid>/statm for the resident memory. Both are
readable for every process whoever owns it. A process that goes between the
listing and the read is left out.
*/
func ProcProcesses(root string) func(context.Context) ([]ProcessSample, error) {
	page := uint64(os.Getpagesize()) //nolint:gosec // a page size is positive
	return func(ctx context.Context) ([]ProcessSample, error) {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", root, err)
		}
		var out []ProcessSample
		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil || !e.IsDir() {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err //nolint:wrapcheck // the context's own message is the whole story
			}
			stat, err := os.ReadFile(filepath.Join(root, e.Name(), "stat")) //nolint:gosec // procfs, or a test's tree
			if err != nil {
				continue
			}
			name, seconds, ok := ParseProcStat(string(stat))
			if !ok {
				continue
			}
			s := ProcessSample{PID: pid, Name: name, CPUSeconds: seconds}
			if statm, err := os.ReadFile(filepath.Join(root, e.Name(), "statm")); err == nil { //nolint:gosec // as above
				s.Memory = parseStatm(string(statm)) * page
			}
			out = append(out, s)
		}
		return out, nil
	}
}

/*
ParseProcStat reads a process's name and processor time out of its stat line:

	1234 (some name) S 1 1234 ... utime stime ...

The name is in brackets and may itself hold brackets and spaces, so it ends
at the last closing bracket and the fields are counted from there. utime and
stime are the fourteenth and fifteenth fields of the line.
*/
func ParseProcStat(line string) (name string, seconds float64, ok bool) {
	open, shut := strings.IndexByte(line, '('), strings.LastIndexByte(line, ')')
	if open < 0 || shut < open {
		return "", 0, false
	}
	name = line[open+1 : shut]
	fields := strings.Fields(line[shut+1:])
	// fields[0] is the state, the third field of the line.
	const utime, stime = 14 - 3, 15 - 3
	if len(fields) <= stime {
		return "", 0, false
	}
	u, err1 := strconv.ParseFloat(fields[utime], 64)
	s, err2 := strconv.ParseFloat(fields[stime], 64)
	if err1 != nil || err2 != nil {
		return "", 0, false
	}
	return name, (u + s) / userHZ, true
}

// parseStatm is the resident set, in pages: the second field of statm.
func parseStatm(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	pages, _ := strconv.ParseUint(fields[1], 10, 64)
	return pages
}

// MemInfoPath is where the kernel says how much memory there is.
const MemInfoPath = "/proc/meminfo"

/*
ProcMemory reads the memory in use from a meminfo file: MemTotal less
MemAvailable, which is the kernel's own estimate of what a new program could
have and so counts the page cache as free. MemFree would read a machine with
a warm cache as full.
*/
func ProcMemory(path string) (Memory, error) {
	body, err := os.ReadFile(path) //nolint:gosec // procfs, or a test's file
	if err != nil {
		return Memory{}, fmt.Errorf("reading %s: %w", path, err)
	}
	fields := map[string]uint64{}
	for line := range strings.SplitSeq(string(body), "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		if v, err := strconv.ParseUint(parts[0], 10, 64); err == nil {
			fields[key] = v * 1024
		}
	}
	total, ok1 := fields["MemTotal"]
	available, ok2 := fields["MemAvailable"]
	if !ok1 || !ok2 || available > total {
		return Memory{}, fmt.Errorf("%s: %w", path, errNoMemory)
	}
	return Memory{Used: total - available, Total: total}, nil
}

var errNoMemory = errors.New("no MemTotal and MemAvailable")
