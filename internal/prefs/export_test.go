package prefs

// SectionPrefKeys are the sections that have preferences of their own.
func SectionPrefKeys() []string {
	out := make([]string, 0, len(sectionPrefs))
	for key := range sectionPrefs {
		out = append(out, key)
	}
	return out
}
