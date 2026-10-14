package gui_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// detailSource is a section with details of the test's making, counting
// every read and every close.
type detailSource struct {
	key     string
	section view.Section

	mu      sync.Mutex
	details map[string]view.Detail
	polls   map[string]int
	closed  map[string]int
}

func newDetailSource(key string, section view.Section, details map[string]view.Detail) *detailSource {
	section.Key = key
	return &detailSource{key: key, section: section, details: details, polls: map[string]int{}, closed: map[string]int{}}
}

func (d *detailSource) Key() string                        { return d.key }
func (d *detailSource) Poll(context.Context) (bool, error) { return true, nil }
func (d *detailSource) Interval() time.Duration            { return time.Hour }
func (d *detailSource) Section() view.Section              { return d.section }
func (d *detailSource) Data() any                          { return nil }

// Details are the details the test gave, in the order a real source offers
// them: the process lists as the cooler does, with the word and the key each
// is chosen by, then anything else by name.
func (d *detailSource) Details() []view.DetailInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []view.DetailInfo
	listed := map[string]bool{}
	for _, id := range []string{view.DetailCPU, view.DetailGPU, view.DetailMemory, view.DetailAccounts} {
		if detail, ok := d.details[id]; ok {
			info := view.ProcessInfo(id)
			info.ID, info.Title = id, detail.Title
			out = append(out, info)
			listed[id] = true
		}
	}
	var rest []string
	for id := range d.details {
		if !listed[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	for _, id := range rest {
		out = append(out, view.DetailInfo{ID: id, Title: d.details[id].Title})
	}
	return out
}

// drop takes a detail away, as a reading that no longer has it does.
func (d *detailSource) drop(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.details, id)
}

func (d *detailSource) PollDetail(_ context.Context, id string) (view.Detail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.polls[id]++
	return d.details[id], nil
}

func (d *detailSource) DetailInterval(string) time.Duration { return 5 * time.Millisecond }

func (d *detailSource) CloseDetail(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed[id]++
}

func (d *detailSource) set(id string, detail view.Detail) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.details[id] = detail
}

func (d *detailSource) count(id string) (polls, closed int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.polls[id], d.closed[id]
}

// plain is a source with no details at all.
type plain struct{ detailSourceless }

type detailSourceless struct {
	key     string
	section view.Section
}

func (d detailSourceless) Key() string                        { return d.key }
func (d detailSourceless) Poll(context.Context) (bool, error) { return true, nil }
func (d detailSourceless) Interval() time.Duration            { return time.Hour }
func (d detailSourceless) Section() view.Section              { return d.section }
func (d detailSourceless) Data() any                          { return nil }

// processes is a list of n programs named p00, p01 and on, greatest first.
func processes(id string, n int, scale float64) view.Detail {
	var uses []view.ProcessUse
	for i := range n {
		uses = append(uses, view.ProcessUse{Name: fmt.Sprintf("p%02d", i), Value: scale * float64(n-i), Second: scale})
	}
	return view.Processes("cooler", id, uses)
}

// coolerSection is a cooler's card with a processor, a card, memory and a
// coolant.
func coolerSection() view.Section {
	return view.Cooler(view.CoolerReading{Probes: []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Load: view.Some(12.0), Temp: view.Some(51.5)},
		{ID: "gpu", Role: view.RoleGPU, Load: view.Some(4.0), Temp: view.Some(43.0)},
		{ID: "memory", Role: view.RoleMemory, Load: view.Some(50.0), Bytes: view.Some(16.0 * (1 << 30)), Of: view.Some(32.0 * (1 << 30))},
		{ID: "coolant", Role: view.RoleCoolant, Temp: view.Some(31.4)},
	}})
}

