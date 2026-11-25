//go:build !windows

package unit

// What the installer says of the service here.
const (
	OfferCaution = "Only your account can open its socket, and it has no right the panel lacks. It reads " +
		"a section only while a program asks for it, and stops itself after five minutes with nobody reading. " +
		"It needs no administrator, and an uninstall removes it."
	OfferNotHere = "This system has no service manager of your own (a systemd user manager), so the service " +
		"is not set up here. hayami-tui serve runs it by hand."
	// NotRunning is what api status says of a service installed and not
	// listening.
	NotRunning = "yes, and not listening: systemctl --user start " + SocketName
	// SetUpNote is what the installer says once it has set the service up.
	SetUpNote = "The service other programs read is set up: hayami-tui api status"
	// RemoveByHand is how to finish a removal that failed.
	RemoveByHand = "To remove it: systemctl --user disable --now " + SocketName + ", then delete " +
		SocketName + " and " + ServiceName + " from ~/.config/systemd/user"
)

// OfferSteps are what installing it does, in order.
func OfferSteps() []string {
	return []string{
		"Writes a socket and a service for your account, " + SocketName + " and " + ServiceName + ", under ~/.config/systemd/user",
		"Listens on the socket now and with every session; the first program to connect starts the service",
	}
}
