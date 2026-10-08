package panel

import (
	"errors"
	"io/fs"

	"github.com/ushineko/sanshoku"
)

/*
permissionDetail is what a device Windows would not let hayami open is told.

There is no rule to install: the HID class driver lets any user open a vendor
collection, and where Windows keeps a keyboard or mouse collection to itself
sanshoku opens it with no access rights, which still carries feature reports.
What is left is a collection another program opened without sharing it, which
is the one thing worth looking for.
*/
const permissionDetail = "Windows refused to open it; another program may hold it without sharing"

/*
permitted is whether an Open failed for want of permission.

sanshoku.IsPermission asks for EACCES and EPERM, which on Windows are Go's own
numbers and never what the system returns: ERROR_ACCESS_DENIED arrives as
itself. fs.ErrPermission is what an Errno says it is on either system, so it is
asked as well.
*/
func permitted(err error) bool {
	return sanshoku.IsPermission(err) || errors.Is(err, fs.ErrPermission)
}

/*
bluetooth is no vendor at all on Windows (spec 035).

sanshoku's Bluetooth drivers read BlueZ, which is Linux's, and Apple's
accessory protocol, which needs an L2CAP socket Windows does not give a
program. Asked here they could only say "BlueZ is a Linux service" on every
poll: a doctor line about software nobody on this system could install.
Leaving them out says the true thing, that Bluetooth batteries are not read
here, which the README states.
*/
func bluetooth() []vendor { return nil }
