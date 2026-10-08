package core

import (
	"errors"
	"io/fs"

	"github.com/ushineko/sanshoku"
)

/*
PermissionDetail is what a device Windows would not let hayami open is told.

There is no rule to install: the HID class driver lets any user open a vendor
collection, and where Windows keeps a keyboard or mouse collection to itself
sanshoku opens it with no access rights, which still carries feature reports.
What is left is a collection another program opened without sharing it, which
is the one thing worth looking for.
*/
const PermissionDetail = "Windows refused to open it; another program may hold it without sharing"

/*
PermissionAbsence is an Open refused for want of permission, as the absence a
reader can act on. ok is false for any other failure.

sanshoku.IsPermission asks for EACCES and EPERM, which on Windows are Go's own
numbers and never what the system returns: ERROR_ACCESS_DENIED arrives as
itself. fs.ErrPermission is what an Errno says it is on either system, so it is
asked as well.
*/
func PermissionAbsence(err error) (a *Absence, ok bool) {
	if !sanshoku.IsPermission(err) && !errors.Is(err, fs.ErrPermission) {
		return nil, false
	}
	return &Absence{Code: AbsencePermission, Detail: PermissionDetail, Actionable: true, Err: err}, true
}
