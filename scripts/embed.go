/*
Package scripts carries the Windows installer scripts into a Go program.

The installer fynstall builds (spec 059) sets LibreHardwareMonitor up by
running install_windows.ps1 -SensorsOnly, from these same files: there is
one description of those steps, and the tests that hold it (tests/structure)
hold what the installer runs.
*/
package scripts

import "embed"

// Windows holds the scripts the installer runs: install_windows.ps1, and
// lhm_settings.ps1, which it calls.
//
//go:embed install_windows.ps1 lhm_settings.ps1
var Windows embed.FS
