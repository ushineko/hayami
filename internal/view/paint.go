package view

// Painter turns a piece of text and the view's verdict on it into text a shell
// can draw.
//
// It exists so that colour can happen without this package knowing what colour
// is. A section says *what* a reading is — good, worth attention, a failure —
// and the shell decides what to do about it; the terminal wraps the text in
// escape codes and the window ignores the painter entirely, because Fyne
// colours its own widgets.
//
// It is deliberately a small hole. A painter sees a string and a status and
// nothing else: not the reading, not the section, not the arrangement. The
// most it can do is colour what the view already classified, which is what
// stops a shell from quietly deciding what a section says.
type Painter func(text string, status Status) string

// Dim is the status of something that is not a reading: a label, a bar's
// empty track, a heading. It is not one of the verdicts — nothing is "dim" —
// so it is a status of its own, past the ones a reading can have.
const Dim Status = -1

// paint applies a painter, or returns the text when there is none.
//
// Nothing is painted without a painter, which is what keeps every existing
// test, every pipe and every redirect to a file seeing exactly the text this
// package built.
func (p Painter) paint(text string, status Status) string {
	if p == nil {
		return text
	}
	return p(text, status)
}
