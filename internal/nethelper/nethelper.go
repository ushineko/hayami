/*
Package nethelper is hayami's network helper (spec 061): a Windows service
that reads the network by process, which Windows gives only to an
administrator, and serves it to a panel that runs as the person at the desk.

It is the smallest thing that does that. It runs as Local Service, which the
kernel's network provider names in its access list, and not as SYSTEM or an
administrator. It is started on demand and stops itself when nobody reads it.
Its program is a copy of hayami-tui in Program Files, where only an
administrator can replace it, because a service whose program its own user
can rewrite is a way to become that service. It writes down a pipe and reads
nothing back, and the pipe opens for the one account it was installed for.

Off Windows there is nothing to help with: Linux gives the list to the
process's owner.
*/
package nethelper

import "errors"

// ErrNotWindows is every operation here on another system.
var ErrNotWindows = errors.New("the network helper is for Windows; this system gives the list without one")

// State is what is known of the helper on this machine. Learning it changes
// nothing and needs no administrator.
type State struct {
	// Applies is false off Windows.
	Applies bool
	// Installed reports whether the service is registered.
	Installed bool
	// Version is the hayami its program was copied from, or "" when the
	// service does not say.
	Version string
	// Running reports whether it is serving now.
	Running bool
	// Path is its program.
	Path string
}