// drawerPanel is a panel over the sources given, drawn once, with its draws
// queued for the test to run. await runs them until cond holds.
func drawerPanel(t *testing.T, sources ...panel.Source) (p *gui.Panel, await func(what string, cond func() bool)) {
	t.Helper()
	a := test.NewTempApp(t)
	// A drawer is as tall as the cards, and one card of four rows has room
	// for four lines. A second card of many rows gives a list room to be
	// read, as a panel of every section does.
	sources = append(sources, tall())
	p = gui.New(a, gui.Options{Sources: sources, Title: "hayami"})
	drain := gui.QueueDraws(p)
	for _, s := range sources {
		p.Draw(s.Key(), s.Section(), true)
	}
	t.Cleanup(func() {
		gui.CloseDrawer(p)
		drain()
	})
	return p, func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			drain()
			if cond() {
				return
			}
			if time.Now().After(deadline) {
				require.Fail(t, "timed out waiting for "+what)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

// tall is a card with enough rows to make the panel a real one's height.
func tall() panel.Source {
	sec := view.Section{Key: "tall", Title: "Tall"}
	for i := range 16 {
		sec.Rows = append(sec.Rows, view.Row{ID: fmt.Sprintf("r%d", i), Label: fmt.Sprintf("Reading %d", i), Value: "  1.0"})
	}
	return plain{detailSourceless{key: "tall", section: sec}}
}

func coolerDetails() *detailSource {
	return newDetailSource("cooler", coolerSection(), map[string]view.Detail{
		"cpu":    processes("cpu", 5, 1),
		"gpu":    processes("gpu", 3, 1),
		"memory": processes("memory", 4, 1<<30),
	})
}

// Spec 062 R1. A click anywhere on a card opens that section's detail,
// whatever is under the pointer, and a second click anywhere on it closes
// the drawer.
func TestAClickAnywhereOnACardOpensItsDetailAndASecondClosesIt(t *testing.T) {
	for _, row := range []string{"", "cpu", "memory", "coolant"} {
		src := coolerDetails()
		p, await := drawerPanel(t, src)
		require.False(t, gui.DrawerOpen(p))

		gui.Tap(p, "cooler", row, -1)
		await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
		await("the list", func() bool { return len(gui.DrawerLines(p)) == 5 })

		assert.Equal(t, "CPU by process", gui.DrawerTitle(p), "a click on %q", row)
		section, id := gui.DrawerSubject(p)
		assert.Equal(t, "cooler", section)
		assert.Equal(t, "cpu", id, "a click on %q chose a list of its own", row)
		assert.True(t, strings.HasPrefix(gui.DrawerLines(p)[0], "p00|"), "greatest first: %v", gui.DrawerLines(p))

		gui.Tap(p, "cooler", "gpu", -1) // another row of the same card
		await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	}
}

// Spec 062 R4, R6. The cooler's drawer has a choice for each list its source
// offers. Choosing one stops the poll of the list shown, starts the other's,
// changes the heading and clears the filter, and leaves the panel the size
// it was. A key typed into a filter is the filter's.
func TestTheSwitchChangesWhatIsShown(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)

	gui.Tap(p, "cooler", "", -1)
	await("the processor's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 5 })
	assert.Equal(t, []string{"CPU", "GPU", "Mem"}, gui.DrawerChoices(p))
	assert.Equal(t, "cpu", gui.DrawerChosen(p))
	size := gui.PanelSize(p)

	gui.FilterType(p, "fp0")
	gui.FilterKey(p, fyne.KeyReturn)
	gui.TypeRune(p, 'm')
	await("the memory's list", func() bool { return len(gui.DrawerLines(p)) == 4 })
	assert.True(t, gui.DrawerOpen(p), "the drawer stayed open")
	assert.Equal(t, "Memory by process", gui.DrawerTitle(p))
	assert.Equal(t, "memory", gui.DrawerChosen(p))
	assert.Contains(t, gui.DrawerLines(p)[0], "GiB")
	assert.Empty(t, gui.FilterText(p), "the filter of the list before was kept")
	assert.Equal(t, size, gui.PanelSize(p), "choosing a list resized the panel")
	_, closed := src.count("cpu")
	assert.Equal(t, 1, closed, "the list no longer shown was let go")

	gui.TypeRune(p, 'f')
	gui.TypeRune(p, 'g')
	await("the draws to settle", func() bool { return true })
	assert.Equal(t, "memory", gui.DrawerChosen(p), "a key typed into a filter chose a list")
	assert.Equal(t, "g", gui.FilterText(p))
	gui.TypeKey(p, fyne.KeyEscape)

	gui.TypeRune(p, 'g')
	await("the card's list", func() bool { return len(gui.DrawerLines(p)) == 3 })
	assert.Equal(t, "GPU by process", gui.DrawerTitle(p))

	gui.Tap(p, "cooler", "memory", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	time.Sleep(20 * time.Millisecond)
	settled, _ := src.count("gpu")
	time.Sleep(60 * time.Millisecond)
	after, _ := src.count("gpu")
	assert.Equal(t, settled, after, "a closed drawer went on reading the list last chosen")
}

// Spec 062 R7. The card opens on the list last shown, and on the first
// offered when that one has gone.
func TestTheCardOpensOnTheListLastShown(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)

	gui.Tap(p, "cooler", "", -1)
	await("the processor's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 5 })
	gui.TypeRune(p, 'm')
	await("the memory's list", func() bool { return len(gui.DrawerLines(p)) == 4 })
	gui.Tap(p, "cooler", "", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 4 })
	assert.Equal(t, "Memory by process", gui.DrawerTitle(p), "the card opened on another list than the one last shown")
	gui.Tap(p, "cooler", "", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })

	src.drop("memory")
	src.drop("cpu")
	gui.Tap(p, "cooler", "", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 3 })
	assert.Equal(t, "GPU by process", gui.DrawerTitle(p))
	assert.Empty(t, gui.DrawerChoices(p), "one list is nothing to choose between")
}

