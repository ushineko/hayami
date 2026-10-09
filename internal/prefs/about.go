package prefs

import (
	"net/url"
	"time"

	"fyne.io/fyne/v2"

	"github.com/ushineko/fynedesygn/markdown"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	hayami "github.com/ushineko/hayami"
	"github.com/ushineko/hayami/internal/config"
)

// projectURL is where the README lives, and the one place this window sends a
// reader outside itself.
const projectURL = "https://github.com/ushineko/hayami"

// openURL opens a link the README's reader taps: nil is the app's own, which
// hands it to the browser. A test replaces it to see the tap arrive.
var openURL func(*url.URL) error

/*
buildAbout is what hayami is, in the library's About shape, with the README
itself below the facts.

The document rather than a shortened restatement of it: a second copy would
be one more thing to keep in step, and the one that drifts is always the copy
nobody is reading. The pane follows the shell's scroller and is released when
the section is replaced, which is what detachAbout is for.
*/
func (w *Window) buildAbout(s *shell.Shell) fyne.CanvasObject {
	a := w.about()
	a.Extra = func(s *shell.Shell) fyne.CanvasObject {
		w.readme = markdown.New(hayami.README(), markdown.Options{
			FS:           hayami.Images(),
			SettleResize: 120 * time.Millisecond,
			OpenURL:      openURL,
		})
		w.readme.Follow(s.Scroller())
		return w.readme
	}
	return shell.AboutSection(a).Build(s)
}

// detachAbout releases the README pane's hold on the scroller.
func (w *Window) detachAbout() {
	if w.readme != nil {
		w.readme.Detach()
		w.readme = nil
	}
}

// about describes this program for the About section.
func (w *Window) about() shell.About {
	c := w.opts.Store.Config()
	return shell.About{
		Icon:    appIcon(),
		Name:    "hayami",
		Version: w.opts.Version,
		Blurb: "A panel for Linux and Windows: peripheral batteries, network traffic, " +
			"processor, graphics and cooler temperatures, and Claude Code and Codex " +
			"usage, on the desktop and in a terminal. Read at a glance.",
		URL:     projectURL,
		URLText: "Project documentation",
		Notes: []shell.Note{
			{Title: "Origin", Detail: "早見 (hayami, \"quick look\"). 早見表 is a chart you read " +
				"at a glance, which is the shape of the window."},
			{Title: "Two shells, one program", Detail: "The desktop panel and the terminal " +
				"panel draw the same sections from the same readings. Neither decides what " +
				"a section says; that is settled once, in one place, which is what makes " +
				"them comparable."},
			{Title: "What it writes", Detail: "One settings file and one cache of the last " +
				"readings. The KWin rule it can install is reversible from the Window " +
				"screen, and nothing else touches your desktop."},
		},
		Facts: []shell.Fact{
			{Label: "Settings file", Value: widgets.OrNone(w.settingsPath(), "not found")},
			{Label: "Sections shown", Value: shownFact(c)},
			{Label: "Licence", Value: "MIT"},
		},
	}
}

// settingsPath is the file this run's settings came from, for the facts
// table: the one --settings named when it named one, which the usual place is
// not.
func (w *Window) settingsPath() string {
	return w.opts.Store.Settings().Path()
}

// shownFact names the sections the panel is drawing, for the facts table.
func shownFact(c config.Config) string {
	if len(c.Sections) == 0 {
		return "none"
	}
	out := ""
	for i, s := range c.Sections {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
