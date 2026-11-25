package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// sunPath is the size of a Unix socket address's path on Linux, its NUL
// included.
const sunPath = 108

// Address is the service's address on this system (R3):
// $XDG_RUNTIME_DIR/hayami/api.sock, in the directory only this account has.
func Address() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set, so there is no directory of this account's to put the socket in; name one with --listen")
	}
	path := filepath.Join(dir, "hayami", "api.sock")
	return path, checkPath(path, sunPath)
}

/*
Activated is the socket systemd made and handed over (sd_listen_fds): the
first descriptor after the standard three, when LISTEN_PID is this process.
ok is false when the service was not started by its socket.
*/
func Activated(log *slog.Logger) (l net.Listener, ok bool, err error) {
	if os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return nil, false, nil
	}
	if n, _ := strconv.Atoi(os.Getenv("LISTEN_FDS")); n < 1 {
		return nil, false, nil
	}
	_ = os.Unsetenv("LISTEN_PID")
	_ = os.Unsetenv("LISTEN_FDS")
	_ = os.Unsetenv("LISTEN_FDNAMES")
	f := os.NewFile(3, "hayami-api.socket")
	defer func() { _ = f.Close() }()
	fl, err := net.FileListener(f)
	if err != nil {
		return nil, true, fmt.Errorf("taking the socket systemd made: %w", err)
	}
	return PeerChecked(fl, log), true, nil
}

// peerUID is the user at the other end of a Unix socket.
func peerUID(c net.Conn) (int, error) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return -1, errors.New("not a Unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return -1, fmt.Errorf("reading the peer: %w", err)
	}
	uid := -1
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			credErr = err
			return
		}
		uid = int(cred.Uid)
	}); err != nil {
		return -1, fmt.Errorf("reading the peer: %w", err)
	}
	return uid, credErr
}