// R4. A detail is not read while the drawer is closed: nothing before the
// first click, and nothing more once it has closed.
func TestADetailIsNotPolledWhileTheDrawerIsClosed(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)

	time.Sleep(30 * time.Millisecond)
	polls, _ := src.count("cpu")
	require.Zero(t, polls, "a panel nobody clicked read a detail")

	gui.Tap(p, "cooler", "cpu", -1)
	await("three readings", func() bool { n, _ := src.count("cpu"); return n >= 3 })

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	_, closed := src.count("cpu")
	assert.Equal(t, 1, closed)

	// One reading may have been in flight when the drawer closed.
	time.Sleep(20 * time.Millisecond)
	settled, _ := src.count("cpu")
	time.Sleep(60 * time.Millisecond)
	after, _ := src.count("cpu")
	assert.Equal(t, settled, after, "a closed drawer went on reading")
}

// R4. Hiding the section the drawer shows closes it and stops its poll.
func TestHidingTheSectionClosesItsDrawer(t *testing.T) {
	src := coolerDetails()
	other := plain{detailSourceless{key: "peripherals", section: view.Section{Key: "peripherals", Title: "Peripherals",
		Rows: []view.Row{{Label: "Mouse", Value: " 80 %"}}}}}
	p, await := drawerPanel(t, src, other)
	gui.Tap(p, "cooler", "cpu", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 5 })

	c := config.Default()
	c.Sections = []string{"peripherals"}
	p.Apply(c)

	assert.False(t, gui.DrawerOpen(p))
	time.Sleep(20 * time.Millisecond)
	settled, closed := src.count("cpu")
	assert.Equal(t, 1, closed)
	time.Sleep(60 * time.Millisecond)
	after, _ := src.count("cpu")
	assert.Equal(t, settled, after, "a hidden section's detail went on being read")
}

// R1. A card whose source has no details does nothing on a click, and
// neither does one whose section names a detail its source does not offer.
func TestACardWithNoDetailDoesNothing(t *testing.T) {
	other := plain{detailSourceless{key: "peripherals", section: view.Section{Key: "peripherals", Title: "Peripherals",
		Opens: "anything", Rows: []view.Row{{ID: "mouse", Label: "Mouse", Value: " 80 %"}}}}}
	src := newDetailSource("bandwidth", view.Section{Title: "Bandwidth", Opens: "processes",
		Rows: []view.Row{{ID: "eth0", Label: "eth0", Value: "1"}}}, map[string]view.Detail{})
	p, await := drawerPanel(t, other, src)

	gui.Tap(p, "peripherals", "", -1)
	gui.Tap(p, "peripherals", "mouse", -1)
	gui.Tap(p, "bandwidth", "eth0", -1) // the view names it; this source does not offer it
	time.Sleep(20 * time.Millisecond)
	await("nothing", func() bool { return true })

	assert.False(t, gui.DrawerOpen(p))
}

