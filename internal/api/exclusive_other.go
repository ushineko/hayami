//go:build !windows

package api

import "syscall"

// exclusiveAddress is nothing to add: Linux and macOS refuse a second bind
// of a listening port by another account without a socket option (review
// F6, checked on Linux).
func exclusiveAddress(_, _ string, _ syscall.RawConn) error { return nil }
