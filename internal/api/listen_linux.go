package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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
activation is what systemd handed over (sd_listen_fds), read once: the
descriptors after the standard three, by their names in LISTEN_FDNAMES, when
LISTEN_PID is this process. The Unix socket's name is its unit's,
hayami-api.socket; the TCP port's is "tcp" (its unit's FileDescriptorName,
spec 115 R2). The variables are cleared, so nothing this process starts
takes them for its own.
*/
var activation = sync.OnceValues(func() (map[string]*os.File, bool) {
	if os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) {
		return nil, false
	}
	n, _ := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if n < 1 {
		return nil, false
	}
	names := strings.Split(os.Getenv("LISTEN_FDNAMES"), ":")
	_ = os.Unsetenv("LISTEN_PID")
	_ = os.Unsetenv("LISTEN_FDS")
	_ = os.Unsetenv("LISTEN_FDNAMES")
	files := make(map[string]*os.File, n)
	for i := range n {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		if name != tcpFDName {
			name = "unix"
		}
		if _, dup := files[name]; dup {
			continue
		}
		files[name] = os.NewFile(uintptr(3+i), name)
	}
	return files, true
})

// tcpFDName is the TCP port's descriptor's name, its socket unit's
// FileDescriptorName.
const tcpFDName = "tcp"

/*
Activated is the Unix socket systemd made and handed over. ok is false when
the service was not started by its sockets.
*/
func Activated(log *slog.Logger) (l net.Listener, ok bool, err error) {
	files, ok := activation()
	f := files["unix"]
	if !ok || f == nil {
		return nil, false, nil
	}
	defer func() { _ = f.Close() }()
	fl, err := net.FileListener(f)
	if err != nil {
		return nil, true, fmt.Errorf("taking the socket systemd made: %w", err)
	}
	return PeerChecked(fl, log), true, nil
}

// ActivatedTCP is the TCP port systemd made and handed over, if it did
// (spec 115 R2).
func ActivatedTCP() (net.Listener, bool, error) {
	files, ok := activation()
	f := files[tcpFDName]
	if !ok || f == nil {
		return nil, false, nil
	}
	defer func() { _ = f.Close() }()
	fl, err := net.FileListener(f)
	if err != nil {
		return nil, true, fmt.Errorf("taking the port systemd made: %w", err)
	}
	return fl, true, nil
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