// R5. The list is as long as the drawer's height fits and no longer, and
// the note takes one of those lines.
func TestTheListIsAsLongAsTheHeightFits(t *testing.T) {
	long := processes("cpu", 200, 1)
	long.Note = "share of every processor together"
	src := newDetailSource("cooler", coolerSection(), map[string]view.Detail{"cpu": long})
	p, await := drawerPanel(t, src)

	gui.Tap(p, "cooler", "cpu", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })

	fit := gui.DrawerFit(p)
	require.Positive(t, fit)
	require.Less(t, fit, 200, "the drawer claims room for two hundred lines")
	assert.Len(t, gui.DrawerLines(p), fit-1)
	assert.Equal(t, []string{"share of every processor together"}, gui.DrawerNoteLines(p))
	assert.True(t, strings.HasPrefix(gui.DrawerLines(p)[0], "p00|"))
}

// R17. The filter narrows the list before it is cut, by a regular
// expression; Escape clears it. It is typed with f.
func TestTheFilterNarrowsAList(t *testing.T) {
	src := newDetailSource("cooler", coolerSection(), map[string]view.Detail{"cpu": processes("cpu", 200, 1)})
	p, await := drawerPanel(t, src)
	gui.Tap(p, "cooler", "cpu", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })
	require.NotContains(t, strings.Join(gui.DrawerLines(p), " "), "p199", "the last of two hundred does not fit")

	gui.FilterType(p, "fp19[89]")
	await("the filter", func() bool { return len(gui.DrawerLines(p)) == 2 })

	assert.Equal(t, "p19[89]", gui.FilterText(p), "f started the filter and is not part of it")
	assert.True(t, strings.HasPrefix(gui.DrawerLines(p)[0], "p198|"))
	assert.True(t, strings.HasPrefix(gui.DrawerLines(p)[1], "p199|"))

	gui.FilterKey(p, fyne.KeyEscape)
	await("the whole list", func() bool { return len(gui.DrawerLines(p)) > 2 })
	assert.Empty(t, gui.FilterText(p))
}

// usageDetails is a usage card of two accounts and its details: every
// account's as one, with limits meters each, and one that cannot be read
// when broken is set.
func usageDetails(now time.Time, limits int, broken bool) *detailSource {
	section := view.Usage(now, []view.UsageWindow{
		{Account: "CC desk", Name: "5h", Fraction: 0.10, ResetsAt: now.Add(2 * time.Hour)},
		{Account: "CX", Name: "5h", Fraction: 0.20, ResetsAt: now.Add(2 * time.Hour)},
	}, now)
	// Each account's own detail is offered too, as the real source offers
	// it for the command line. Neither is one of a choice.
	all := accounts(now, limits, 0.5, broken)
	return newDetailSource("usage", section, map[string]view.Detail{
		view.DetailAccounts: all, "CC desk": all.Parts[0], "CX": all.Parts[1],
	})
}

func accounts(now time.Time, limits int, fraction float64, broken bool) view.Detail {
	breakdown := func(account string, limits int) view.Detail {
		b := view.UsageBreakdown{Account: account, SharesTitle: "Week by use",
			Shares: []view.UsageShare{{Name: "Chats", Fraction: 0.25}}}
		for i := range limits {
			b.Limits = append(b.Limits, view.UsageLimit{Name: fmt.Sprintf("Limit %d", i), Fraction: fraction})
		}
		return view.UsageDetail(now, b)
	}
	second := breakdown("CX", limits-1)
	if broken {
		second = view.Detail{Section: "usage", ID: "CX", Title: "CX", Reasons: []view.Reason{{Text: "not available"}}}
	}
	d := view.UsageAccounts([]view.Detail{breakdown("CC desk", limits), second})
	d.Title = "Usage by account"
	return d
}

