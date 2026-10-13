package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// detailStub is a stub with details: each read returns the next of the
// readings a test queued, and the last for every read after.
type detailStub struct {
	stub
	offered  []view.DetailInfo
	readings map[string][]view.Detail
	reads    map[string]int
	closed   map[string]int
	polled   *int
}

func (d *detailStub) Poll(ctx context.Context) (bool, error) {
	if d.polled != nil {
		*d.polled++
	}
	return d.stub.Poll(ctx)
}

func (d *detailStub) Details() []view.DetailInfo { return d.offered }

func (d *detailStub) PollDetail(_ context.Context, id string) (view.Detail, error) {
	if d.reads == nil {
		d.reads = map[string]int{}
	}
	queue := d.readings[id]
	got := queue[min(d.reads[id], len(queue)-1)]
	d.reads[id]++
	return got, nil
}

func (d *detailStub) DetailInterval(string) time.Duration { return time.Second }

func (d *detailStub) CloseDetail(id string) {
	if d.closed == nil {
		d.closed = map[string]int{}
	}
	d.closed[id]++
}

func coolerStub() *detailStub {
	first := view.Processes("cooler", view.DetailCPU, nil)
	second := view.Processes("cooler", view.DetailCPU, []view.ProcessUse{{Name: "editor", Value: 12.5}, {Name: "browser", Value: 3}})
	second.Note = "share of every processor together"
	return &detailStub{
		stub:    stub{key: "cooler", drawn: true},
		offered: []view.DetailInfo{{ID: "cpu", Title: "CPU by process"}, {ID: "memory", Title: "Memory by process"}},
		readings: map[string][]view.Detail{
			"cpu":    {first, second},
			"memory": {view.Processes("cooler", view.DetailMemory, []view.ProcessUse{{Name: "editor", Value: 3 << 30}})},
		},
	}
}

// Spec 060 R21. `detail <section>/<id>` prints one detail as text: the
// second of two readings, because a rate read once has nothing in it, after
// the section has been polled, and the detail is let go afterwards.
func TestDetailPrintsTheSecondReadingAsText(t *testing.T) {
	polled := 0
	src := coolerStub()
	src.polled = &polled
	var out bytes.Buffer

	err := cli.Detail(t.Context(), &out, []panel.Source{stub{key: "usage"}, src}, "cooler/cpu", false, time.Millisecond)

	require.NoError(t, err)
	assert.Equal(t, "CPU by process\n"+
		"  editor          12.5 %\n"+
		"  browser          3.0 %\n"+
		"  share of every processor together\n", out.String())
	assert.Equal(t, 1, polled, "a detail is of a reading: the section is polled first")
	assert.Equal(t, 2, src.reads["cpu"])
	assert.Equal(t, 1, src.closed["cpu"])
}

// R21. A section alone is its first detail, and --json prints the detail as
// data.
func TestDetailOfASectionIsItsFirstAndPrintsAsJSON(t *testing.T) {
	src := coolerStub()
	var out bytes.Buffer

	require.NoError(t, cli.Detail(t.Context(), &out, []panel.Source{src}, "cooler", true, time.Millisecond))

	var got view.Detail
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "cooler", got.Section)
	assert.Equal(t, "cpu", got.ID)
	assert.Equal(t, "CPU by process", got.Title)
	require.Len(t, got.Lines, 2)
	assert.Equal(t, "editor", got.Lines[0].Name)
	assert.True(t, got.Filterable)
}

// A detail that could not be read is printed with its reason and read once:
// waiting for a second reading of an absence is waiting for nothing.
func TestDetailPrintsAnAbsenceWithoutWaiting(t *testing.T) {
	src := &detailStub{
		stub:    stub{key: "bandwidth", drawn: true},
		offered: []view.DetailInfo{{ID: "processes", Title: "Bandwidth by process"}},
		readings: map[string][]view.Detail{"processes": {{Title: "Bandwidth by process", Reasons: []view.Reason{
			{Text: "needs an administrator", Detail: "only an administrator may trace"},
		}}}},
	}
	var out bytes.Buffer

	require.NoError(t, cli.Detail(t.Context(), &out, []panel.Source{src}, "bandwidth", false, time.Hour))

	assert.Equal(t, "Bandwidth by process\n  needs an administrator\n    only an administrator may trace\n", out.String())
	assert.Equal(t, 1, src.reads["processes"])
}

// What cannot be asked for is refused in words that say what can.
func TestDetailRefusesWhatThereIsNot(t *testing.T) {
	sources := []panel.Source{coolerStub(), stub{key: "peripherals", drawn: true}}
	var out bytes.Buffer

	err := cli.Detail(t.Context(), &out, sources, "weather", false, 0)
	assert.ErrorContains(t, err, `no section named "weather"`)

	err = cli.Detail(t.Context(), &out, sources, "peripherals", false, 0)
	assert.ErrorContains(t, err, "the peripherals section has no detail to show")

	err = cli.Detail(t.Context(), &out, sources, "cooler/fans", false, 0)
	assert.ErrorContains(t, err, `no detail "fans"; there is cpu, memory`)
	assert.Empty(t, out.String())
}

// R12. `readings` prints each detail a section offers beside its data, read
// once and let go.
func TestReadingsPrintsTheDetails(t *testing.T) {
	src := coolerStub()
	var out bytes.Buffer

	require.NoError(t, cli.Readings(&out, []panel.Source{src, stub{key: "usage", drawn: true}}, func(panel.Source) error { return nil }))

	var got map[string]struct {
		Details map[string]view.Detail `json:"details"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got["cooler"].Details, 2)
	assert.Equal(t, "Memory by process", got["cooler"].Details["memory"].Title)
	assert.Equal(t, "editor", got["cooler"].Details["memory"].Lines[0].Name)
	assert.Empty(t, got["usage"].Details)
	assert.Equal(t, map[string]int{"cpu": 1, "memory": 1}, src.closed)
}
