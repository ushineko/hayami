package view

import (
	"fmt"
	"strings"
)

// The coolant's bands, in degrees.
//
// The hardware's numbers and not round ones. The monitor this comes from
// records that on the cooler these were measured on, the pump-head
// over-temperature alarm tripped at 57.1 and cleared near 50; a panel that
// went amber at a tidy 55 would be inventing a threshold the machine does not
// have.
const (
	CoolantWarm = 50.0
	CoolantHot  = 55.0
)

// Role is what a probe measures, which decides how it is drawn: the label it
// falls back to, the kind of row, whether its value is coloured and where its
// trace goes on the plot (coolerRoles).
type Role string

// The roles a cooler reading's probes take.
const (
	RoleCPU     Role = "cpu"
	RoleGPU     Role = "gpu"
	RoleCoolant Role = "coolant"
	RoleFan     Role = "fan"
	RolePump    Role = "pump"
)

/*
Probe is one thing the machine measures about its own temperature: a
processor, a graphics card, a coolant loop, a fan, a pump (spec 044).

A reading is a list of these, not a field per part, so a machine with a second
graphics card, a motherboard temperature or a second cooler is more probes and
no new code here. Each value is an Opt: a processor with a load and no
temperature -- every Windows machine without LibreHardwareMonitor -- is a load
that is there and a temperature that is not, which a zero could not say.

A probe carries no colour of its own. Whether a value is a verdict is a fact
about the kind of reading -- a coolant at 56 is hot whoever read it -- so it
lives with the role, in coolerRoles, and not with whichever source supplied it.
*/
type Probe struct {
	// ID names the probe for as long as it is the same part: "cpu", "gpu",
	// "gpu:1", "coolant". It is the row's ID (Row.ID).
	ID   string `json:"id"`
	Role Role   `json:"role"`

	// Name is the part's name as its source gives it -- "AMD Ryzen 5 2600X
	// Six-Core Processor", "NZXT Kraken Elite V2" -- or empty where none
	// could be read (spec 031). The row is labelled with it, shortened; the
	// full form is the row's tip.
	Name string `json:"name,omitempty"`

	// Load is a utilisation in percent since the previous poll; Temp a
	// temperature in degrees; RPM a speed.
	Load Opt[float64] `json:"load"`
	Temp Opt[float64] `json:"temp"`
	RPM  Opt[int]     `json:"rpm"`

	// Trail is the probe's recent samples for the plot, oldest first: the
	// coolant raw, a processor's already averaged (the monitor's sixty-second
	// mean -- a processor swings thirty-five degrees on a compile and the raw
	// line would be noise). Empty until this process has watched a while.
	Trail []float64 `json:"trail,omitempty"`

	// Stale marks a value kept from an earlier poll because this one had
	// none -- nvidia-smi missing its timeout under load. The row stays, dim;
	// the rest of the card is live.
	Stale bool `json:"stale,omitempty"`
}

// CoolerReading is what the machine said about its own temperature, measured
// and not yet formatted: whatever probes it has, in no particular order.
type CoolerReading struct {
	Probes []Probe `json:"probes"`
}

// rowKind is how a role's probe is drawn.
type rowKind int

const (
	// kindProcessor is a load and a temperature on one line, uncoloured.
	kindProcessor rowKind = iota
	// kindTemperature is a temperature, coloured by the role's bands.
	kindTemperature
	// kindSpeed is a speed, drawn with every other speed on the one Speeds
	// row: a fan and a pump are two numbers of the same kind and a panel
	// 260 px wide has better uses for a second row.
	kindSpeed
)

// roleSpec is how one role is drawn.
type roleSpec struct {
	role Role
	// label is the row's label where the part has no name, and the trace's
	// name on the plot. A second probe of the role is "GPU 2".
	label string
	kind  rowKind
	// bands colour a temperature row and its trace; nil leaves both
	// uncoloured.
	bands *Bands[Status]
	// series is whether the role's trace takes a series colour of its own.
	series bool
	// word names a speed on the Speeds row.
	word string
}

/*
coolerRoles is every role, in the order its rows are drawn: the processor, the
graphics card, the coolant, then the speeds on one row (rule 2 of
docs/architecture.md: a table, not a switch).

The processor is **not** coloured. A high boost temperature is normal, and a
colour that is always on is not a signal -- the monitor makes the same point,
and colouring the processor would make every compilation look like a fault.
The coolant is: its bands are the pump's own alarm.
*/
var coolerRoles = []roleSpec{
	{role: RoleCPU, label: "CPU", kind: kindProcessor, series: true},
	{role: RoleGPU, label: "GPU", kind: kindProcessor, series: true},
	{role: RoleCoolant, label: "Coolant", kind: kindTemperature, bands: &CoolantBands},
	{role: RoleFan, label: "Fan", kind: kindSpeed, word: "fan"},
	{role: RolePump, label: "Pump", kind: kindSpeed, word: "pump"},
}

// SpeedsID is the Speeds row's ID: one row for every speed, whoever measured
// it.
const SpeedsID = "speeds"

