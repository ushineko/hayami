package nethelper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ushineko/hayami/internal/core"
)

const (
	displayName = "hayami network helper"
	// account is who the service runs as: the least of the three the
	// kernel's network provider allows.
	account = `NT AUTHORITY\LocalService`
	// program is the service's copy of hayami-tui, under Program Files.
	program = "hayami-network.exe"
)

// description is the service's, which is also where its version is kept: a
// panel that is not an administrator can read it and cannot run a query of
// the program.
func description(version string) string {
	return fmt.Sprintf("Reads the network by process for the hayami panel, while a panel is showing it (%s).", version)
}

var versionInDescription = regexp.MustCompile(`\(([^()]+)\)\.$`)

// Dir is where the service's program is kept.
func Dir() string {
	return filepath.Join(os.Getenv("ProgramFiles"), "hayami")
}

// Probe finds what there is of the helper. It needs no administrator.
func Probe() State {
	state := State{Applies: true}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return state
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()
	name, _ := windows.UTF16PtrFromString(core.NetHelperService)
	handle, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return state
	}
	service := &mgr.Service{Name: core.NetHelperService, Handle: handle}
	defer func() { _ = service.Close() }()
	state.Installed = true
	if config, err := service.Config(); err == nil {
		state.Path = programOf(config.BinaryPathName)
		if m := versionInDescription.FindStringSubmatch(config.Description); m != nil {
			state.Version = m[1]
		}
	}
	if status, err := service.Query(); err == nil {
		state.Running = status.State == svc.Running
	}
	return state
}

// programOf is the program in a service's command line.
func programOf(command string) string {
	if rest, ok := strings.CutPrefix(command, `"`); ok {
		path, _, _ := strings.Cut(rest, `"`)
		return path
	}
	path, _, _ := strings.Cut(command, " ")
	return path
}

/*
Install registers the service for the account whose SID is allow, with a
copy of this program as its own. It needs an administrator, and is safe to
run again: a service already there is stopped, given the new program and the
new account, and left stopped for a panel to start.
*/
func Install(allow, version string) error {
	sid, err := windows.StringToSid(allow)
	if err != nil {
		return fmt.Errorf("%q is not an account's SID: %w", allow, err)
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("the service manager, which needs an administrator: %w", err)
	}
	defer func() { _ = manager.Disconnect() }()

	service, err := manager.OpenService(core.NetHelperService)
	exists := err == nil
	if exists {
		defer func() { _ = service.Close() }()
		if err := stopService(service); err != nil {
			return err
		}
	}

	path, err := copyProgram()
	if err != nil {
		return err
	}
	args := []string{"network-helper", "run", "--allow", sid.String()}
	config := mgr.Config{
		ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:        mgr.StartManual,
		ErrorControl:     mgr.ErrorNormal,
		ServiceStartName: account,
		DisplayName:      displayName,
		Description:      description(version),
	}
	if exists {
		config.BinaryPathName = commandLine(path, args)
		if err := service.UpdateConfig(config); err != nil {
			return fmt.Errorf("updating the service: %w", err)
		}
	} else {
		service, err = manager.CreateService(core.NetHelperService, path, config, args...)
		if err != nil {
			return fmt.Errorf("registering the service: %w", err)
		}
		defer func() { _ = service.Close() }()
	}
	return allowStart(service, sid)
}

func commandLine(path string, args []string) string {
	parts := []string{syscall.EscapeArg(path)}
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}

/*
allowStart gives the service the usual access list and one entry more: the
account it was installed for may start it and ask how it is. Stopping it,
changing it and removing it stay an administrator's.
*/
func allowStart(service *mgr.Service, sid *windows.SID) error {
	const usual = "D:(A;;CCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCSWLOCRRC;;;IU)(A;;CCLCSWLOCRRC;;;SU)"
	sd, err := windows.SecurityDescriptorFromString(usual + "(A;;CCLCRPLORC;;;" + sid.String() + ")")
	if err != nil {
		return fmt.Errorf("the service's access list: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("the service's access list: %w", err)
	}
	if err := windows.SetSecurityInfo(service.Handle, windows.SE_SERVICE,
		windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("letting the account start the service: %w", err)
	}
	return nil
}

// copyProgram puts this program in Program Files as the service's own, and
// returns where. The directory inherits Program Files' access list: readable
// by everyone, written by administrators.
func copyProgram() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding this program: %w", err)
	}
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // Program Files' own access list governs it
		return "", fmt.Errorf("making %s: %w", dir, err)
	}
	dest := filepath.Join(dir, program)
	if strings.EqualFold(self, dest) {
		return dest, nil
	}
	in, err := os.Open(self) //nolint:gosec // this program's own file
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", self, err)
	}
	defer func() { _ = in.Close() }()
	tmp := dest + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // a program, in Program Files
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", fmt.Errorf("putting %s in place: %w", dest, err)
	}
	return dest, nil
}

