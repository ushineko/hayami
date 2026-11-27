//go:build !windows

package tcpkey

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

/*
openPrivate opens the key file only if it is this account's alone (R6,
R11): a regular file, not a link, owned by this account, with no permission
for the group or others. The checks are made on the file opened, not on the
path, so the file cannot be swapped between the look and the read.
*/
func openPrivate(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // the key's own path
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("%s is a link; the key must be a file of its own", path)
		}
		return nil, err //nolint:wrapcheck // os.ErrNotExist is looked for by Read
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	switch {
	case !info.Mode().IsRegular():
		err = fmt.Errorf("%s is not a regular file", path)
	case !ok || int(st.Uid) != os.Getuid():
		err = fmt.Errorf("%s belongs to another account", path)
	case info.Mode().Perm()&0o077 != 0:
		err = fmt.Errorf("%s can be read by other accounts (mode %04o); it must be 0600", path, info.Mode().Perm())
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// writePrivate writes body to path as a file only this account can read,
// through a file beside it moved into place.
func writePrivate(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".api-tcp-*")
	if err != nil {
		return fmt.Errorf("writing the key: %w", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
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
