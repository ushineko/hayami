package core

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processorFiles lays out a /proc and a sysfs of four possible processors,
// the third offline, under a test's directory.
func processorFiles(t *testing.T, stat string) ProcessorPaths {
	t.Helper()
	dir := t.TempDir()
	p := ProcessorPaths{
		Stat: filepath.Join(dir, "stat"), Possible: filepath.Join(dir, "possible"),
		CPUDir: filepath.Join(dir, "cpu"), LoadAvg: filepath.Join(dir, "loadavg"), Proc: filepath.Join(dir, "proc"),
	}
	write := func(path, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(p.Stat, stat)
	write(p.Possible, "0-3\n")
	for i, khz := range map[string][2]string{"0": {"4000000", "5700000"}, "1": {"2000000", "4400000"}, "3": {"3000000", "4400000"}} {
		write(filepath.Join(p.CPUDir, "cpu"+i, "cpufreq", "scaling_cur_freq"), khz[0]+"\n")
		write(filepath.Join(p.CPUDir, "cpu"+i, "cpufreq", "cpuinfo_max_freq"), khz[1]+"\n")
	}
	write(p.LoadAvg, "3.88 4.38 3.74 5/10878 2521036\n")
	for _, pid := range []string{"1", "42", "999"} {
		require.NoError(t, os.MkdirAll(filepath.Join(p.Proc, pid), 0o750))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(p.Proc, "self"), 0o750))
	return p
}

const statFirst = "cpu  100 0 100 800 0 0 0 0 0 0\n" +
	"cpu0 10 0 10 80 0 0 0 0 0 0\n" +
	"cpu1 10 0 10 80 0 0 0 0 0 0\n" +
	"cpu3 10 0 10 80 0 0 0 0 0 0\n" +
	"intr 1 2 3\n"

const statSecond = "cpu  200 0 200 1600 0 0 0 0 0 0\n" +
	"cpu0 60 0 50 90 0 0 0 0 0 0\n" + // 90 busy of 100: 90 %
	"cpu1 10 0 10 180 0 0 0 0 0 0\n" + // 0 of 100
	"cpu3 30 0 20 130 0 0 0 0 0 0\n" // 30 of 80: 37.5 %

// Spec 095 R2, R4. Each core's load since the previous reading, one place per
// possible processor: the offline one keeps its place with no load.
func TestEachCoresLoadKeepsItsPlace(t *testing.T) {
	p := processorFiles(t, statFirst)
	r := LinuxProcessor(p)
	r.wait = func() { require.NoError(t, os.WriteFile(p.Stat, []byte(statSecond), 0o600)) }

	got := r.Read(t.Context())
	require.True(t, got.HasCores)
	require.Len(t, got.Cores, 4)
	assert.InDelta(t, 90, got.Cores[0], 1e-9)
	assert.InDelta(t, 0, got.Cores[1], 1e-9)
	assert.Equal(t, NoCore, got.Cores[2], "offline")
	assert.InDelta(t, 37.5, got.Cores[3], 1e-9)
}

// R4. A core that comes online between two readings has no load on the
// first: its times are since boot, not since the last reading.
func TestACoreComingOnlineHasNoLoadYet(t *testing.T) {
	p := processorFiles(t, statFirst)
	r := LinuxProcessor(p)
	r.wait = func() {
		require.NoError(t, os.WriteFile(p.Stat, []byte(statSecond+"cpu2 5000 0 5000 100 0 0 0 0 0 0\n"), 0o600))
	}

	got := r.Read(t.Context())
	assert.Equal(t, NoCore, got.Cores[2])
}

// R4. The clock is the online cores' average of their current, of the highest
// maximum; the load average and the threads are /proc/loadavg's; the
// processes are /proc's numbered entries.
func TestTheClockTheLoadAverageAndTheTasks(t *testing.T) {
	p := processorFiles(t, statFirst)
	got := LinuxProcessor(p).Read(t.Context())

	require.True(t, got.HasClock)
	assert.InDelta(t, 3000, got.Clock, 1e-9)
	assert.InDelta(t, 5700, got.MaxClock, 1e-9)
	require.True(t, got.HasLoadAvg)
	assert.Equal(t, [3]float64{3.88, 4.38, 3.74}, got.LoadAvg)
	require.True(t, got.HasTasks)
	assert.Equal(t, 3, got.Processes)
	assert.Equal(t, 10878, got.Threads)
	assert.False(t, got.HasQueue)
}

