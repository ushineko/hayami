//go:build !windows

package installer

import (
	"context"

	"github.com/ushineko/fynstall/extension"
)

// probeSensors has nothing to find off Windows: the kernel gives the
// processor's temperature there (hwmon), and the page does not apply.
func probeSensors(context.Context) Sensors { return Sensors{} }

// setUpSensors is never asked off Windows: the page leaves the parameter at
// No there.
func setUpSensors(context.Context, *extension.Context) error { return nil }

// stopPrograms stops nothing off Windows, where a running program's file
// can be removed.
func stopPrograms(string) []string { return nil }
