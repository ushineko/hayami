//go:build !windows && !linux

package installer

// stopPrograms stops nothing where hayami has no installer.
func stopPrograms(string) []string { return nil }
