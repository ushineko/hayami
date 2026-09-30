package panel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/cooling"
	"github.com/ushineko/sanshoku/hwmon"
	"github.com/ushineko/sanshoku/nzxt"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

// CoolerInterval is how often the cooler is asked.
//
// Five seconds. A thermal probe and a byte counter share a window and nothing
// else: coolant moves over minutes, and one of these polls starts a subprocess.
const CoolerInterval = 5 * time.Second

// CoolerTrail is how many samples the coolant's plot holds.
//
// Sixty at five seconds is five minutes, which is the monitor's window and
// about as far back as a temperature is still worth looking at.
const CoolerTrail = 60

// CPUAverageWindow is how many samples the processor's trailing mean covers.
//
// Twelve at five seconds is a minute, which is the monitor's window and the
// number it settled on for the same reading on the same machine. Shorter and
// the trace is still a compile's spike; longer and it stops following
// anything.
const CPUAverageWindow = 12

// Cooler is the machine's own temperature: the processor from the kernel, the
// coolant and the pump from the cooler itself.
type Cooler struct {
	mu      sync.Mutex
	reading view.CoolerReading
	trail   *view.Series
	cpu     *view.Averaged
	gpu     *view.Averaged

	// gone marks a cooler that was answering and has stopped. The reading is
	// kept as it was and drawn dim rather than being emptied, which is the
	// monitor's rule and the reason it gives for it: a blip must not make the
	// window jump around.
	gone bool

	// reasons are what this poll could not read, rebuilt every time. They are
	// never restored from the cache and never carried over from a previous
	// poll: a reason is a statement about now.
	reasons []view.Reason

	// polling serialises Poll, which owns the held cooler.
	polling sync.Mutex

	// sensor is the processor's temperature, and held the cooler open across
	// polls with the scan that finds it. A test replaces both, so neither the
	// real hwmon tree nor a real device is touched.
	sensor func() (float64, error)
	held   *held

	// load is the processor's utilisation since the previous poll, and
	// graphics the card's temperature and load. Absent unless NewCooler sets
	// them, so a test that does not ask for them reads neither /proc/stat nor
	// the card -- and never runs nvidia-smi.
	load     func() (float64, bool)
	graphics func(context.Context) core.Graphics
}

// coolerDrivers are the drivers the cooler section asks.
func coolerDrivers() []sanshoku.Driver { return []sanshoku.Driver{nzxt.Driver{}} }

// NewCooler builds the cooler source over sanshoku's NZXT driver and the
// kernel's processor sensors.
func NewCooler() *Cooler {
	c := newCooler(sanshoku.Scan, cpuPackage)
	c.load = core.NewCPULoad(core.ProcStatPath).Load
	c.graphics = core.NewGraphicsReader().Read
	return c
}

// newCooler builds the source over a scan and a processor sensor, which is
// the seam the tests use.
func newCooler(scan Scan, sensor func() (float64, error)) *Cooler {
	return &Cooler{
		trail:  view.NewSeries(CoolerTrail),
		cpu:    view.NewAveraged(CoolerTrail, CPUAverageWindow),
		gpu:    view.NewAveraged(CoolerTrail, CPUAverageWindow),
		sensor: sensor,
		held:   newHeld(scan),

		load:     func() (float64, bool) { return 0, false },
		graphics: func(context.Context) core.Graphics { return core.Graphics{} },
	}
}

// cpuPackage is the first processor sensor this machine has, in hwmon.CPU's
// order.
func cpuPackage() (float64, error) {
	_, v, err := hwmon.First(hwmon.Root, hwmon.CPU)
	if err != nil {
		return 0, fmt.Errorf("reading the processor temperature: %w", err)
	}
	return v, nil
}

// cpuSensors names every sensor looked for, for the reason given when none
// reads: "no coretemp/Package id 0" on an AMD machine sent somebody looking
// for an Intel driver that was never going to be there.
func cpuSensors() string {
	names := make([]string, 0, len(hwmon.CPU))
	for _, s := range hwmon.CPU {
		names = append(names, s.String())
	}
	return strings.Join(names, ", ")
}

// gpuSensors is every route to the card's temperature, for the reason given
// when none answers.
func gpuSensors() string {
	names := make([]string, 0, len(hwmon.GPU)+1)
	for _, s := range hwmon.GPU {
		names = append(names, s.String())
	}
	return strings.Join(append(names, "nvidia-smi"), ", ")
}

// Key names the section.
func (c *Cooler) Key() string { return "cooler" }

// Interval is CoolerInterval.
func (c *Cooler) Interval() time.Duration { return CoolerInterval }

