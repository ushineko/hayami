//go:build !windows

package api

import "context"

// Serve is Run: there is no service protocol to speak here; systemd and
// launchd run the service as a plain process.
func Serve(ctx context.Context, o Options) error { return Run(ctx, o) }
