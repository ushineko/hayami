package panel

import (
	"context"
	"errors"
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

// Cooler is the machine's own temperature: the processor from the kernel, the
// coolant and the pump from liquidctl.
type Cooler struct {
	mu      sync.Mutex
	reading view.CoolerReading
	trail   *view.Series

	// sensor and liquid are the two sources, replaced by a test so neither
	// the real hwmon tree nor a real subprocess is touched.
	sensor func() (float64, error)
	liquid func(context.Context) (cooler.Liquid, error)
}

// NewCooler builds the cooler source.
func NewCooler() *Cooler {
	return &Cooler{
		trail:  view.NewSeries(CoolerTrail),
		sensor: cooler.CPUPackage.Temperature,
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

	if v, err := c.sensor(); err == nil {
		out.CPU, out.HasCPU = v, true
	}

	liquid, err := c.liquid(ctx)
	switch {
	case err == nil:
		out.Coolant, out.HasLiquid = liquid.Coolant, true
		out.PumpRPM, out.HasPump = liquid.PumpRPM, liquid.HasPump
		out.FanRPM, out.HasFan = liquid.FanRPM, liquid.HasFan
	case errors.Is(err, cooler.ErrNoLiquidctl), errors.Is(err, cooler.ErrNoCooler):
		// Not a problem. A machine without a liquid cooler is a machine this
		// program looks at, and the processor is still worth drawing.
	default:
		// A liquidctl that ran and failed is worth neither hiding nor
		// shouting about: the section draws what it has and the error goes to
		// the caller, which logs it once.
		if !out.HasCPU {
			return false, err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if out.HasLiquid {
		c.trail.Add(out.Coolant)
	}
	out.Trail = c.trail.Samples()
	c.reading = out
	return out.HasCPU || out.HasLiquid, nil
}

// Section turns the reading into rows and a plot.
func (c *Cooler) Section() view.Section {
	c.mu.Lock()
	defer c.mu.Unlock()
	return view.Cooler(c.reading)
}

// Data is the reading as plain values, for the JSON the command line prints.
func (c *Cooler) Data() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reading
}
