/*
Package unit installs and removes the service other programs read (spec 113
R5, R25): on Linux a systemd user socket and service, which systemd starts
on the first connection and which exits after five minutes with nobody
reading.

It is the installer's: there is no `api install` command, and the installer
links this package without the service itself. It imports the standard
library alone.
*/
package unit

import "errors"

// The units, as systemd knows them.
const (
	SocketName  = "hayami-api.socket"
	ServiceName = "hayami-api.service"
)

// OfferWhy is what the installer says the service is for, on every system;
// what differs by system is in the system's file.
const OfferWhy = "The service gives the readings, their details and drills to other programs of your account, " +
	"as JSON over a local socket: a status bar, a script, another panel. It is what hayami-tui api reads."

// Errors the installer can be answered with.
var (
	// ErrNotHere is a system with no service manager this package writes for.
	ErrNotHere = errors.New("the service is not set up on this system; hayami-tui serve runs it by hand")

	// ErrNotOurs is a file at a unit's path that hayami did not write.
	ErrNotOurs = errors.New("that unit is not one hayami wrote, so it is left alone")

	// ErrBadProgram is a path a unit cannot name.
	ErrBadProgram = errors.New("the service's program cannot be named in a unit")
)

// State is what the service manager says of the service.
type State struct {
	// Installed is hayami's units being there.
	Installed bool
	// Listening is the socket unit active: a connection starts the service.
	Listening bool
	// Running is the service running now, not only its socket.
	Running bool
}
