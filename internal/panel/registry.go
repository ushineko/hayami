package panel

import (
	"fmt"
	"sync"

	"github.com/ushineko/sanshoku"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

/*
Env is what the sources are built over: the settings they read and the seams a
test replaces. One value rather than a parameter per section's setting, which
is how Sources had grown (the interfaces positionally, the LibreHardwareMonitor
address as a variadic option). A section's own settings are in Settings, each
under the config.Setting the section declares (spec 046): a section that gains
a setting declares it there and its builder reads it, and Env gains nothing.
*/
type Env struct {
	// Counters reads the interface counters; nil is the system's own table.
	Counters func() (map[string]core.Counters, error)

	// Settings are the configuration the sections read their own settings
	// from, through config.Bandwidth, config.Cooler and the rest.
	Settings config.Config

	// Scan lists the devices the device sections find; nil is DefaultScan.
	Scan Scan

	// Host is what this platform offers the sources: the chains the cooler
	// reads through, the network readers, the advice for a device that would
	// not open, the Bluetooth drivers (spec 043). Nil is this platform's own,
	// built over the cooler's LibreHardwareMonitor address. A test passes one of its own.
	Host *core.Host
}

// scan is the env's scan, or the default.
func (e Env) scan() Scan {
	if e.Scan != nil {
		return e.Scan
	}
	return DefaultScan
}

// platform is this platform's host as a test seam builds over it: the
// constructors a test calls directly (newCooler, newPeripherals) take their
// words and drivers from it, as the program's sources do from their Env. Built
// once; nothing reassigns it.
var platform = sync.OnceValue(func() *core.Host {
	h := core.NewHost(core.HostConfig{})
	return &h
})

// withHost is the env with its Host filled in: this platform's, where it
// names none, built once for every source that reads it.
func (e Env) withHost() Env {
	if e.Host == nil {
		h := core.NewHost(core.HostConfig{LHM: config.Cooler.Get(e.Settings).LHM})
		e.Host = &h
	}
	return e
}

// bandwidth is the bandwidth source over the env's counters, or the host's
// counters and Wi-Fi where the env names no counters. A test's counters read
// no Wi-Fi, as they always have.
func (e Env) bandwidth() Source {
	names := config.Bandwidth.Get(e.Settings).Interfaces
	if e.Counters != nil {
		return NewBandwidth(names, e.Counters)
	}
	b := NewBandwidth(names, e.Host.Counters)
	b.SetWirelessReader(e.Host.Wireless)
	return b
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
	view.BandwidthInfo.Key:   Env.bandwidth,
	view.UsageInfo.Key:       func(Env) Source { return NewUsage() },
	view.CoolerInfo.Key:      func(e Env) Source { return NewCooler(e.Host, e.scan()) },
	view.PeripheralsInfo.Key: func(e Env) Source { return NewPeripherals(e.scan(), e.Host) },
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
		out = append(out, Spec{SectionInfo: info, New: func(e Env) Source { return build(e.withHost()) }})
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
	env = env.withHost()
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

/*
Ordered is every section this build has, in the order a shell lays them out:
the shown ones first, in the order given (the settings' order), then the rest
in the registry's order (spec 052). A key this build does not know is left
out, as Sources leaves it out.

A shell builds a source for each, shown or not, so showing a section or moving
it is a change it can follow while running; a section that is not shown is
never polled, so building it costs nothing.
*/
func Ordered(shown []string) []string {
	known := map[string]bool{}
	for _, k := range Keys() {
		known[k] = true
	}
	out := make([]string, 0, len(known))
	seen := map[string]bool{}
	for _, k := range shown {
		if known[k] && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	for _, k := range Keys() {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}
