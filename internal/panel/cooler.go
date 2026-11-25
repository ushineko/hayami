package panel

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

	// load is the processor's load over the last CoolerTrail polls, which
	// the CPU drawer's summary plots (spec 095 R3): fed by the poll, so the
	// plot has its minutes when the drawer opens.
	loadTrail *view.Series

	// readTrail and writeTrail are the disks' rates over the same polls,
	// for the IO drawer's plot (spec 097).
	readTrail, writeTrail *view.Series

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
	processor    *core.ProcessorReader
	card         core.CardReader
	disks        *core.DiskReader
	uptime       func(context.Context) (time.Duration, error)
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
	if h.Processor != nil {
		c.processor = h.Processor()
	}
	c.card = h.Card
	if h.Disks != nil {
		c.disks = h.Disks()
	}
	c.uptime = h.Uptime
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
		trail: view.NewSeries(CoolerTrail),
		cpu:   view.NewAveraged(CoolerTrail, CPUAverageWindow),
		gpu:   view.NewAveraged(CoolerTrail, CPUAverageWindow),

		loadTrail:  view.NewSeries(CoolerTrail),
		readTrail:  view.NewSeries(CoolerTrail),
		writeTrail: view.NewSeries(CoolerTrail),
		sensor:     sensor,
		held:       newHeld(scan, h.Permission),

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
			out.memory = m
		} else {
			// Aside: a machine whose memory cannot be read has a card without
			// the row, and the hover note and doctor say why.
			reasons = append(reasons, reason(view.Reason{
				Label: "Memory", Text: "not read", Status: view.Info, Aside: true,
			}, err))
		}
	}

	// The disks are read on the card's poll, for the IO row (spec 097 R1):
	// one file on Linux, the counters already open on Windows.
	if c.disks != nil {
		out.disks = c.disks.Read(ctx)
		out.IORead, out.IOWrite, out.HasIO = out.disks.Read, out.disks.Write, out.disks.HasRates
	}

	// The uptime is the card's last row (spec 099). Aside, as the memory's:
	// the row is kept from the last poll, and the hover note says why.
	if c.uptime != nil {
		if d, err := c.uptime(ctx); err == nil {
			out.Uptime, out.HasUptime = d, true
		} else {
			reasons = append(reasons, reason(view.Reason{
				Label: "Uptime", Text: "not read", Status: view.Info, Aside: true,
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

	// An uptime that was read and is not is kept, so the row does not leave
	// and come back (spec 099 R5). It is a minute behind at worst.
	if !out.HasUptime && c.reading.HasUptime {
		out.Uptime, out.HasUptime = c.reading.Uptime, true
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
	if out.HasCPULoad {
		c.loadTrail.Add(out.CPULoad)
	}
	if out.HasGPU && !out.GPUStale && !c.gone {
		c.gpu.Add(out.GPU)
	}
	out.Trail = c.trail.Samples()
	out.CPUTrail = c.cpu.Mean()
	out.loadTrail = c.loadTrail.Samples()
	if out.HasIO {
		c.readTrail.Add(out.IORead)
		c.writeTrail.Add(out.IOWrite)
	}
	out.readTrail, out.writeTrail = c.readTrail.Samples(), c.writeTrail.Samples()
	out.GPUTrail = c.gpu.Mean()

	if out.HasGPU && !out.GPUStale {
		c.gpuVia = out.gpuVia
	}

	c.reading = out
	return out.HasCPU || out.HasCPULoad || out.HasGPU || out.HasLiquid || out.HasMemory || out.HasIO
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

	// memory is the whole of this poll's reading of the memory, which the
	// Memory drawer's summary says (spec 094 R8): the card's line and the
	// summary are one reading, and never disagree.
	memory core.Memory

	// loadTrail is the processor's load over the recent polls, for the CPU
	// drawer's plot (spec 095).
	loadTrail []float64

	// IORead and IOWrite are the disks' rates in bytes per second, kept with
	// the reading so a cached card has its IO row from the first draw
	// (spec 097); disks is the whole of the poll's reading of them, and
	// readTrail and writeTrail their recent history.
	IORead, IOWrite       float64
	HasIO                 bool
	disks                 core.Disks
	readTrail, writeTrail []float64

	// Uptime is how long the machine has been up (spec 099). It does not make
	// the card present: record's answer is about the hardware.
	Uptime    time.Duration
	HasUptime bool
}

// The probes' IDs: one of each part this source reads.
const (
	probeCPU     = "cpu"
	probeGPU     = "gpu"
	probeMemory  = "memory"
	probeIO      = "io"
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
	if s.HasIO {
		out = append(out, view.Probe{ID: probeIO, Role: view.RoleIO,
			Read: view.Some(s.IORead), Write: view.Some(s.IOWrite)})
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
	return view.CoolerReading{Probes: out, Uptime: view.Maybe(s.Uptime, s.HasUptime)}
}

// Details are the lists this reading has: the processor's and the memory's
// where the processes can be read, and the card's where there is a card.
func (c *Cooler) Details() []view.DetailInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []view.DetailInfo
	// In the rows' order, which is the order a switch between them reads
	// in (spec 062 R4).
	if c.processes != nil && (c.reading.HasCPU || c.reading.HasCPULoad) {
		out = append(out, view.ProcessInfo(view.DetailCPU))
	}
	if c.gpuProcesses != nil && c.reading.HasGPU {
		out = append(out, view.ProcessInfo(view.DetailGPU))
	}
	if c.processes != nil && c.reading.HasMemory {
		out = append(out, view.ProcessInfo(view.DetailMemory))
	}
	if c.processes != nil && c.reading.HasIO {
		out = append(out, view.ProcessInfo(view.DetailIO))
	}
	return out
}

// DetailInterval is ProcessInterval: every detail here is a list of
// processes.
func (c *Cooler) DetailInterval(string) time.Duration { return ProcessInterval }

// CloseDetail has nothing to let go of: the readers hold nothing open.
func (c *Cooler) CloseDetail(string) {}

// coolerDetails is how each of the lists is read, by ID.
var coolerDetails = map[string]func(*Cooler, context.Context) ([]RawUse, string, error){
	view.DetailCPU:    (*Cooler).cpuUse,
	view.DetailMemory: (*Cooler).memoryUse,
	view.DetailGPU:    (*Cooler).gpuUse,
	view.DetailIO:     (*Cooler).ioUse,
}

// PollDetail reads one list of processes, and draws it.
func (c *Cooler) PollDetail(ctx context.Context, id string) (view.Detail, error) {
	return pollRaw(ctx, c, id)
}

/*
ReadDetail reads one list of processes, or a drill into one. Only names and
figures leave here: a process is its name (spec 060 R6). Each use keeps its
PID, which a list of names drops and a drill is about.
*/
func (c *Cooler) ReadDetail(ctx context.Context, id string) (RawDetail, error) {
	key := c.Key()
	if c.Drills(id) {
		return c.readDrill(ctx, id)
	}
	read, ok := coolerDetails[id]
	if !ok {
		return RawDetail{}, fmt.Errorf("the %s section has no detail %q", key, id)
	}
	uses, note, err := read(c, ctx)
	if err != nil {
		return unreadRaw(key, id, view.ProcessTitle(id), err), err
	}
	raw := RawDetail{Section: key, ID: id, Title: view.ProcessTitle(id), At: time.Now()}
	list := &RawProcesses{List: id, Uses: uses, Note: note}
	switch {
	case id == view.DetailCPU && c.processor != nil:
		f := c.cpuFigures(ctx)
		list.CPU = &f
	case id == view.DetailGPU && c.card != nil:
		f := c.gpuFigures(ctx)
		list.GPU = &f
	case id == view.DetailIO:
		f := c.ioFigures()
		list.IO = &f
	case id == view.DetailMemory:
		f, inUse := c.memoryFigures()
		list.Memory, list.InUse = &f, inUse
	}
	raw.Processes = list
	return raw, nil
}

// DrawDetail is a list of processes, or a drill into one, as the shells show
// it.
func (c *Cooler) DrawDetail(raw RawDetail) view.Detail {
	if d, ok := drawUnread(raw); ok {
		return d
	}
	switch {
	case raw.Process != nil:
		return drawCoolerProcess(raw)
	case raw.Processes != nil && raw.Processes.Name != "":
		return drawCoolerName(raw)
	case raw.Processes != nil:
		return drawCoolerList(raw)
	}
	return view.Detail{}
}

// drawCoolerList is one of the cooler's lists: a line per name, greatest
// first, each leading to the processes behind it, and the list's summary.
func drawCoolerList(raw RawDetail) view.Detail {
	list := raw.Processes
	id := list.List
	named := make([]view.ProcessUse, len(list.Uses))
	for i, u := range list.Uses {
		named[i] = u.ProcessUse
	}
	d := view.Processes(raw.Section, id, named)
	// Every row leads to the processes behind its name. The row carries
	// what it asks for; what a click does with it is the shell's.
	for i, l := range d.Lines {
		if l.Heading || l.Name == "" || l.Name == view.OtherProcesses {
			continue
		}
		d.Lines[i].Open = DrillUnder(id, l.Name)
	}
	d.Note = list.Note
	if list.InUse != nil {
		d.Note = memoryNote(*list.InUse)
	}
	var summary view.Summary
	var reasons []view.Reason
	switch {
	case list.CPU != nil:
		summary, reasons = view.CPUSummary(*list.CPU)
	case list.GPU != nil:
		summary, reasons = view.GPUSummary(*list.GPU)
	case list.IO != nil:
		summary, reasons = view.IOSummary(*list.IO)
	case list.Memory != nil:
		summary, reasons = view.MemorySummary(*list.Memory)
	default:
		return d
	}
	d.Summary = &summary
	d.Reasons = append(d.Reasons, reasons...)
	return d
}

// memoryNote is the memory list's note: how much of the whole is in use.
func memoryNote(m MemoryInUse) string {
	used, usedUnit := view.Size(float64(m.Used))
	total, totalUnit := view.Size(float64(m.Total))
	return strings.TrimSpace(used) + " " + usedUnit + " of " + strings.TrimSpace(total) + " " + totalUnit + " in use"
}

// readProcesses is one reading of the processes and the memory's total, which
// a share of it is of.
func (c *Cooler) readProcesses(ctx context.Context) ([]core.Process, uint64, error) {
	if c.processes == nil {
		return nil, 0, errNoProcesses
	}
	processes, err := c.processes.Read(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the processes: %w", err)
	}
	c.mu.Lock()
	total := c.reading.MemoryTotal
	c.mu.Unlock()
	return processes, total, nil
}

// figures are every figure one process reading has of a process, for the
// lists' columns (spec 092); the list sets which of them it is ordered by.
func figures(p core.Process, total uint64) RawUse {
	return RawUse{PID: p.PID, ProcessUse: view.ProcessUse{
		Name:   p.Name,
		CPU:    view.Maybe(p.CPU, p.HasCPU),
		Memory: view.Some(float64(p.Memory)),
		Share:  view.Maybe(100*float64(p.Memory)/float64(total), total > 0),
	}}
}

// cpuUse is each process's share of the processors since the last reading.
func (c *Cooler) cpuUse(ctx context.Context) ([]RawUse, string, error) {
	processes, total, err := c.readProcesses(ctx)
	if err != nil {
		return nil, "", err
	}
	uses := make([]RawUse, 0, len(processes))
	for _, p := range processes {
		if p.HasCPU {
			u := figures(p, total)
			u.Value = p.CPU
			uses = append(uses, u)
		}
	}
	return uses, "share of every processor together", nil
}

// memoryUse is the memory each process holds.
func (c *Cooler) memoryUse(ctx context.Context) ([]RawUse, string, error) {
	processes, total, err := c.readProcesses(ctx)
	if err != nil {
		return nil, "", err
	}
	uses := make([]RawUse, 0, len(processes))
	for _, p := range processes {
		u := figures(p, total)
		u.Value = float64(p.Memory)
		uses = append(uses, u)
	}
	// The list's note is how much of the whole is in use, which is a figure
	// and is drawn from the reading's (memoryFigures, memoryNote).
	return uses, "", nil
}

/*
gpuUse is each process's use of the card the row is about.

The card's reader knows a process by its PID and its load and nothing else,
so its other figures come from a reading of the processes joined by PID
(spec 092 R3). A process that reading lacks -- gone between the two, or
another account's on Windows -- keeps its load and has no other figure: the
empty form, not a zero. A reading of the processes that fails costs the
columns and not the list.
*/
func (c *Cooler) gpuUse(ctx context.Context) ([]RawUse, string, error) {
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
	byPID := map[int]core.Process{}
	var total uint64
	if all, t, err := c.readProcesses(ctx); err == nil {
		total = t
		for _, p := range all {
			byPID[p.PID] = p
		}
	}
	uses := make([]RawUse, 0, len(processes))
	for _, gp := range processes {
		u := RawUse{PID: gp.PID}
		if p, ok := byPID[gp.PID]; ok && gp.PID != 0 {
			u = figures(p, total)
		}
		u.Name, u.Value = gp.Name, gp.Load
		uses = append(uses, u)
	}
	return uses, "each program's busiest engine", nil
}

// errNoProcesses is a source built without a process reader: a test's.
var errNoProcesses = errors.New("this source reads no processes")

// cpuFigures are the CPU drawer's summary's figures (spec 095): the load and
// its plot from the poll, the cores, the clock, the waiting and the tasks
// read now.
func (c *Cooler) cpuFigures(ctx context.Context) view.CPUFigures {
	p := c.processor.Read(ctx)
	c.mu.Lock()
	state := c.reading
	trail := slices.Clone(state.loadTrail)
	c.mu.Unlock()
	kind := view.LoadAverage
	if p.Queues {
		kind = view.LoadQueue
	}
	return view.CPUFigures{
		Load: view.Maybe(state.CPULoad, state.HasCPULoad), Trail: trail, TrailEvery: CoolerInterval,
		Cores: p.Cores, HasCores: p.HasCores,
		Clock: view.Maybe(p.Clock, p.HasClock), MaxClock: p.MaxClock,
		LoadKind: kind, LoadAvg: view.Maybe(p.LoadAvg, p.HasLoadAvg), Queue: view.Maybe(p.Queue, p.HasQueue),
		Processes: p.Processes, Threads: p.Threads, HasTasks: p.HasTasks,
	}
}

// gpuFigures are the GPU drawer's summary's figures (spec 096), read now from
// the card the list is about, by the route its reading came from. A card
// that could not be read is every figure's empty form.
func (c *Cooler) gpuFigures(ctx context.Context) view.GPUFigures {
	c.mu.Lock()
	via := c.gpuVia
	c.mu.Unlock()
	card, err := c.card(ctx, via)
	if err != nil {
		card = core.Card{}
	}
	return view.GPUFigures{
		VRAM: card.VRAMUsed, VRAMTotal: card.VRAMTotal, HasVRAM: card.HasVRAM,
		Clock: view.Maybe(card.Clock, card.HasClock), MemClock: view.Maybe(card.MemClock, card.HasMemClock),
		Power: view.Maybe(card.Power, card.HasPower), PowerLimit: card.PowerLimit,
	}
}

/*
ioUse is each process's read and write rates since the last reading (spec
097 R2). On Linux a process this account may not trace has no figure, and
what the disks did beyond what the readable processes account for is one
line of its own, the list's "other" (R3), never below nothing. On Windows a
process's figure is every transfer, files, network and devices, which is not
a share of the disks' rates, so there is no "other" and the note says so.
*/
func (c *Cooler) ioUse(ctx context.Context) ([]RawUse, string, error) {
	processes, total, err := c.readProcesses(ctx)
	if err != nil {
		return nil, "", err
	}
	c.mu.Lock()
	disks := c.reading.disks
	c.mu.Unlock()
	uses := make([]RawUse, 0, len(processes))
	var read, write float64
	for _, p := range processes {
		if !p.HasIO {
			continue
		}
		u := figures(p, total)
		u.Value, u.Second = p.IORead, p.IOWrite
		read, write = read+p.IORead, write+p.IOWrite
		uses = append(uses, u)
	}
	if disks.AllIO {
		return uses, "all IO: files, network and devices", nil
	}
	if disks.HasRates {
		other := RawUse{ProcessUse: view.ProcessUse{
			Value: max(disks.Read-read, 0), Second: max(disks.Write-write, 0),
		}}
		uses = append(uses, other)
	}
	return uses, "to and from the disks; other: what this account cannot see", nil
}

// ioFigures are the IO drawer's summary's figures (spec 097): every one from
// the card's poll, so the row and the summary are one reading.
func (c *Cooler) ioFigures() view.IOFigures {
	c.mu.Lock()
	s := c.reading
	readTrail, writeTrail := slices.Clone(s.readTrail), slices.Clone(s.writeTrail)
	c.mu.Unlock()
	d := s.disks
	busy := make([]float64, len(d.Busy))
	for i, b := range d.Busy {
		busy[i] = b
		if b < 0 {
			busy[i] = -1
		}
	}
	return view.IOFigures{
		Read: view.Maybe(d.Read, d.HasRates), Write: view.Maybe(d.Write, d.HasRates),
		ReadTrail: readTrail, WriteTrail: writeTrail, TrailEvery: CoolerInterval,
		Busy: busy, HasBusy: d.HasBusy,
		Queues: d.Queues, Pressure: view.Maybe(d.Pressure, d.HasPressure), Queue: view.Maybe(d.Queue, d.HasQueue),
		SwapIn: d.SwapIn, SwapOut: d.SwapOut, HasSwap: d.HasSwap,
	}
}

// memoryFigures are the memory drawer's summary's figures, and the memory in
// use the list's note says, from the poll.
func (c *Cooler) memoryFigures() (view.MemoryFigures, *MemoryInUse) {
	c.mu.Lock()
	state := c.reading
	c.mu.Unlock()
	m := state.memory
	var inUse *MemoryInUse
	if state.HasMemory && state.MemoryTotal > 0 {
		inUse = &MemoryInUse{Used: state.Memory, Total: state.MemoryTotal}
	}
	return view.MemoryFigures{
		Used: float64(m.Used), Total: float64(m.Total),
		Cache: view.Maybe(float64(m.Cache), m.HasCache),
		Swap:  view.Maybe(float64(m.SwapUsed), m.HasSwap), SwapTotal: float64(m.SwapTotal),
		SwapName: m.SwapName,
	}, inUse
}
