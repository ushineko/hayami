package core

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diskstatsLine is a /proc/diskstats line for name, with sectors read and
// written and milliseconds doing IO where the kernel puts them.
func diskstatsLine(name string, read, written, io int) string {
	return "   8   0 " + name + " 100 0 " + itoa(read) + " 0 50 0 " + itoa(written) + " 0 0 " + itoa(io) + " 0 0 0 0 0 0 0\n"
}

func itoa(n int) string { return strconv.Itoa(n) }

// diskFiles lays out /proc and /sys/block for two whole disks, a partition of
// one, a zram swap and a loop device.
func diskFiles(t *testing.T, stats, vmstat, pressure string) DiskPaths {
	t.Helper()
	dir := t.TempDir()
	p := DiskPaths{
		DiskStats: filepath.Join(dir, "diskstats"), VMStat: filepath.Join(dir, "vmstat"),
		Pressure: filepath.Join(dir, "pressure"), SysBlock: filepath.Join(dir, "block"),
	}
	for _, name := range []string{"nvme0n1", "sda", "zram0", "loop0"} {
		require.NoError(t, os.MkdirAll(filepath.Join(p.SysBlock, name), 0o750))
	}
	require.NoError(t, os.WriteFile(p.DiskStats, []byte(stats), 0o600))
	require.NoError(t, os.WriteFile(p.VMStat, []byte(vmstat), 0o600))
	require.NoError(t, os.WriteFile(p.Pressure, []byte(pressure), 0o600))
	return p
}

func disksFirst() string {
	return diskstatsLine("nvme0n1", 1000, 2000, 100) + diskstatsLine("nvme0n1p1", 1000, 2000, 100) +
		diskstatsLine("sda", 0, 0, 0) + diskstatsLine("zram0", 9e6, 9e6, 9e6) + diskstatsLine("loop0", 9e6, 9e6, 9e6)
}

func disksSecond() string {
	// Over the two seconds between: nvme0n1 reads 2048 sectors (1 MiB) and
	// writes 4096 (2 MiB), busy 500 of 2000 ms; sda reads 2048, busy 2000.
	return diskstatsLine("nvme0n1", 3048, 6096, 600) + diskstatsLine("nvme0n1p1", 3048, 6096, 600) +
		diskstatsLine("sda", 2048, 0, 2000) + diskstatsLine("zram0", 1e7, 1e7, 1e7) + diskstatsLine("loop0", 1e7, 1e7, 1e7)
}

// diskReader is LinuxDisks over p with a clock two seconds a step, and the
// second sample written between the first call's two.
func diskReader(t *testing.T, p DiskPaths, second, vmSecond string) *DiskReader {
	t.Helper()
	r := LinuxDisks(p)
	at := time.Unix(1000, 0)
	r.now = func() time.Time { at = at.Add(2 * time.Second); return at }
	r.wait = func() {
		require.NoError(t, os.WriteFile(p.DiskStats, []byte(second), 0o600))
		require.NoError(t, os.WriteFile(p.VMStat, []byte(vmSecond), 0o600))
	}
	return r
}

// Spec 097 R5. The whole disks' rates and each one's busy share: a partition
// is its disk's IO again, and zram and loop are no disk, so none of them is
// counted.
func TestTheDisksRatesAndBusyShares(t *testing.T) {
	p := diskFiles(t, disksFirst(), "pswpin 10\npswpout 20\n", "some avg10=1.50 avg60=0.10 avg300=0.00 total=1\nfull avg10=0.5 avg60=0 avg300=0 total=1\n")
	d := diskReader(t, p, disksSecond(), "pswpin 12\npswpout 20\n").Read(t.Context())

	require.True(t, d.HasRates)
	assert.Equal(t, []string{"nvme0n1", "sda"}, d.Names)
	assert.InDelta(t, float64(2*(1<<20))/2, d.Read, 1e-6, "1 MiB from each disk over two seconds")
	assert.InDelta(t, float64(2*(1<<20))/2, d.Write, 1e-6)
	assert.InDelta(t, 0.25, d.Busy[0], 1e-9)
	assert.InDelta(t, 1, d.Busy[1], 1e-9)
	require.True(t, d.HasPressure)
	assert.InDelta(t, 1.5, d.Pressure, 1e-9)
	require.True(t, d.HasSwap)
	assert.InDelta(t, 2*pageBytes/2, d.SwapIn, 1e-6)
	assert.Zero(t, d.SwapOut)
	assert.False(t, d.Queues)
}