// stopService stops a running service and waits for it to have stopped.
func stopService(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("asking the service how it is: %w", err)
	}
	if status.State == svc.Stopped {
		return nil
	}
	if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return fmt.Errorf("stopping the service: %w", err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if status, err := service.Query(); err == nil && status.State == svc.Stopped {
			return nil
		}
	}
	return errors.New("the service did not stop in ten seconds") //nolint:err113 // said plainly
}

// Remove stops the service, unregisters it and removes its program. It
// needs an administrator. A helper that is not there is not an error.
func Remove() error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("the service manager, which needs an administrator: %w", err)
	}
	defer func() { _ = manager.Disconnect() }()
	if service, err := manager.OpenService(core.NetHelperService); err == nil {
		defer func() { _ = service.Close() }()
		if err := stopService(service); err != nil {
			return err
		}
		if err := service.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return fmt.Errorf("unregistering the service: %w", err)
		}
	}
	dir := Dir()
	if err := os.Remove(filepath.Join(dir, program)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the service's program: %w", err)
	}
	// Only when that leaves it empty: the directory may hold what is not ours.
	_ = os.Remove(dir)
	return nil
}

/*
Run serves the network by process to the account whose SID is allow, until
nobody has read it for a while. Started by the service manager it answers the
manager's requests; started from a terminal, which needs an administrator, it
runs until interrupted, which is how it is tried by hand.
*/
func Run(ctx context.Context, allow, version string) error {
	sid, err := windows.StringToSid(allow)
	if err != nil {
		return fmt.Errorf("%q is not an account's SID: %w", allow, err)
	}
	trace := core.NewNetTrace()
	s := &server{
		pipe: core.NetHelperPipe,
		// SYSTEM, administrators and the service itself have the pipe; the
		// account it was installed for reads it. Nobody else opens it.
		sddl:  "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;LS)(A;;GR;;;" + sid.String() + ")",
		hello: core.NetHelperLine{V: core.NetHelperProtocol, Version: version, Counts: "TCP and UDP"},
		take:  trace.Take,
		stop:  trace.Close,
		tick:  tick,
		idle:  idle,
	}
	service, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("asking whether this is a service: %w", err)
	}
	if !service {
		ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return s.serve(ctx)
	}
	h := &handler{ctx: ctx, s: s}
	if err := svc.Run(core.NetHelperService, h); err != nil {
		return fmt.Errorf("running as a service: %w", err)
	}
	return h.err
}

// handler is the service as the service manager sees it.
type handler struct {
	ctx context.Context //nolint:containedctx // the manager's callback takes none
	s   *server
	err error
}

// Execute serves until told to stop or until the server has nobody left.
func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.s.serve(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case h.err = <-done:
			status <- svc.Status{State: svc.StopPending}
			if h.err != nil {
				return false, 1
			}
			return false, 0
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
			default:
			}
		}
	}
}

// Elevated reports whether this process is an administrator's, elevated.
func Elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// CurrentUser is the SID of the account this process runs as.
func CurrentUser() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("asking who this is: %w", err)
	}
	return user.User.Sid.String(), nil
}

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	InstApp    windows.Handle
	IDList     uintptr
	Class      *uint16
	KeyClass   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

/*
RunElevated runs a program as an administrator and waits for it. Windows
asks the person at the desk first, and their saying no is the error. The
program gets no console of its own to show: it would open and close beside
whatever asked.
*/
func RunElevated(ctx context.Context, exe string, args ...string) error {
	const (
		noCloseProcess = 0x00000040
		noAsync        = 0x00000100
	)
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err //nolint:wrapcheck // a path with a NUL in it
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	params, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return err //nolint:wrapcheck // an argument with a NUL in it
	}
	info := shellExecuteInfo{Mask: noCloseProcess | noAsync, Verb: verb, File: file, Parameters: params, Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info)) //nolint:gosec // the structure's own size
	if r, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 { //nolint:gosec // the structure Windows asks for
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("Windows asked for an administrator and was told no") //nolint:err113,staticcheck // said plainly; Windows is a name
		}
		return fmt.Errorf("asking Windows for an administrator: %w", err)
	}
	if info.Process == 0 {
		return nil
	}
	defer func() { _ = windows.CloseHandle(info.Process) }()
	done := make(chan struct{})
	go func() {
		_, _ = windows.WaitForSingleObject(info.Process, windows.INFINITE)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // the context's own message is the whole story
	case <-done:
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.Process, &code); err != nil {
		return fmt.Errorf("asking how the elevated step ended: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("the elevated step ended with code %d", code) //nolint:err113 // the code is the message
	}
	return nil
}
