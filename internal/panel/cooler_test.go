package panel_test

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/hwmon"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
kraken is a fake cooler: the candidate a fake scan lists for the nzxt driver,
and the device its Open yields.

It carries only what the cooler's reasons turn on: a status, an error from
Open or from a read (sanshoku.ErrGone, ErrAbsent, ErrUnsupported and a
permission error among them). The fields are changed between polls.
*/
type kraken struct {
	path    string
	status  cooling.Status
	openErr error
	readErr error

	opens, closes int
}

func (k *kraken) Identity() sanshoku.Identity {
	return sanshoku.Identity{Name: "NZXT Kraken Elite V2", Path: k.path}
}

func (k *kraken) Close() error { k.closes++; return nil }

func (k *kraken) Status(context.Context) (cooling.Status, error) { return k.status, k.readErr }

// rig is a machine's cooling: the processor's reading, the coolers the scan
// lists, and the nzxt driver's own failure to list.
type rig struct {
	cpu     float64
	cpuErr  error
	coolers []*kraken
	failing error

	// load is the processor's, and gpu the graphics card; a rig with neither
	// set has no load yet and no card.
	load    float64
	hasLoad bool
	gpu     core.Graphics
}

func (r *rig) scan(_ context.Context, drivers ...sanshoku.Driver) ([]sanshoku.Candidate, error) {
	var out []sanshoku.Candidate
	for _, d := range drivers {
		if d.Name() != "nzxt" {
			continue
		}
		for _, k := range r.coolers {
			out = append(out, sanshoku.Candidate{
				Identity: k.Identity(), Driver: "nzxt",
				Open: func(context.Context) (sanshoku.Device, error) {
					k.opens++
					if k.openErr != nil {
						return nil, k.openErr
					}
					return k, nil
				},
			})
		}
	}
	return out, r.failing
}

func (r *rig) sensor() (float64, error) { return r.cpu, r.cpuErr }

func (r *rig) section() *panel.Cooler {
	c := panel.NewCoolerOver(r.scan, r.sensor)
	panel.SetProcessors(c,
		func() (float64, bool) { return r.load, r.hasLoad },
		func(context.Context) core.Graphics { return r.gpu })
	return c
}

// card is a graphics card answering at 41 degrees and 4 percent.
var card = core.Graphics{Temperature: 41, HasTemperature: true, Load: 4, HasLoad: true}

// labelled is the section's row with a label.
func labelled(t *testing.T, s view.Section, label string) view.Row {
	t.Helper()
	for _, r := range s.Rows {
		if r.Label == label {
			return r
		}
	}
	require.FailNowf(t, "no such row", "%q is not a row", label)
	return view.Row{}
}

// withKraken is a machine at 60 degrees with one cooler answering.
func withKraken(coolant float64, pump int) (*rig, *kraken) {
	k := &kraken{path: "/dev/hidraw6", status: cooling.Status{Coolant: coolant, PumpRPM: pump, HasPump: true}}
	return &rig{cpu: 60, coolers: []*kraken{k}}, k
}

// The bug this is about: the cooler is a hidraw node other programs hold too,
// so a poll comes back empty now and then. Replacing the reading with what
// just arrived took the coolant row away, shortened the card and changed the
// height of the panel, for a second, at random.
func TestACoolerThatMissesAPollKeepsWhatItKnew(t *testing.T) {
	r, k := withKraken(38.9, 2650)
	c := r.section()

	require.True(t, poll(t, c))
	require.False(t, c.Section().Gone)

	k.readErr = errors.New("no 7501 reply: no reply")
	r.cpu = 61
	drawn, err := c.Poll(t.Context())

	require.Error(t, err, "the failure still reaches the caller, which logs it")
	assert.True(t, drawn, "a section that was drawn stays drawn")

	sec := c.Section()
	assert.True(t, sec.Gone, "the heading says the numbers are not live")
	require.Len(t, sec.Rows, 3)
	assert.Contains(t, sec.Rows[1].Value, "38.9", "the coolant kept its last value")
	assert.Contains(t, sec.Rows[2].Value, "2650", "and so did the pump")
}

