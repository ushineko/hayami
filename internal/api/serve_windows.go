package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"
)

// ServiceName is the per-user service template's name (R6). Its instances
// are this and an underscore and a number Windows chooses.
const ServiceName = "hayami-api"

/*
Serve is Run, spoken as a service when the service manager started this
process (R1, R6): an instance of the per-user template, running as the
account that signed in. Started from a terminal it is Run.

An instance for an account that has never run hayami, whose settings
directory is not there, stops at once and says it ended well: the template
makes one for every account that signs in, and the recovery actions restart
only one that failed.
*/
func Serve(ctx context.Context, o Options) error {
	service, err := isService()
	if err != nil {
		return fmt.Errorf("asking whether this is a service: %w", err)
	}
	if !service {
		return Run(ctx, o)
	}
	h := &handler{ctx: ctx, o: o, run: Run}
	if !hasSettingsDir(o.SettingsPath) {
		o.Log.Info("this account has never run hayami, so there is nothing to serve; stopping")
		h.run = nil
	}
	if err := runService(ServiceName, h); err != nil {
		return fmt.Errorf("running as a service: %w", err)
	}
	return h.err
}

// isService and runService are the service manager's, variables so a test
// can be the manager.
var (
	isService  = svc.IsWindowsService
	runService = svc.Run
)

// hasSettingsDir reports whether the settings file's directory is there:
// whether this account has ever run hayami.
func hasSettingsDir(settings string) bool {
	info, err := os.Stat(filepath.Dir(settings))
	return err == nil && info.IsDir()
}

// handler is the service as the service manager sees it.
type handler struct {
	ctx context.Context //nolint:containedctx // the manager's callback takes none
	o   Options
	// run is what serves; nil stops at once, having served nothing.
	run func(context.Context, Options) error
	err error
}

/*
Execute serves until the manager says stop, or until serving ends. An end
with an error is reported as one, which is what the recovery actions restart
on; a stop asked for, or nothing to serve, is reported as a clean one.
*/
func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	if h.run == nil {
		status <- svc.Status{State: svc.StopPending}
		return false, 0
	}
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx, h.o) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			status <- svc.Status{State: svc.StopPending}
			if err != nil && !errors.Is(err, context.Canceled) {
				h.err = err
				h.o.Log.Error("serving ended", "err", err)
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
