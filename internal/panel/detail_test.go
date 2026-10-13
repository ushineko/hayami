package panel_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// gpuReader is a card's processes, recording the provider it was asked by.
type gpuReader struct {
	found []core.GPUProcess
	err   error
	via   []string
}

func (g *gpuReader) Read(_ context.Context, via string) ([]core.GPUProcess, error) {
	g.via = append(g.via, via)
	return g.found, g.err
}

// detailHost is fakeHost with a memory, a process table and a card, all of
// the test's: sixteen of thirty-two gibibytes in use, the samples given, and
// a card at 40 degrees read through the provider named via.
func detailHost(samples func(context.Context) ([]core.ProcessSample, error), gpu core.GPUProcessReader, via string) *core.Host {
	h := fakeHost()
	h.Memory = core.Chain[core.Memory]{Providers: []core.Provider[core.Memory]{
		core.ProviderFunc[core.Memory]{N: "the test memory", F: func(context.Context) (core.Memory, error) {
			return core.Memory{Used: 16 << 30, Total: 32 << 30}, nil
		}},
	}}
	h.Graphics = &core.GraphicsReader{NativeName: "the test card", Native: func(context.Context) core.Graphics {
		return core.Graphics{Temperature: 40, HasTemperature: true, Load: 9, HasLoad: true, Via: via}
	}}
	h.Processes = func() *core.ProcessList { return core.NewProcessList(samples) }
	h.GPUProcesses = gpu
	return h
}

// stepping returns the samples given one call at a time, and the last for
// every call after.
func stepping(samples ...[]core.ProcessSample) func(context.Context) ([]core.ProcessSample, error) {
	call := 0
	return func(context.Context) ([]core.ProcessSample, error) {
		s := samples[min(call, len(samples)-1)]
		call++
		return s, nil
	}
}

func lineNames(d view.Detail) []string {
	out := make([]string, 0, len(d.Lines))
	for _, l := range d.Lines {
		out = append(out, l.Name)
	}
	return out
}

// Spec 060 R8. The cooler reads the memory with everything else: a probe
// with the amount, the total and the share, and a row after the card's.
func TestTheCoolerReadsTheMemory(t *testing.T) {
	c := panel.NewCoolerOn(detailHost(stepping(nil), &gpuReader{}, ""), noScan)

	drawn, _ := c.Poll(t.Context())
	require.True(t, drawn)

	var memory view.Probe
	for _, p := range c.Data().(view.CoolerReading).Probes {
		if p.Role == view.RoleMemory {
			memory = p
		}
	}
	assert.Equal(t, "memory", memory.ID)
	assert.Equal(t, view.Some(float64(16<<30)), memory.Bytes)
	assert.Equal(t, view.Some(float64(32<<30)), memory.Of)
	assert.Equal(t, view.Some(50.0), memory.Load)

	sec := c.Section()
	var labels []string
	for _, r := range sec.Rows {
		labels = append(labels, r.Label)
	}
	assert.Equal(t, []string{"GPU", "Memory"}, labels)
	assert.Equal(t, " 50 %  16.0", sec.Rows[1].Value)
}

// R8. Memory that cannot be read is a typed absence said aside: no row, and
// a reason for the hover note and doctor that names what was asked.
func TestMemoryThatCannotBeReadIsAnAside(t *testing.T) {
	h := detailHost(stepping(nil), &gpuReader{}, "")
	h.Memory = core.Chain[core.Memory]{Providers: []core.Provider[core.Memory]{
		core.ProviderFunc[core.Memory]{N: "the test memory", F: func(context.Context) (core.Memory, error) {
			return core.Memory{}, errors.New("unplugged")
		}},
	}}
	h.MemoryMissing = func(tried []string, err error) *core.Absence {
		return &core.Absence{Code: core.AbsenceMemory, Detail: "asked " + joinNames(tried), Err: err}
	}
	c := panel.NewCoolerOn(h, noScan)

	_, _ = c.Poll(t.Context())
	sec := c.Section()

	for _, r := range sec.Rows {
		assert.NotEqual(t, "Memory", r.Label)
	}
	var found *view.Reason
	for i, r := range sec.Reasons {
		if r.Label == "Memory" {
			found = &sec.Reasons[i]
		}
	}
	require.NotNil(t, found)
	assert.True(t, found.Aside)
	assert.Equal(t, "asked the test memory", found.Detail)
}