// instance is each probe of r with the label its row and trace take: the
// role's own for the first of a role, "GPU 2" for the second.
type instance struct {
	Probe
	spec  roleSpec
	label string
}

// instances are r's probes in the order coolerRoles draws them, each with its
// spec and label. A probe of a role the table does not know is not drawn.
func instances(r CoolerReading) []instance {
	var out []instance
	for _, spec := range coolerRoles {
		n := 0
		for _, p := range r.Probes {
			if p.Role != spec.role {
				continue
			}
			n++
			label := spec.label
			if n > 1 {
				label = fmt.Sprintf("%s %d", spec.label, n)
			}
			out = append(out, instance{Probe: p, spec: spec, label: label})
		}
	}
	return out
}

// Cooler turns a reading into a section: a row per probe, in coolerRoles'
// order, the speeds together on one, and a trace per probe with samples.
func Cooler(r CoolerReading) Section {
	s := CoolerInfo.section()
	unit := UnitWidth("°C", "rpm")
	all := instances(r)

	// Each row is labelled with the part it reads (spec 031): a machine has
	// one processor, but which one is a fact the panel knows, and the two
	// desks it runs on differ.
	var speeds []string
	for _, in := range all {
		var row Row
		switch in.spec.kind {
		case kindProcessor:
			// The processor's row is drawn on its load alone where there is
			// no temperature to read (spec 034).
			if !in.Load.OK && !in.Temp.OK {
				continue
			}
			row = named(processor(in.label, in.Load.V, in.Load.OK, in.Temp.V, in.Temp.OK, unit), in.label, in.Name)
		case kindTemperature:
			if !in.Temp.OK {
				continue
			}
			row = named(temperature(in.label, in.Temp.V, in.spec.verdict(in.Temp.V), unit), in.label, in.Name)
		case kindSpeed:
			if in.RPM.OK {
				speeds = append(speeds, in.spec.word+" "+strings.TrimSpace(Count(in.RPM.V)))
			}
			continue
		}
		row.ID = in.ID
		row.Stale = in.Stale
		s.Rows = append(s.Rows, row)
	}
	if len(speeds) > 0 {
		s.Rows = append(s.Rows, Row{ID: SpeedsID, Label: "Speeds", Value: strings.Join(speeds, "  "), Unit: PadUnit("rpm", unit)})
	}

	// The banded traces first, because a plot draws its series in the order
	// it is given them and the coolant is the primary trace: it is what the
	// eye should land on, and the processor is context for it.
	for _, in := range all {
		if in.spec.bands != nil && len(in.Trail) > 0 {
			s.Trails = append(s.Trails, Trail{Name: in.label, Samples: in.Trail, Status: in.spec.verdict(in.Temp.V)})
		}
	}
	// Then a series colour each, so the window can tell the lines apart: the
	// CPU the link blue it has always been drawn in, the GPU violet, a second
	// card the next. Info, the muted colour: the processor is not the reading
	// anyone is watching for, it is the thing the coolant reacts to.
	series := 0
	for _, in := range all {
		if !in.spec.series {
			continue
		}
		if len(in.Trail) > 0 {
			s.Trails = append(s.Trails, Trail{Name: in.label, Samples: in.Trail, Status: Info, Series: series, Coloured: true})
		}
		series++
	}
	return s
}

// verdict is the colour of a value of this role: its bands', or Info.
func (s roleSpec) verdict(v float64) Status {
	if s.bands == nil {
		return Info
	}
	return s.bands.Of(v)
}

// LoadWidth is the width a load takes in a processor's row: "100 %".
const LoadWidth = 5

/*
processor is a processor's load and temperature on one line: " 12 %   78.0 °C".

The load is padded to LoadWidth and, before it has arrived, is that many
spaces. The temperature stays where it is either way, in the column the
coolant's is in, and the row is as wide on the first poll as on the second: a
row that grew five seconds after the window opened would move the panel.

A processor with a load and no temperature -- every Windows machine, which
offers none without a kernel driver (spec 034) -- has its temperature and its
unit as spaces of their own widths, so the load stays in the column the
graphics card's is in. Not Blank: "--" says a value is on its way, and this
one is not coming.
*/
func processor(label string, load float64, hasLoad bool, v float64, hasTemp bool, unit int) Row {
	l := strings.Repeat(" ", LoadWidth)
	if hasLoad {
		l = fmt.Sprintf("%3.0f %%", load)
	}
	if !hasTemp {
		return Row{Label: label, Value: l + " " + strings.Repeat(" ", NumberWidth), Unit: PadUnit("", unit), Status: Info}
	}
	return Row{Label: label, Value: l + " " + Quantity(v), Unit: PadUnit("°C", unit), Status: Info}
}

// temperature is one degree reading, at the fixed width every one shares.
func temperature(label string, v float64, status Status, unit int) Row {
	return Row{Label: label, Value: Quantity(v), Unit: PadUnit("°C", unit), Status: status}
}