// Spec 062 R9, R10. A click on any meter opens every account's breakdown,
// one under another: each its heading, its limits as meters and its lines.
// An account that cannot be read says why under its heading. There is no
// filter and no switch, and a second click on any meter closes it.
func TestAClickOnTheUsageCardOpensEveryAccount(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	for meter := range 2 {
		src := usageDetails(now, 3, false)
		p, await := drawerPanel(t, src)

		gui.Tap(p, "usage", "", meter)
		await("the accounts", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerPartHeads(p)) == 2 })
		assert.Equal(t, "Usage by account", gui.DrawerTitle(p))
		assert.Equal(t, []string{"CC desk", "CX"}, gui.DrawerPartHeads(p), "a click on meter %d", meter)
		assert.Equal(t, []int{3, 2}, gui.DrawerPartMeters(p))
		assert.Equal(t, 1, gui.DrawerRules(p), "one rule, between the two accounts and not above the first")
		assert.Equal(t, []string{"Week by use", "Chats| 25 %"}, gui.DrawerPartLines(p, 1))
		assert.Empty(t, gui.DrawerChoices(p))

		gui.FilterType(p, "fzzz")
		await("nothing", func() bool { return true })
		assert.Empty(t, gui.FilterText(p), "a breakdown is not filtered")

		src.set(view.DetailAccounts, accounts(now, 3, 0.5, true))
		await("the account that cannot be read", func() bool { return len(gui.DrawerPartMeters(p)) == 2 && gui.DrawerPartMeters(p)[1] == 0 })
		assert.Equal(t, []string{"CC desk", "CX"}, gui.DrawerPartHeads(p))
		assert.Equal(t, []string{"not available"}, gui.DrawerPartLines(p, 1))
		assert.Equal(t, 3, gui.DrawerPartMeters(p)[0], "the account that could be read lost its limits")

		gui.Tap(p, "usage", "", 1-meter)
		await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	}
}

// Spec 062 R11, R12. A breakdown taller than the cards leaves the panel the
// height it was and scrolls inside the drawer, and a reading that arrives
// leaves it where it was scrolled to.
func TestABreakdownTallerThanTheCardsScrolls(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	src := usageDetails(now, 60, false)
	p, await := drawerPanel(t, src)
	closed := gui.PanelSize(p)

	gui.Tap(p, "usage", "", 0)
	await("the accounts", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerPartHeads(p)) == 2 })
	require.Equal(t, []int{60, 59}, gui.DrawerPartMeters(p), "not everything is drawn, so nothing needs scrolling")
	assert.Equal(t, closed.Height, gui.PanelSize(p).Height, "the breakdown made the panel taller")
	assert.Zero(t, gui.DrawerOffset(p))

	gui.TypeKey(p, fyne.KeyPageDown)
	at := gui.DrawerOffset(p)
	require.Positive(t, at, "a breakdown taller than the cards does not scroll")
	assert.True(t, gui.DrawerScrolls(p))

	polls, _ := src.count(view.DetailAccounts)
	src.set(view.DetailAccounts, accounts(now, 60, 0.9, false))
	await("another reading", func() bool { n, _ := src.count(view.DetailAccounts); return n >= polls+2 })
	assert.Equal(t, at, gui.DrawerOffset(p), "a reading moved what was being read")
	assert.Equal(t, closed.Height, gui.PanelSize(p).Height)

	gui.Tap(p, "usage", "", 0)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
}

// A list shown after a breakdown is a list again: cut to the height, with
// its filter, and with nowhere to scroll to.
func TestAListAfterABreakdownIsCutToTheHeight(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	cooler := newDetailSource("cooler", coolerSection(), map[string]view.Detail{"cpu": processes("cpu", 200, 1)})
	p, await := drawerPanel(t, usageDetails(now, 60, false), cooler)

	gui.Tap(p, "usage", "", 0)
	await("the accounts", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerPartHeads(p)) == 2 })
	gui.TypeKey(p, fyne.KeyPageDown)
	gui.Tap(p, "cooler", "", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })

	assert.Equal(t, "CPU by process", gui.DrawerTitle(p))
	assert.Empty(t, gui.DrawerPartHeads(p))
	assert.Less(t, len(gui.DrawerLines(p)), 200)
	assert.LessOrEqual(t, len(gui.DrawerLines(p)), gui.DrawerFit(p))
	assert.False(t, gui.DrawerScrolls(p), "a list scrolls")
	gui.FilterType(p, "fp19[89]")
	await("the filter", func() bool { return len(gui.DrawerLines(p)) == 2 })
}

// A detail that could not be read shows its reason where the list would be.
func TestADetailThatCouldNotBeReadShowsItsReason(t *testing.T) {
	src := newDetailSource("bandwidth", view.Section{Title: "Bandwidth", Opens: "processes",
		Rows: []view.Row{{ID: "eth0", Label: "eth0", Value: "1"}}}, map[string]view.Detail{
		"processes": {Section: "bandwidth", ID: "processes", Title: "Bandwidth by process",
			Reasons: []view.Reason{{Text: "needs hayami's helper", Detail: "a long sentence nobody draws"}}},
	})
	p, await := drawerPanel(t, src)

	gui.Tap(p, "bandwidth", "eth0", -1)
	await("the reason", func() bool { return len(gui.DrawerNoteLines(p)) == 1 })

	assert.Equal(t, "Bandwidth by process", gui.DrawerTitle(p))
	assert.Equal(t, []string{"needs hayami's helper"}, gui.DrawerNoteLines(p))
	assert.Empty(t, gui.DrawerLines(p))
}

