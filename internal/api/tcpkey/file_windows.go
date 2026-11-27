//go:build windows

package tcpkey

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// accountSID is the account this process runs as.
func accountSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("asking which account this is: %w", err)
	}
	return user.User.Sid, nil
}

/*
openPrivate opens the key file only if it is this account's alone (R6,
R11): owned by this account, with a protected access list (nothing
inherited) whose every entry allows this account and nobody else. The list
is read from the file opened, not from its path.
*/
func openPrivate(path string) (*os.File, error) {
	f, err := os.Open(path) //nolint:gosec // the key's own path
	if err != nil {
		return nil, err //nolint:wrapcheck // os.ErrNotExist is looked for by Read
	}
	if err := private(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func private(f *os.File, path string) error {
	me, err := accountSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("reading who may read %s: %w", path, err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(me) {
		return fmt.Errorf("%s belongs to another account", path)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%s takes its permissions from its folder; it must grant this account alone", path)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("%s can be read by anyone; it must grant this account alone", path)
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("reading who may read %s: %w", path, err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) //nolint:gosec // the ACE's SID follows its header, as Windows lays it out
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !sid.Equals(me) {
			return fmt.Errorf("%s can be read by other accounts; it must grant this account alone", path)
		}
	}
	return nil
}

// privateDACL is the key's access list: this account, full control, nothing
// inherited.
func privateDACL(me *windows.SID) (*windows.ACL, error) {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + me.String() + ")")
	if err != nil {
		return nil, fmt.Errorf("making the key's permissions: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, fmt.Errorf("making the key's permissions: %w", err)
	}
	return dacl, nil
}

// writePrivate writes body to path as a file only this account can read,
// through a file beside it given its permissions first, then moved into
// place.
func writePrivate(path string, body []byte) error {
	me, err := accountSID()
	if err != nil {
		return err
	}
	dacl, err := privateDACL(me)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".api-tcp-*")
	if err != nil {
		return fmt.Errorf("writing the key: %w", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := windows.SetNamedSecurityInfo(name, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		me, nil, dacl, nil); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("making the key this account's alone: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing the key: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing the key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the key: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("putting the key in place: %w", err)
	}
	return nil
}
