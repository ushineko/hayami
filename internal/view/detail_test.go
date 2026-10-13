package view_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// names are a detail's lines' names, top to bottom.
func names(d view.Detail) []string {
	out := make([]string, 0, len(d.Lines))
	for _, l := range d.Lines {
		out = append(out, l.Name)
	}
	return out
}

// Spec 060 R5, R6. A list is one line per name, the figures of processes
// that share a name summed, greatest first.
func TestProcessesWithOneNameAreOneLineSummedGreatestFirst(t *testing.T) {
	d := view.Processes("cooler", view.DetailCPU, []view.ProcessUse{
		{Name: "editor", Value: 3},
		{Name: "browser", Value: 4},
		{Name: "browser", Value: 5},
		{Name: "idle", Value: 0},
		{Name: "browser", Value: 1.5},
	})

	assert.Equal(t, []string{"browser", "editor", "idle"}, names(d))
	assert.Equal(t, " 10.5", d.Lines[0].Value, "three processes named browser are one line of their sum")
	assert.Equal(t, "%", d.Lines[0].Unit)
	assert.Equal(t, "CPU by process", d.Title)
	assert.True(t, d.Filterable)
	assert.Equal(t, "cooler", d.Section)
	assert.Equal(t, view.DetailCPU, d.ID)
}

// Two programs doing the same keep their places between polls: a tie is
// broken by name, not by the order the system listed them in.
func TestProcessesThatTieAreOrderedByName(t *testing.T) {
	a := view.Processes("cooler", view.DetailCPU, []view.ProcessUse{{Name: "zeta"}, {Name: "alpha"}, {Name: "mid"}})
	b := view.Processes("cooler", view.DetailCPU, []view.ProcessUse{{Name: "mid"}, {Name: "zeta"}, {Name: "alpha"}})

	assert.Equal(t, []string{"alpha", "mid", "zeta"}, names(a))
	assert.Equal(t, names(a), names(b))
}

// R5. A value is the same width at every magnitude, so a new reading moves
// nothing: each kind of list, from nothing to more than it can hold.
func TestALinesValueIsTheSameWidthAtEveryMagnitude(t *testing.T) {
	magnitudes := []float64{0, 0.04, 1, 9.96, 10, 99.9, 100, 1023, 1024, 1 << 20, 5 << 30, 3 << 40, 1e15}

	for _, id := range []string{view.DetailCPU, view.DetailGPU, view.DetailMemory, view.DetailProcesses} {
		var uses []view.ProcessUse
		for i, m := range magnitudes {
			uses = append(uses, view.ProcessUse{
				Name: strings.Repeat("p", i+1), Value: m, Second: magnitudes[len(magnitudes)-1-i],
			})
		}
		d := view.Processes("s", id, uses)
		require.Len(t, d.Lines, len(magnitudes), id)

		width := func(l view.DetailLine) int { return utf8.RuneCountInString(l.Value) + utf8.RuneCountInString(l.Unit) }
		for _, l := range d.Lines {
			assert.Equal(t, width(d.Lines[0]), width(l), "%s: %q %q", id, l.Value, l.Unit)
		}
	}
}

// A percentage past what the column holds is held to it. The engine
// counters round, and a share of 1000 would be a sixth character.
func TestAShareTooLargeForItsColumnIsHeldToIt(t *testing.T) {
	d := view.Processes("s", view.DetailGPU, []view.ProcessUse{{Name: "a", Value: 12345}, {Name: "b", Value: -3}})

	assert.Equal(t, "999.9", d.Lines[0].Value)
	assert.Equal(t, "  0.0", d.Lines[1].Value)
}

// R11. Bandwidth is two rates on a line, ordered by their sum, each in its
// own band; the parts joined are the value.
func TestBandwidthIsDownAndUpOrderedByTheirSum(t *testing.T) {
	d := view.Processes("bandwidth", view.DetailProcesses, []view.ProcessUse{
		{Name: "downloader", Value: 2 << 20, Second: 1024},
		{Name: "uploader", Value: 0, Second: 30 << 20},
		{Name: "", Value: 10, Second: 10},
	})

	assert.Equal(t, []string{"uploader", "downloader", "other"}, names(d))
	up := d.Lines[0]
	assert.Equal(t, "↓    0 B/s   ↑ 30.0 MiB/s", up.Value)
	joined := ""
	for _, p := range up.Parts {
		joined += p.Text
	}
	assert.Equal(t, up.Value, joined)
	assert.Equal(t, view.Info, up.Parts[1].Status, "nothing coming down is not emphasised")
	assert.NotEqual(t, view.Info, up.Parts[3].Status, "thirty mebibytes a second going up is")
}

