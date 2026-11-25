package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// sunPath is the size of a Unix socket address's path on macOS, its NUL
// included.
const sunPath = 104

// LaunchdSockets is the name of the agent's socket in its plist's Sockets
// dictionary, which launch_activate_socket asks for.
const LaunchdSockets = "Listeners"

/*
Address is the service's address on this system (R3): $TMPDIR/hayami/api.sock,
$TMPDIR being the per-user directory under /var/folders that only this account
has. Where the variable is unset, the directory is the system's answer for
this account (`getconf DARWIN_USER_TEMP_DIR`, confstr's
_CS_DARWIN_USER_TEMP_DIR).
*/
func Address() (string, error) {
	dir, err := userTempDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "hayami", "api.sock")
	return path, checkPath(path, sunPath)
}

// userTempDir is this account's temporary directory.
func userTempDir() (string, error) {
	if dir := os.Getenv("TMPDIR"); dir != "" {
		return dir, nil
	}
	out, err := exec.Command("/usr/bin/getconf", "DARWIN_USER_TEMP_DIR").Output() //nolint:gosec,noctx // a fixed system program
	if err != nil {
		return "", fmt.Errorf("TMPDIR is not set and the system did not say this account's temporary directory: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", errors.New("TMPDIR is not set and the system has no temporary directory for this account; name one with --listen")
	}
	return dir, nil
}

/*
Activated is the socket launchd made from the agent's Sockets entry and handed
over (launch_activate_socket), when launchd started this process for it. ok is
false when it did not: run from a terminal, or by a plist without the entry.
*/
func Activated(log *slog.Logger) (l net.Listener, ok bool, err error) {
	fds, err := launchActivateSocket(LaunchdSockets)
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("taking the socket launchd made: %w", err)
	}
	if len(fds) == 0 {
		return nil, false, nil
	}
	for _, fd := range fds[1:] {
		_ = syscall.Close(fd)
	}
	f := os.NewFile(uintptr(fds[0]), "launchd socket")
	defer func() { _ = f.Close() }()
	fl, err := net.FileListener(f)
	if err != nil {
		return nil, true, fmt.Errorf("taking the socket launchd made: %w", err)
	}
	return PeerChecked(fl, log), true, nil
}

// peerUID is the user at the other end of a Unix socket (LOCAL_PEERCRED).
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
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
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
