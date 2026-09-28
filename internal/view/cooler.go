package view

import "strings"

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
}

// Cooler turns a reading into a section.
//
// The processor is **not** coloured. A high boost temperature is normal, and a
// colour that is always on is not a signal — the monitor makes the same point,
// and colouring the processor would make every compilation look like a fault.
func Cooler(r CoolerReading) Section {
	s := Section{Key: "cooler", Title: "Cooler"}
	unit := UnitWidth("°C", "rpm")

	if r.HasCPU {
		s.Rows = append(s.Rows, temperature("CPU", r.CPU, Info, unit))
	}
	if r.HasLiquid {
		s.Rows = append(s.Rows, temperature("Coolant", r.Coolant, coolant(r.Coolant), unit))
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
		s.Trails = append(s.Trails, Trail{Name: "CPU", Samples: r.CPUTrail, Status: Info})
	}
	return s
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
