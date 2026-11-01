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
	"os"
	"path/filepath"
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

/*
WithoutHome is text with the user's home directory written as ~ wherever it
appears as a path: an error from the filesystem names its file, the file is
under the home, and the home names the user. What doctor prints and the
readings JSON carries is pasted into bug reports (spec 076).
*/
func WithoutHome(text string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return text
	}
	return WithoutHomeOf(text, home)
}

/*
WithoutHomeOf is WithoutHome for a given home. The home is replaced only
where it is a whole path, followed by a separator or by the end of a word: a
home of /home/al leaves /home/alice alone. A home of fewer than two
characters below the root is not replaced at all. On Windows the comparison
ignores case and either slash.
*/
func WithoutHomeOf(text, home string) string {
	home = strings.TrimRight(filepath.Clean(home), `/\`)
	if len(strings.Trim(home, `/\:`)) < 2 {
		return text
	}
	forms := []string{home}
	if filepath.Separator == '\\' {
		forms = append(forms, filepath.ToSlash(home))
	}
	for _, form := range forms {
		text = replacePath(text, form, filepath.Separator == '\\')
	}
	return text
}

// replacePath writes ~ for every occurrence of home in text that ends at a
// path boundary.
func replacePath(text, home string, fold bool) string {
	hay, needle := text, home
	if fold {
		hay, needle = strings.ToLower(text), strings.ToLower(home)
	}
	var b strings.Builder
	last := 0
	for i := 0; ; {
		j := strings.Index(hay[i:], needle)
		if j < 0 {
			break
		}
		start, end := i+j, i+j+len(needle)
		if end == len(text) || strings.ContainsRune(`/\ :'"`+"`"+`)]},;`, rune(text[end])) {
			b.WriteString(text[last:start])
			b.WriteString("~")
			last = end
		}
		i = end
	}
	b.WriteString(text[last:])
	return b.String()
}
