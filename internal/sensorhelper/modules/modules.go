// Package modules carries the signed PawnIO modules the sensor helper loads
// (spec 103). See README.md for where they come from.
package modules

import _ "embed"

// AMDFamily17 reads an AMD Zen processor's SMN registers.
//
//go:embed AMDFamily17.bin
var AMDFamily17 []byte

// IntelMSR reads an Intel processor's thermal MSRs.
//
//go:embed IntelMSR.bin
var IntelMSR []byte

// Files are the modules by the names PawnIO's loader is given.
func Files() map[string][]byte {
	return map[string][]byte{"AMDFamily17.bin": AMDFamily17, "IntelMSR.bin": IntelMSR}
}
