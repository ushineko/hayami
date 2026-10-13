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
	reading coolerState
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
	sensor func(context.Context) (float64, error)
	held   *held

	// load is the processor's utilisation since the previous poll, and
	// graphics the card's temperature and load. Absent unless NewCooler sets
	// them, so a test that does not ask for them reads neither /proc/stat nor
	// the card -- and never runs nvidia-smi.
	load     func() (float64, bool)
	graphics func(context.Context) (core.Graphics, error)

	// cpuName is the processor's model, read on the first poll that asks and
	// kept: it does not change while the machine is up. Empty unless
	// NewCooler sets it, for the reason load is.
	cpuName  func() string
	cpuNamed bool
	cpuModel string

	// cpuDetail and gpuDetail are the platform's account of a missing
	// temperature, for an error that gives none of its own: every route the
	// host's chain has (spec 043).
	cpuDetail func() string
	gpuDetail func() string

	// memory is the memory in use, and nil for a source that reads none: a
	// test that does not ask. processes and gpuProcesses are the readers the
	// details ask, nil likewise; gpuVia is the provider the last reading of
	// the card came from, which the list of its processes is chosen by
	// (spec 060).
	memory       func(context.Context) (core.Memory, error)
	processes    *core.ProcessList
	gpuProcesses core.GPUProcessReader
	gpuVia       string
}

// coolerDrivers are the drivers the cooler section asks.
func coolerDrivers() []sanshoku.Driver { return []sanshoku.Driver{nzxt.Driver{}} }

// NewCooler builds the cooler source over sanshoku's NZXT driver and the
// host's chains: the processor's temperature, its load and name, and the
// graphics card (spec 043), over scan.
func NewCooler(h *core.Host, scan Scan) *Cooler {
	c := newCoolerOn(h, scan, h.ReadCPUTemperature)
	c.load = h.CPULoad().Load
	c.graphics = h.ReadGraphics
	c.cpuName = h.CPUName
	c.memory = h.ReadMemory
	if h.Processes != nil {
		c.processes = h.Processes()
	}
	c.gpuProcesses = h.GPUProcesses
	return c
}

// newCooler builds the source over a scan and a processor sensor, which is
// the seam the tests use: nothing else is read, and a missing reading is
// accounted for in this platform's words.
func newCooler(scan Scan, sensor func(context.Context) (float64, error)) *Cooler {
	return newCoolerOn(platform(), scan, sensor)
}

// newCoolerOn is newCooler with the host whose words a missing reading gets.
func newCoolerOn(h *core.Host, scan Scan, sensor func(context.Context) (float64, error)) *Cooler {
	return &Cooler{
		trail:  view.NewSeries(CoolerTrail),
		cpu:    view.NewAveraged(CoolerTrail, CPUAverageWindow),
		gpu:    view.NewAveraged(CoolerTrail, CPUAverageWindow),
		sensor: sensor,
		held:   newHeld(scan, h.Permission),

		load:      func() (float64, bool) { return 0, false },
		graphics:  func(context.Context) (core.Graphics, error) { return core.Graphics{}, nil },
		cpuName:   func() string { return "" },
		cpuDetail: h.CPUSensorDetail,
		gpuDetail: h.GPUSensorDetail,
	}
}

