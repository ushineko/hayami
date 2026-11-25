package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// pipePrefix is where every named pipe on this machine is.
const pipePrefix = `\\.\pipe\`

// pipeBuffer is each direction's buffer on a pipe instance.
const pipeBuffer = 64 << 10

// Address is the service's address on this system (R3): the pipe
// \\.\pipe\hayami-api-<SID>, the SID being this account's.
func Address() (string, error) {
	sid, err := accountSID()
	if err != nil {
		return "", err
	}
	return pipePrefix + "hayami-api-" + sid, nil
}

// accountSID is the SID of the account this process runs as.
func accountSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("asking which account this is: %w", err)
	}
	return user.User.Sid.String(), nil
}

// pipeSDDL is the pipe's access list (R4): this account alone, and nothing
// inherited.
func pipeSDDL(sid string) string { return "D:P(A;;GA;;;" + sid + ")" }

/*
Listen makes the pipe at path (R4). Its access list grants this account and
nobody else, a client on another machine is refused, and the pipe is made as
its first instance: a pipe already at the name, another service's or one
someone else made first, is a refusal to start rather than a second listener.
*/
func Listen(path string, _ *slog.Logger) (net.Listener, error) {
	if !strings.HasPrefix(strings.ToLower(path), pipePrefix) {
		return nil, fmt.Errorf("%s is not a pipe's name; a pipe's begins %s", path, pipePrefix)
	}
	sid, err := accountSID()
	if err != nil {
		return nil, err
	}
	l, err := winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: pipeSDDL(sid),
		InputBufferSize:    pipeBuffer,
		OutputBufferSize:   pipeBuffer,
	})
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PIPE_BUSY) ||
			errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return nil, fmt.Errorf("a service is already listening at %s, or someone else made the pipe first: %w", path, err)
		}
		return nil, fmt.Errorf("listening at %s: %w", path, err)
	}
	return l, nil
}

// Activated is never: nothing on Windows hands the service a pipe.
func Activated(*slog.Logger) (net.Listener, bool, error) { return nil, false, nil }

// Supported reports whether the service runs on this system.
func Supported() bool { return true }