// Recovery clears it. A marker that never goes away is a marker nobody reads.
func TestACoolerThatComesBackIsNotGoneAnyMore(t *testing.T) {
	r, k := withKraken(38.9, 2650)
	r.gpu = card
	c := r.section()
	poll(t, c)

	k.readErr = errors.New("no 7501 reply: no reply")
	_, _ = c.Poll(t.Context())

	k.readErr, k.status.Coolant = nil, 39.4
	poll(t, c)

	sec := c.Section()
	assert.False(t, sec.Gone)
	assert.Contains(t, labelled(t, sec, "Kraken Elite V2").Value, "39.4")
	assert.Empty(t, sec.Reasons, "a reason outlived the thing it was about")
}

// R2.1. The cooler is opened once and held: its driver coalesces reads by
// freshness, which only means something to a handle that lives.
func TestTheCoolerIsOpenedOnceAndHeld(t *testing.T) {
	r, k := withKraken(38.9, 2650)
	c := r.section()

	for range 3 {
		poll(t, c)
	}

	assert.Equal(t, 1, k.opens)
	assert.Zero(t, k.closes)
}

// R2.2. A cooler that has gone is closed, not reported as a failure, and
// opened afresh when the scan lists it again.
func TestACoolerThatHasGoneIsClosedAndFoundAgain(t *testing.T) {
	r, k := withKraken(38.9, 2650)
	c := r.section()
	poll(t, c)

	k.readErr = fmt.Errorf("reading /dev/hidraw6: %w", sanshoku.ErrGone)
	drawn, err := c.Poll(t.Context())
	require.NoError(t, err)
	assert.True(t, drawn)
	assert.Equal(t, 1, k.closes)
	assert.True(t, c.Section().Gone, "the numbers it had are kept, dim")

	k.readErr = nil
	poll(t, c)
	assert.Equal(t, 2, k.opens)
	assert.False(t, c.Section().Gone)
}

// A Kraken lists more than one node and only one answers. The one that does
// not is absent, not a failure, and once the cooler is held it is not asked
// again: every probe of it costs the driver's timeout.
func TestANodeThatDoesNotAnswerBesideOneThatDoesIsQuiet(t *testing.T) {
	silent := &kraken{path: "/dev/hidraw5",
		openErr: fmt.Errorf("NZXT Kraken at /dev/hidraw5 did not answer: %w", sanshoku.ErrAbsent)}
	r, k := withKraken(38.9, 2650)
	r.coolers = []*kraken{silent, k}
	r.gpu = card
	c := r.section()

	poll(t, c)
	poll(t, c)

	assert.Empty(t, c.Section().Reasons)
	assert.Equal(t, 1, silent.opens, "a node that did not answer was probed again with the cooler already held")
}

// Gone is for a source that answered and has stopped. A machine with no
// liquid cooler has never had one, and a panel that marked it stale would be
// claiming to have lost something it never had.
func TestAMachineWithNoCoolerIsNotGoneItIsAMachineWithNoCooler(t *testing.T) {
	r := &rig{cpu: 60}
	c := r.section()

	poll(t, c)
	r.cpu = 61
	poll(t, c)

	sec := c.Section()
	assert.False(t, sec.Gone)
	require.Len(t, sec.Rows, 1)
	assert.Equal(t, "CPU", sec.Rows[0].Label)
}

// A machine with neither has no section at all, rather than an empty one.
func TestAMachineWithNeitherDrawsNothing(t *testing.T) {
	r := &rig{cpuErr: hwmon.ErrNoSensor}
	c := r.section()

	assert.False(t, poll(t, c))
	assert.False(t, c.Section().Gone)
}

