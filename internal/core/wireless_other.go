//go:build !linux && !windows

package core

import "context"

// ReadWireless has no reader on this system: there is no Wi-Fi to describe.
func ReadWireless(context.Context) (map[string]Wireless, error) { return nil, nil }
