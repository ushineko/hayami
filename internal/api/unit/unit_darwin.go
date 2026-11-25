package unit

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/ushineko/hayami/internal/api/client"
)

// Label is the launchd agent's label (R7), and its plist's name.
const Label = "io.ushineko.hayami.api"

// sockets is the agent's socket's name in its plist's Sockets dictionary,
// which the service asks launchd for (api.LaunchdSockets).
const sockets = "Listeners"

// mark is a comment in the plist, by which a file at the plist's path is known
// to be hayami's own and so hayami's to replace or remove.
const mark = "Written by hayami (spec 113). hayami-tui api-service remove, or uninstalling hayami, removes it."

// What hayami says of the service here.
const (
	OfferCaution = "Only your account can open its socket, and it has no right the panel lacks. It reads " +
		"a section only while a program asks for it, and stops itself after five minutes with nobody reading. " +
		"It is a launchd agent of your own and needs no administrator."
	OfferNotHere = "launchd is not answering for this account, so the service is not set up here. " +
		"hayami-tui serve runs it by hand."
	// NotRunning is what api status says of an agent written and not loaded.
	NotRunning = "yes, and not loaded: launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/" + Label + ".plist"
	// SetUpNote is what is said once the service is set up.
	SetUpNote = "The service other programs read is set up: hayami-tui api status"
	// RemoveByHand is how to finish a removal that failed.
	RemoveByHand = "To remove it: launchctl bootout gui/$(id -u)/" + Label + ", then delete ~/Library/LaunchAgents/" + Label + ".plist"
)

// OfferSteps are what installing it does, in order.
func OfferSteps() []string {
	return []string{
		"Writes a launchd agent for your account, " + Label + ", under ~/Library/LaunchAgents",
		"Loads it: launchd listens on the socket, and the first program to connect starts the service",
	}
}

// launchctl runs launchctl; a variable, so a test is the manager.
var launchctl = func(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput() //nolint:gosec // launchctl, with arguments this package composes
	if err != nil {
		return string(out), fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Offered is true: every macOS hayami runs on has launchd. A variable, as on
// the other systems.
var Offered = func() bool { return true }

// dir is the person's LaunchAgents directory; a variable, for the tests.
var dir = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

// uid is this account's, which names its launchd domain.
var uid = os.Getuid

func domain() string { return "gui/" + strconv.Itoa(uid()) }

// plistPath is the agent's file.
func plistPath() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, Label+".plist"), nil
}

/*
PlistText is the agent for program: run `program serve` as this account; launchd
holds the socket at the contract's address, mode 0600, and starts the service on
the first connection; restarted after a failure, five seconds apart, and not
after a clean exit, which is how it stops when idle.

Every value is escaped as XML text (xml.EscapeText), so a path cannot add a
key; the tags are the writer's own. It writes an empty boolean as `<false/>`,
since launchd refuses `<false></false>`, which plutil accepts and
encoding/xml would write (found on the Mac).
*/
func PlistText(program, socket string) (string, error) {
	if err := checkPath(program); err != nil {
		return "", err
	}
	if err := checkPath(socket); err != nil {
		return "", err
	}
	w := &plistWriter{}
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	w.b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	w.b.WriteString("<!-- " + mark + " -->\n")
	w.b.WriteString(`<plist version="1.0">` + "\n")
	w.open("dict")
	w.key("Label")
	w.str(Label)
	w.key("ProgramArguments")
	w.open("array")
	w.str(program)
	w.str("serve")
	w.close("array")
	w.key("Sockets")
	w.open("dict")
	w.key(sockets)
	w.open("dict")
	w.key("SockPathName")
	w.str(socket)
	w.key("SockPathMode")
	w.integer(0o600)
	w.close("dict")
	w.close("dict")
	w.key("KeepAlive")
	w.open("dict")
	w.key("SuccessfulExit")
	w.boolean(false)
	w.close("dict")
	w.key("ThrottleInterval")
	w.integer(5)
	w.key("ProcessType")
	w.str("Background")
	w.close("dict")
	w.b.WriteString("</plist>\n")
	if w.err != nil {
		return "", fmt.Errorf("writing the agent: %w", w.err)
	}
	return w.b.String(), nil
}

// checkPath refuses a path the agent cannot name: relative, or with a
// control character, which no file name on this system needs.
func checkPath(path string) error {
	if !filepath.IsAbs(path) || strings.ContainsFunc(path, unicode.IsControl) {
		return fmt.Errorf("%w: %q", ErrBadProgram, path)
	}
	return nil
}

// plistWriter writes a property list's elements, indented, each value
// escaped.
type plistWriter struct {
	b     bytes.Buffer
	depth int
	err   error
}

func (w *plistWriter) line(s string) {
	w.b.WriteString(strings.Repeat("\t", w.depth))
	w.b.WriteString(s)
	w.b.WriteString("\n")
}

func (w *plistWriter) open(tag string)  { w.line("<" + tag + ">"); w.depth++ }
func (w *plistWriter) close(tag string) { w.depth--; w.line("</" + tag + ">") }

// text is s escaped as XML character data.
func (w *plistWriter) text(s string) string {
	var b bytes.Buffer
	if err := xml.EscapeText(&b, []byte(s)); err != nil && w.err == nil {
		w.err = err
	}
	return b.String()
}

func (w *plistWriter) key(k string)   { w.line("<key>" + w.text(k) + "</key>") }
func (w *plistWriter) str(s string)   { w.line("<string>" + w.text(s) + "</string>") }
func (w *plistWriter) integer(n int)  { w.line("<integer>" + strconv.Itoa(n) + "</integer>") }
func (w *plistWriter) boolean(v bool) { w.line(map[bool]string{true: "<true/>", false: "<false/>"}[v]) }

