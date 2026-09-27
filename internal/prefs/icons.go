package prefs

import (
	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"
)

// The two icons the section list moves things with. Named here rather than
// used inline so the pair is chosen once and stays a pair.
func upIcon() fyne.Resource   { return fynetheme.MoveUpIcon() }
func downIcon() fyne.Resource { return fynetheme.MoveDownIcon() }
