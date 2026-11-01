package safetext_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/safetext"
)

// Spec 076. What a terminal would act on is taken out: ESC and BEL (OSC 52,
// a title, a hyperlink), the eight-bit CSI, DEL, and invalid UTF-8; what is
// left is the words, and text with none of them is unchanged.
func TestOnlyTheWordsReachTheTerminal(t *testing.T) {
	for in, want := range map[string]string{
		"Logitech G502 X PLUS":                       "Logitech G502 X PLUS",
		"café ↓1.2 MiB/s":                            "café ↓1.2 MiB/s",
		"boom\x1b]52;c;aGk=\x07done":                 "boom]52;c;aGk=done",
		"\x1b]8;;https://x.test\x07link\x1b]8;;\x07": "]8;;https://x.testlink]8;;",
		"\u009b31mred\x7f":                           "31mred",
		"a\tb\nc\rd":                                 "abcd",
		"bad\xffutf8":                                "badutf8",
	} {
		assert.Equal(t, want, safetext.Printable(in, 0), "%q", in)
	}
}

// A limit is in bytes and never splits a character.
func TestALimitCutsBetweenCharacters(t *testing.T) {
	assert.Equal(t, "ab", safetext.Printable("abcdef", 2))
	assert.Equal(t, "a", safetext.Printable("aé", 2), "é is two bytes and does not fit after a")
	assert.Equal(t, "abcdef", safetext.Printable("abcdef", 0))
}