// R1. The cooler offers a list for each row it draws that has one, and a
// source asked for a detail it does not offer says so.
func TestTheCoolerOffersAListForEachRowThatHasOne(t *testing.T) {
	c := panel.NewCoolerOn(detailHost(stepping(nil), &gpuReader{}, ""), noScan)
	assert.Empty(t, c.Details(), "nothing is offered before there is a reading")

	_, _ = c.Poll(t.Context())

	var ids []string
	for _, info := range c.Details() {
		ids = append(ids, info.ID)
		assert.Equal(t, view.ProcessTitle(info.ID), info.Title)
	}
	assert.Equal(t, []string{"memory", "gpu"}, ids, "this host has no processor load, so no processor list")
	assert.True(t, panel.Offers(c, "gpu"))
	assert.False(t, panel.Offers(c, "cpu"))
	assert.False(t, panel.Offers(c, "nonsense"))
	assert.Equal(t, panel.ProcessInterval, c.DetailInterval("memory"))

	_, err := c.PollDetail(t.Context(), "nonsense")
	assert.Error(t, err)
}

// R7, R9. The processor's list is each program's share since the last
// reading and the memory's what each holds, one line per name.
func TestTheProcessorsAndTheMemorysLists(t *testing.T) {
	c := panel.NewCoolerOn(detailHost(stepping(
		[]core.ProcessSample{{PID: 1, Name: "browser", CPUSeconds: 10}, {PID: 2, Name: "browser", CPUSeconds: 1}, {PID: 3, Name: "editor", CPUSeconds: 4}},
		[]core.ProcessSample{
			{PID: 1, Name: "browser", CPUSeconds: 10.2, Memory: 2 << 30},
			{PID: 2, Name: "browser", CPUSeconds: 1.1, Memory: 1 << 30},
			{PID: 3, Name: "editor", CPUSeconds: 4.5, Memory: 5 << 30},
			{PID: 4, Name: "arrived", CPUSeconds: 99, Memory: 1 << 20},
		},
	), &gpuReader{}, ""), noScan)
	panel.SetProcessors(c, func() (float64, bool) { return 12, true }, func(context.Context) core.Graphics { return core.Graphics{} })
	_, _ = c.Poll(t.Context())

	cpu, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	assert.Equal(t, []string{"editor", "browser"}, lineNames(cpu),
		"half a second against three tenths; a process first seen has no share and no line")
	assert.Equal(t, "cooler", cpu.Section)
	assert.True(t, cpu.Filterable)
	assert.NotEmpty(t, cpu.Note)

	memory, err := c.PollDetail(t.Context(), view.DetailMemory)
	require.NoError(t, err)
	assert.Equal(t, []string{"editor", "browser", "arrived"}, lineNames(memory))
	assert.Equal(t, "  3.0", memory.Lines[1].Value, "two processes named browser are one line of both")
	assert.Equal(t, "16.0 GiB of 32.0 GiB in use", memory.Note)
}

// R10. The card's list is asked for the card the row is about: by the
// provider its reading came from.
func TestTheCardsListIsAskedByTheProviderTheReadingCameFrom(t *testing.T) {
	gpu := &gpuReader{found: []core.GPUProcess{{PID: 5, Name: "game", Load: 71}, {PID: 6, Name: "compositor", Load: 3}}}
	c := panel.NewCoolerOn(detailHost(stepping(nil), gpu, "nvidia-smi"), noScan)
	_, _ = c.Poll(t.Context())

	d, err := c.PollDetail(t.Context(), view.DetailGPU)

	require.NoError(t, err)
	assert.Equal(t, []string{"nvidia-smi"}, gpu.via)
	assert.Equal(t, []string{"game", "compositor"}, lineNames(d))
	assert.Equal(t, " 71.0", d.Lines[0].Value)
}

