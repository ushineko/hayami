//go:build !linux && !windows && !darwin

package api

import (
	"log/slog"
	"net"
)

// Address has no answer here yet: Windows and macOS have their own phases of
// spec 113.
func Address() (string, error) { return "", ErrNotHere }

// Listen refuses: there is no socket with a peer check here yet.
func Listen(string, *slog.Logger) (net.Listener, error) { return nil, ErrNotHere }

// Activated is never: nothing here hands the service a socket yet.
func Activated(*slog.Logger) (net.Listener, bool, error) { return nil, false, nil }

// ActivatedTCP is never: nothing here hands the service a port; it listens
// on its key's address itself (spec 115 R4).
func ActivatedTCP() (net.Listener, bool, error) { return nil, false, nil }

// Supported reports whether the service runs on this system.
func Supported() bool { return false }
