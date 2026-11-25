package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

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
Listen makes the socket at path (R4): its directory made 0700 and the socket
0600, and every connection's peer checked against this process's user. A
socket already there that answers is another service, and is a refusal to
start; one that does not is left from a service that ended, and is replaced.
*/
func Listen(path string, log *slog.Logger) (net.Listener, error) {
	if err := checkPath(path, sunPath); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("making the socket's directory: %w", err)
	}
	// The directory is looked at as it is, not through a link: a link put
	// there by someone else would have this account's directory made 0700, or
	// the socket made where they choose. Its parent is this account's or the
	// system's, so nobody else can swap the directory after this look.
	if err := ownedDir(dir, false); err != nil {
		return nil, err
	}
	if err := ownedDir(filepath.Dir(dir), true); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory needs its search bit, and 0700 is this account's alone
		return nil, fmt.Errorf("making the socket's directory this account's alone: %w", err)
	}
	ctx := context.Background()
	if _, err := os.Lstat(path); err == nil {
		dialer := net.Dialer{Timeout: time.Second}
		if c, err := dialer.DialContext(ctx, "unix", path); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("a service is already listening at %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("removing a socket left by a service that ended: %w", err)
		}
	}
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening at %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("making the socket this account's alone: %w", err)
	}
	return PeerChecked(l, log), nil
}

// ownedDir refuses a path that is a link, not a directory, or another
// account's; root's is allowed where orRoot, as for /tmp or /run/user.
func ownedDir(path string, orRoot bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reading the socket's directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s is not a directory but a link or a file", path)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot tell who owns %s", path)
	}
	if uid := int(st.Uid); uid != os.Getuid() && (!orRoot || uid != 0) {
		return fmt.Errorf("%s belongs to another account", path)
	}
	return nil
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

/*
PeerChecked is l with every connection's peer checked: one from another
account is closed without a word (R4). The socket's mode already keeps other
accounts out; this is the second lock, for a socket whose mode was changed
or a directory made by someone else.
*/
func PeerChecked(l net.Listener, log *slog.Logger) net.Listener {
	return &peerListener{Listener: l, uid: os.Getuid(), log: log}
}

type peerListener struct {
	net.Listener
	uid int
	log *slog.Logger
}

func (p *peerListener) Accept() (net.Conn, error) {
	for {
		c, err := p.Listener.Accept()
		if err != nil {
			return nil, err //nolint:wrapcheck // the listener's own error, which http.Server inspects
		}
		uid, err := peerUID(c)
		if err == nil && uid == p.uid {
			return c, nil
		}
		p.log.Warn("closed a connection from another account", "uid", uid)
		_ = c.Close()
	}
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

// Supported reports whether the service runs on this system.
func Supported() bool { return true }