// Key names the section.
func (c *Cooler) Key() string { return view.CoolerInfo.Key }

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

	var out coolerState
	var reasons []view.Reason

	out.CPULoad, out.HasCPULoad = c.load()
	if v, err := c.sensor(ctx); err == nil {
		out.CPU, out.HasCPU = v, true
	} else {
		// Aside where the row is drawn anyway, on its load (spec 034): on
		// Windows that is every machine, and a line under the row saying the
		// temperature is missing would be the card talking about itself.
		// Where there is no row it is the only word about the processor, and
		// is said on the card.
		reasons = append(reasons, reason(view.Reason{
			Label: "CPU", Text: "no sensor", Status: view.Info, Aside: out.HasCPULoad,
			// Every sensor looked for, not the last one tried -- or, where
			// the source can say which way it is missing, that (spec 036).
			Detail: c.cpuDetail(),
		}, err))
	}
	if out.HasCPU || out.HasCPULoad {
		out.CPUName = c.processorName()
	}

	if g, err := c.graphics(ctx); g.HasTemperature {
		out.GPU, out.HasGPU = g.Temperature, true
		out.GPULoad, out.HasGPULoad = g.Load, g.HasLoad
		out.GPUName = g.Name
		out.gpuVia = g.Via
	} else {
		// Aside: most machines have no card this build can read, and a line
		// saying so on every one of them would be the card talking about
		// itself. Doctor and the hover note still say it, naming every route
		// the chain tried.
		reasons = append(reasons, reason(view.Reason{
			Text: "no GPU sensor", Status: view.Info, Aside: true,
			Detail: c.gpuDetail(),
		}, err))
	}

	if c.memory != nil {
		if m, err := c.memory(ctx); err == nil {
			out.Memory, out.MemoryTotal, out.HasMemory = m.Used, m.Total, true
		} else {
			// Aside: a machine whose memory cannot be read has a card without
			// the row, and the hover note and doctor say why.
			reasons = append(reasons, reason(view.Reason{
				Label: "Memory", Text: "not read", Status: view.Info, Aside: true,
			}, err))
		}
	}

	status, name, why, err := c.liquid(ctx)
	reasons = append(reasons, why...)
	if status != nil {
		out.Coolant, out.HasLiquid = status.Coolant, true
		out.CoolerName = name
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
//
// The name is the cooler's own, as sanshoku identifies it ("NZXT Kraken Elite
// V2"), for the coolant's label (spec 031).
func (c *Cooler) liquid(ctx context.Context) (*cooling.Status, string, []view.Reason, error) {
	var reasons []view.Reason
	var errs []error

	failed := func(err error) {
		// A cooler that is there and would not answer is a thing somebody may
		// want to fix, so it is marked rather than stated -- and it is the
		// section's own business, not only the log's. The error goes to the
		// caller too, which logs it once.
		errs = append(errs, err)
		reasons = append(reasons, reason(view.Reason{
			Text: "the cooler would not answer", Status: view.Warn,
		}, err))
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
	name := ""
	for _, cand := range candidates {
		if !c.held.has(cand) && len(c.held.devices) > 0 {
			continue
		}
		dev, err := c.held.device(ctx, cand)
		if err != nil {
			r, bad := c.held.openFailure(cand, err)
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
				name = dev.Identity().Name
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
			Detail: "no supported cooler detected",
		})
	}
	return status, name, reasons, errors.Join(errs...)
}

// processorName is the processor's model, read once.
func (c *Cooler) processorName() string {
	if !c.cpuNamed {
		c.cpuNamed = true
		c.cpuModel = c.cpuName()
	}
	return c.cpuModel
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
func (c *Cooler) record(out coolerState) bool {
	c.gone = false

	if !out.HasCPU && c.reading.HasCPU {
		out.CPU, out.HasCPU = c.reading.CPU, true
		out.CPUName = c.reading.CPUName
		c.gone = true
	}
	if !out.HasLiquid && c.reading.HasLiquid {
		out.Coolant, out.HasLiquid = c.reading.Coolant, true
		out.CoolerName = c.reading.CoolerName
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
		out.GPUName = c.reading.GPUName
		out.GPUStale = true
	}

	// A name, once heard, is kept by a row that is still there: a poll that
	// read the temperature and not the name (nvidia-smi's answer cut short)
	// should not put "GPU" back for five seconds and move nothing but the
	// reader's attention.
	out.CPUName = keepName(out.HasCPU || out.HasCPULoad, out.CPUName, c.reading.CPUName)
	out.GPUName = keepName(out.HasGPU, out.GPUName, c.reading.GPUName)
	out.CoolerName = keepName(out.HasLiquid, out.CoolerName, c.reading.CoolerName)

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

	if out.HasGPU && !out.GPUStale {
		c.gpuVia = out.gpuVia
	}

	c.reading = out
	return out.HasCPU || out.HasCPULoad || out.HasGPU || out.HasLiquid || out.HasMemory
}

// keepName is a row's name, or the one it had when this poll brought none.
func keepName(has bool, name, was string) string {
	if has && name == "" {
		return was
	}
	return name
}

// Section turns the reading into rows and a plot.
func (c *Cooler) Section() view.Section {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := view.Cooler(c.reading.probes())
	s.Gone = c.gone
	s.Reasons = c.reasons
	return s
}

// Data is the reading as plain values, for the JSON the command line prints:
// its probes.
func (c *Cooler) Data() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reading.probes()
}

/*
coolerState is what this source knows between polls: the processor and the
graphics card from the host's chains (spec 043), the coolant, pump and fan from
the cooler, each with whether it is there, and the trails.

It stays a struct of the parts this source reads because record's rules are
about those parts -- the coolant kept and the section Gone, the card kept and
its row dim -- and probes turns it into the list the view draws (spec 044). A
source that reads more probes, a second card or a motherboard's sensors, adds
them in probes and nothing in the view changes.
*/
type coolerState struct {
	CPU       float64
	HasCPU    bool
	Coolant   float64
	HasLiquid bool
	PumpRPM   int
	HasPump   bool
	FanRPM    int
	HasFan    bool

	CPULoad    float64
	HasCPULoad bool

	GPU        float64
	HasGPU     bool
	GPULoad    float64
	HasGPULoad bool

	Trail    []float64
	CPUTrail []float64
	GPUTrail []float64

	CPUName    string
	GPUName    string
	CoolerName string

	GPUStale bool

	// Memory is the memory in use and MemoryTotal all there is, in bytes.
	Memory      uint64
	MemoryTotal uint64
	HasMemory   bool

	// gpuVia is the provider this poll's reading of the card came from.
	gpuVia string
}

// The probes' IDs: one of each part this source reads.
const (
	probeCPU     = "cpu"
	probeGPU     = "gpu"
	probeMemory  = "memory"
	probeCoolant = "coolant"
	probeFan     = "fan"
	probePump    = "pump"
)

// probes is the state as the view's list: a probe for each part that is
// there, which is the rule the fixed rows had -- a processor with a load or a
// temperature, a card with a temperature, a coolant, a fan or pump speed.
func (s coolerState) probes() view.CoolerReading {
	var out []view.Probe
	if s.HasCPU || s.HasCPULoad {
		out = append(out, view.Probe{
			ID: probeCPU, Role: view.RoleCPU, Name: s.CPUName,
			Load: view.Maybe(s.CPULoad, s.HasCPULoad), Temp: view.Maybe(s.CPU, s.HasCPU),
			Trail: s.CPUTrail,
		})
	}
	if s.HasGPU {
		out = append(out, view.Probe{
			ID: probeGPU, Role: view.RoleGPU, Name: s.GPUName,
			Load: view.Maybe(s.GPULoad, s.HasGPULoad), Temp: view.Some(s.GPU),
			Trail: s.GPUTrail, Stale: s.GPUStale,
		})
	}
	if s.HasMemory {
		p := view.Probe{ID: probeMemory, Role: view.RoleMemory,
			Bytes: view.Some(float64(s.Memory)), Of: view.Maybe(float64(s.MemoryTotal), s.MemoryTotal > 0)}
		if s.MemoryTotal > 0 {
			p.Load = view.Some(100 * float64(s.Memory) / float64(s.MemoryTotal))
		}
		out = append(out, p)
	}
	if s.HasLiquid {
		out = append(out, view.Probe{
			ID: probeCoolant, Role: view.RoleCoolant, Name: s.CoolerName,
			Temp: view.Some(s.Coolant), Trail: s.Trail,
		})
	}
	if s.HasFan {
		out = append(out, view.Probe{ID: probeFan, Role: view.RoleFan, Name: s.CoolerName, RPM: view.Some(s.FanRPM)})
	}
	if s.HasPump {
		out = append(out, view.Probe{ID: probePump, Role: view.RolePump, Name: s.CoolerName, RPM: view.Some(s.PumpRPM)})
	}
	return view.CoolerReading{Probes: out}
}

// Details are the lists this reading has: the processor's and the memory's
// where the processes can be read, and the card's where there is a card.
func (c *Cooler) Details() []view.DetailInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []view.DetailInfo
	if c.processes != nil {
		if c.reading.HasCPU || c.reading.HasCPULoad {
			out = append(out, view.DetailInfo{ID: view.DetailCPU, Title: view.ProcessTitle(view.DetailCPU)})
		}
		if c.reading.HasMemory {
			out = append(out, view.DetailInfo{ID: view.DetailMemory, Title: view.ProcessTitle(view.DetailMemory)})
		}
	}
	if c.gpuProcesses != nil && c.reading.HasGPU {
		out = append(out, view.DetailInfo{ID: view.DetailGPU, Title: view.ProcessTitle(view.DetailGPU)})
	}
	return out
}

