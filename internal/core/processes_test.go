package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statLine is a /proc/<pid>/stat line for a process named name with the
// given user and kernel ticks.
func statLine(pid int, name string, utime, stime int) string {
	return strconv.Itoa(pid) + " (" + name + ") S 1 2 3 4 5 6 7 8 9 10 " +
		strconv.Itoa(utime) + " " + strconv.Itoa(stime) + " 0 0 20 0 1 0 100 1000 200\n"
}

// procTree writes a /proc of the processes given, each with the files the
// readers open, and returns its root.
func procTree(t *testing.T, stats map[int]string) string {
	t.Helper()
	root := t.TempDir()
	for pid, stat := range stats {
		dir := filepath.Join(root, strconv.Itoa(pid))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o600))
	}
	return root
}

// Spec 060 R7. A process's name ends at the last bracket, whatever it holds,
// and its time is the fourteenth and fifteenth fields in seconds.
func TestAStatLineGivesTheNameAndTheProcessorTime(t *testing.T) {
	name, seconds, ok := ParseProcStat(statLine(42, "Web Content (2) x", 250, 50))
	require.True(t, ok)
	assert.Equal(t, "Web Content (2) x", name)
	assert.InDelta(t, 3.0, seconds, 1e-9)

	_, _, ok = ParseProcStat("42 (short) S 1 2 3")
	assert.False(t, ok, "a line cut short is not read as a process with no time")
	_, _, ok = ParseProcStat("nothing")
	assert.False(t, ok)
}

// R6. A process is read as its name, its time and its memory and nothing
// else: a tree holding a command line, an environment and an owner beside
// each stat file gives up none of them.
func TestAProcessIsReadAsItsNameAndNothingElseAboutIt(t *testing.T) {
	root := procTree(t, map[int]string{
		7:  statLine(7, "editor", 100, 0),
		19: statLine(19, "sync", 20, 5),
	})
	secrets := []string{"/home/someone/private.txt", "--token=hunter2", "PASSWORD=hunter2", "Uid:\t1000"}
	for _, pid := range []string{"7", "19"} {
		for i, file := range []string{"cmdline", "exe", "environ", "status"} {
			require.NoError(t, os.WriteFile(filepath.Join(root, pid, file), []byte(secrets[i]), 0o600))
		}
		require.NoError(t, os.WriteFile(filepath.Join(root, pid, "statm"), []byte("5000 300 100 1 0 200 0\n"), 0o600))
	}
	// Not processes: the kernel's own files, and a directory that is not a
	// number.
	require.NoError(t, os.WriteFile(filepath.Join(root, "uptime"), []byte("1 1\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "self"), 0o755))

	got, err := ProcProcesses(root)(t.Context())
	require.NoError(t, err)
	sort.Slice(got, func(i, j int) bool { return got[i].PID < got[j].PID })

	page := uint64(os.Getpagesize()) //nolint:gosec // positive
	assert.Equal(t, []ProcessSample{
		{PID: 7, Name: "editor", CPUSeconds: 1, Memory: 300 * page},
		{PID: 19, Name: "sync", CPUSeconds: 0.25, Memory: 300 * page},
	}, got)
}

// A process that went between the listing and the read is left out.
func TestAProcessThatHasGoneIsLeftOut(t *testing.T) {
	root := procTree(t, map[int]string{3: statLine(3, "here", 1, 1)})
	require.NoError(t, os.MkdirAll(filepath.Join(root, "4"), 0o755)) // no stat

	got, err := ProcProcesses(root)(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "here", got[0].Name)
}

// R7. A share is processor time since the last reading over the time that
// passed on every processor: one of four processors busy is 25.
func TestAShareIsOfTheWholeMachine(t *testing.T) {
	samples := [][]ProcessSample{
		{{PID: 1, Name: "busy", CPUSeconds: 10}, {PID: 2, Name: "idle", CPUSeconds: 5}},
		{{PID: 1, Name: "busy", CPUSeconds: 12}, {PID: 2, Name: "idle", CPUSeconds: 5}, {PID: 3, Name: "new", CPUSeconds: 9, Memory: 77}},
		{{PID: 1, Name: "busy", CPUSeconds: 12}, {PID: 3, Name: "new", CPUSeconds: 17}},
	}
	call := 0
	l := NewProcessList(func(context.Context) ([]ProcessSample, error) {
		s := samples[call]
		call++
		return s, nil
	})
	clock := time.Unix(1000, 0)
	l.cpus, l.now, l.wait = 4, func() time.Time { return clock }, func() { clock = clock.Add(2 * time.Second) }

	// The first call takes two samples, the wait apart.
	got, err := l.Read(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, Process{PID: 1, Name: "busy", CPU: 25, HasCPU: true}, got[0])
	assert.Equal(t, Process{PID: 2, Name: "idle", CPU: 0, HasCPU: true}, got[1])
	assert.Equal(t, Process{PID: 3, Name: "new", Memory: 77}, got[2], "a process first seen has no share yet")

	clock = clock.Add(4 * time.Second)
	got, err = l.Read(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 2, "a process that ended is no longer listed")
	assert.InDelta(t, 0, got[0].CPU, 1e-9)
	assert.InDelta(t, 50, got[1].CPU, 1e-9, "eight seconds of processor in four seconds of four processors")
}

// A process ID is reused: time that went backwards is another process, and
// has no share until it has been seen twice.
func TestAReusedProcessIDHasNoShare(t *testing.T) {
	samples := [][]ProcessSample{
		{{PID: 9, Name: "old", CPUSeconds: 500}},
		{{PID: 9, Name: "young", CPUSeconds: 1}},
	}
	call := 0
	l := NewProcessList(func(context.Context) ([]ProcessSample, error) { call++; return samples[call-1], nil })
	clock := time.Unix(1000, 0)
	l.now, l.wait = func() time.Time { return clock }, func() { clock = clock.Add(time.Second) }

	got, err := l.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []Process{{PID: 9, Name: "young"}}, got)
}

// A reader that fails says so and is asked again next time.
func TestAProcessReadThatFailsIsAnError(t *testing.T) {
	boom := errors.New("no table")
	l := NewProcessList(func(context.Context) ([]ProcessSample, error) { return nil, boom })

	_, err := l.Read(t.Context())
	assert.ErrorIs(t, err, boom)
}

// R8. Memory in use is the total less what the kernel says is available,
// not less what is free: a warm page cache is not memory in use.
func TestMemoryInUseIsTotalLessAvailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meminfo")
	require.NoError(t, os.WriteFile(path, []byte(
		"MemTotal:       32000000 kB\nMemFree:         1000000 kB\nMemAvailable:   20000000 kB\nBuffers:  5 kB\n"), 0o600))

	m, err := ProcMemory(path)
	require.NoError(t, err)
	assert.Equal(t, Memory{Used: 12000000 * 1024, Total: 32000000 * 1024}, m)
}

// A meminfo without the two lines, or with nonsense in them, is an error
// and not a machine with no memory in use.
func TestMemoryThatCannotBeReadIsAnError(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"none":      "Buffers: 5 kB\n",
		"total":     "MemTotal: 100 kB\n",
		"backwards": "MemTotal: 100 kB\nMemAvailable: 200 kB\n",
	} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		_, err := ProcMemory(path)
		assert.Error(t, err, name)
	}
	_, err := ProcMemory(filepath.Join(dir, "absent"))
	assert.Error(t, err)
}

