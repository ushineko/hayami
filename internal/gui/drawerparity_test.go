package gui_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

/*
Spec 098 R12. The window's drawer and the terminal's draw the same detail
alike: for a list under a summary, every fact the window's drawer shows (the
title, each summary line's name, each list line's name, the note or its
absence where the summary covers it) is in the terminal's drawer at the same
height, and the terminal shows no line of the list the window leaves out.
*/
func TestTheWindowAndTheTerminalDrawersShowTheSameFacts(t *testing.T) {
	details := map[string]view.Detail{
		view.DetailMemory: summed(200, 12),
		view.DetailCPU:    cpuSummed(200, 40, 90, -1, 10),
	}
	for id, d := range details {
		t.Run(id, func(t *testing.T) {
			src := newDetailSource("cooler", coolerSection(), map[string]view.Detail{id: d})
			p, await := drawerPanel(t, src)
			gui.Tap(p, "cooler", id, -1)
			await("the summary", func() bool { lines, _ := gui.DrawerSummary(p); return len(lines) > 0 })

			window := gui.DrawerLines(p)
			summary, _ := gui.DrawerSummary(p)
			notes := gui.DrawerNoteLines(p)
			// The terminal's drawer at the height the window's list, summary
			// and tail take, with its heading, switch and filter lines over
			// them as the window's drawer has its own.
			height := len(window) + len(notes) + view.SummaryKeepsLines
			pane := strings.Join(view.RenderDetail(view.PaneDetail(d, ""), 120, height, view.DrawerShape{Marked: -1}), "\n")

			assert.Contains(t, pane, d.Title)
			for _, l := range summary {
				name := strings.Split(l, "|")[0]
				assert.Contains(t, pane, "  "+name, "a summary line the terminal leaves out")
			}
			names := map[string]bool{}
			for _, l := range window {
				name := strings.Split(l, "|")[0]
				names[name] = true
				assert.Contains(t, pane, "  "+name+" ", "a list line the terminal leaves out")
			}
			require.NotEmpty(t, names)
			for _, l := range d.Lines {
				if !names[l.Name] && strings.Contains(pane, "  "+l.Name+" ") {
					// The terminal may fit a line or two more than the window
					// at this height; never one the window shows above it.
					assert.Greater(t, indexOf(d.Lines, l.Name), len(names)-1, "%q drawn out of order", l.Name)
				}
			}
			if d.Note != "" {
				assert.Equal(t, len(notes) > 0, strings.Contains(pane, d.Note), "the note, or its absence, differs")
			}
		})
	}
}

func indexOf(lines []view.DetailLine, name string) int {
	for i, l := range lines {
		if l.Name == name {
			return i
		}
	}
	return -1
}
