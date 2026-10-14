//go:build !linux

package installer

import (
	"context"

	"github.com/ushineko/fynstall/extension"
)

// stopEarlier stops nothing off Linux: on Windows the panel is stopped
// before its files are replaced, by beforeUninstall.
func stopEarlier(string) []string { return nil }

// placeRule and removeRule do nothing off Linux, where there is no udev.
func placeRule(context.Context, *extension.Context)  {}
func removeRule(context.Context, *extension.Context) {}