// Glance rule, R5. A value changing moves nothing: the panel is the size it
// was when every figure in the list has changed magnitude.
func TestAValueChangingInTheDrawerMovesNothing(t *testing.T) {
	src := newDetailSource("cooler", coolerSection(), map[string]view.Detail{"memory": processes("memory", 6, 1)})
	p, await := drawerPanel(t, src)
	gui.Tap(p, "cooler", "memory", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 6 })
	before, first := gui.PanelSize(p), gui.DrawerLines(p)[0]

	src.set("memory", processes("memory", 6, 3<<40))
	await("the new figures", func() bool { return gui.DrawerLines(p)[0] != first })

	assert.Equal(t, before, gui.PanelSize(p))
}

// R16. The side is the setting: a named side, or by room, which is the
// default.
func TestTheDrawersSideIsTheSetting(t *testing.T) {
	p, _ := drawerPanel(t, coolerDetails())
	c := config.Default()

	p.Apply(c)
	_, auto := gui.DrawerSide(p)
	assert.True(t, auto, "a settings file that says nothing chooses by room")

	c.DrawerSide = config.DrawerLeft
	p.Apply(c)
	side, auto := gui.DrawerSide(p)
	assert.Equal(t, glance.Left, side)
	assert.False(t, auto)

	c.DrawerSide = config.DrawerRight
	p.Apply(c)
	side, auto = gui.DrawerSide(p)
	assert.Equal(t, glance.Right, side)
	assert.False(t, auto)

	c.DrawerSide = "sideways"
	p.Apply(c)
	_, auto = gui.DrawerSide(p)
	assert.True(t, auto, "a side this build does not know is by room")
}

// R20, spec 062 R3. In the row arrangement a click on any line of a section
// opens its detail as in the others.
func TestARowArrangementOpensDetailsToo(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)
	c := config.Default()
	c.Arrangement = "row"
	c.Sections = []string{"cooler"}
	p.Apply(c)

	gui.Tap(p, "cooler", "memory", -1)
	await("the processor's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) > 0 })

	assert.Equal(t, "CPU by process", gui.DrawerTitle(p))
}

// A panel built without a Desk asks nothing of the compositor: the tests in
// this package build panels, and none of them may reach the desk they run
// on.
func TestAPanelBuiltWithoutADeskAsksNothing(t *testing.T) {
	p, _ := drawerPanel(t, coolerDetails())

	assert.True(t, gui.AsksNothing(p))
}

// compositor is a Desk of the test's: the room it reports, and what it was
// asked.
type compositor struct {
	mu          sync.Mutex
	left, right float32
	answers     bool
	rooms       int
	held        []bool
}

func (d *compositor) Asks() bool { return true }

func (d *compositor) Room(context.Context) (float32, float32, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rooms++
	return d.left, d.right, d.answers
}

func (d *compositor) HoldEdge(_ context.Context, on bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.held = append(d.held, on)
	return on
}

func (d *compositor) Yield(context.Context) {}
func (d *compositor) Stop()                 {}

func (d *compositor) asked() (rooms int, held []bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rooms, append([]bool(nil), d.held...)
}

