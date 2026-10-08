//go:build !windows

package core

// ProbeLHMHost finds nothing off Windows: LibreHardwareMonitor and PawnIO are
// Windows programs, and Linux reads the processor through hwmon.
func ProbeLHMHost() LHMHost { return LHMHost{} }
