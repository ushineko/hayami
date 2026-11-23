package view

import "strings"

/*
Reflow is lines with each prose line (DetailLine.Prose) flowed into as many
wide lines as fits says it takes (spec 112): words kept whole where they fit
a line, a word longer than a line broken where it must be. fits reports
whether a line's text, at the line's indent, fits the width the shell has.
Every other line is as it was.

The window measures with the face it draws in; the terminal counts columns.
A shell flows at the width it has now, so a drawer made wider flows again.
*/
func Reflow(lines []DetailLine, fits func(text string, indent int) bool) []DetailLine {
	out := make([]DetailLine, 0, len(lines))
	for _, l := range lines {
		if !l.Prose {
			out = append(out, l)
			continue
		}
		for _, text := range flow(l.Name, func(s string) bool { return fits(s, l.Indent) }) {
			row := l
			row.Name, row.Prose = text, false
			out = append(out, row)
		}
	}
	return out
}

// flow breaks text into lines that fit, at spaces where it can.
func flow(text string, fits func(string) bool) []string {
	var out []string
	cur := ""
	for w := range strings.FieldsSeq(text) {
		if cur != "" && fits(cur+" "+w) {
			cur += " " + w
			continue
		}
		if cur != "" {
			out = append(out, cur)
		}
		// A word longer than a line is broken at the most that fits, a
		// character at least.
		word := []rune(w)
		for !fits(string(word)) && len(word) > 1 {
			// The longest head that fits, found by halves: a long hash is
			// measured a few times, not once a character.
			lo, hi := 1, len(word)-1
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if fits(string(word[:mid])) {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			out = append(out, string(word[:lo]))
			word = word[lo:]
		}
		cur = string(word)
	}
	if cur != "" || len(out) == 0 {
		out = append(out, cur)
	}
	return out
}

// FitsColumns is Reflow's fits for a shell that counts columns: a line at
// its indent, two columns a step, within width.
func FitsColumns(width int) func(string, int) bool {
	return func(s string, indent int) bool { return runeLen(s)+2*indent <= width }
}