// The plot is a record of what was measured. A trail fed the value it already
// held would draw a flat line through an outage and call it a steady
// temperature, which is the one thing a trend line must not do.
func TestAMissedPollPutsNoSampleOnThePlot(t *testing.T) {
	r, k := withKraken(38.9, 2650)
	c := r.section()

	poll(t, c)
	before := c.Section().Trails
	require.Len(t, before, 2, "the coolant and the processor")

	k.readErr = errors.New("no 7501 reply: no reply")
	_, _ = c.Poll(t.Context())
	after := c.Section().Trails

	require.Len(t, after, 2)
	assert.Len(t, after[0].Samples, len(before[0].Samples), "the coolant gained a made-up sample")
	assert.Len(t, after[1].Samples, len(before[1].Samples))
}

// Two series, named and in order: the coolant is the primary trace and is
// given to the plot first, because a plot draws them in the order it gets them
// and the coolant is what the eye should land on.
func TestTheCoolerPlotsTheCoolantAndTheProcessor(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	c := r.section()

	poll(t, c)

	trails := c.Section().Trails
	require.Len(t, trails, 2)
	assert.Equal(t, "Coolant", trails[0].Name)
	assert.Equal(t, "CPU", trails[1].Name)
	assert.InDelta(t, 60.0, trails[1].Samples[0], 0.001,
		"a partial window is averaged as it stands, so the trace starts on the first sample")
}

/*
R3.1. A cooler on a machine with no cooler draws the processor and says why the
coolant is missing.

The shape the panel had on the machine that prompted this: the processor was
being read perfectly well and the whole card was hidden, because the coolant's
absence was treated as a fault (issue #54).
*/
func TestACoolerWithNoLiquidDrawsTheProcessorAndSaysWhy(t *testing.T) {
	c := (&rig{cpu: 38}).section()

	drawn, err := c.Poll(t.Context())
	require.NoError(t, err, "a machine with no cooler is not a machine with a problem")
	assert.True(t, drawn)

	sec := c.Section()
	r := find(t, sec, "no cooler")
	assert.Equal(t, view.Info, r.Status, "absent hardware must not be marked as a failure")
	assert.Equal(t, "Coolant", r.Label)
	assert.NotEmpty(t, r.Detail, "the reason should say what was looked for")
}

// R3.4. A cooler that would not answer is marked, and keeps its error where a
// reader can reach it.
func TestACoolerThatWouldNotAnswerIsMarkedAndKeepsItsError(t *testing.T) {
	k := &kraken{path: "/dev/hidraw6", openErr: errors.New("open /dev/hidraw6: input/output error")}
	c := (&rig{cpu: 38, coolers: []*kraken{k}}).section()

	drawn, err := c.Poll(t.Context())
	require.Error(t, err, "a failure is still reported to the caller that logs it")
	assert.True(t, drawn, "the processor is still worth drawing")

	sec := c.Section()
	r := find(t, sec, "the cooler would not answer")
	assert.Equal(t, view.Warn, r.Status, "a source that failed is not the same as hardware that is absent")
	assert.Contains(t, r.Detail, "input/output error")
	assert.NotContains(t, sec.Lines()[len(sec.Lines())-1].Value, "input/output",
		"an error is not a glance; it belongs in the hover")
}

// R3.4. The nzxt driver failing to list is the same fact in the same words.
func TestACoolerDriverThatFailsToListWouldNotAnswer(t *testing.T) {
	c := (&rig{cpu: 38, failing: errors.New("nzxt: listing hidraw: permission denied")}).section()

	_, err := c.Poll(t.Context())

	require.Error(t, err)
	assert.Equal(t, view.Warn, find(t, c.Section(), "the cooler would not answer").Status)
}

