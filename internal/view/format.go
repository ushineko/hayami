package view

import (
	"fmt"
	"strings"
)

// NumberWidth is the character count a changing number is padded to.
//
// Five, because the widest a scaled value reaches is "999.9" before it becomes
// "1.0" of the next unit. The design system's glance.Rate uses the same
// number for the same reason, and the two have to agree: a rate is drawn in
// the window by that formatter and here by this one, and a column that moved
// in one shell and not the other would be the parity test's first finding.
const NumberWidth = 5

// Blank is a value that has not arrived. It is padded like a number, so a row
// is the same width before its first reading as after it.
const Blank = "--"

var byteUnits = []string{"B", "KiB", "MiB", "GiB", "TiB"}

// Rate formats bytes per second as a padded number and a separate unit.
//
// The number and the unit are returned separately because they are aligned
// separately: the number right in a fixed column, the unit left in another. A
// single string could not hold that still.
func Rate(bytesPerSecond float64) (number, unit string) {
	n, u := scale(bytesPerSecond, byteUnits)
	return pad(n), u + "/s"
}

// NoRate is a rate that has not arrived, at the width one will take.
func NoRate() (number, unit string) { return pad(Blank), "" }

// Size formats a byte total the same way, for a cumulative line.
func Size(bytes float64) (number, unit string) {
	n, u := scale(bytes, byteUnits)
	return pad(n), u
}

// NoSize is a total that has not arrived.
func NoSize() (number, unit string) { return pad(Blank), "" }

// scale picks the unit from the size of the value, then formats the number.
// The unit never sets the width: B, KiB and MiB are different widths and the
// column is not.
func scale(v float64, units []string) (string, string) {
	if v < 0 {
		v = 0
	}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	// Below the first step the value is a count of bytes and a decimal point
	// on it is noise; above it, one decimal is what keeps the width fixed.
	if i == 0 {
		return fmt.Sprintf("%.0f", v), units[i]
	}
	return fmt.Sprintf("%.1f", v), units[i]
}

// pad right-aligns a number in the fixed column.
func pad(s string) string {
	if n := NumberWidth - runeLen(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// UnitWidth is the width every unit in a column is padded to, so the unit
// column does not move either. It is computed from the units that can occur
// rather than guessed.
func UnitWidth(units ...string) int {
	w := 0
	for _, u := range units {
		w = max(w, runeLen(u))
	}
	return w
}

// PadUnit left-aligns a unit in a column of a given width.
func PadUnit(u string, width int) string {
	if n := width - runeLen(u); n > 0 {
		return u + strings.Repeat(" ", n)
	}
	return u
}
