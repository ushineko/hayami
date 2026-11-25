//go:build !windows

package client

import (
	"context"
	"net"
)

// Dial connects to the service's socket at address.
func Dial(ctx context.Context, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", address) //nolint:wrapcheck // the dialer's error names the address
}
