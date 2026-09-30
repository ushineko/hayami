package main

import "runtime/debug"

/*
GCPercent is the desktop panel's garbage-collection target: the heap may grow
to half as much again as what is live before it is collected, rather than to
twice as much.

The panel allocates little once it is running and holds a lot that stays --
some 67 MB, most of it parsed fonts (issue #79) -- so Go's default of 100
spends most of its headroom on memory that is never going to be freed. At 50
the resident size measured about 21 MB lower, for twelve more collections a
minute and no measurable CPU.

A percentage and not a memory limit. A limit saved 32 MB, but it is a fixed
number and the live heap is not: each appearance change adds about 12 MB of
fonts that Fyne's shaper keeps, and a limit at 80 MiB, measured, collected 219
times a minute and took four times the CPU. A percentage follows the heap up
and has no such wall.

The terminal panel is not given this. It is a tenth the size and has nothing
to save.
*/
const GCPercent = 50

// tuneGC applies GCPercent unless GOGC is set, so the variable still does
// what it does for any Go program and a measurement can be rerun against the
// default. It reports whether it changed anything.
func tuneGC(getenv func(string) string) bool {
	if getenv("GOGC") != "" {
		return false
	}
	debug.SetGCPercent(GCPercent)
	return true
}