// deskPanel is drawerPanel over a compositor of the test's.
func deskPanel(t *testing.T, d *compositor, sources ...panel.Source) (p *gui.Panel, await func(what string, cond func() bool)) {
	t.Helper()
	a := test.NewTempApp(t)
	sources = append(sources, tall())
	p = gui.New(a, gui.Options{Sources: sources, Title: "hayami", Desk: d})
	drain := gui.QueueDraws(p)
	for _, s := range sources {
		p.Draw(s.Key(), s.Section(), true)
	}
	t.Cleanup(func() {
		gui.CloseDrawer(p)
		drain()
	})
	return p, func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			drain()
			if cond() {
				return
			}
			if time.Now().After(deadline) {
				require.Fail(t, "timed out waiting for "+what)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

// Spec 060 R16, R18. Where only the compositor knows the room, it is asked
// at each opening: room on the left alone opens the drawer on the left with
// the edge held, and room on the right opens it on the right with the edge
// let go.
func TestTheCompositorIsAskedForTheRoomAtEachOpening(t *testing.T) {
	d := &compositor{left: 5000, right: 10, answers: true}
	p, await := deskPanel(t, d, coolerDetails())

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
	rooms, held := d.asked()
	assert.Equal(t, 1, rooms)
	assert.Equal(t, []bool{true}, held, "a drawer on the left needs the edge held")
	assert.Equal(t, glance.Left, gui.DrawerOnSide(p))

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })

	d.mu.Lock()
	d.left, d.right = 10, 5000
	d.mu.Unlock()
	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to open again", func() bool { return gui.DrawerOpen(p) })
	rooms, held = d.asked()
	assert.Equal(t, 2, rooms, "the room is asked again: the panel may have been moved")
	assert.Equal(t, []bool{true, false}, held)
	assert.Equal(t, glance.Right, gui.DrawerOnSide(p))
}

// A compositor that does not answer costs nothing but the question: the
// drawer opens on the right.
func TestACompositorThatDoesNotAnswerLeavesTheDrawerOnTheRight(t *testing.T) {
	d := &compositor{}
	p, await := deskPanel(t, d, coolerDetails())

	gui.Tap(p, "cooler", "memory", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })

	assert.Equal(t, glance.Right, gui.DrawerOnSide(p))
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 5 })
}

// A named side is not asked about: the room is the compositor's to report
// only for a drawer that chooses by it. A named left still needs its edge.
func TestANamedSideDoesNotAskForTheRoom(t *testing.T) {
	d := &compositor{left: 10, right: 5000, answers: true}
	p, await := deskPanel(t, d, coolerDetails())
	c := config.Default()
	c.DrawerSide = config.DrawerLeft
	c.Sections = []string{"cooler"}
	p.Apply(c)

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })

	rooms, held := d.asked()
	assert.Zero(t, rooms)
	assert.Equal(t, []bool{true}, held)
	assert.Equal(t, glance.Left, gui.DrawerOnSide(p))
}

// A click on another card while the compositor is still being asked is not
// two drawers: the answer to the first question is dropped.
func TestAClickWhileTheCompositorIsAskedIsNotLost(t *testing.T) {
	d := &compositor{left: 10, right: 5000, answers: true}
	src := coolerDetails()
	net := newDetailSource("bandwidth", view.Section{Title: "Bandwidth", Opens: "processes",
		Rows: []view.Row{{ID: "eth0", Label: "eth0", Value: "1"}}},
		map[string]view.Detail{"processes": view.Processes("bandwidth", view.DetailProcesses, []view.ProcessUse{{Name: "sync", Value: 9}})})
	p, await := deskPanel(t, d, src, net)

	gui.Tap(p, "cooler", "cpu", -1)
	gui.Tap(p, "bandwidth", "", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 1 })

	assert.Equal(t, "Bandwidth by process", gui.DrawerTitle(p))

	// The first click's list was never shown, so it is not being read: a
	// poll started for it would have nothing to stop it.
	time.Sleep(30 * time.Millisecond)
	await("the draws to settle", func() bool { return true })
	polls, _ := src.count("cpu")
	assert.Zero(t, polls, "the list the first click asked for is being read behind the second")
}