// R5. A disk that arrives between two readings has no share yet; one whose
// counters went back is another under an old name, and has none either.
func TestANewDiskHasNoShareYet(t *testing.T) {
	p := diskFiles(t, diskstatsLine("nvme0n1", 1000, 1000, 100), "", "")
	d := diskReader(t, p, diskstatsLine("nvme0n1", 10, 10, 10)+diskstatsLine("sda", 10, 10, 10), "").Read(t.Context())

	assert.Equal(t, []float64{NoCore, NoCore}, d.Busy)
	assert.Zero(t, d.Read)
	assert.False(t, d.HasSwap)
	assert.False(t, d.HasPressure)
}

// R6. Windows's counters: the rates and the queue from _Total, each disk's
// busy share from its idle time, the pages as bytes.
func TestWindowsDiskCounters(t *testing.T) {
	d := WindowsDisks(
		map[string]float64{"_Total": 300, "0 C:": 100, "1 D:": 200},
		map[string]float64{"_Total": 50},
		map[string]float64{"_Total": 80, "1 D:": 100, "0 C:": 60},
		map[string]float64{"_Total": 1.5},
		map[string]float64{"": 2}, map[string]float64{"": 3}, 4096)

	assert.InDelta(t, 300, d.Read, 1e-9)
	assert.InDelta(t, 50, d.Write, 1e-9)
	assert.Equal(t, []string{"0 C:", "1 D:"}, d.Names)
	assert.InDelta(t, 0.4, d.Busy[0], 1e-9)
	assert.InDelta(t, 0, d.Busy[1], 1e-9)
	assert.True(t, d.HasQueue && d.Queues && d.AllIO)
	assert.InDelta(t, 1.5, d.Queue, 1e-9)
	assert.InDelta(t, 8192, d.SwapIn, 1e-9)
	assert.InDelta(t, 12288, d.SwapOut, 1e-9)
}

// R5. A process's io file: what it took from and gave to the block layer, not
// the page cache's rchar and wchar.
func TestAProcessesIOIsItsBlockLayerBytes(t *testing.T) {
	r, w, ok := ParseProcIO("rchar: 2884\nwchar: 8\nsyscr: 6\nsyscw: 1\nread_bytes: 4096\nwrite_bytes: 8192\ncancelled_write_bytes: 0\n")
	require.True(t, ok)
	assert.InDelta(t, 4096, r, 1e-9)
	assert.InDelta(t, 8192, w, 1e-9)

	_, _, ok = ParseProcIO("rchar: 1\n")
	assert.False(t, ok)
}

// R5, R3. The process list's IO is a rate since the previous reading, and a
// process whose io cannot be read has none: it is not zero.
func TestTheProcessListsIOIsARate(t *testing.T) {
	step := 0
	l := NewProcessList(func(context.Context) ([]ProcessSample, error) {
		step++
		n := float64(step)
		return []ProcessSample{
			{PID: 1, Name: "copy", CPUSeconds: n, IORead: 1000 * n, IOWrite: 3000 * n, HasIO: true},
			{PID: 2, Name: "hidden", CPUSeconds: n},
		}, nil
	})
	at := time.Unix(0, 0)
	l.now = func() time.Time { at = at.Add(2 * time.Second); return at }
	l.wait = func() {}

	got, err := l.Read(t.Context())
	require.NoError(t, err)
	assert.True(t, got[0].HasIO)
	assert.InDelta(t, 500, got[0].IORead, 1e-9)
	assert.InDelta(t, 1500, got[0].IOWrite, 1e-9)
	assert.False(t, got[1].HasIO)
}

func FuzzParseDiskStats(f *testing.F) {
	f.Add(disksSecond())
	f.Fuzz(func(t *testing.T, s string) {
		for _, c := range ParseDiskStats(s, func(string) bool { return true }) {
			if c.SectorsRead < 0 || c.SectorsWritten < 0 || c.IOMillis < 0 {
				t.Fatalf("%q: %+v", s, c)
			}
		}
	})
}

func FuzzParseProcIO(f *testing.F) {
	f.Add("read_bytes: 4096\nwrite_bytes: 8192\n")
	f.Fuzz(func(t *testing.T, s string) {
		if r, w, ok := ParseProcIO(s); ok && (r < 0 || w < 0) {
			t.Fatalf("%q: %v %v", s, r, w)
		}
	})
}

func FuzzParsePressure(f *testing.F) {
	f.Add("some avg10=1.50 avg60=0.10 avg300=0.00 total=1\n")
	f.Fuzz(func(t *testing.T, s string) {
		if p, err := ParsePressure(s); err == nil && (p < 0 || p > 100) {
			t.Fatalf("%q: %v", s, p)
		}
	})
}
