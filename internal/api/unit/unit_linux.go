package unit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// mark is each unit's first line, by which a file at the unit's path is
// known to be hayami's own and so hayami's to replace or remove.
const mark = "# Written by hayami's installer (spec 113). Running the installer again, or uninstalling hayami, removes it."

// systemctl asks the person's own service manager for something, and
// returns what it printed. A variable, so a test answers for one.
var systemctl = func(ctx context.Context, args ...string) (string, error) {
	all := append([]string{"--user"}, args...)
	out, err := exec.CommandContext(ctx, "systemctl", all...).CombinedOutput() //nolint:gosec // a fixed program; the arguments are this file's own
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("systemctl %s: %w: %s", strings.Join(all, " "), err, text)
	}
	return text, nil
}

/*
Offered says this system has a service manager of the person's own: systemd
booted the machine, and a user manager runs for this account (its directory
in the runtime directory). A variable, so a test answers for a machine it is
not on.
*/
var Offered = func() bool {
	if fi, err := os.Stat("/run/systemd/system"); err != nil || !fi.IsDir() {
		return false
	}
	run := os.Getenv("XDG_RUNTIME_DIR")
	if run == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(run, "systemd"))
	return err == nil && fi.IsDir()
}

// dir is where systemd reads a user's own units. A variable, so a test
// writes them under its own directory.
var dir = func() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding the configuration directory: %w", err)
	}
	return filepath.Join(d, "systemd", "user"), nil
}

/*
SocketText is the socket unit: the contract's address, $XDG_RUNTIME_DIR
(systemd's %t) /hayami/api.sock, 0600 in a 0700 directory, and the file
removed when the socket stops, which systemd otherwise leaves (Spike
findings). It listens with every session of the account.
*/
func SocketText() string {
	return mark + `
[Unit]
Description=hayami's service socket: the readings, for programs of this account

[Socket]
ListenStream=%t/hayami/api.sock
SocketMode=0600
DirectoryMode=0700
RemoveOnStop=yes

[Install]
WantedBy=sockets.target
`
}

/*
ServiceText is the service for program: `serve`, started by its socket,
which it takes as systemd hands it over and gives back after five minutes
with nobody reading. It runs as the person with no added right. A clean exit
stays stopped until the next connection; a crash comes back.
*/
func ServiceText(program string) (string, error) {
	quoted, err := execArgument(program)
	if err != nil {
		return "", err
	}
	return mark + `
[Unit]
Description=hayami's service: the readings, for programs of this account
Requires=` + SocketName + `
After=` + SocketName + `

[Service]
ExecStart=` + quoted + ` serve
Restart=on-failure
RestartSec=5s
NoNewPrivileges=yes
`, nil
}

/*
execArgument is an absolute path as the program of ExecStart, written as the
VPN keepalive helper's unit writes it (spec 085, where each rule was seen on
a real manager): quoted, a percent doubled since it is a specifier to
systemd, and a quote, a backslash or a control character refused, since
systemd runs no program with one in its path and a line break would be
another line of the unit.
*/
func execArgument(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %q is not absolute", ErrBadProgram, path)
	}
	if strings.ContainsFunc(path, unicode.IsControl) || strings.ContainsAny(path, `"'\`) {
		return "", fmt.Errorf("%w: systemd will not run %q, for a quote, a backslash or a control character in it", ErrBadProgram, path)
	}
	return `"` + strings.ReplaceAll(path, `%`, `%%`) + `"`, nil
}

// paths are the two units' files.
func paths() (socket, service string, err error) {
	d, err := dir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(d, SocketName), filepath.Join(d, ServiceName), nil
}

// ours says whether the file at path is hayami's: nil, or why it is not one
// to touch.
func ours(path string) error {
	body, err := os.ReadFile(path) //nolint:gosec // a path this file composed
	if err != nil {
		return fmt.Errorf("reading the unit: %w", err)
	}
	if !strings.HasPrefix(string(body), mark+"\n") {
		return fmt.Errorf("%w: %s", ErrNotOurs, path)
	}
	return nil
}

// Installed reports whether hayami's two units are there.
func Installed() bool {
	socket, service, err := paths()
	return err == nil && ours(socket) == nil && ours(service) == nil
}

/*
Install writes the units for program and listens, or puts this version in
place of one installed: a service running the old program is restarted on
the new one, and one not running starts on it at the next connection.
*/
func Install(ctx context.Context, program string) error {
	if !Offered() {
		return ErrNotHere
	}
	service, err := ServiceText(program)
	if err != nil {
		return err
	}
	socketPath, servicePath, err := paths()
	if err != nil {
		return err
	}
	for _, p := range []string{socketPath, servicePath} {
		if err := ours(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := write(socketPath, SocketText()); err != nil {
		return err
	}
	if err := write(servicePath, service); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", SocketName}, {"try-restart", ServiceName}} {
		if _, err := systemctl(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

/*
Remove stops the service and its socket and takes both units away; stopping
the socket removes its file. It removes only hayami's own, and having nothing
to remove is not a failure. Whatever the manager says, the files go: a
session with no manager to ask would otherwise outlive them. What it said is
reported after.
*/
func Remove(ctx context.Context) error {
	socketPath, servicePath, err := paths()
	if err != nil {
		return err
	}
	socketOurs, err := mine(socketPath)
	if err != nil {
		return err
	}
	serviceOurs, err := mine(servicePath)
	if err != nil {
		return err
	}
	// Only hayami's own is stopped as well as removed: a socket or service of
	// the same name that someone else wrote goes on as it was.
	var disabled error
	if socketOurs {
		_, disabled = systemctl(ctx, "disable", "--now", SocketName)
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("removing %s: %w", SocketName, err)
		}
		// The link enable made, which disable takes when the manager can be
		// asked, and which would otherwise be left pointing at nothing.
		dropWant(socketPath)
	}
	if serviceOurs {
		// A service never started is not there to stop, and says so.
		_, _ = systemctl(ctx, "stop", ServiceName)
		if err := os.Remove(servicePath); err != nil {
			return fmt.Errorf("removing %s: %w", ServiceName, err)
		}
	}
	if !socketOurs && !serviceOurs {
		return nil
	}
	_, reloaded := systemctl(ctx, "daemon-reload")
	if disabled != nil {
		// A manager that could not be asked still has the socket listening,
		// until the session ends; the files it would start from are gone.
		disabled = fmt.Errorf("%w; the socket may listen until you sign out", disabled)
	}
	return errors.Join(disabled, reloaded)
}

// mine says whether the file at path is hayami's: false with no error for
// one that is not there.
func mine(path string) (bool, error) {
	switch err := ours(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// dropWant removes sockets.target.wants' link to the socket unit, where it
// is a link to that unit and nothing else.
func dropWant(socketPath string) {
	link := filepath.Join(filepath.Dir(socketPath), "sockets.target.wants", SocketName)
	if target, err := os.Readlink(link); err == nil && target == socketPath {
		_ = os.Remove(link)
	}
}

// Status is what the service manager says of the service now. It asks; it
// connects to nothing, so it starts nothing.
func Status(ctx context.Context) State {
	s := State{Installed: Installed()}
	if !s.Installed {
		return s
	}
	// is-active prints the state, and fails for any state but active.
	socket, _ := systemctl(ctx, "is-active", SocketName)
	service, _ := systemctl(ctx, "is-active", ServiceName)
	s.Listening = socket == "active"
	s.Running = service == "active"
	return s
}
