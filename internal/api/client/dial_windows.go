package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// pipePrefix is where every named pipe on this machine is.
const pipePrefix = `\\.\pipe\`

/*
Address is the service's address on this system, by the contract's rule
(spec 113 R3): \\.\pipe\hayami-api-<SID>, the SID being this account's. It
is the service's own rule written again, as every client writes it; a test
holds the two together.
*/
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

// ErrNotOurs is a pipe at the address served by another account's process:
// someone made it first. The client says nothing to it.
var ErrNotOurs = errors.New("the pipe is served by a process of another account")

/*
Dial opens the service's pipe at address and checks who serves it before a byte is
written (docs/api.md, "Who is at the address"). The pipe's name holds this
account's SID and the service makes it as the first instance, but a name can
be taken by anyone first; the process at the other end must run as this
account. The client is anonymous to the server, so a server that is not the
service cannot act as this account with the connection.
*/
func Dial(ctx context.Context, address string) (net.Conn, error) {
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return nil, fmt.Errorf("the pipe's name %q: %w", address, err)
	}
	var h windows.Handle
	for {
		h, err = windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
			windows.OPEN_EXISTING,
			windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_ANONYMOUS, 0)
		if err == nil {
			break
		}
		// Every instance is taken for the moment: the service makes the
		// next as it accepts. Anything else is an answer.
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, &net.OpError{Op: "dial", Net: "pipe", Addr: pipeAddr(address), Err: err}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err() //nolint:wrapcheck // the context's own message is the whole story
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := servedByThisAccount(h); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("%s: %w", address, err)
	}
	f := os.NewFile(uintptr(h), address)
	if f == nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("opening %s", address) //nolint:err113 // said plainly
	}
	return &pipeConn{File: f, addr: pipeAddr(address)}, nil
}

// servedByThisAccount checks that the process serving the pipe h runs as
// this account.
func servedByThisAccount(h windows.Handle) error {
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(h, &pid); err != nil {
		return fmt.Errorf("asking which process serves the pipe: %w", err)
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		// A process of this account can be asked; one that cannot is not
		// this account's.
		return fmt.Errorf("%w (process %d cannot be asked who it is: %w)", ErrNotOurs, pid, err)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("%w (process %d cannot be asked who it is: %w)", ErrNotOurs, pid, err)
	}
	defer func() { _ = token.Close() }()
	theirs, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("asking who process %d is: %w", pid, err)
	}
	ours, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("asking which account this is: %w", err)
	}
	if !windows.EqualSid(theirs.User.Sid, ours.User.Sid) {
		return fmt.Errorf("%w (process %d, %s)", ErrNotOurs, pid, theirs.User.Sid)
	}
	return nil
}

// pipeAddr is a pipe's name as a net.Addr.
type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// pipeConn is an open pipe as a net.Conn. The handle was opened for
// overlapped I/O, so the runtime polls it and its deadlines work.
type pipeConn struct {
	*os.File
	addr pipeAddr
}

func (c *pipeConn) LocalAddr() net.Addr  { return c.addr }
func (c *pipeConn) RemoteAddr() net.Addr { return c.addr }
