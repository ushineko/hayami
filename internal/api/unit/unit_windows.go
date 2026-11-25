package unit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ushineko/hayami/internal/nethelper"
)

// TemplateName is the per-user service template (R6). Windows names each
// instance it makes from it this, an underscore, and a number of its own.
const TemplateName = "hayami-api"

// ProgramName is the template's program, under Program Files' hayami: a copy
// of hayami-tui only an administrator can change, because every account that
// signs in runs it.
const ProgramName = "hayami-api.exe"

// serviceUserOwnProcess is SERVICE_USER_OWN_PROCESS: a template Windows
// makes an instance of for each account that signs in.
const serviceUserOwnProcess = windows.SERVICE_WIN32_OWN_PROCESS | 0x40

// What the installer says of the service here.
const (
	OfferCaution = "Only your account can open its pipe, and it has no right the panel lacks. It reads " +
		"a section only while a program asks for it. Registering it asks for an administrator once, " +
		"and it starts at your next sign-in. An uninstall removes it."
	OfferNotHere = "This system has no service manager hayami sets the service up with, so it is not set up " +
		"here. hayami-tui serve runs it by hand."
	// NotRunning is what api status says of a service installed and not
	// running for this account.
	NotRunning = "yes, not running for this account yet: it starts at your next sign-in"
	// SetUpNote is what the installer says once it has set the service up.
	SetUpNote = "The service other programs read is set up, and starts at your next sign-in: then hayami-tui api status"
	// RemoveByHand is how to finish a removal that failed.
	RemoveByHand = "To remove it, as an administrator: sc.exe stop and sc.exe delete each hayami-api_ service, " +
		"then sc.exe delete hayami-api"
)

// OfferSteps are what installing it does, in order.
func OfferSteps() []string {
	return []string{
		"Copies hayami-tui to Program Files as " + ProgramName + ", which only an administrator can change",
		"Registers the service " + TemplateName + ", which Windows starts as your account when you sign in",
		"Asks Windows for an administrator once to do both; an upgrade asks again to update it",
	}
}

// Offered is true: every Windows hayami runs on has per-user services. A
// variable, as on Linux.
var Offered = func() bool { return true }

// Dir is where the template's program is.
func Dir() string { return nethelper.Dir() }

// Program is the template's program.
func Program() string { return filepath.Join(Dir(), ProgramName) }

// Installed reports whether the template is registered. It needs no
// administrator.
func Installed() bool {
	m, err := connect(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	name, _ := windows.UTF16PtrFromString(TemplateName)
	h, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	_ = windows.CloseServiceHandle(h)
	return true
}

func connect(access uint32) (windows.Handle, error) {
	return windows.OpenSCManager(nil, nil, access) //nolint:wrapcheck // the caller says what it was asking
}

/*
Status is the template's being there, and this account's service answering:
an instance is one per signed-in account, and the one that is this account's
is the one whose pipe is this account's.
*/
func Status(context.Context) State {
	if !Installed() {
		return State{}
	}
	running := pipeThere()
	return State{Installed: true, Listening: running, Running: running}
}

// pipeThere reports whether this account's pipe exists.
func pipeThere() bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false
	}
	name, err := windows.UTF16PtrFromString(`\\.\pipe\hayami-api-` + user.User.Sid.String())
	if err != nil {
		return false
	}
	// A pipe's attributes are read without opening one of its instances.
	_, err = windows.GetFileAttributes(name)
	return err == nil
}

