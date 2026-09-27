package view

import "unicode/utf8"

// runeLen counts characters, not bytes. An interface named in anything but
// ASCII is unusual and a column that counted bytes would still be wrong.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

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
