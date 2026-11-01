/*
Package safetext makes text from outside hayami safe to write to a terminal
(spec 076).

A device's name, a process's name, a label in a provider's reply, a cache
file's name, an error a helper sends: hayami does not choose any of them, and
an escape sequence in one would be acted on by the terminal it is written to
(OSC 52 writes the clipboard, others retitle the window or fake a link).
Everything the terminal pane, `doctor` and `detail` write passes through
Printable on its way out, before the pane adds colours of its own.
*/
package safetext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Printable is text with every control character taken out, C0, C1 and DEL,
// and invalid UTF-8 with it, at most n bytes long; n <= 0 is no limit. Text
// that has none is returned as it is.
func Printable(text string, n int) string {
	clean := true
	for _, r := range text {
		if r == utf8.RuneError || unicode.IsControl(r) {
			clean = false
			break
		}
	}
	if clean && (n <= 0 || len(text) <= n) {
		return text
	}
	var b strings.Builder
	for _, r := range text {
		if n > 0 && b.Len()+utf8.RuneLen(r) > n {
			break
		}
		if r == utf8.RuneError || unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
