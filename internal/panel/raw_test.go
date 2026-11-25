package panel_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
Spec 113 R14: a detail is read once, as plain values, and drawn from them.
What a reading holds is the fact -- a name whole, a PID, a figure in its
unit -- and the cutting to a drawer's width happens in the drawing.
*/

// A name of forty characters, which a drawer's line cuts to twenty.
const longName = "a-program-with-a-name-of-forty-letters-x"

// Every Detailer of the panel reads raw.
var (
	_ panel.RawDetailer = (*panel.Cooler)(nil)
	_ panel.RawDetailer = (*panel.Bandwidth)(nil)
	_ panel.RawDetailer = (*panel.Usage)(nil)
)

func TestAListIsReadWithWholeNamesAndItsPIDs(t *testing.T) {
	require.Len(t, longName, 40)
	c := panel.NewCoolerOn(detailHost(stepping(
		[]core.ProcessSample{{PID: 41, Name: longName, CPUSeconds: 1}, {PID: 42, Name: longName, CPUSeconds: 1}},
		[]core.ProcessSample{{PID: 41, Name: longName, CPUSeconds: 1.3}, {PID: 42, Name: longName, CPUSeconds: 1.1}},
	), &gpuReader{}, ""), noScan)
	panel.SetProcessors(c, func() (float64, bool) { return 12, true }, func(context.Context) core.Graphics { return core.Graphics{} })
	_, _ = c.Poll(t.Context())

	raw, err := c.ReadDetail(t.Context(), view.DetailCPU)

	require.NoError(t, err)
	require.NotNil(t, raw.Processes)
	assert.Equal(t, view.DetailCPU, raw.Processes.List)
	require.Len(t, raw.Processes.Uses, 2, "one use a process, not a line a name")
	for _, u := range raw.Processes.Uses {
		assert.Equal(t, longName, u.Name, "the reading holds the name whole")
		assert.Contains(t, []int{41, 42}, u.PID)
	}

	d := c.DrawDetail(raw)
	require.Len(t, d.Lines, 1, "the drawing sums them by name")
	assert.Equal(t, view.DetailNameWidth, len([]rune(d.Lines[0].Name)), "and cuts the name to the drawer")
	assert.True(t, strings.HasSuffix(d.Lines[0].Name, "…"))
}

func TestOneReadingIsDrawnAnyNumberOfTimesWithoutReadingAgain(t *testing.T) {
	reader := &netReader{use: netDrillUse(7)}
	b := bandwidthWith(t, 8000, 2000, reader)

	raw, err := b.ReadDetail(t.Context(), "peers")
	require.NoError(t, err)
	first, second := b.DrawDetail(raw), b.DrawDetail(raw)

	assert.Equal(t, 1, reader.reads, "drawing reads nothing")
	assert.Equal(t, first, second)
	polled, err := b.PollDetail(t.Context(), "peers")
	require.NoError(t, err)
	assert.Equal(t, 2, reader.reads, "a poll is one read")
	assert.Equal(t, first, polled, "and draws what the reading draws")
}

func TestADrawingNeedsOnlyItsReading(t *testing.T) {
	// A source that has read nothing draws a reading it was handed.
	c := panel.NewCoolerOn(detailHost(stepping(nil), &gpuReader{}, ""), noScan)
	raw := panel.RawDetail{Section: "cooler", ID: view.DetailMemory, Title: view.ProcessTitle(view.DetailMemory),
		Processes: &panel.RawProcesses{
			List:  view.DetailMemory,
			Uses:  []panel.RawUse{{PID: 3, ProcessUse: view.ProcessUse{Name: "editor", Value: 5 << 30}}},
			InUse: &panel.MemoryInUse{Used: 16 << 30, Total: 32 << 30},
		}}

	d := c.DrawDetail(raw)

	assert.Equal(t, []string{"editor"}, lineNames(d))
	assert.Equal(t, "memory/editor", d.Lines[0].Open)
	assert.Equal(t, "16.0 GiB of 32.0 GiB in use", d.Note)
}

func TestAProcesssPageIsReadWithoutItsCommandLine(t *testing.T) {
	self := os.Getpid()
	b := bandwidthWith(t, 8000, 2000, &netReader{use: netDrillUse(self)})

	raw, err := b.ReadDetail(t.Context(), "peers/eth0/browser/"+strconv.Itoa(self))

	require.NoError(t, err)
	require.NotNil(t, raw.Net)
	require.NotNil(t, raw.Net.Proc)
	p := *raw.Net.Proc
	assert.Equal(t, self, p.PID)
	assert.False(t, p.Gone)
	assert.NotEmpty(t, p.Name)
	assert.Equal(t, os.Getppid(), p.PPID)
	assert.NotEmpty(t, p.ParentName, "the parent by its own name, not a formatted line")
	encoded, err := json.Marshal(p)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), os.Args[0], "no path or command line (spec 060 R6)")
}

func TestAnAccountIsReadWhole(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return now })
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return []view.UsageWindow{{Account: longName, Name: "5h", Fraction: 0.1}}, now, nil, nil
	})
	panel.SetUsageBreakdown(u, func(label string) (view.UsageBreakdown, error) {
		return view.UsageBreakdown{Account: label, Limits: []view.UsageLimit{{Name: "Week", Fraction: 0.4, ResetsAt: now.Add(time.Hour)}}}, nil
	})
	_, err := u.Poll(t.Context())
	require.NoError(t, err)

	raw, err := u.ReadDetail(t.Context(), longName)
	require.NoError(t, err)
	require.NotNil(t, raw.Account)
	assert.Equal(t, longName, raw.Account.Account)
	assert.Equal(t, now, raw.At)
	assert.Equal(t, now.Add(time.Hour), raw.Account.Limits[0].ResetsAt, "a time, not how long until it")

	all, err := u.ReadDetail(t.Context(), view.DetailAccounts)
	require.NoError(t, err)
	require.NotNil(t, all.Overview)
	require.Len(t, all.Overview.Accounts, 1)
	assert.Equal(t, longName, all.Overview.Accounts[0].Breakdown.Account)
}

func TestADetailNotOfferedIsNoReading(t *testing.T) {
	b := bandwidthWith(t, 1, 1, &netReader{})

	raw, err := b.ReadDetail(t.Context(), "nothing-here")

	require.Error(t, err)
	assert.Equal(t, panel.RawDetail{}, raw)
	assert.Equal(t, view.Detail{}, b.DrawDetail(raw))
}

func TestADetailThatCannotBeReadCarriesWhy(t *testing.T) {
	gpu := &gpuReader{err: &core.Absence{Code: core.AbsenceGPUProcesses, Text: "not available", Detail: "tried two things"}}
	c := panel.NewCoolerOn(detailHost(stepping(nil), gpu, ""), noScan)
	_, _ = c.Poll(t.Context())

	raw, err := c.ReadDetail(t.Context(), view.DetailGPU)

	require.Error(t, err)
	assert.Nil(t, raw.Processes)
	var absence *core.Absence
	require.True(t, errors.As(raw.Unread, &absence), "the reader's own account, typed")
	assert.Equal(t, core.AbsenceGPUProcesses, absence.Code)
	assert.Equal(t, "GPU by process", raw.Title)
}
