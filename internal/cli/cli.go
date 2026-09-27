/*
Package cli is what both panels share before they draw anything: the flags,
the settings, and the sources those two produce.

The flags override the settings file for one run and are never written back. A
pane started with an argument must not rewrite the settings of the window that
is already open.
*/
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// Options are a run's choices, after the file and the flags have been
// combined.
type Options struct {
	Config      config.Config
	Arrangement view.Arrangement

	// Store is the settings this run read, kept so the window can hand it to
	// its preferences and follow a change made there.
	Store *config.Store

	// Preferences opens the preferences window at start as well as the panel.
	Preferences bool

	// Unreadable is a settings file that could not be parsed. The panel draws
	// on defaults and says so rather than refusing to start.
	Unreadable error
}

// Resolve reads the settings and applies the overrides.
//
// sections and arrangement are the flag values, empty when unset. An empty
// flag leaves the file's value alone; a flag that names something unknown is
// an error, because a user who typed it meant something.
func Resolve(store *config.Store, sections, arrangement string) (Options, error) {
	o := Options{Config: store.Config(), Unreadable: store.Unreadable()}

	if sections != "" {
		keys, err := ParseSections(sections)
		if err != nil {
			return o, err
		}
		o.Config.Sections = keys
	}
	if arrangement != "" {
		o.Config.Arrangement = arrangement
	}

	a, err := o.Config.ParseArrangement()
	if err != nil {
		return o, err
	}
	o.Arrangement = a
	return o, nil
}

// ParseSections reads a comma-separated list of section keys, refusing one
// this build does not have.
func ParseSections(s string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(s, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			continue
		}
		if !known(key) {
			return nil, fmt.Errorf("no section named %q; there is %s",
				key, strings.Join(panel.Keys(), ", "))
		}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no sections named; there is %s", strings.Join(panel.Keys(), ", "))
	}
	return out, nil
}

func known(key string) bool {
	for _, k := range panel.Keys() {
		if k == key {
			return true
		}
	}
	return false
}

// Sources builds the sources this run draws.
func (o Options) Sources(read func() (map[string]core.Counters, error)) []panel.Source {
	return panel.Sources(o.Config.Sections, o.Config.Interfaces, read)
}

// Readings prints every source's current numbers as JSON, which is how the
// data layer is debugged on a machine with no display. It polls once: a rate
// needs two samples, so the first run reports totals and no rates, which is
// the honest answer and not a bug.
func Readings(w io.Writer, sources []panel.Source, poll func(panel.Source) error) error {
	out := map[string]any{}
	for _, s := range sources {
		if err := poll(s); err != nil {
			return err
		}
		out[s.Key()] = s.Data()
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("writing the readings: %w", err)
	}
	return nil
}