// A list that cannot be read is a detail with its heading and the reader's
// own account of why, not an empty list.
func TestAListThatCannotBeReadSaysWhy(t *testing.T) {
	gpu := &gpuReader{err: &core.Absence{Code: core.AbsenceGPUProcesses, Text: "not available", Detail: "tried two things"}}
	c := panel.NewCoolerOn(detailHost(func(context.Context) ([]core.ProcessSample, error) {
		return nil, errors.New("the table is shut")
	}, gpu, ""), noScan)
	_, _ = c.Poll(t.Context())

	d, err := c.PollDetail(t.Context(), view.DetailGPU)
	require.Error(t, err)
	assert.Equal(t, "GPU by process", d.Title)
	assert.Empty(t, d.Lines)
	require.Len(t, d.Reasons, 1)
	assert.Equal(t, "not available", d.Reasons[0].Text)
	assert.Equal(t, "tried two things", d.Reasons[0].Detail)

	d, err = c.PollDetail(t.Context(), view.DetailMemory)
	require.Error(t, err)
	require.Len(t, d.Reasons, 1)
	assert.Contains(t, d.Reasons[0].Detail, "the table is shut")
}

// R6. No command line, path, user name or address reaches a detail. The
// processes are read from a tree whose every process has all four beside
// its name, and the detail, encoded whole, holds none of them.
func TestNothingButANameLeavesAProcess(t *testing.T) {
	root := t.TempDir()
	secrets := map[string]string{
		"cmdline": "/opt/tool\x00--password=hunter2\x00/home/someone/letter.txt",
		"exe":     "/home/someone/bin/tool",
		"environ": "USER=someone\x00HOME=/home/someone\x00TOKEN=abc123",
		"status":  "Name:\ttool\nUid:\t1000\t1000\n",
		"comm":    "tool\n",
	}
	for pid, name := range map[int]string{11: "tool", 12: "editor"} {
		dir := filepath.Join(root, strconv.Itoa(pid))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		stat := strconv.Itoa(pid) + " (" + name + ") S 1 2 3 4 5 6 7 8 9 10 500 100 0 0 20 0 1 0 100 1000 200\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "statm"), []byte("9000 4000 100 1 0 200 0\n"), 0o600))
		for file, body := range secrets {
			require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600))
		}
	}
	gpu := &gpuReader{found: []core.GPUProcess{{PID: 11, Name: "tool", Load: 4}}}
	c := panel.NewCoolerOn(detailHost(core.ProcProcesses(root), gpu, ""), noScan)
	panel.SetProcessors(c, func() (float64, bool) { return 1, true }, func(context.Context) core.Graphics {
		return core.Graphics{Temperature: 40, HasTemperature: true}
	})
	_, _ = c.Poll(t.Context())

	for _, id := range []string{view.DetailCPU, view.DetailMemory, view.DetailGPU} {
		d, err := c.PollDetail(t.Context(), id)
		require.NoError(t, err, id)
		require.NotEmpty(t, d.Lines, id)
		for _, form := range []string{mustJSON(t, d), d.Text()} {
			for _, leak := range []string{"hunter2", "someone", "/home", "/opt", "abc123", "1000\t", "letter", "password", "TOKEN"} {
				assert.NotContains(t, form, leak, id)
			}
		}
		for _, l := range d.Lines {
			assert.Contains(t, []string{"tool", "editor"}, l.Name)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// netReader is the network by process as a test gives it.
type netReader struct {
	use    core.NetUse
	err    error
	reads  int
	closed int
}

func (n *netReader) Read(context.Context) (core.NetUse, error) { n.reads++; return n.use, n.err }
func (n *netReader) Close()                                    { n.closed++ }

// bandwidthWith is the bandwidth source over eth0 at the rates given, polled
// twice so it has them, with a reader of the network by process.
func bandwidthWith(t *testing.T, rx, tx uint64, reader *netReader) *panel.Bandwidth {
	t.Helper()
	counters := core.Counters{}
	b := panel.NewBandwidth([]string{"eth0"}, func() (map[string]core.Counters, error) {
		return map[string]core.Counters{"eth0": counters}, nil
	})
	clock := time.Unix(1000, 0)
	panel.SetBandwidthClock(b, func() time.Time { return clock })
	_, err := b.Poll(t.Context())
	require.NoError(t, err)
	clock = clock.Add(time.Second)
	counters = core.Counters{Rx: rx, Tx: tx}
	_, err = b.Poll(t.Context())
	require.NoError(t, err)
	if reader != nil {
		b.SetNetProcesses(func() core.NetReader { return reader })
	}
	return b
}

// Spec 060 R11. The list is partial and says so: what it counts, and the
// share of the interfaces' traffic it comes to; what could not be put to a
// process is "other".
func TestTheBandwidthListSaysWhatItDoesNotCount(t *testing.T) {
	reader := &netReader{use: core.NetUse{HasRate: true, Counts: "TCP only", Processes: []core.NetProcess{
		{PID: 7, Name: "browser", Rx: 6000, Tx: 1000},
		// No process: whatever name came with it is not a program's.
		{PID: 0, Name: "left-over", Rx: 500, Tx: 500},
	}}}
	b := bandwidthWith(t, 8000, 2000, reader)

	d, err := b.PollDetail(t.Context(), view.DetailProcesses)

	require.NoError(t, err)
	assert.Equal(t, "Bandwidth by process", d.Title)
	assert.Equal(t, []string{"browser", "other"}, lineNames(d))
	assert.Equal(t, "TCP only, 80 % of the interfaces' traffic", d.Note,
		"eight of the ten kilobytes a second the interface carried")
	assert.True(t, d.Filterable)
}

// The share is never more than the whole, is left out where the interfaces
// carry too little to divide by, and a first reading says it is measuring.
func TestTheBandwidthNoteHoldsToWhatItKnows(t *testing.T) {
	over := &netReader{use: core.NetUse{HasRate: true, Counts: "TCP only", Processes: []core.NetProcess{{PID: 7, Name: "a", Rx: 50000}}}}
	d, err := bandwidthWith(t, 8000, 2000, over).PollDetail(t.Context(), view.DetailProcesses)
	require.NoError(t, err)
	assert.Equal(t, "TCP only, 100 % of the interfaces' traffic", d.Note)

	quiet := &netReader{use: core.NetUse{HasRate: true, Counts: "TCP only"}}
	d, err = bandwidthWith(t, 100, 0, quiet).PollDetail(t.Context(), view.DetailProcesses)
	require.NoError(t, err)
	assert.Equal(t, "TCP only", d.Note)

	first := &netReader{use: core.NetUse{Counts: "TCP and UDP"}}
	d, err = bandwidthWith(t, 8000, 2000, first).PollDetail(t.Context(), view.DetailProcesses)
	require.NoError(t, err)
	assert.Equal(t, "TCP and UDP, measuring", d.Note)
	assert.Empty(t, d.Lines)
}

// R11. On Windows without the trace session the list is the typed absence,
// in the reader's own words, and names what is needed.
func TestTheBandwidthListWithoutATraceIsATypedAbsence(t *testing.T) {
	reader := &netReader{err: &core.Absence{Code: core.AbsenceNetProcessesDenied,
		Text: "needs hayami's helper", Detail: "only an administrator may trace"}}
	b := bandwidthWith(t, 8000, 2000, reader)

	d, err := b.PollDetail(t.Context(), view.DetailProcesses)

	require.Error(t, err)
	a, ok := core.AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, core.AbsenceNetProcessesDenied, a.Code)
	assert.Empty(t, d.Lines)
	require.Len(t, d.Reasons, 1)
	assert.Equal(t, "needs hayami's helper", d.Reasons[0].Text)
	assert.Equal(t, "only an administrator may trace", d.Reasons[0].Detail)
	assert.Equal(t, "Bandwidth by process", d.Title)
}

// R4. The reader is built when the list is first read, held while it is
// shown and let go when it is not: a closed drawer holds nothing open.
func TestTheNetworkReaderIsHeldOnlyWhileTheListIsShown(t *testing.T) {
	reader := &netReader{use: core.NetUse{Counts: "TCP only"}}
	built := 0
	b := bandwidthWith(t, 1, 1, nil)
	b.SetNetProcesses(func() core.NetReader { built++; return reader })
	assert.Zero(t, built, "nothing is built until the list is asked for")

	_, _ = b.PollDetail(t.Context(), view.DetailProcesses)
	_, _ = b.PollDetail(t.Context(), view.DetailProcesses)
	assert.Equal(t, 1, built)
	assert.Equal(t, 2, reader.reads)

	b.CloseDetail(view.DetailProcesses)
	b.CloseDetail(view.DetailProcesses)
	assert.Equal(t, 1, reader.closed)

	_, _ = b.PollDetail(t.Context(), view.DetailProcesses)
	assert.Equal(t, 2, built, "shown again, it is built again")
}

// A bandwidth source with no reader, a test's counters, has no list and its
// card opens nothing.
func TestBandwidthWithoutAReaderHasNoList(t *testing.T) {
	b := bandwidthWith(t, 1, 1, nil)

	assert.Empty(t, b.Details())
	assert.Empty(t, b.Section().Opens)
	_, err := b.PollDetail(t.Context(), view.DetailProcesses)
	assert.Error(t, err)

	b.SetNetProcesses(func() core.NetReader { return &netReader{} })
	assert.Equal(t, view.DetailProcesses, b.Section().Opens)
	assert.True(t, panel.Offers(b, view.DetailProcesses))
}

// Spec 060 R13. Usage offers a breakdown per account, by the label its
// meter carries, read from the cache and never fetched.
func TestUsageOffersABreakdownPerAccount(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return now })
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return []view.UsageWindow{
			{Account: "CC desk", Name: "5h", Fraction: 0.1}, {Account: "CC desk", Name: "7d", Fraction: 0.2},
			{Account: "CX", Name: "5h", Fraction: 0.3},
		}, now, nil, nil
	})
	asked := []string{}
	panel.SetUsageBreakdown(u, func(label string) (view.UsageBreakdown, error) {
		asked = append(asked, label)
		if label == "CX" {
			return view.UsageBreakdown{}, errors.New("nothing in the usage cache for CX")
		}
		return view.UsageBreakdown{Account: label, Limits: []view.UsageLimit{{Name: "Week", Fraction: 0.4}}}, nil
	})
	_, err := u.Poll(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []view.DetailInfo{{ID: "CC desk", Title: "CC desk"}, {ID: "CX", Title: "CX"}}, u.Details())
	assert.Equal(t, panel.UsageInterval, u.DetailInterval("CX"))

	d, err := u.PollDetail(t.Context(), "CC desk")
	require.NoError(t, err)
	assert.Equal(t, "usage", d.Section)
	require.Len(t, d.Meters, 1)
	assert.Equal(t, "Week", d.Meters[0].Label)
	assert.False(t, d.Filterable)

	d, err = u.PollDetail(t.Context(), "CX")
	require.Error(t, err)
	assert.Equal(t, "CX", d.Title)
	require.Len(t, d.Reasons, 1)
	assert.Contains(t, d.Reasons[0].Detail, "nothing in the usage cache")
	assert.Equal(t, []string{"CC desk", "CX"}, asked)
}

// A source with no details is not a Detailer's worth of anything: the
// peripherals have none (spec 060, Out of scope).
func TestThePeripheralsOfferNoDetail(t *testing.T) {
	p := panel.NewPeripheralsOn(fakeHost(), noScan, time.Now)

	assert.False(t, panel.Offers(p, "anything"))
}