// The memory's absence is typed, and names what was asked.
func TestNoMemoryReadingIsATypedAbsence(t *testing.T) {
	h := Host{Memory: linuxMemory(filepath.Join(t.TempDir(), "absent")), MemoryMissing: memoryMissing}

	_, err := h.ReadMemory(t.Context())

	a, ok := AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, AbsenceMemory, a.Code)
	assert.Contains(t, a.Detail, "absent")

	// A host with no chain at all, a test's or another platform's.
	_, err = (&Host{}).ReadMemory(t.Context())
	a, ok = AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, AbsenceMemory, a.Code)
}

// This machine's own processes and memory, read for real: the reader finds
// this test, by name, holding memory.
func TestThisPlatformReadsItsOwnProcesses(t *testing.T) {
	h := NewHost(HostConfig{})
	if h.Platform == "other" {
		t.Skip("no process reader on this system")
	}
	got, err := h.Processes().Read(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, got)

	me := os.Getpid()
	found := false
	for _, p := range got {
		assert.NotEmpty(t, p.Name)
		if p.PID == me {
			found = true
			assert.True(t, p.HasCPU)
			assert.Positive(t, p.Memory)
			assert.NotContains(t, p.Name, string(filepath.Separator), "a name, not a path")
		}
	}
	assert.True(t, found, "this process is not among %d", len(got))

	m, err := h.ReadMemory(t.Context())
	require.NoError(t, err)
	assert.Positive(t, m.Used)
	assert.Greater(t, m.Total, m.Used)
}

// Each platform's memory chain is one provider, named for what a person is
// told was asked, and a chain that found nothing says so by that name: the
// wording is checked here on every system (spec 043).
func TestEachPlatformsMemoryChainNamesWhatItAsks(t *testing.T) {
	nothing := func(context.Context) (Memory, error) { return Memory{}, errors.New("no") }

	windows := windowsMemory(nothing)
	assert.Equal(t, []string{"GlobalMemoryStatusEx"}, windows.Names())
	linux := linuxMemory("/proc/meminfo")
	assert.Equal(t, []string{"/proc/meminfo"}, linux.Names())

	o := windows.Read(t.Context())
	require.False(t, o.Answered)
	a := memoryMissing(o.Tried, o.Err)
	assert.Equal(t, AbsenceMemory, a.Code)
	assert.Equal(t, "asked GlobalMemoryStatusEx", a.Detail)
	assert.Equal(t, "this build reads no memory on this system", memoryMissing(nil, nil).Detail)

	got := windowsMemory(func(context.Context) (Memory, error) { return Memory{Used: 1, Total: 2}, nil }).Read(t.Context())
	assert.Equal(t, Memory{Used: 1, Total: 2}, got.Value)
}