// A card whose section had no detail when it was built opens one that
// arrives later: what a click opens is kept with every reading, not only
// with the first.
func TestADetailThatArrivesLaterIsOpened(t *testing.T) {
	early := coolerSection()
	early.Opens = ""
	src := newDetailSource("cooler", early, map[string]view.Detail{"cpu": processes("cpu", 5, 1)})
	p, await := drawerPanel(t, src)

	gui.Tap(p, "cooler", "", -1)
	time.Sleep(20 * time.Millisecond)
	await("nothing", func() bool { return true })
	require.False(t, gui.DrawerOpen(p), "a section that names no detail opened one")

	p.Draw("cooler", coolerSection(), true)
	gui.Tap(p, "cooler", "", -1)
	await("the processor's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 5 })
}

// placed is the place the settings hold.
func placed(s *config.Store) [2]int {
	x, y, _ := s.Config().Position()
	return [2]int{x, y}
}

/*
The place the panel is remembered at is where its cards are, whatever the
drawer is doing (spec 062). A drawer on the left moves the window's corner by
its own reach, and the corner of that wider window was saved as the panel's
place: when the drawer was out, and again for a moment after it had closed,
when the compositor puts the window back to its open width. The corner is
worked out from the width the compositor reports.
*/
func TestThePanelIsRememberedWhereItsCardsAre(t *testing.T) {
	s := store(t)
	d := &compositor{left: 5000, right: 10, answers: true}
	p, await := deskPanel(t, d, coolerDetails())
	moved := func(x, y, width int) {
		gui.Moved(p, s, x, y, width)
		await("the move to be taken", func() bool { return true })
	}
	cards, reach := int(gui.CardsWidth(p)+0.5), int(gui.DrawerReach(p)+0.5)
	require.Greater(t, reach, 100)

	moved(3700, 2037, cards)
	require.Equal(t, [2]int{3700, 2037}, placed(s), "a move with the drawer closed is remembered")

	gui.Tap(p, "cooler", "", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
	require.Equal(t, glance.Left, gui.DrawerOnSide(p))
	moved(3700-reach, 2037, cards+reach) // the corner, a drawer's reach to the left
	assert.Equal(t, [2]int{3700, 2037}, placed(s), "the drawer's corner was saved as the panel's place")

	// Dragged with the drawer out, the panel is remembered where it was
	// dragged to and not where it was.
	moved(3000-reach, 2100, cards+reach)
	assert.InDelta(t, 3000, placed(s)[0], 1)
	assert.Equal(t, 2100, placed(s)[1])
	at := placed(s)

	// The compositor's corner and width are whole pixels, each rounded, so
	// the place worked out from them can be a pixel from the one held. It
	// is the same place and is not written again.
	moved(at[0]-reach+1, 2100, cards+reach)
	assert.Equal(t, at, placed(s), "a pixel of rounding was saved as a move")

	// Closed, and put back to its open width for a moment by the
	// compositor: the corner is still not the cards'.
	gui.Tap(p, "cooler", "", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	moved(3000-reach, 2100, cards+reach)
	assert.Equal(t, at, placed(s), "the open corner of a closed panel was saved as its place")
	moved(at[0], 2100, cards)
	assert.Equal(t, at, placed(s))

	// A window wider than its cards by something that is not a drawer is a
	// window somebody is making wider, and its corner is theirs.
	moved(2500, 2100, cards+77)
	assert.Equal(t, [2]int{2500, 2100}, placed(s))
}

// A drawer on the right leaves the corner where it is, so the corner of the
// wider window is the cards'.
func TestADrawerOnTheRightLeavesThePlaceAlone(t *testing.T) {
	s := store(t)
	p, await := deskPanel(t, &compositor{left: 10, right: 5000, answers: true}, coolerDetails())
	cards, reach := int(gui.CardsWidth(p)+0.5), int(gui.DrawerReach(p)+0.5)

	gui.Tap(p, "cooler", "", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
	require.Equal(t, glance.Right, gui.DrawerOnSide(p))
	gui.Moved(p, s, 500, 60, cards+reach)
	await("the move to be taken", func() bool { return true })

	assert.Equal(t, [2]int{500, 60}, placed(s))
}

// Where the desktop cannot say the window's width in the panel's own units,
// a move with the drawer out is not remembered, and one with it closed is.
func TestWithoutAWidthAMoveWithTheDrawerOutIsNotRemembered(t *testing.T) {
	s := store(t)
	require.NoError(t, s.SetConfig(s.Config().WithPosition(3690, 2037)))
	p, await := drawerPanel(t, coolerDetails())

	gui.Moved(p, s, 3700, 2037, 0)
	require.Equal(t, [2]int{3700, 2037}, placed(s))

	gui.Tap(p, "cooler", "", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
	gui.Moved(p, s, 3451, 2037, 0)
	assert.Equal(t, [2]int{3700, 2037}, placed(s), "the drawer's corner was saved as the panel's place")

	gui.Tap(p, "cooler", "", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	gui.Moved(p, s, 3702, 2040, 0)
	assert.Equal(t, [2]int{3702, 2040}, placed(s))
}