// R4. A core without cpufreq leaves the clock to the rest.
func TestACoreWithoutAClockLeavesItToTheRest(t *testing.T) {
	p := processorFiles(t, statFirst)
	require.NoError(t, os.RemoveAll(filepath.Join(p.CPUDir, "cpu1")))

	got := LinuxProcessor(p).Read(t.Context())
	require.True(t, got.HasClock)
	assert.InDelta(t, 3500, got.Clock, 1e-9, "cpu0 and cpu3")
}

// R6. A machine without cpufreq has no clock, and a load average that is not
// one is no load average; the cores are still read.
func TestAMissingPartIsNotRead(t *testing.T) {
	p := processorFiles(t, statFirst)
	require.NoError(t, os.RemoveAll(p.CPUDir))
	require.NoError(t, os.WriteFile(p.LoadAvg, []byte("nonsense\n"), 0o600))
	r := LinuxProcessor(p)
	r.wait = func() {}

	got := r.Read(t.Context())
	assert.False(t, got.HasClock)
	assert.False(t, got.HasLoadAvg)
	assert.False(t, got.HasTasks)
	assert.True(t, got.HasCores)
}

func TestACPUListIsItsPlaces(t *testing.T) {
	for in, want := range map[string]int{"0-31\n": 32, "0-7,10-31": 32, "0": 1, "0,2": 3} {
		n, err := ParseCPUList(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, n, in)
	}
	for _, bad := range []string{"", "a-b", "3-1", "0-99999"} {
		_, err := ParseCPUList(bad)
		assert.Error(t, err, bad)
	}
}

// processorEntry is one SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION.
func processorEntry(idle, kernel, user int64) []byte {
	b := make([]byte, 48)
	binary.LittleEndian.PutUint64(b, uint64(idle))
	binary.LittleEndian.PutUint64(b[8:], uint64(kernel))
	binary.LittleEndian.PutUint64(b[16:], uint64(user))
	return b
}

// R5. Windows's processor table: busy is kernel and user less idle, which the
// kernel time includes.
func TestTheProcessorTableIsReadFromItsBytes(t *testing.T) {
	cores, err := ParseProcessorTimes(append(processorEntry(80, 100, 20), processorEntry(10, 10, 0)...))
	require.NoError(t, err)
	assert.Equal(t, []ProcessorTimes{{Busy: 40, Idle: 80, Online: true}, {Busy: 0, Idle: 10, Online: true}}, cores)

	for name, buf := range map[string][]byte{
		"cut":     processorEntry(1, 2, 3)[:40],
		"idle":    processorEntry(100, 10, 0),
		"negated": processorEntry(-1, 10, 0),
	} {
		_, err := ParseProcessorTimes(buf)
		assert.Error(t, err, name)
	}
}

func FuzzParseProcessorTimes(f *testing.F) {
	f.Add(append(processorEntry(80, 100, 20), processorEntry(10, 10, 0)...))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, buf []byte) {
		cores, err := ParseProcessorTimes(buf)
		if err != nil {
			return
		}
		for _, c := range cores {
			if c.Busy < 0 || c.Idle < 0 {
				t.Fatalf("a core with negative time: %+v", c)
			}
		}
	})
}

func FuzzParseLoadAvg(f *testing.F) {
	f.Add("3.88 4.38 3.74 5/10878 2521036\n")
	f.Fuzz(func(t *testing.T, s string) {
		avg, threads, err := ParseLoadAvg(s)
		if err == nil && (threads < 0 || avg[0] < 0 || avg[1] < 0 || avg[2] < 0) {
			t.Fatalf("%q read as %v, %d", s, avg, threads)
		}
	})
}