// R3.2. A cooler that may not be opened says what to do: on Linux the udev
// rule, which liquidctl's package used to install for us; on Windows the
// program holding it (spec 035).
func TestACoolerThatMayNotBeOpenedSaysWhatToDo(t *testing.T) {
	k := &kraken{path: "/dev/hidraw6", openErr: &fsError{syscall.EPERM}}
	c := (&rig{cpu: 38, coolers: []*kraken{k}}).section()

	drawn, err := c.Poll(t.Context())

	require.NoError(t, err)
	assert.True(t, drawn)
	r := find(t, c.Section(), "NZXT Kraken Elite V2 is not permitted")
	assert.Equal(t, view.Warn, r.Status)
	assert.Equal(t, panel.PermissionDetail, r.Detail)
	assert.NotContains(t, reasonTexts(c.Section()), "no cooler",
		"a cooler that is there and may not be opened is not an absent cooler")
}

// R3.3. An NZXT product the driver will not write to is named and left alone.
func TestAnUnsupportedCoolerIsNamed(t *testing.T) {
	k := &kraken{path: "/dev/hidraw6",
		openErr: fmt.Errorf("NZXT Kraken X (1e71:2007): not a product this driver speaks to: %w", sanshoku.ErrUnsupported)}
	c := (&rig{cpu: 38, coolers: []*kraken{k}}).section()

	poll(t, c)

	r := find(t, c.Section(), "unsupported")
	assert.Equal(t, "NZXT Kraken Elite V2", r.Label)
	assert.Equal(t, view.Info, r.Status)
}

// A cooler with both readings says nothing extra. A card that explained itself
// while showing its numbers would be a panel talking about itself.
func TestACoolerThatReadsEverythingSaysNothingExtra(t *testing.T) {
	r, _ := withKraken(30, 2000)
	r.gpu = card
	c := r.section()

	poll(t, c)

	assert.Empty(t, c.Section().Reasons)
}

// A reason is a statement about now. A poll that recovers must take its reason
// away rather than leaving it under a live reading.
func TestARecoveredSourceDropsItsReason(t *testing.T) {
	r := &rig{cpu: 38, gpu: card}
	c := r.section()
	poll(t, c)
	require.NotEmpty(t, c.Section().Reasons)

	_, k := withKraken(30, 2000)
	r.coolers = []*kraken{k}
	poll(t, c)

	assert.Empty(t, c.Section().Reasons, "a reason outlived the thing it was about")
}

// R1.2, R2.2. The processor and the graphics card are one line each, load and
// temperature, and the card's temperature is a third trail beside the others.
func TestTheProcessorsAreOneLineEachAndTheCardIsATrail(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.load, r.hasLoad, r.gpu = 12, true, card
	c := r.section()

	poll(t, c)
	sec := c.Section()

	assert.Equal(t, " 12 %  60.0", labelled(t, sec, "CPU").Value)
	assert.Equal(t, "  4 %  41.0", labelled(t, sec, "GPU").Value)
	require.Len(t, sec.Trails, 3)
	assert.Equal(t, "GPU", sec.Trails[2].Name)
	assert.Equal(t, []float64{41}, sec.Trails[2].Samples)
	assert.Empty(t, sec.Reasons)
}

// R1.2. No card is no row, and a reason kept off the card for doctor.
func TestNoGraphicsCardIsNoRowAndAReasonForDoctor(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	c := r.section()

	poll(t, c)
	sec := c.Section()

	for _, row := range sec.Rows {
		assert.NotEqual(t, "GPU", row.Label)
	}
	reason := find(t, sec, "no GPU sensor")
	assert.Equal(t, view.Info, reason.Status)
	assert.True(t, reason.Aside, "a machine without a card is not told so on the card")
	assert.Contains(t, reason.Detail, "nvidia-smi")
}

// A card that was answering and has stopped -- nvidia-smi timing out under
// load -- keeps its row, dim, and only its row: the card is not Gone and the
// CPU and coolant stay live. A row that came and went would move the panel.
func TestACardThatMissesAPollKeepsItsRow(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.gpu = card
	c := r.section()
	poll(t, c)

	r.gpu = core.Graphics{}
	poll(t, c)
	sec := c.Section()

	assert.False(t, sec.Gone, "a late card does not dim the cooler")
	gpu := labelled(t, sec, "GPU")
	assert.Contains(t, gpu.Value, "41.0")
	assert.True(t, gpu.Stale)
	assert.False(t, labelled(t, sec, "CPU").Stale)
	assert.False(t, labelled(t, sec, "Kraken Elite V2").Stale)
	assert.Len(t, sec.Trails[2].Samples, 1, "a missed poll is not a sample")

	// And the next answer is live again.
	r.gpu = card
	poll(t, c)
	assert.False(t, labelled(t, c.Section(), "GPU").Stale)
}

