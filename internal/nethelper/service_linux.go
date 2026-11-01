package nethelper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/netcount"
)

// Where the helper is installed, under sysRoot.
const (
	unitName    = "hayami-network"
	unitDir     = "etc/systemd/system"
	programDir  = "usr/lib/hayami"
	programName = "hayami-network"
)

// ProgramName is the terminal program's file name here.
const ProgramName = "hayami-tui"

// What the installer's Network page says of the helper here (spec 078).
const (
	OfferWhy = "Linux gives UDP, connections too short to be seen between two readings, and other accounts' programs only " +
		"to a privileged program, and the panel runs as you. The helper is a small system service that counts them and hands " +
		"the panel process IDs, the two ends of each connection and byte counts, and nothing else. Installing it:"
	OfferWithout = "Without it the bandwidth card's list of programs counts TCP only, and the rest of the panel is as it is."
	OfferCaution = "It is a system service, and stays until it is removed: uninstalling hayami removes it, and asks for an " +
		"administrator to. The panel works without it, and you can install it later by running this installer again."
)

// OfferSteps are what installing it does, in order.
func OfferSteps() []string {
	return []string{
		"copies hayami's terminal program to /usr/lib/hayami, where only root can change it",
		"writes the systemd units hayami-network.socket and hayami-network.service, and starts the socket, which only your account may open",
		"the service runs as an account made for the run, with the three capabilities eBPF needs and none of root's others",
		"starts nothing: the service runs while a panel shows the list and stops when it is closed",
		"your desktop asks for an administrator once, for all of this",
	}
}

// What the command line says of the helper here.
const (
	About = "On Linux it is a systemd service, hayami-network, started by its socket. It runs\n" +
		"as an account made for the run, with the three capabilities that loading eBPF\n" +
		"programs needs, and serves process IDs, the two ends of each connection and byte\n" +
		"counts to the one account it was installed for."
	InstallSteps = "Copies this program to /usr/lib/hayami, writes the units hayami-network.socket and\n" +
		"hayami-network.service to /etc/systemd/system, and starts the socket, which only\n" +
		"your account may open. It needs root: run as yourself, it asks for an administrator\n" +
		"through polkit (pkexec); with sudo, it asks nothing."
	InstallNeeds = "it asks for an administrator through polkit, or run it with sudo"
)

// sysRoot is the root the helper's files are written under, and systemctl
// what runs systemd's. Variables, so a test installs into a directory of
// its own and answers for systemd.
var (
	sysRoot   = "/"
	systemctl = func(ctx context.Context, args ...string) error {
		out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput() //nolint:gosec // a fixed program; the arguments are this file's own
		if err != nil {
			return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
)

func servicePath() string { return filepath.Join(sysRoot, unitDir, unitName+".service") }
func socketPath() string  { return filepath.Join(sysRoot, unitDir, unitName+".socket") }
func programPath() string { return filepath.Join(sysRoot, programDir, programName) }

// installedBy is the line each unit carries saying which hayami wrote it
// and for whom; Probe reads the version back from it.
var installedBy = regexp.MustCompile(`^# Installed by hayami (\S+) for uid (\d+)\.$`)

// Probe finds the helper's units. It reads files and asks systemd whether
// the service runs, and needs no privilege.
func Probe() State {
	_, err := lookPkexec()
	state := State{Applies: true, Offered: err == nil}
	b, err := os.ReadFile(servicePath())
	if err != nil {
		return state
	}
	state.Installed = true
	state.Path = filepath.Join("/", programDir, programName)
	for line := range strings.SplitSeq(string(b), "\n") {
		if m := installedBy.FindStringSubmatch(line); m != nil {
			state.Version, state.For = m[1], m[2]
		}
	}
	state.Running = systemctl(context.Background(), "is-active", "--quiet", unitName+".service") == nil
	return state
}

// Elevated reports running as root, which installing needs.
func Elevated() bool { return os.Geteuid() == 0 }

/*
CurrentUser is the account the helper is for, as a user ID: the one sudo
was run from when this runs under sudo, and this process's own otherwise.
*/
func CurrentUser() (string, error) {
	if os.Geteuid() == 0 {
		for _, from := range []string{"SUDO_UID", "PKEXEC_UID"} {
			if uid := os.Getenv(from); uid != "" {
				return uid, nil
			}
		}
	}
	return strconv.Itoa(os.Getuid()), nil
}

// lookPkexec finds polkit's pkexec. A variable, so a test answers for a
// machine with or without it.
var lookPkexec = func() (string, error) { return exec.LookPath("pkexec") }

/*
RunElevated runs program as root through polkit (spec 078): the desktop asks
for an administrator, or a terminal does, and the person approves or not.
It is asked only when the person chose it: the installer's Network page, or
`network-helper install` typed at a prompt. hayami never becomes root on its
own. Without pkexec it says the sudo line to run instead.
*/
func RunElevated(ctx context.Context, program string, args ...string) error {
	pkexec, err := lookPkexec()
	if err != nil {
		return fmt.Errorf("this needs root, and there is no pkexec to ask for it: run `sudo %s %s`", //nolint:err113 // the command is the message
			program, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, pkexec, append([]string{program}, args...)...) //nolint:gosec // the installed program, run as the person approved
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
			// pkexec's own codes: the request was dismissed, or not
			// authorised.
			return errors.New("the administrator prompt was dismissed or refused") //nolint:err113 // said plainly
		}
		return fmt.Errorf("the elevated step: %w", err)
	}
	return nil
}

// uidOf is an account, by name or number, as its user ID.
func uidOf(account string) (uint32, error) {
	if n, err := strconv.ParseUint(account, 10, 32); err == nil {
		return uint32(n), nil
	}
	u, err := user.Lookup(account)
	if err != nil {
		return 0, fmt.Errorf("the account %q: %w", account, err)
	}
	n, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("the account %q has user ID %q: %w", account, u.Uid, err)
	}
	return uint32(n), nil
}