// DetailInterval is ProcessInterval: every detail here is a list of
// processes.
func (c *Cooler) DetailInterval(string) time.Duration { return ProcessInterval }

// CloseDetail has nothing to let go of: the readers hold nothing open.
func (c *Cooler) CloseDetail(string) {}

// coolerDetails is how each of the lists is read, by ID.
var coolerDetails = map[string]func(*Cooler, context.Context) ([]view.ProcessUse, string, error){
	view.DetailCPU:    (*Cooler).cpuUse,
	view.DetailMemory: (*Cooler).memoryUse,
	view.DetailGPU:    (*Cooler).gpuUse,
}

// PollDetail reads one list of processes. Only names and figures leave here:
// a process is its name (spec 060 R6).
func (c *Cooler) PollDetail(ctx context.Context, id string) (view.Detail, error) {
	key := c.Key()
	read, ok := coolerDetails[id]
	if !ok {
		return view.Detail{}, fmt.Errorf("the %s section has no detail %q", key, id)
	}
	uses, note, err := read(c, ctx)
	if err != nil {
		return unread(key, id, view.ProcessTitle(id), err), err
	}
	d := view.Processes(key, id, uses)
	d.Note = note
	return d, nil
}

// cpuUse is each process's share of the processors since the last reading.
func (c *Cooler) cpuUse(ctx context.Context) ([]view.ProcessUse, string, error) {
	if c.processes == nil {
		return nil, "", errNoProcesses
	}
	processes, err := c.processes.Read(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("reading the processes: %w", err)
	}
	uses := make([]view.ProcessUse, 0, len(processes))
	for _, p := range processes {
		if p.HasCPU {
			uses = append(uses, view.ProcessUse{Name: p.Name, Value: p.CPU})
		}
	}
	return uses, "share of every processor together", nil
}

