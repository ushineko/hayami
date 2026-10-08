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

// CoolerReading is what the machine said about its own temperature, measured
// and not yet formatted.
type CoolerReading struct {
	CPU       float64
	HasCPU    bool
	Coolant   float64
	HasLiquid bool
	PumpRPM   int
	HasPump   bool
	FanRPM    int
	HasFan    bool

	// CPULoad is the processor's utilisation in percent since the previous
	// poll. The first poll has none: a rate needs two samples.
	CPULoad    float64
	HasCPULoad bool

	// GPU is the graphics card's temperature, and GPULoad its utilisation.
	// A machine with no card this build can read has neither, and no row.
	GPU        float64
	HasGPU     bool
	GPULoad    float64
	HasGPULoad bool

	// Trail is the coolant's recent samples, oldest first. Empty until this
	// process has watched for a while, which is the honest state after a
	// restart.
	Trail []float64

	// CPUTrail is the processor's, already averaged.
	//
	// **Averaged, not raw.** A processor spikes to a hundred degrees on any
	// compile and swings about thirty-five where the coolant moves under one,
	// which is unplottable at this size: the line is noise and the eye takes
	// nothing from it. The monitor plots a trailing mean over sixty seconds
	// and says so, and this is that mean.
	CPUTrail []float64

	// GPUTrail is the graphics card's, averaged the same way and for the
	// same reason.
	GPUTrail []float64

	// CPUName, GPUName and CoolerName are the parts' names as their sources
	// give them -- "Intel(R) Core(TM) i9-14900K", "NVIDIA GeForce RTX 4090",
	// "NZXT Kraken Elite V2" -- or empty where a name could not be read
	// (spec 031). The rows are labelled with them, shortened; the full form
	// is the row's tip.
	CPUName    string `json:",omitempty"`
	GPUName    string `json:",omitempty"`
	CoolerName string `json:",omitempty"`

	// GPUStale marks a GPU reading kept from an earlier poll because this one
	// had none -- nvidia-smi missing its timeout under load. The row stays,
	// dim; the rest of the card is live.
	GPUStale bool
}

// Cooler turns a reading into a section.
//
// The processor is **not** coloured. A high boost temperature is normal, and a
// colour that is always on is not a signal — the monitor makes the same point,
// and colouring the processor would make every compilation look like a fault.
func Cooler(r CoolerReading) Section {
	s := Section{Key: "cooler", Title: "Cooler", Icon: IconCooler}
	unit := UnitWidth("°C", "rpm")

	// Each row is labelled with the part it reads (spec 031): a machine has
	// one processor, but which one is a fact the panel knows, and the two
	// desks it runs on differ.
	//
	// The processor's row is drawn on its load alone where there is no
	// temperature to read (spec 034).
	if r.HasCPU || r.HasCPULoad {
		s.Rows = append(s.Rows, named(processor("CPU", r.CPULoad, r.HasCPULoad, r.CPU, r.HasCPU, unit), "CPU", r.CPUName))
	}
	if r.HasGPU {
		row := named(processor("GPU", r.GPULoad, r.HasGPULoad, r.GPU, true, unit), "GPU", r.GPUName)
		row.Stale = r.GPUStale
		s.Rows = append(s.Rows, row)
	}
	if r.HasLiquid {
		s.Rows = append(s.Rows, named(temperature("Coolant", r.Coolant, coolant(r.Coolant), unit), "Coolant", r.CoolerName))
	}
	if r.HasPump || r.HasFan {
		s.Rows = append(s.Rows, speeds(r, unit))
	}
	// The coolant first, because a plot draws its series in the order it is
	// given them and the coolant is the primary trace: it is what the eye
	// should land on, and the processor is context for it.
	if len(r.Trail) > 0 {
		s.Trails = append(s.Trails, Trail{
			Name:    "Coolant",
			Samples: r.Trail,
			Status:  coolant(r.Coolant),
		})
	}
	if len(r.CPUTrail) > 0 {
		// Info, which is the muted colour. A plot with two traces of equal
		// weight has no primary, and the processor is not the reading anyone
		// is watching for: it is the thing the coolant is reacting to.
		s.Trails = append(s.Trails, Trail{Name: "CPU", Samples: r.CPUTrail, Status: Info, Series: 0, Coloured: true})
	}
	if len(r.GPUTrail) > 0 {
		// A series colour each, so the window can tell the three lines
		// apart: the CPU the link blue it has always been drawn in, the GPU
		// violet. The coolant is not coloured and keeps its band.
		s.Trails = append(s.Trails, Trail{Name: "GPU", Samples: r.GPUTrail, Status: Info, Series: 1, Coloured: true})
	}
	return s
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

// speeds is the pump and the fan on one line. They are two numbers of the same
// kind and a panel 260 px wide has better uses for a second row.
func speeds(r CoolerReading, unit int) Row {
	var parts []string
	if r.HasFan {
		parts = append(parts, "fan "+strings.TrimSpace(Count(r.FanRPM)))
	}
	if r.HasPump {
		parts = append(parts, "pump "+strings.TrimSpace(Count(r.PumpRPM)))
	}
	return Row{Label: "Speeds", Value: strings.Join(parts, "  "), Unit: PadUnit("rpm", unit)}
}

// coolant is the verdict on a liquid temperature.
func coolant(v float64) Status {
	switch {
	case v >= CoolantHot:
		return Bad
	case v >= CoolantWarm:
		return Warn
	default:
		return Good
	}
}