/*
Install registers the template to run a copy of program, or puts the copy of
program in place of the one it runs (R6, R25). It needs an administrator.

The instances there are stopped first, since Windows will not replace a
running program, and the ones that were running are started again on the
new copy. The template's recovery actions are set as it is registered: an
instance takes them from the template when it is made and refuses them
afterwards. A first install makes no instance; Windows makes one at the next
sign-in.
*/
func Install(ctx context.Context, program string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("the service manager, which needs an administrator: %w", err)
	}
	defer func() { _ = m.Disconnect() }()

	running, err := stopInstances(ctx, m, false)
	if err != nil {
		return err
	}
	path, err := nethelper.CopyToProgramFiles(program, ProgramName)
	if err != nil {
		return err //nolint:wrapcheck // says which step
	}
	config := mgr.Config{
		ServiceType:  serviceUserOwnProcess,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  "hayami's service",
		Description:  "hayami's readings, for programs of the account signed in (spec 113). The installer registers and removes it.",
	}
	s, err := m.OpenService(TemplateName)
	if err == nil {
		defer func() { _ = s.Close() }()
		// Only the program is changed. mgr's UpdateConfig sets every field,
		// the SID type and delayed start among them, and Windows refuses
		// those on a per-user template ("The parameter is incorrect").
		command, err := windows.UTF16PtrFromString(syscall.EscapeArg(path) + " serve")
		if err != nil {
			return fmt.Errorf("the service's program %q: %w", path, err)
		}
		if err := windows.ChangeServiceConfig(s.Handle, windows.SERVICE_NO_CHANGE, mgr.StartAutomatic,
			windows.SERVICE_NO_CHANGE, command, nil, nil, nil, nil, nil, nil); err != nil {
			return fmt.Errorf("updating the service: %w", err)
		}
	} else {
		s, err = m.CreateService(TemplateName, path, config, "serve")
		if err != nil {
			return fmt.Errorf("registering the service: %w", err)
		}
		defer func() { _ = s.Close() }()
	}
	// Restart after 5 s, twice, then stay stopped; a day without a failure
	// starts the count again.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.NoAction},
	}, 24*60*60); err != nil {
		return fmt.Errorf("setting what Windows does when the service fails: %w", err)
	}
	for _, name := range running {
		if is, err := m.OpenService(name); err == nil {
			if err := is.Start(); err != nil {
				_ = is.Close()
				return fmt.Errorf("starting %s again on the new program: %w", name, err)
			}
			_ = is.Close()
		}
	}
	return nil
}

/*
Remove stops and unregisters each instance, then the template, then removes
the template's program (R25). Deleting the template first would leave its
instances registered. It needs an administrator; a service that is not
there is not an error.
*/
func Remove(ctx context.Context) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("the service manager, which needs an administrator: %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	if _, err := stopInstances(ctx, m, true); err != nil {
		return err
	}
	if s, err := m.OpenService(TemplateName); err == nil {
		err := s.Delete()
		_ = s.Close()
		if err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return fmt.Errorf("unregistering the service: %w", err)
		}
	}
	if err := os.Remove(Program()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the service's program: %w", err)
	}
	// Only when that leaves it empty: the network helper's program may be
	// there too.
	_ = os.Remove(Dir())
	return nil
}

// isInstance reports whether name is one of the template's instances.
func isInstance(name string) bool {
	rest, ok := strings.CutPrefix(name, TemplateName+"_")
	return ok && rest != ""
}

/*
stopInstances stops each instance of the template, and unregisters it too
where remove, returning those that were running.
*/
func stopInstances(ctx context.Context, m *mgr.Mgr, remove bool) ([]string, error) {
	names, err := m.ListServices()
	if err != nil {
		return nil, fmt.Errorf("listing the services: %w", err)
	}
	var running []string
	for _, name := range names {
		if !isInstance(name) {
			continue
		}
		s, err := m.OpenService(name)
		if err != nil {
			continue
		}
		was, err := stop(ctx, s)
		if err == nil && remove {
			if derr := s.Delete(); derr != nil && !errors.Is(derr, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
				err = fmt.Errorf("unregistering %s: %w", name, derr)
			}
		}
		_ = s.Close()
		if err != nil {
			return nil, err
		}
		if was {
			running = append(running, name)
		}
	}
	return running, nil
}

// stop stops s and waits for it, reporting whether it was running.
func stop(ctx context.Context, s *mgr.Service) (bool, error) {
	status, err := s.Query()
	if err != nil {
		return false, fmt.Errorf("asking %s how it is: %w", s.Name, err)
	}
	if status.State == svc.Stopped {
		return false, nil
	}
	if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return true, fmt.Errorf("stopping %s: %w", s.Name, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		if status, err := s.Query(); err == nil && status.State == svc.Stopped {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return true, fmt.Errorf("%s did not stop in ten seconds", s.Name) //nolint:err113 // said plainly
		case <-time.After(100 * time.Millisecond):
		}
	}
}