// A name is cut to its column, with an ellipsis, so a long one moves
// nothing.
func TestALongNameIsCutToItsColumn(t *testing.T) {
	d := view.Processes("s", view.DetailMemory, []view.ProcessUse{{Name: "a-program-with-a-very-long-name", Value: 1}})

	assert.Equal(t, view.DetailNameWidth, utf8.RuneCountInString(d.Lines[0].Name))
	assert.True(t, strings.HasSuffix(d.Lines[0].Name, "…"))
}

// R5. The list is as long as a height fits, and a filter narrows it before
// it is cut, so a name far down can be found.
func TestTheListIsNarrowedAndThenCutToAHeight(t *testing.T) {
	var uses []view.ProcessUse
	for i := range 30 {
		uses = append(uses, view.ProcessUse{Name: string(rune('a'+i%26)) + strings.Repeat("x", i/26), Value: float64(100 - i)})
	}
	d := view.Processes("s", view.DetailCPU, uses)

	assert.Len(t, d.Top(8).Lines, 8)
	assert.Equal(t, names(d)[:8], names(d.Top(8)))
	assert.Len(t, d.Top(100).Lines, 30)
	assert.Empty(t, d.Top(-1).Lines)
	assert.Len(t, d.Lines, 30, "cutting a copy leaves the detail whole")

	last := names(d)[29]
	found := d.Matching(func(name string) bool { return name == last }).Top(8)
	assert.Equal(t, []string{last}, names(found), "the thirtieth line is found by a filter, though only eight fit")
}

// A heading is kept by a filter: it names the lines under it.
func TestAFilterKeepsHeadings(t *testing.T) {
	d := view.Detail{Lines: []view.DetailLine{
		{Name: "Plan", Value: "team"}, {Name: "Week by use", Heading: true}, {Name: "Chats", Value: "  3 %"},
	}}

	got := d.Matching(func(name string) bool { return name == "Chats" })

	assert.Equal(t, []string{"Week by use", "Chats"}, names(got))
}

// R13, R15. A breakdown is a meter per limit, then the facts, then the
// shares under their heading.
func TestABreakdownIsMetersThenFactsThenShares(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	d := view.UsageDetail(now, view.UsageBreakdown{
		Account: "CC desk",
		Plan:    "team",
		Limits: []view.UsageLimit{
			{Name: "Session", Fraction: 0.12, ResetsAt: now.Add(2 * time.Hour)},
			{Name: "Week", Fraction: 0.91, ResetsAt: now.Add(72 * time.Hour), Severity: "critical"},
			{Name: "Individual", Fraction: 0.25, Amount: "300.5 / 1200"},
		},
		SharesTitle: "Week by use",
		Shares:      []view.UsageShare{{Name: "Claude Code", Fraction: 0.8}, {Name: "Chats", Fraction: 0.2}},
		Facts:       []view.UsageFact{{Name: "Extra usage", Value: "off"}},
	})

	assert.Equal(t, "usage", d.Section)
	assert.Equal(t, "CC desk", d.ID)
	assert.Equal(t, "CC desk", d.Title)
	assert.False(t, d.Filterable, "a breakdown is short and is not filtered")

	require.Len(t, d.Meters, 3)
	assert.Equal(t, "Session", d.Meters[0].Label)
	assert.Equal(t, " 12 %", d.Meters[0].Caption)
	assert.Equal(t, view.Good, d.Meters[0].Status)
	assert.Equal(t, view.Bad, d.Meters[1].Status, "the provider's own verdict wins")
	assert.InDelta(t, 0.91, d.Meters[1].Fraction, 1e-9)
	assert.Equal(t, "300.5 / 1200", d.Meters[2].StatsRight)

	assert.Equal(t, []string{"Plan", "Extra usage", "Week by use", "Claude Code", "Chats"}, names(d))
	assert.Equal(t, "team", d.Lines[0].Value)
	assert.True(t, d.Lines[2].Heading)
	assert.Equal(t, " 80 %", d.Lines[3].Value)
}

// R14. A breakdown with nothing in it is a detail with nothing in it, not a
// row of blanks.
func TestAnEmptyBreakdownHasNoLines(t *testing.T) {
	d := view.UsageDetail(time.Now(), view.UsageBreakdown{Account: "CX"})

	assert.Empty(t, d.Meters)
	assert.Empty(t, d.Lines)
	assert.True(t, d.Quiet())
}

