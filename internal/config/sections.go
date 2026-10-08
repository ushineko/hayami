package config

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/ushineko/hayami/internal/view"
)

/*
Setting is one section's own settings in the file: under
`sectionSettings.<key>`, decoded into T (spec 046).

A section that needs a setting declares one Setting value here, and its source
reads it from the configuration the registry hands every source. Before this,
each such setting was a field on the root of Config and a field on panel.Env,
threaded by hand through the CLI, the window and the preferences -- the
interfaces as a root `interfaces` list, the LibreHardwareMonitor address as a
root `lhm`.

**Expand and migrate.** Files written before spec 046 carry the root fields
and nothing under `sectionSettings`. Get reads the section's own entry when it
is there and falls back to the root fields when it is not; Set writes both, so
a build from before spec 046 reading the same file still finds its setting.
Dropping the root fields is a later contract step, once no build that reads
only them is in use.

The zero T is the default: an empty interface list watches nothing, an empty
address is LibreHardwareMonitor's default one.
*/
type Setting[T any] struct {
	// Key is the section's key, as view.Sections lists it.
	Key string

	// legacy reads the root fields a file from before spec 046 carries, and
	// says whether it carried them.
	legacy func(Config) (T, bool)

	// root writes the root fields, so a build from before spec 046 reads the
	// same setting.
	root func(Config, T) Config
}

// Get is the section's settings: its own entry, or the root fields an older
// file carries, or the zero T. An entry that does not decode is treated as
// absent rather than failing the whole file.
func (s Setting[T]) Get(c Config) T {
	if raw, ok := c.SectionSettings[s.Key]; ok {
		var v T
		if err := json.Unmarshal(raw, &v); err == nil {
			return v
		}
	}
	if s.legacy != nil {
		if v, ok := s.legacy(c); ok {
			return v
		}
	}
	var zero T
	return zero
}

// Set is the configuration with the section's settings replaced, under its
// own key and in the root fields both.
func (s Setting[T]) Set(c Config, v T) Config {
	raw, err := json.Marshal(v)
	if err != nil {
		// T is one of the plain structs below; marshalling one cannot fail.
		panic("config: section settings would not encode: " + err.Error())
	}
	m := maps.Clone(c.SectionSettings)
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	m[s.Key] = raw
	c.SectionSettings = m
	if s.root != nil {
		c = s.root(c, v)
	}
	return c
}

// BandwidthSettings are the bandwidth section's: which interfaces it watches.
type BandwidthSettings struct {
	Interfaces []string `json:"interfaces,omitempty"`
}

// CoolerSettings are the cooler section's: where LibreHardwareMonitor serves
// its sensor tree (spec 036), empty for its default address. Settings file
// only, as spec 036 decided: it is set once, to match a changed port.
type CoolerSettings struct {
	LHM string `json:"lhm,omitempty"`
}

// Bandwidth is the bandwidth section's setting.
var Bandwidth = Setting[BandwidthSettings]{
	Key: view.BandwidthInfo.Key,
	legacy: func(c Config) (BandwidthSettings, bool) {
		return BandwidthSettings{Interfaces: slices.Clone(c.Interfaces)}, c.Interfaces != nil
	},
	root: func(c Config, v BandwidthSettings) Config {
		c.Interfaces = slices.Clone(v.Interfaces)
		return c
	},
}

// Cooler is the cooler section's setting.
var Cooler = Setting[CoolerSettings]{
	Key: view.CoolerInfo.Key,
	legacy: func(c Config) (CoolerSettings, bool) {
		return CoolerSettings{LHM: c.LHM}, c.LHM != ""
	},
	root: func(c Config, v CoolerSettings) Config {
		c.LHM = v.LHM
		return c
	},
}

// SettingKeys are the sections that declare a setting, for the test that
// holds each to a section this build has.
func SettingKeys() []string { return []string{Bandwidth.Key, Cooler.Key} }
