package panel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/cooler"
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
// coolant and the pump from liquidctl.
type Cooler struct {
	mu      sync.Mutex
	reading view.CoolerReading
	trail   *view.Series
	cpu     *view.Averaged

	// gone marks a cooler that was answering and has stopped. The reading is
	// kept as it was and drawn dim rather than being emptied, which is the
	// monitor's rule and the reason it gives for it: a blip must not make the
	// window jump around.
	gone bool

	// reasons are what this poll could not read, rebuilt every time. They are
	// never restored from the cache and never carried over from a previous
	// poll: a reason is a statement about now.
	reasons []view.Reason

	// sensor and liquid are the two sources, replaced by a test so neither
	// the real hwmon tree nor a real subprocess is touched.
	sensor func() (float64, error)
	liquid func(context.Context) (cooler.Liquid, error)
}

// NewCooler builds the cooler source.
func NewCooler() *Cooler {
	return &Cooler{
		trail:  view.NewSeries(CoolerTrail),
		cpu:    view.NewAveraged(CoolerTrail, CPUAverageWindow),
		sensor: cooler.CPUPackage,
		liquid: cooler.Cooling,
	}
}

// Key names the section.
func (c *Cooler) Key() string { return "cooler" }

// Interval is CoolerInterval.
func (c *Cooler) Interval() time.Duration { return CoolerInterval }

// Poll takes one reading from each source.
//
// Either source alone is a section worth drawing: a machine with no liquidctl
// still has a processor, and one whose processor this build cannot find may
// still have a cooler. Neither is a section that is not drawn, which is what a
// machine without the hardware should look like.
func (c *Cooler) Poll(ctx context.Context) (bool, error) {
	var out view.CoolerReading
	var reasons []view.Reason

	if v, err := c.sensor(); err == nil {
		out.CPU, out.HasCPU = v, true
	} else {
		reasons = append(reasons, view.Reason{
			Label: "CPU", Text: "no sensor", Status: view.Info,
			// Every sensor looked for, not the last one tried: "no
			// coretemp/Package id 0" on an AMD machine sent somebody looking
			// for an Intel driver that was never going to be there.
			Detail: fmt.Sprintf("looked under %s for %s",
				cooler.HwmonRoot, cooler.CPUSensorNames()),
		})
	}

	liquid, err := c.liquid(ctx)
	switch {
	case err == nil:
		out.Coolant, out.HasLiquid = liquid.Coolant, true
		out.PumpRPM, out.HasPump = liquid.PumpRPM, liquid.HasPump
		out.FanRPM, out.HasFan = liquid.FanRPM, liquid.HasFan
	case errors.Is(err, cooler.ErrNoLiquidctl):
		// Not a problem. A machine without it is a machine this program looks
		// at, and the processor is still worth drawing -- but say so, because
		// an absent row and an absent program are different answers.
		reasons = append(reasons, view.Reason{
			Label: "Coolant", Text: "no liquidctl", Status: view.Info,
			Detail: "liquidctl is not on PATH",
		})
		err = nil
	case errors.Is(err, cooler.ErrNoCooler):
		reasons = append(reasons, view.Reason{
			Label: "Coolant", Text: "no cooler", Status: view.Info,
			Detail: match(),
		})
		err = nil
	default:
		// A liquidctl that ran and failed is a thing somebody may want to
		// fix, so it is marked rather than stated -- and it is the section's
		// own business, not only the log's. The error goes to the caller too,
		// which logs it once.
		reasons = append(reasons, view.Reason{
			Text: "liquidctl failed", Status: view.Warn, Detail: err.Error(),
		})
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.reasons = reasons
	return c.record(out), err
}

// match says which devices liquidctl was asked about, which is the first thing
// to check when a cooler that exists is not being found.
func match() string {
	if m := cooler.Match(); m != "" {
		return fmt.Sprintf("liquidctl --match %q matched nothing", m)
	}
	return "liquidctl reports no device with a liquid temperature"
}

/*
record keeps what the poll learned, or keeps what it knew.

A reading that lost something it used to have is not a reading: it is the same
cooler with a question unanswered. liquidctl opens a hidraw node and this
machine has a history of contention on those, so a poll that comes back without
the coolant is an ordinary event several times an hour -- and replacing the
reading with what just arrived would take the row away, shorten the card and
change the height of the panel, for a second, at random.

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

	// Only a sample that was actually taken goes on the plot. A trail fed the
	// value it already held would draw a flat line through an outage and call
	// it a steady temperature.
	if out.HasLiquid && !c.gone {
		c.trail.Add(out.Coolant)
	}
	if out.HasCPU && !c.gone {
		c.cpu.Add(out.CPU)
	}
	out.Trail = c.trail.Samples()
	out.CPUTrail = c.cpu.Mean()

	c.reading = out
	return out.HasCPU || out.HasLiquid
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
