package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"

	"github.com/ushineko/hayami/internal/view"
)

// Painter colours a pane.
//
// The colours are the terminal's own numbers rather than hex values: a pane
// sits inside somebody's colour scheme, and a green chosen here would be a
// green that fights whatever else is on their screen. Asking for "green" lets
// their scheme answer.
func Painter() view.Painter {
	if !colourful() {
		return nil
	}

	good := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)) // green
	warn := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)) // yellow
	bad := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))  // red
	dim := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))  // bright black
	plain := lipgloss.NewStyle()

	return func(text string, status view.Status) string {
		switch status {
		case view.Dim:
			return dim.Render(text)
		case view.Good:
			return good.Render(text)
		case view.Warn:
			return warn.Render(text)
		case view.Bad:
			return bad.Render(text)
		default:
			return plain.Render(text)
		}
	}
}

// colourful reports whether this pane should be coloured at all.
//
// It asks one question: has the person said no. NO_COLOR is honoured because
// it is the convention and because a pane is read in places a colour cannot
// reach — over a pipe, in a file, by somebody who cannot tell red from green.
// Every figure a colour emphasises is in the text as well, which is what makes
// turning it off cost nothing.
//
// **It does not ask lipgloss whether the terminal has colour.** Photographed
// in alacritty with TERM=alacritty, `lipgloss.ColorProfile()` answers 0 — the
// value that means no colour at all — on the same line that renders green.
// Gating on it turned colour off everywhere. Styles degrade on their own where
// there is none, so the right move is to let them.
func colourful() bool {
	// Present *and not empty*, which is what the convention says: a variable
	// set to nothing is a variable somebody cleared.
	return os.Getenv("NO_COLOR") == ""
}