// ours says whether the file at path is hayami's: nil, or why it is not one
// to touch.
func ours(path string) error {
	body, err := os.ReadFile(path) //nolint:gosec // a path this file composed
	if err != nil {
		return fmt.Errorf("reading the agent: %w", err)
	}
	if !bytes.Contains(body, []byte("<!-- "+mark+" -->")) {
		return fmt.Errorf("%w: %s", ErrNotOurs, path)
	}
	return nil
}

// Installed reports whether hayami's agent is written.
func Installed() bool {
	p, err := plistPath()
	return err == nil && ours(p) == nil
}

/*
Install writes the agent for program and loads it, or puts this version in
place of one installed: the agent is taken out of launchd and loaded again, so
a service running the old program stops and the next connection starts the
new one. It needs no administrator.
*/
func Install(ctx context.Context, program string) error {
	if err := onlyTheirs(program); err != nil {
		return err
	}
	path, err := plistPath()
	if err != nil {
		return err
	}
	if err := ours(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	socket, err := client.Address()
	if err != nil {
		return err //nolint:wrapcheck // says what is missing
	}
	text, err := PlistText(program, socket)
	if err != nil {
		return err
	}
	if err := write(path, text); err != nil {
		return err
	}
	// launchd makes the socket and not its directory: without it there is no
	// socket, and the agent fails at each start (found on the Mac).
	if err := socketDir(filepath.Dir(socket)); err != nil {
		return err
	}
	// Not loaded is not an error: a first install. bootout returns before
	// launchd has let the agent go, and loading it again meanwhile fails
	// with "Input/output error" (found on the Mac), so this waits.
	if _, err := launchctl(ctx, "bootout", domain()+"/"+Label); err == nil {
		if err := unloaded(ctx); err != nil {
			return err
		}
	}
	if _, err := launchctl(ctx, "bootstrap", domain(), path); err != nil {
		return err
	}
	// launchd makes the socket again a moment after the agent is loaded, and
	// until then a client is refused (found on the Mac): done is when it
	// answers.
	return waitAnswer(ctx, socket)
}

// unloaded waits until launchd no longer has the agent.
func unloaded(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, answerWait)
	defer cancel()
	for Loaded(ctx) {
		select {
		case <-ctx.Done():
			return errors.New("launchd did not let the agent go in ten seconds") //nolint:err113 // said plainly
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}

// answerWait is how long Install waits for launchd's socket to answer.
const answerWait = 10 * time.Second

// waitAnswer is answers; a variable, so a test with no launchd does not wait.
var waitAnswer = answers

// answers waits until the socket at path takes a connection.
func answers(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, answerWait)
	defer cancel()
	var d net.Dialer
	for {
		c, err := d.DialContext(ctx, "unix", path)
		if err == nil {
			_ = c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the agent is loaded, but its socket does not answer yet: %w", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

/*
onlyTheirs refuses a program another account could change, and its directory:
launchd runs the agent's program as this account whenever any of its programs
connects, with nobody watching, so the file must be this account's or root's
and writable by neither group nor others, and so must the directory it is in
(the adversarial review, 5a). Links are followed to the file that runs.
*/
func onlyTheirs(program string) error {
	runs, err := filepath.EvalSymlinks(program)
	if err != nil {
		return fmt.Errorf("finding the program %s: %w", program, err)
	}
	for _, p := range []string{runs, filepath.Dir(runs)} {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		if o := owner(info); o != uid() && o != 0 {
			return fmt.Errorf("%w: %s belongs to another account, which could change what the agent runs as you", ErrBadProgram, p)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%w: %s can be changed by other accounts (%s), and the agent would run it as you", ErrBadProgram, p, info.Mode().Perm())
		}
	}
	return nil
}

// owner is the account a file belongs to, or -1 when it cannot be told.
func owner(info os.FileInfo) int {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}

// socketDir makes the socket's directory this account's alone, 0700, and
// refuses one that is a link or not a directory: the directory is looked at
// as it is, not through what it points to.
func socketDir(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("making the socket's directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reading the socket's directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s is not a directory but a link or a file", path) //nolint:err113 // said plainly
	}
	if owner(info) != uid() {
		return fmt.Errorf("%s belongs to another account", path) //nolint:err113 // said plainly
	}
	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // a directory needs its search bit, and 0700 is this account's alone
		return fmt.Errorf("making the socket's directory this account's alone: %w", err)
	}
	return nil
}

/*
Remove takes the agent out of launchd and removes its file and its socket.
It removes only hayami's own, and having nothing to remove is not a failure.
*/
func Remove(ctx context.Context) error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	switch err := ours(path); {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	_, bootout := launchctl(ctx, "bootout", domain()+"/"+Label)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing the agent: %w", err)
	}
	// launchd leaves the socket's file; a removal leaves none.
	if socket, err := client.Address(); err == nil {
		if info, err := os.Lstat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
			_ = os.Remove(socket)
			_ = os.Remove(filepath.Dir(socket))
		}
	}
	if bootout != nil && !Loaded(ctx) {
		return nil
	}
	return bootout
}

// Loaded reports whether launchd has the agent.
func Loaded(ctx context.Context) bool {
	_, err := launchctl(ctx, "print", domain()+"/"+Label)
	return err == nil
}

// Status is what launchd says of the agent now. It asks; it connects to
// nothing, so it starts nothing.
func Status(ctx context.Context) State {
	s := State{Installed: Installed()}
	if !s.Installed {
		return s
	}
	out, err := launchctl(ctx, "print", domain()+"/"+Label)
	s.Listening = err == nil
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "state = running" {
			s.Running = true
		}
	}
	return s
}
