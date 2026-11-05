package keepalive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// UnitName is the user service, as systemd knows it.
const UnitName = "hayami-vpn-keepalive.service"

// unitMark is the unit's first line, by which a file at that path is known to be
// hayami's own and so hayami's to remove.
const unitMark = "# Written by hayami-tui vpn-keepalive install. hayami-tui vpn-keepalive remove takes it away."

// What the installer and the install command say, so there is one account of it.
const (
	OfferWhy = "The keepalive helper keeps a VPN working with no panel open: it runs the checks you " +
		"chose, bounces a tunnel that fails one, and re-dials one that drops."
	OfferWithout = "Without it, VPNs are shown and pressed in the panel, and nothing acts on one unasked."
	OfferCaution = "It does nothing until you switch it on for a VPN in preferences. A failed check then bounces " +
		"that VPN, and traffic leaves outside the tunnel while it is down, for as long as it will not reconnect. " +
		"Three failed checks in a row disconnect it until you re-enable it. You are told when either leaves it " +
		"down. It needs no administrator, and an uninstall removes it."
)

// OfferSteps are what installing it does, in order.
func OfferSteps() []string {
	return []string{
		"Writes a service for your account, " + UnitName + ", under ~/.config/systemd/user",
		"Starts it now, and with your desktop session from then on",
	}
}

// Errors the service's commands can answer with.
var (
	// ErrNoService is a platform with no systemd for the helper to run under.
	ErrNoService = errors.New("the VPN keepalive helper is installed on Linux, and this is not")

	// ErrNotOurs is a file at the unit's path that hayami did not write.
	ErrNotOurs = errors.New("that service is not one hayami wrote, so it is left alone")

	// ErrBadProgram is a path a unit cannot name.
	ErrBadProgram = errors.New("the helper's program cannot be named in a service")

	// ErrDidNotStart is a service the manager started and the helper never ran under.
	ErrDidNotStart = errors.New("the service is installed and the helper did not start: journalctl --user -u " + UnitName)
)

// startWait is how long a helper is given to be seen running after a start.
const startWait = 5 * time.Second

// systemctl asks the person's own service manager for something. A variable, so a test
// answers for one.
var systemctl = func(ctx context.Context, args ...string) error {
	all := append([]string{"--user"}, args...)
	out, err := exec.CommandContext(ctx, "systemctl", all...).CombinedOutput() //nolint:gosec // a fixed program; the arguments are this file's own
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(all, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// running waits for the helper to hold its lock. A variable, so a test answers for a
// helper that is not there. A manager's start returns before the program has run, so it
// says nothing of a program that cannot.
var running = func(ctx context.Context) error {
	files, err := Here()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, startWait)
	defer cancel()
	for !files.Running() {
		select {
		case <-ctx.Done():
			return ErrDidNotStart
		case <-time.After(startStep):
		}
	}
	return nil
}

// startStep is how often that looks.
const startStep = 100 * time.Millisecond

// Offered says this platform has a service for the helper to be.
func Offered() bool { return runtime.GOOS == "linux" }

// UnitPath is where systemd reads a user's own services.
func UnitPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding the configuration directory: %w", err)
	}
	return filepath.Join(dir, "systemd", "user", UnitName), nil
}

// UnitText is the service for a program. It runs as the user with no added right, in
// the desktop session, because a notification and a connection's secrets are the
// session's. A clean stop stays stopped; a crash comes back (spec 085 R21).
func UnitText(program string) (string, error) {
	quoted, err := execArgument(program)
	if err != nil {
		return "", err
	}
	return unitMark + `
[Unit]
Description=hayami's VPN keepalive helper
After=graphical-session.target
PartOf=graphical-session.target

[Service]
ExecStart=` + quoted + ` vpn-keepalive run
Restart=on-failure
RestartSec=5s
NoNewPrivileges=yes

[Install]
WantedBy=graphical-session.target
`, nil
}

// execArgument is an absolute path as the program of ExecStart, so a home directory with
// a space or a percent sign in its name is that name and nothing else. A percent is a
// specifier to systemd. A dollar stays as it is, since variables are only expanded in the
// program's arguments, and doubled it is two dollars in the path. systemd will not run a
// program with a quote or a backslash in its path however it is written, and a line break
// would be another line of the unit, so those are refused here and not left as a service
// that cannot load. All of this was seen on a real manager (spec 085).
func execArgument(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %q is not absolute", ErrBadProgram, path)
	}
	if strings.ContainsFunc(path, unicode.IsControl) || strings.ContainsAny(path, `"'\`) {
		return "", fmt.Errorf("%w: systemd will not run %q, for a quote, a backslash or a control character in it", ErrBadProgram, path)
	}
	return `"` + strings.ReplaceAll(path, `%`, `%%`) + `"`, nil
}

// Installed reports whether hayami's unit is there.
func Installed() bool {
	path, err := UnitPath()
	if err != nil {
		return false
	}
	return ours(path) == nil
}

// ours says whether the file at path is hayami's: nil, or why it is not one to touch.
func ours(path string) error {
	body, err := os.ReadFile(path) //nolint:gosec // a path this file composed
	if err != nil {
		return fmt.Errorf("reading the service: %w", err)
	}
	if !strings.HasPrefix(string(body), unitMark+"\n") {
		return fmt.Errorf("%w: %s", ErrNotOurs, path)
	}
	return nil
}

// Install writes the service for program and starts it, or puts this version in place
// of one running: a restart is what takes up a new program.
func Install(ctx context.Context, program string) error {
	if !Offered() {
		return ErrNoService
	}
	text, err := UnitText(program)
	if err != nil {
		return err
	}
	path, err := UnitPath()
	if err != nil {
		return err
	}
	if err := ours(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeUnit(path, text); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", UnitName}, {"restart", UnitName}} {
		if err := systemctl(ctx, args...); err != nil {
			return err
		}
	}
	return running(ctx)
}

// writeUnit puts the unit in place in one step. The rename replaces a link at the path
// rather than writing through it.
func writeUnit(path, text string) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("making the service directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+UnitName+".*")
	if err != nil {
		return fmt.Errorf("writing the service: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing the service: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the service: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing the service: %w", err)
	}
	return nil
}

// Remove stops the service and takes it away. It removes only hayami's own, and having
// nothing to remove is not a failure. The hold and the disable are a person's and are
// left, as the settings are.
func Remove(ctx context.Context) error {
	path, err := UnitPath()
	if err != nil {
		return err
	}
	if err := ours(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// Whatever the manager says, the file goes: a session with no manager to ask is one
	// the unit would otherwise outlive. What it said is reported after, except of the
	// stop, which fails on a unit it never loaded, and there is then nothing to stop.
	disabled := systemctl(ctx, "disable", UnitName)
	_ = systemctl(ctx, "stop", UnitName)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing the service: %w", err)
	}
	return errors.Join(disabled, systemctl(ctx, "daemon-reload"))
}
