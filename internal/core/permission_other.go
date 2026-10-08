//go:build !linux && !windows

package core

import (
	"github.com/ushineko/sanshoku"
)

// PermissionDetail is what a device this system would not let hayami open is
// told: there is no rule hayami knows to install here.
const PermissionDetail = "the system refused to open it"

// PermissionAbsence is an Open refused for want of permission, as the absence
// a reader can act on. ok is false for any other failure.
func PermissionAbsence(err error) (a *Absence, ok bool) {
	if !sanshoku.IsPermission(err) {
		return nil, false
	}
	return &Absence{Code: AbsencePermission, Detail: PermissionDetail, Actionable: true, Err: err}, true
}
