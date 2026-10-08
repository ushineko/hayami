package core

import "github.com/ushineko/sanshoku"

// PermissionDetail is what a device that may not be opened is told to do. The
// rule used to arrive with liquidctl's and OpenRazer's packages; reading the
// devices directly, nothing installs it but hayami's own installer.
const PermissionDetail = "install the udev rule (60-sanshoku.rules) and replug; see the README"

// PermissionAbsence is an Open refused for want of permission -- a hidraw node
// the logged-in user may not open, which is what the udev rule grants -- as
// the absence a reader can act on. ok is false for any other failure.
func PermissionAbsence(err error) (a *Absence, ok bool) {
	if !sanshoku.IsPermission(err) {
		return nil, false
	}
	return &Absence{Code: AbsencePermission, Detail: PermissionDetail, Actionable: true, Err: err}, true
}