/*
Install copies this program to /usr/lib/hayami, writes the two units for the
account allow, and starts the socket. Run again it puts this version in
place of the one there, and moves the socket to allow if it was another's.
*/
func Install(allow, version string) error {
	ctx := context.Background()
	uid, err := uidOf(allow)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding this program: %w", err)
	}
	if err := copyProgram(self, programPath()); err != nil {
		return err
	}
	if err := writeUnit(socketPath(), SocketUnit(version, uid)); err != nil {
		return err
	}
	if err := writeUnit(servicePath(), ServiceUnit(version, uid)); err != nil {
		return err
	}
	// The service of an earlier version is let go, and the socket made
	// again for the account named now.
	for _, args := range [][]string{
		{"daemon-reload"},
		{"stop", unitName + ".service"},
		{"enable", unitName + ".socket"},
		{"restart", unitName + ".socket"},
	} {
		if err := systemctl(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// Remove stops the socket and the service and removes both units and the
// program.
func Remove() error {
	ctx := context.Background()
	_ = systemctl(ctx, "disable", "--now", unitName+".socket")
	_ = systemctl(ctx, "stop", unitName+".service")
	for _, p := range []string{socketPath(), servicePath(), programPath()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", p, err)
		}
	}
	_ = os.Remove(filepath.Dir(programPath())) // only if nothing else is in it
	return systemctl(ctx, "daemon-reload")
}

// copyProgram puts a copy of src at dest, whole or not at all, owned by
// whoever runs this (root) and writable by nobody else.
func copyProgram(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { //nolint:gosec // a program directory is read by everyone
		return fmt.Errorf("making %s: %w", filepath.Dir(dest), err)
	}
	in, err := os.Open(src) //nolint:gosec // this program's own path
	if err != nil {
		return fmt.Errorf("reading this program: %w", err)
	}
	defer func() { _ = in.Close() }()
	tmp := dest + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755) //nolint:gosec // a program is run by everyone
	if err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("putting %s in place: %w", dest, err)
	}
	return nil
}