// Spec 031, R2.1-R2.3. Each row is labelled with the part it reads: the
// processor's model, the card's, and the cooler's as sanshoku identifies it.
// The full names are the rows' tips.
func TestTheRowsAreNamedForTheirHardware(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.gpu = card
	r.gpu.Name = "NVIDIA GeForce RTX 4090"
	c := r.section()
	panel.SetCPUName(c, func() string { return "Intel(R) Core(TM) i9-14900K" })

	poll(t, c)
	sec := c.Section()

	assert.Equal(t, "CPU: Intel(R) Core(TM) i9-14900K", labelled(t, sec, "i9-14900K").Tip)
	assert.Equal(t, "GPU: NVIDIA GeForce RTX 4090", labelled(t, sec, "RTX 4090").Tip)
	assert.Equal(t, "Coolant: NZXT Kraken Elite V2", labelled(t, sec, "Kraken Elite V2").Tip)
}

// R2.5. The processor's model is read once, not every five seconds: it does
// not change while the machine is up.
func TestTheProcessorIsNamedOnce(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	c := r.section()
	asked := 0
	panel.SetCPUName(c, func() string { asked++; return "AMD Ryzen 9 7950X 16-Core Processor" })

	poll(t, c)
	poll(t, c)

	assert.Equal(t, 1, asked)
	labelled(t, c.Section(), "Ryzen 9 7950X")
}

// R2.5. A card whose name did not come with this poll keeps the one it had,
// rather than going back to "GPU" for five seconds.
func TestANameOnceHeardIsKept(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.gpu = card
	r.gpu.Name = "NVIDIA GeForce RTX 3080"
	c := r.section()
	poll(t, c)

	r.gpu.Name = ""
	poll(t, c)

	labelled(t, c.Section(), "RTX 3080")
}

// Spec 034. A processor with a load and no temperature -- every Windows
// machine -- is a row of its own, named, and the reason the temperature is
// missing is kept off the card: the row already says what it can.
func TestAProcessorWithALoadAndNoTemperatureIsARow(t *testing.T) {
	r := &rig{cpuErr: hwmon.ErrNoSensor, load: 12, hasLoad: true}
	c := r.section()
	panel.SetCPUName(c, func() string { return "AMD Ryzen 5 2600X Six-Core Processor" })

	drawn, err := c.Poll(context.Background())
	require.NoError(t, err)
	sec := c.Section()

	assert.True(t, drawn, "a load alone is a section worth drawing")
	row := labelled(t, sec, view.NameLabel("CPU", "AMD Ryzen 5 2600X Six-Core Processor"))
	assert.Equal(t, " 12 %      ", row.Value, "the temperature's column is kept, empty")
	assert.Equal(t, "AMD Ryzen 5 2600X Six-Core Processor", c.Data().(view.CoolerReading).CPUName)
	reason := find(t, sec, "no sensor")
	assert.True(t, reason.Aside, "a row with a load is not told on the card that it has no temperature")
}

// Spec 034. With neither a load nor a temperature there is no row, and the
// reason is the only word about the processor, so it stays on the card.
func TestAProcessorWithNothingIsNoRowAndAReasonOnTheCard(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.cpuErr = hwmon.ErrNoSensor
	c := r.section()

	poll(t, c)
	sec := c.Section()

	for _, row := range sec.Rows {
		assert.NotEqual(t, "CPU", row.Label)
	}
	assert.False(t, find(t, sec, "no sensor").Aside)
}
