/*
Package nethelper is hayami's network helper: a service that reads the
network by process, which the system gives only to a privileged program,
and serves it to a panel that runs as the person at the desk.

On Windows (spec 061) it reads the kernel's network provider through event
tracing. It runs as Local Service, which the provider names in its access
list, and not as SYSTEM or an administrator. Its program is a copy of
hayami-tui in Program Files, where only an administrator can replace it,
because a service whose program its own user can rewrite is a way to become
that service. It writes down a pipe and reads nothing back, and the pipe
opens for the one account it was installed for.

On Linux (spec 072) it loads eBPF programs that count every packet by the
process and the two ends, and every packet forwarded. It is a systemd
service started by its socket, runs as a dynamic user with the three
capabilities loading the programs needs and nothing more, from a copy under
/usr/lib that only root can replace. The socket opens for the one account
it was installed for.

Each runs only while a panel reads it.
*/
package nethelper

import "errors"

// ErrNotWindows is every operation here on a system with no helper.
var ErrNotWindows = errors.New("the network helper is for Windows and Linux")

// State is what is known of the helper on this machine. Learning it changes
// nothing and needs no administrator.
type State struct {
	// Applies is false where there is no helper.
	Applies bool
	// Offered reports a helper hayami's installer installs. Linux's is
	// installed by hand with sudo, because the installer never becomes
	// root.
	Offered bool
	// Installed reports whether the service is registered.
	Installed bool
	// Version is the hayami its program was copied from, or "" when the
	// service does not say.
	Version string
	// Running reports whether it is serving now.
	Running bool
	// Path is its program.
	Path string
	// For is the account it serves, where the helper says: a user ID on
	// Linux.
	For string
}
