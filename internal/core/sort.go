package core

import "sort"

// sortStrings is sort.Strings, named here so the one place order is decided is
// easy to find.
func sortStrings(s []string) { sort.Strings(s) }
