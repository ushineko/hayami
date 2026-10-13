package gui_test

import (
	"context"
	"fmt"
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

func (d *detailSource) Details() []view.DetailInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []view.DetailInfo
	for id, detail := range d.details {
		out = append(out, view.DetailInfo{ID: id, Title: detail.Title})
	}
	return out
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

// Spec 060 R3. A click on a card opens that section's detail, and a second
// click on the same thing closes the drawer.
func TestAClickOnACardOpensItsDetailAndASecondClosesIt(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)
	require.False(t, gui.DrawerOpen(p))

	gui.Tap(p, "cooler", "coolant", -1) // a row with no detail of its own
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 5 })

	assert.Equal(t, "CPU by process", gui.DrawerTitle(p))
	section, id := gui.DrawerSubject(p)
	assert.Equal(t, "cooler", section)
	assert.Equal(t, "cpu", id, "the card's own detail is the processor's")
	assert.True(t, strings.HasPrefix(gui.DrawerLines(p)[0], "p00|"), "greatest first: %v", gui.DrawerLines(p))

	gui.Tap(p, "cooler", "", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
}

// R3. A click on a row with a detail of its own opens the row's; a click on
// another changes what is shown and leaves the drawer open.
func TestAClickOnAnotherRowChangesWhatIsShown(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)

	gui.Tap(p, "cooler", "memory", -1)
	await("the memory's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 4 })
	assert.Equal(t, "Memory by process", gui.DrawerTitle(p))
	assert.Contains(t, gui.DrawerLines(p)[0], "GiB")

	gui.Tap(p, "cooler", "gpu", -1)
	await("the card's list", func() bool { return len(gui.DrawerLines(p)) == 3 })
	assert.True(t, gui.DrawerOpen(p), "the drawer stayed open")
	assert.Equal(t, "GPU by process", gui.DrawerTitle(p))
	_, closed := src.count("memory")
	assert.Equal(t, 1, closed, "the list no longer shown was let go")

	gui.Tap(p, "cooler", "gpu", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
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
// neither does a row that names a detail its source does not offer.
func TestACardWithNoDetailDoesNothing(t *testing.T) {
	other := plain{detailSourceless{key: "peripherals", section: view.Section{Key: "peripherals", Title: "Peripherals",
		Opens: "anything", Rows: []view.Row{{ID: "mouse", Label: "Mouse", Value: " 80 %", Opens: "anything"}}}}}
	src := newDetailSource("cooler", coolerSection(), map[string]view.Detail{"cpu": processes("cpu", 2, 1)})
	p, await := drawerPanel(t, other, src)

	gui.Tap(p, "peripherals", "", -1)
	gui.Tap(p, "peripherals", "mouse", -1)
	gui.Tap(p, "cooler", "gpu", -1) // the view names it; this source does not offer it
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

// R13, R15, R17. A click on a meter opens that account's breakdown: its
// limits as meters, its lines under them, and no filter.
func TestAClickOnAMeterOpensItsBreakdown(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	section := view.Usage(now, []view.UsageWindow{
		{Account: "CC desk", Name: "5h", Fraction: 0.10, ResetsAt: now.Add(2 * time.Hour)},
		{Account: "CX", Name: "5h", Fraction: 0.20, ResetsAt: now.Add(2 * time.Hour)},
	}, now)
	breakdown := func(account string, limits int) view.Detail {
		b := view.UsageBreakdown{Account: account, SharesTitle: "Week by use",
			Shares: []view.UsageShare{{Name: "Chats", Fraction: 0.25}}}
		for i := range limits {
			b.Limits = append(b.Limits, view.UsageLimit{Name: fmt.Sprintf("Limit %d", i), Fraction: 0.5})
		}
		return view.UsageDetail(now, b)
	}
	src := newDetailSource("usage", section, map[string]view.Detail{
		"CC desk": breakdown("CC desk", 3), "CX": breakdown("CX", 2),
	})
	p, await := drawerPanel(t, src)

	gui.Tap(p, "usage", "", 1)
	await("the second account", func() bool { return gui.DrawerOpen(p) && gui.DrawerMeterCount(p) == 2 })
	assert.Equal(t, "CX", gui.DrawerTitle(p))
	assert.Equal(t, []string{"Week by use", "Chats| 25 %"}, gui.DrawerLines(p))

	gui.FilterType(p, "fzzz")
	await("nothing", func() bool { return true })
	assert.Empty(t, gui.FilterText(p), "a breakdown is not filtered")
	assert.Len(t, gui.DrawerLines(p), 2)

	gui.Tap(p, "usage", "", 0)
	await("the first account", func() bool { return gui.DrawerMeterCount(p) == 3 })
	assert.Equal(t, "CC desk", gui.DrawerTitle(p))
}

// A detail that could not be read shows its reason where the list would be.
func TestADetailThatCouldNotBeReadShowsItsReason(t *testing.T) {
	src := newDetailSource("bandwidth", view.Section{Title: "Bandwidth", Opens: "processes",
		Rows: []view.Row{{ID: "eth0", Label: "eth0", Value: "1"}}}, map[string]view.Detail{
		"processes": {Section: "bandwidth", ID: "processes", Title: "Bandwidth by process",
			Reasons: []view.Reason{{Text: "needs an administrator", Detail: "a long sentence nobody draws"}}},
	})
	p, await := drawerPanel(t, src)

	gui.Tap(p, "bandwidth", "eth0", -1)
	await("the reason", func() bool { return len(gui.DrawerNoteLines(p)) == 1 })

	assert.Equal(t, "Bandwidth by process", gui.DrawerTitle(p))
	assert.Equal(t, []string{"needs an administrator"}, gui.DrawerNoteLines(p))
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

// R20. In the row arrangement a click on a line opens its detail as in the
// others.
func TestARowArrangementOpensDetailsToo(t *testing.T) {
	src := coolerDetails()
	p, await := drawerPanel(t, src)
	c := config.Default()
	c.Arrangement = "row"
	c.Sections = []string{"cooler"}
	p.Apply(c)

	gui.Tap(p, "cooler", "memory", -1)
	await("the memory's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) > 0 })

	assert.Equal(t, "Memory by process", gui.DrawerTitle(p))
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
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 4 })
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

// A second click while the compositor is still being asked is not two
// drawers: the answer to the first question is dropped.
func TestAClickWhileTheCompositorIsAskedIsNotLost(t *testing.T) {
	d := &compositor{left: 10, right: 5000, answers: true}
	src := coolerDetails()
	p, await := deskPanel(t, d, src)

	gui.Tap(p, "cooler", "cpu", -1)
	gui.Tap(p, "cooler", "memory", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 4 })

	assert.Equal(t, "Memory by process", gui.DrawerTitle(p))

	// The first click's list was never shown, so it is not being read: a
	// poll started for it would have nothing to stop it.
	time.Sleep(30 * time.Millisecond)
	await("the draws to settle", func() bool { return true })
	polls, _ := src.count("cpu")
	assert.Zero(t, polls, "the list the first click asked for is being read behind the second")
}

// A row that arrives after the card was built opens its detail like one that
// was always there: what a click opens is kept with every reading, not only
// with the first.
func TestARowThatArrivesLaterOpensItsDetail(t *testing.T) {
	early := view.Cooler(view.CoolerReading{Probes: []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Load: view.Some(12.0), Temp: view.Some(51.5)},
	}})
	src := newDetailSource("cooler", early, map[string]view.Detail{
		"cpu": processes("cpu", 5, 1), "memory": processes("memory", 4, 1<<30),
	})
	p, await := drawerPanel(t, src)

	p.Draw("cooler", coolerSection(), true) // the memory's row arrives
	gui.Tap(p, "cooler", "memory", -1)
	await("the memory's list", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerLines(p)) == 4 })

	assert.Equal(t, "Memory by process", gui.DrawerTitle(p))
}

// The place the panel is remembered at is the cards' and not the drawer's.
// A drawer on the left moves the window's corner by its own width; that
// corner is not saved while the drawer is out, and the place reported when
// it has closed is.
func TestThePanelsPlaceIsNotSavedWhileTheDrawerIsOut(t *testing.T) {
	s := store(t)
	require.NoError(t, s.SetConfig(s.Config().WithPosition(3690, 2037)))
	p, await := drawerPanel(t, coolerDetails())

	gui.Moved(p, s, 3700, 2037)
	x, _, _ := s.Config().Position()
	require.Equal(t, 3700, x, "a move with the drawer closed is remembered")

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to open", func() bool { return gui.DrawerOpen(p) })
	gui.Moved(p, s, 3451, 2037) // the corner, a drawer's width to the left
	x, _, _ = s.Config().Position()
	assert.Equal(t, 3700, x, "the drawer's corner was saved as the panel's place")

	gui.Tap(p, "cooler", "cpu", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	gui.Moved(p, s, 3702, 2040)
	x, y, _ := s.Config().Position()
	assert.Equal(t, 3702, x)
	assert.Equal(t, 2040, y)
}
