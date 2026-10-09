package cli

import (
	"os"
	"slices"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// AllSources is a source for every section this build has, the shown ones
// first in the settings' order (spec 052). A shell that holds them all can
// follow a section shown or moved while it runs; one not shown is never
// polled.
func (o Options) AllSources(read func() (map[string]core.Counters, error)) []panel.Source {
	return panel.Sources(panel.Ordered(o.Config.Sections), o.Env(read))
}

/*
watchSettings is the terminal panel's look at its settings file (spec 052):
when the file's modification time moves, it is read again, and the shown
sections and the arrangement it gives are returned with changed true when they
differ from the last. A value given on the command line (fixedSections,
fixedArrangement) stays as given, because a pane started with --sections usage
is that pane whatever the file says.
*/
func watchSettings(path string, o Options, fixedSections, fixedArrangement bool) func() ([]string, view.Arrangement, bool) {
	shown := slices.Clone(o.Config.Sections)
	arr := o.Arrangement
	var seen time.Time
	if info, err := os.Stat(path); err == nil {
		seen = info.ModTime()
	}
	return func() ([]string, view.Arrangement, bool) {
		info, err := os.Stat(path)
		if err != nil || info.ModTime().Equal(seen) {
			return shown, arr, false
		}
		seen = info.ModTime()
		store, err := config.Open(path)
		if err != nil {
			return shown, arr, false
		}
		c := store.Config()
		_ = store.Close()

		nextShown, nextArr := shown, arr
		if !fixedSections {
			nextShown = c.Sections
		}
		if !fixedArrangement {
			if a, err := c.ParseArrangement(); err == nil {
				nextArr = a
			}
		}
		if slices.Equal(nextShown, shown) && nextArr == arr {
			return shown, arr, false
		}
		shown, arr = slices.Clone(nextShown), nextArr
		return shown, arr, true
	}
}