// memoryUse is the memory each process holds.
func (c *Cooler) memoryUse(ctx context.Context) ([]view.ProcessUse, string, error) {
	if c.processes == nil {
		return nil, "", errNoProcesses
	}
	processes, err := c.processes.Read(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("reading the processes: %w", err)
	}
	uses := make([]view.ProcessUse, 0, len(processes))
	for _, p := range processes {
		uses = append(uses, view.ProcessUse{Name: p.Name, Value: float64(p.Memory)})
	}
	c.mu.Lock()
	state := c.reading
	c.mu.Unlock()
	note := ""
	if state.HasMemory && state.MemoryTotal > 0 {
		used, usedUnit := view.Size(float64(state.Memory))
		total, totalUnit := view.Size(float64(state.MemoryTotal))
		note = strings.TrimSpace(used) + " " + usedUnit + " of " + strings.TrimSpace(total) + " " + totalUnit + " in use"
	}
	return uses, note, nil
}

// gpuUse is each process's use of the card the row is about.
func (c *Cooler) gpuUse(ctx context.Context) ([]view.ProcessUse, string, error) {
	if c.gpuProcesses == nil {
		return nil, "", errNoProcesses
	}
	c.mu.Lock()
	via := c.gpuVia
	c.mu.Unlock()
	processes, err := c.gpuProcesses.Read(ctx, via)
	if err != nil {
		return nil, "", err //nolint:wrapcheck // the reader's own account of the absence
	}
	uses := make([]view.ProcessUse, 0, len(processes))
	for _, p := range processes {
		uses = append(uses, view.ProcessUse{Name: p.Name, Value: p.Load})
	}
	return uses, "each program's busiest engine", nil
}

// errNoProcesses is a source built without a process reader: a test's.
var errNoProcesses = errors.New("this source reads no processes")
