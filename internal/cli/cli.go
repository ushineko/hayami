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
	"errors"
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

	// Preferences opens the preferences window at start as well as the panel,
	// on the page it names. Empty means the window is not opened.
	Preferences string

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
	return panel.Sources(o.Config.Sections, o.Env(read))
}

// Env is what this run's sources are built over, from its settings. read is
// the source of interface counters; nil is the system's own table.
func (o Options) Env(read func() (map[string]core.Counters, error)) panel.Env {
	return panel.Env{Settings: o.Config, Counters: read}
}

/*
Reading is one source's answer: what it read, what it could not, and what went
wrong.

Three fields rather than one, because they are three different answers and the
shape that flattened them lost two. `reasons` is a section's own account of
what is absent -- no cooler, no adapter, no account -- and `error` is a poll
that failed outright. Either can be present with data beside it: a cooler with
a processor reading and no coolant has both.
*/
type Reading struct {
	Data    any          `json:"data"`
	Reasons []JSONReason `json:"reasons,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// JSONReason is a view.Reason with its status spelled out. The view's Status
// is an integer and this output is read by people.
type JSONReason struct {
	Label  string `json:"label,omitempty"`
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
}

// reasons converts a section's reasons for the JSON.
func reasons(in []view.Reason) []JSONReason {
	if len(in) == 0 {
		return nil
	}
	out := make([]JSONReason, 0, len(in))
	for _, r := range in {
		out = append(out, JSONReason{
			Label: r.Label, Text: r.Text, Detail: r.Detail, Status: StatusName(r.Status),
		})
	}
	return out
}

// StatusName is a status as the word a person reads.
func StatusName(s view.Status) string {
	switch s {
	case view.Good:
		return "good"
	case view.Warn:
		return "warn"
	case view.Bad:
		return "bad"
	case view.Accent:
		return "accent"
	case view.Strong:
		return "strong"
	case view.Dim:
		return "dim"
	default:
		return "info"
	}
}

/*
Readings prints every source's current numbers as JSON, which is how the data
layer is debugged on a machine with no display. It polls once: a rate needs two
samples, so the first run reports totals and no rates, which is the honest
answer and not a bug.

**Every source, whatever the others did.** It used to return on the first
failure, so on a machine where the cooler could not be read the command printed that one
error and nothing at all about the other three sections -- the one command
meant for debugging a machine you cannot see, made useless by the machine being
unusual (issue #54). A source that failed reports its failure in its own entry
and the rest are still printed.

The error is returned only when *no* source answered, so a caller can still
tell a machine with a problem from a machine with a quirk; the JSON is written
either way.
*/
func Readings(w io.Writer, sources []panel.Source, poll func(panel.Source) error) error {
	out := map[string]Reading{}
	var failures []error
	answered := false

	for _, s := range sources {
		r := Reading{}
		if err := poll(s); err != nil {
			r.Error = err.Error()
			failures = append(failures, err)
		} else {
			answered = true
		}
		r.Data = s.Data()
		r.Reasons = reasons(s.Section().Reasons)
		out[s.Key()] = r
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("writing the readings: %w", err)
	}
	if !answered && len(failures) > 0 {
		return errors.Join(failures...)
	}
	return nil
}
