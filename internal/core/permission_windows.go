package core

import (
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

sanshoku.IsPermission recognises ERROR_ACCESS_DENIED since sanshoku v0.1.9
(its #39); before that hayami asked fs.ErrPermission itself (spec 035).
*/
func PermissionAbsence(err error) (a *Absence, ok bool) {
	if !sanshoku.IsPermission(err) {
		return nil, false
	}
	return &Absence{Code: AbsencePermission, Detail: PermissionDetail, Actionable: true, Err: err}, true
}
