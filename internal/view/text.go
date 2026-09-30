package view

import "unicode/utf8"

// runeLen counts characters, not bytes. An interface named in anything but
// ASCII is unusual and a column that counted bytes would still be wrong.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// visibleLen counts the characters a painted string puts on screen: runeLen
// with the escape codes a painter wraps text in left out. Padding a painted
// cell by its runeLen pads it short by the length of its colours.
func visibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// A CSI sequence: parameters, then one final byte in @..~.
			i += 2
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return n
}

// truncate cuts a string to a width, with an ellipsis when there is room for
// one. A cut that fits exactly keeps every character.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	r := []rune(s)
	return string(r[:width-1]) + "…"
}
