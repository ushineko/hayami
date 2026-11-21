package sensorhelper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ushineko/sanshoku/cpu"
	"github.com/ushineko/sanshoku/pawnio"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/nethelper"
	"github.com/ushineko/hayami/internal/sensorhelper/modules"
)

const (
	// tick is how often a reading is sent; the panel polls every two seconds.
	tick = time.Second
	// idle is how long the helper stays with nobody reading it.
	idle = 15 * time.Second
	// missLimit is how many reads in a row may fail before the helper stops
	// and says why. One fails when another program holds the PCI mutex past
	// its wait, which any account can do; that costs a reading, not the
	// helper.
	missLimit = 30
)

// service is the sensor helper as the service manager knows it.
var service = nethelper.Service{
	Name:    core.SensorHelperService,
	Display: "hayami sensor helper",
	// PawnIO's device opens for SYSTEM and administrators, and a service
	// account that is neither cannot open it.
	Account: "LocalSystem",
	Program: "hayami-sensors.exe",
	Command: "sensor-helper",
	Describe: func(version string) string {
		return fmt.Sprintf("Reads the processor's temperature for the hayami panel, while a panel is showing it (%s).", version)
	},
	Files: modules.Files(),
}

// What the command line says of the helper here.
const (
	About = "On Windows it runs as LocalSystem, which PawnIO's driver requires, loads PawnIO's\n" +
		"signed AMDFamily17 or IntelMSR module, and serves the processor's temperature and\n" +
		"nothing else. It needs PawnIO installed (winget install namazso.PawnIO)."
	InstallSteps = "Copies this program and the two PawnIO modules it carries to Program Files,\n" +
		"registers the service hayami-sensors to run it as LocalSystem, and lets your\n" +
		"account start it. It needs an administrator: run from a terminal that is not\n" +
		"one, it asks Windows for one."
)

// Probe finds what there is of the helper. It needs no administrator.
func Probe() nethelper.State { return nethelper.ProbeService(service) }

// Install registers the helper for the account whose SID is allow. It needs
// an administrator, and is safe to run again.
func Install(allow, version string) error {
	return nethelper.InstallService(service, allow, version) //nolint:wrapcheck // says which step
}

// Remove stops the helper, unregisters it and removes its program and
// modules. It needs an administrator.
func Remove() error { return nethelper.RemoveService(service) } //nolint:wrapcheck // says which step

/*
Run serves the processor's temperature to the account whose SID is allow,
until nobody has read it for a while.
*/
func Run(ctx context.Context, allow, version string) error {
	sid, err := windows.StringToSid(allow)
	if err != nil {
		return fmt.Errorf("%q is not an account's SID: %w", allow, err)
	}
	// PawnIO's library is loaded by its full path; what it loads in turn
	// comes from System32 alone, not the machine's PATH, as this runs as
	// LocalSystem.
	if err := windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32); err != nil {
		return fmt.Errorf("limiting where libraries load from: %w", err)
	}
	r := &reading{dir: nethelper.Dir(), open: cpu.Open}
	s := &nethelper.Server{
		Pipe: core.SensorHelperPipe,
		// SYSTEM and administrators have the pipe; the account it was
		// installed for reads it. Nobody else opens it.
		SDDL:  "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GR;;;" + sid.String() + ")",
		Hello: core.SensorHelperLine{V: core.SensorHelperProtocol, Version: version},
		Take:  r.take,
		Stop:  r.stop,
		Tick:  tick,
		Idle:  idle,
		Errorf: func(err error) any {
			return core.SensorHelperLine{Error: err.Error()}
		},
	}
	return nethelper.RunService(ctx, service.Name, s) //nolint:wrapcheck // says which step
}

/*
reading is the processor's temperature, read through a module loaded on the
first take and let go of when the last reader leaves.
*/
type reading struct {
	dir    string
	open   func() (*cpu.Reader, error)
	r      *cpu.Reader
	missed int
}

func (r *reading) take() (any, error) {
	if r.r == nil {
		if err := checkModules(r.dir); err != nil {
			return nil, err
		}
		pawnio.ModuleDir = r.dir
		reader, err := r.open()
		if err != nil {
			return nil, explain(err)
		}
		r.r = reader
	}
	c, label, err := r.r.Temperature()
	if err != nil {
		if r.missed++; r.missed < missLimit {
			return nil, nil //nolint:nilnil // no line this tick
		}
		return nil, fmt.Errorf("reading the processor's temperature: %w", err)
	}
	r.missed = 0
	return core.SensorHelperReading(c, label), nil
}

func (r *reading) stop() {
	if r.r != nil {
		_ = r.r.Close()
		r.r = nil
	}
}

/*
checkModules refuses to load a module whose bytes are not the ones this
program carries. PawnIO checks a module's signature, and loads any module its
author ever signed, older ones with known faults included; this pins the
two it loads to the release they were tested from.
*/
func checkModules(dir string) error {
	for name, want := range modules.Files() {
		got, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // Program Files, the helper's own directory
		if err != nil {
			return fmt.Errorf("the PawnIO module %s: %w", name, err)
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("the PawnIO module %s in %s is not the one this hayami carries: install the helper again", name, dir) //nolint:err113 // said plainly
		}
	}
	return nil
}

// explain says what to do about PawnIO's refusals.
func explain(err error) error {
	switch {
	case errors.Is(err, pawnio.ErrNoPawnIO):
		return fmt.Errorf("PawnIO is not installed (winget install namazso.PawnIO): %w", err)
	case errors.Is(err, pawnio.ErrDenied):
		return fmt.Errorf("PawnIO refused the helper: %w", err)
	}
	return fmt.Errorf("loading PawnIO's module: %w", err)
}
