package tui

import "github.com/ushineko/hayami/internal/view"

// ArrangementOf is the arrangement the model draws in.
func ArrangementOf(m Model) view.Arrangement { return m.opts.Arrangement }