// R21. The command line prints a detail as text: the heading, then a line
// each, the note and each reason with its detail under it.
func TestADetailPrintsAsText(t *testing.T) {
	d := view.Processes("cooler", view.DetailMemory, []view.ProcessUse{{Name: "editor", Value: 3 << 30}})
	d.Note = "9.0 GiB of 32.0 GiB in use"
	d.Reasons = []view.Reason{{Text: "not available", Detail: "tried one thing"}}

	assert.Equal(t, "Memory by process\n"+
		"  editor           3.0 GiB\n"+
		"  9.0 GiB of 32.0 GiB in use\n"+
		"  not available\n"+
		"    tried one thing\n", d.Text())
}

// Spec 060 R8. The memory is a row after the graphics card's: its share,
// and the amount in the temperature's column.
func TestTheMemoryIsARowAfterTheGraphicsCards(t *testing.T) {
	r := processors()
	r.Probes = append(r.Probes, view.Probe{ID: "memory", Role: view.RoleMemory,
		Load: view.Some(43.2), Bytes: view.Some(13.6 * (1 << 30)), Of: view.Some(31.9 * (1 << 30))})

	s := view.Cooler(r)

	require.Len(t, s.Rows, 4)
	memory := s.Rows[2]
	assert.Equal(t, "Memory", memory.Label)
	assert.Equal(t, " 43 %  13.6", memory.Value)
	assert.Equal(t, "GiB", strings.TrimSpace(memory.Unit))
	assert.Equal(t, "memory", memory.ID)
	assert.Equal(t, "Coolant", s.Rows[3].Label, "the coolant stays last of the temperatures")

	cpu := s.Rows[0]
	assert.Equal(t, utf8.RuneCountInString(cpu.Value), utf8.RuneCountInString(memory.Value))
	assert.Equal(t, utf8.RuneCountInString(cpu.Unit), utf8.RuneCountInString(memory.Unit))
}

// The memory row is the same width whatever the amount, and without a total
// its share is spaces of its own width.
func TestTheMemoryRowHoldsItsWidth(t *testing.T) {
	row := func(p view.Probe) view.Row {
		p.ID, p.Role = "memory", view.RoleMemory
		s := view.Cooler(view.CoolerReading{Probes: []view.Probe{p}})
		require.Len(t, s.Rows, 1)
		return s.Rows[0]
	}
	small := row(view.Probe{Load: view.Some(1.0), Bytes: view.Some(900.0 * (1 << 20))})
	large := row(view.Probe{Load: view.Some(100.0), Bytes: view.Some(512.0 * (1 << 30))})
	bare := row(view.Probe{Bytes: view.Some(2.0 * (1 << 30))})

	assert.Equal(t, "  1 % 900.0", small.Value)
	assert.Equal(t, "100 % 512.0", large.Value)
	assert.Equal(t, "        2.0", bare.Value, "no total, no share: spaces of its width")
	for _, r := range []view.Row{large, bare} {
		assert.Equal(t, utf8.RuneCountInString(small.Value), utf8.RuneCountInString(r.Value))
		assert.Equal(t, utf8.RuneCountInString(small.Unit), utf8.RuneCountInString(r.Unit))
	}
}

// A machine whose memory could not be read has no memory row: the probe is
// absent, not zero.
func TestNoMemoryIsNoRow(t *testing.T) {
	s := view.Cooler(view.CoolerReading{Probes: []view.Probe{{ID: "memory", Role: view.RoleMemory}}})

	assert.Empty(t, s.Rows)
}

// R3. A click on the processor's, the card's and the memory's rows asks for
// each one's own list, the coolant's for none of its own, and the card for
// the processor's.
func TestTheCoolersRowsNameTheDetailTheyOpen(t *testing.T) {
	r := processors()
	r.Probes = append(r.Probes, view.Probe{ID: "memory", Role: view.RoleMemory, Bytes: view.Some(1.0)})

	s := view.Cooler(r)

	opens := map[string]string{}
	for _, row := range s.Rows {
		opens[row.ID] = row.Opens
	}
	assert.Equal(t, map[string]string{"cpu": "cpu", "gpu": "gpu", "memory": "memory", "coolant": ""}, opens)
	assert.Equal(t, "cpu", s.Opens)
}

// A usage meter opens its own account's breakdown, and the card the first.
func TestAUsageMeterOpensItsAccount(t *testing.T) {
	now := time.Now()
	s := view.Usage(now, []view.UsageWindow{
		{Account: "CC desk", Name: "5h", Fraction: 0.1, Span: 5 * time.Hour},
		{Account: "CC desk", Name: "7d", Fraction: 0.2, Span: 7 * 24 * time.Hour},
		{Account: "CX", Name: "5h", Fraction: 0.3, Span: 5 * time.Hour},
	}, now)

	require.Len(t, s.Meters, 2)
	assert.Equal(t, "CC desk", s.Meters[0].Opens)
	assert.Equal(t, "CX", s.Meters[1].Opens)
	assert.Equal(t, "CC desk", s.Opens)
}
