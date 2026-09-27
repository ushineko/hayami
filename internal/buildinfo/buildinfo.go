/*
Package buildinfo is what the program says when asked which one it is.

The version is set at link time by the Makefile and falls back to what the Go
toolchain recorded, so a binary built with `go build` and no flags still knows
something rather than saying "dev" and leaving a bug report guessing.
*/
package buildinfo

import "runtime/debug"

// version is set with -ldflags at build time.
var version string

// Version is this build's version.
func Version() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
