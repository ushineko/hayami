//go:build windows

package api

import (
	"syscall"

	"golang.org/x/sys/windows"
)

/*
exclusiveAddress claims the port for this socket alone (review F6):
SO_EXCLUSIVEADDRUSE, so no other socket, even one asking SO_REUSEADDR, can
bind it while the service listens. Windows already keeps another account
from binding over a socket bound with the default options; this closes the
same door to a program of this account.
*/
func exclusiveAddress(_, _ string, c syscall.RawConn) error {
	// winsock2.h: #define SO_EXCLUSIVEADDRUSE ((int)(~SO_REUSEADDR)); not in
	// golang.org/x/sys/windows.
	const soExclusiveAddrUse = ^windows.SO_REUSEADDR
	var serr error
	if err := c.Control(func(fd uintptr) {
		serr = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); err != nil {
		return err //nolint:wrapcheck // the socket's own error
	}
	return serr //nolint:wrapcheck // the socket's own error
}
