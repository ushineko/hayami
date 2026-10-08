package panel

import (
	"fmt"

	"github.com/ushineko/sanshoku"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

/*
Env is what the sources are built over: the settings each one reads and the
seams a test replaces. One value rather than a parameter per section's
setting, which is how Sources had grown (the interfaces positionally, the
LibreHardwareMonitor address as a variadic option): a section that gains a
setting gains a field here and its constructor reads it.
*/
type Env struct {
	// Interfaces are the network interfaces the bandwidth section watches.
	Interfaces []string

	// Counters reads the interface counters; nil is the system's own table.
	Counters func() (map[string]core.Counters, error)

	// LHM is where LibreHardwareMonitor serves its sensor tree, for the
	// processor's temperature on Windows (spec 036); empty is its default
	// address.
	LHM string

	// Scan lists the devices the device sections find; nil is DefaultScan.
	Scan Scan
}

// scan is the env's scan, or the default.
func (e Env) scan() Scan {
	if e.Scan != nil {
		return e.Scan
	}
	return DefaultScan
}

// Spec is one section: what it is called and drawn with, and how its source
// is built.
type Spec struct {
	view.SectionInfo
	New func(Env) Source
}

/*
builders are how each section's source is built, by key. The sections
themselves -- which there are, in what order, with what title and icon -- are
view.Sections; this adds the one thing the view cannot know. A test holds the
two to each other: a section in one and not the other fails it rather than
being a section the settings can name and nothing draws.
*/
var builders = map[string]func(Env) Source{
	view.BandwidthInfo.Key:   func(e Env) Source { return NewBandwidth(e.Interfaces, e.Counters) },
	view.UsageInfo.Key:       func(Env) Source { return NewUsage() },
	view.CoolerInfo.Key:      func(e Env) Source { return NewCooler(e.LHM, e.scan()) },
	view.PeripheralsInfo.Key: func(e Env) Source { return NewPeripherals(e.scan()) },
}

// Specs are the sections this build has, in view.Sections' order, each with
// its builder. A section with no builder, or a builder for no section, is a
// programming error, and the registry test is where it shows.
func Specs() []Spec {
	infos := view.Sections()
	out := make([]Spec, 0, len(infos))
	for _, info := range infos {
		build, ok := builders[info.Key]
		if !ok {
			panic(fmt.Sprintf("panel: section %q has no builder", info.Key))
		}
		out = append(out, Spec{SectionInfo: info, New: build})
	}
	if len(builders) != len(out) {
		panic(fmt.Sprintf("panel: %d builders for %d sections: one builds a section view.Sections does not list",
			len(builders), len(out)))
	}
	return out
}

// Sources builds the sources keys ask for, in their order. A key that names
// no section is skipped: a settings file written by a newer build should not
// stop an older one starting.
func Sources(keys []string, env Env) []Source {
	var out []Source
	for _, key := range keys {
		if build, ok := builders[key]; ok {
			out = append(out, build(env))
		}
	}
	return out
}

// Keys are the keys of every section this build has, in order, for the
// command line's help and validation, the preferences and the parity test.
func Keys() []string {
	infos := view.Sections()
	out := make([]string, 0, len(infos))
	for _, info := range infos {
		out = append(out, info.Key)
	}
	return out
}

// DefaultScan is the scan an Env with none is built over: sanshoku.Scan. It is
// a variable for one reason, the commands a test runs end to end, which build
// their Env from the settings inside the command: testenv.NoDevices replaces
// it there, so the suite opens no device where the transport reads the
// system's own device list (Windows, spec 035). Everything else passes a Scan
// in the Env.
var DefaultScan Scan = sanshoku.Scan