// Poll takes one reading from each source.
//
// Either source alone is a section worth drawing: a machine with no liquid
// cooler still has a processor, and one whose processor this build cannot find
// may still have a cooler. Neither is a section that is not drawn, which is
// what a machine without the hardware should look like.
func (c *Cooler) Poll(ctx context.Context) (bool, error) {
	c.polling.Lock()
	defer c.polling.Unlock()

	var out view.CoolerReading
	var reasons []view.Reason

	if v, err := c.sensor(); err == nil {
		out.CPU, out.HasCPU = v, true
	} else {
		reasons = append(reasons, view.Reason{
			Label: "CPU", Text: "no sensor", Status: view.Info,
			// Every sensor looked for, not the last one tried.
			Detail: fmt.Sprintf("looked under %s for %s", hwmon.Root, cpuSensors()),
		})
	}
	out.CPULoad, out.HasCPULoad = c.load()

	if g := c.graphics(ctx); g.HasTemperature {
		out.GPU, out.HasGPU = g.Temperature, true
		out.GPULoad, out.HasGPULoad = g.Load, g.HasLoad
	} else {
		// Aside: most machines have no card this build can read, and a line
		// saying so on every one of them would be the card talking about
		// itself. Doctor and the hover note still say it.
		reasons = append(reasons, view.Reason{
			Text: "no GPU sensor", Status: view.Info, Aside: true,
			Detail: fmt.Sprintf("looked under %s and tried %s", hwmon.Root, gpuSensors()),
		})
	}

	status, why, err := c.liquid(ctx)
	reasons = append(reasons, why...)
	if status != nil {
		out.Coolant, out.HasLiquid = status.Coolant, true
		out.PumpRPM, out.HasPump = status.PumpRPM, status.HasPump
		out.FanRPM, out.HasFan = status.FanRPM, status.HasFan
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.reasons = uniqueReasons(reasons)
	return c.record(out), err
}

/*
liquid is the cooler's status, and the reasons it has none.

One cooler. A Kraken lists more than one node and only one answers, so once a
device is held no further candidate is opened: opening a node that does not
answer costs the driver's probe timeout, every five seconds, for nothing.
*/
func (c *Cooler) liquid(ctx context.Context) (*cooling.Status, []view.Reason, error) {
	var reasons []view.Reason
	var errs []error

	failed := func(err error) {
		// A cooler that is there and would not answer is a thing somebody may
		// want to fix, so it is marked rather than stated -- and it is the
		// section's own business, not only the log's. The error goes to the
		// caller too, which logs it once.
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: "the cooler would not answer", Status: view.Warn, Detail: err.Error(),
		})
	}

	c.held.begin()
	var candidates []sanshoku.Candidate
	for _, d := range coolerDrivers() {
		found, err := c.held.list(ctx, d)
		if err != nil {
			failed(err)
		}
		candidates = append(candidates, found...)
	}
	c.held.prune()

	var status *cooling.Status
	for _, cand := range candidates {
		if !c.held.has(cand) && len(c.held.devices) > 0 {
			continue
		}
		dev, err := c.held.device(ctx, cand)
		if err != nil {
			r, bad := openFailure(cand, err)
			switch {
			case bad:
				failed(err)
			case r != nil:
				reasons = append(reasons, *r)
			}
			continue
		}
		src, ok := dev.(cooling.Source)
		if !ok {
			continue
		}
		s, err := src.Status(ctx)
		switch {
		case err == nil:
			if status == nil {
				status = &s
			}
		case errors.Is(err, sanshoku.ErrGone):
			c.held.drop(cand)
		default:
			failed(fmt.Errorf("reading the cooler: %w", err))
		}
	}

	if status == nil && len(reasons) == 0 {
		reasons = append(reasons, view.Reason{
			Label: "Coolant", Text: "no cooler", Status: view.Info,
			Detail: "no NZXT Kraken answered on USB",
		})
	}
	return status, reasons, errors.Join(errs...)
}

/*
record keeps what the poll learned, or keeps what it knew.

A reading that lost something it used to have is not a reading: it is the same
cooler with a question unanswered. The cooler is a hidraw node other programs
hold too, so a poll that comes back without the coolant is an ordinary event --
and replacing the reading with what just arrived would take the row away,
shorten the card and change the height of the panel, for a second, at random.

So a field that was there and is not is kept and the section is marked Gone,
which dims every row and says so in the heading. The monitor does the same and
gives the same reason: keep the last values but dim them, so a blip does not
make the window jump around.

A field that was never there stays absent. Gone is for a source that answered
and has stopped, not for hardware this machine does not have.
*/
func (c *Cooler) record(out view.CoolerReading) bool {
	c.gone = false

	if !out.HasCPU && c.reading.HasCPU {
		out.CPU, out.HasCPU = c.reading.CPU, true
		c.gone = true
	}
	if !out.HasLiquid && c.reading.HasLiquid {
		out.Coolant, out.HasLiquid = c.reading.Coolant, true
		out.PumpRPM, out.HasPump = c.reading.PumpRPM, c.reading.HasPump
		out.FanRPM, out.HasFan = c.reading.FanRPM, c.reading.HasFan
		c.gone = true
	}

	// The card's miss is the card's row. nvidia-smi can miss its timeout on a
	// machine under load, and that is one reading late, not the cooler gone:
	// the GPU row keeps its value and is drawn dim, and the rest of the card
	// stays live. It is decided after the coolant's rule so that rule's Gone
	// is not triggered by the card.
	if !out.HasGPU && c.reading.HasGPU {
		out.GPU, out.HasGPU = c.reading.GPU, true
		out.GPULoad, out.HasGPULoad = c.reading.GPULoad, c.reading.HasGPULoad
		out.GPUStale = true
	}

	// Only a sample that was actually taken goes on the plot. A trail fed the
	// value it already held would draw a flat line through an outage and call
	// it a steady temperature.
	if out.HasLiquid && !c.gone {
		c.trail.Add(out.Coolant)
	}
	if out.HasCPU && !c.gone {
		c.cpu.Add(out.CPU)
	}
	if out.HasGPU && !out.GPUStale && !c.gone {
		c.gpu.Add(out.GPU)
	}
	out.Trail = c.trail.Samples()
	out.CPUTrail = c.cpu.Mean()
	out.GPUTrail = c.gpu.Mean()

	c.reading = out
	return out.HasCPU || out.HasGPU || out.HasLiquid
}

// Section turns the reading into rows and a plot.
func (c *Cooler) Section() view.Section {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := view.Cooler(c.reading)
	s.Gone = c.gone
	s.Reasons = c.reasons
	return s
}

// Data is the reading as plain values, for the JSON the command line prints.
func (c *Cooler) Data() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reading
}