func writeUnit(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // systemd's directory
		return fmt.Errorf("making %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil { //nolint:gosec // a unit is read by everyone
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// SocketUnit is the socket's unit: the one account's, and started at boot,
// which costs a listening socket and nothing else until a panel connects.
func SocketUnit(version string, uid uint32) string {
	return fmt.Sprintf(`# Installed by hayami %[1]s for uid %[2]d.
# Removed by: sudo hayami-tui network-helper remove
[Unit]
Description=hayami's network helper, for the bandwidth card's list of programs

[Socket]
ListenStream=%[3]s
SocketUser=%[2]d
SocketMode=0600
# A stopped socket leaves no file in /run that a panel would take for one.
RemoveOnStop=yes

[Install]
WantedBy=sockets.target
`, version, uid, core.NetHelperSocket)
}

/*
ServiceUnit is the service's unit. Each hardening line is there because the
helper does not need what it takes away; the capabilities are the three that
loading and attaching the programs needs (spec 072 R6).
*/
func ServiceUnit(version string, uid uint32) string {
	return fmt.Sprintf(`# Installed by hayami %[1]s for uid %[2]d.
# Removed by: sudo hayami-tui network-helper remove
[Unit]
Description=hayami's network helper, for the bandwidth card's list of programs
Requires=%[3]s.socket
After=%[3]s.socket

[Service]
ExecStart=%[4]s network-helper run --allow %[2]d
# An account made for the run and gone after it.
DynamicUser=yes
# Loading programs (BPF), attaching them to the cgroup tree and the forward
# hook (NET_ADMIN), and to kernel functions (PERFMON). Nothing else.
AmbientCapabilities=CAP_BPF CAP_NET_ADMIN CAP_PERFMON
CapabilityBoundingSet=CAP_BPF CAP_NET_ADMIN CAP_PERFMON
NoNewPrivileges=yes
# It reads the kernel's type information and writes nothing anywhere.
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
# Its socket, and netlink for interface names.
RestrictAddressFamilies=AF_UNIX AF_NETLINK
SystemCallArchitectures=native
SystemCallFilter=@system-service bpf perf_event_open
`, version, uid, unitName, filepath.Join("/", programDir, programName))
}

/*
Run serves the network by process to the account whose user ID is allow,
until nobody has read it for a while. Started by systemd it is handed the
socket; started by hand, as root, it makes the socket itself and runs until
interrupted, which is how it is tried.
*/
func Run(ctx context.Context, allow, version string) error {
	uid, err := uidOf(allow)
	if err != nil {
		return err
	}
	listener, err := activated()
	if err != nil {
		return err
	}
	if listener == nil {
		if listener, err = listen(core.NetHelperSocket, int(uid)); err != nil {
			return err
		}
		var cancel context.CancelFunc
		ctx, cancel = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer cancel()
	}
	s := &server{
		listener: listener,
		allow:    uid,
		hello:    core.NetHelperLine{V: core.NetHelperFlowsProtocol, Version: version, Counts: "every protocol"},
		open:     openCounter,
		tick:     tick,
		idle:     idle,
	}
	return s.serve(ctx)
}

// openCounter loads the programs, and says in the journal what they lack.
func openCounter() (counter, error) {
	c, err := netcount.Open()
	if err != nil {
		return nil, err //nolint:wrapcheck // says which step
	}
	if c.Missing != "" {
		_, _ = fmt.Fprintln(os.Stderr, "hayami-network:", c.Missing)
	}
	return c, nil
}

// activated is the socket systemd handed over, or nil when it handed none
// (sd_listen_fds: the first passed descriptor is 3).
func activated() (*net.UnixListener, error) {
	if os.Getenv("LISTEN_PID") != strconv.Itoa(os.Getpid()) || os.Getenv("LISTEN_FDS") == "" {
		return nil, nil
	}
	f := os.NewFile(3, "hayami-network.socket")
	l, err := net.FileListener(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("the socket systemd handed over: %w", err)
	}
	ul, ok := l.(*net.UnixListener)
	if !ok {
		_ = l.Close()
		return nil, errors.New("the socket systemd handed over is not a Unix socket") //nolint:err113 // said plainly
	}
	return ul, nil
}

// listen makes the socket by hand, the one account's.
func listen(path string, uid int) (*net.UnixListener, error) {
	_ = os.Remove(path)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("making %s: %w", path, err)
	}
	if err := os.Chown(path, uid, -1); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("giving %s to uid %d: %w", path, uid, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("closing %s to others: %w", path, err)
	}
	return l, nil
}
