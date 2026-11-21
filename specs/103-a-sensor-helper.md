# 103 — a sensor helper

**Issue**: #365

## Status: INCOMPLETE

## Executive Summary

On Windows the panel reads the processor's temperature from a new service,
`hayami-sensors`, which reads it through PawnIO with sanshoku 0.6.0's `cpu`
package, instead of from LibreHardwareMonitor's web server.
LibreHardwareMonitor stays as the next provider for a desk that has it and
not the helper. Review `internal/sensorhelper/service_windows.go` (what runs
as LocalSystem), the generalised `internal/nethelper/service_windows.go`, and
`internal/core/sensorhelper*.go` first.

## Context

Spec 036 made LibreHardwareMonitor the source of the processor's temperature
on Windows, because Windows keeps it behind a kernel driver and hayami loads
none. That temperature is hayami's only use of LibreHardwareMonitor, which is
a second program, running elevated at every logon, with a web server that
also takes fan-control requests. It also holds the SMBus and ISA bus mutexes
for seconds at a time, which stalls other programs' lighting drivers
(sanshoku specs 044, 045).

LibreHardwareMonitor reads the temperature through PawnIO, a signed kernel
driver that runs signed modules. sanshoku 0.6.0 reads it the same way
(`cpu` package, sanshoku spec 047). PawnIO's device opens for SYSTEM and
administrators only, and the panel runs as the person at the desk, so the
reading needs a privileged piece: a service, as the network by process did
(spec 061).

## Requirements

### The helper

- R1 A Windows service, `hayami-sensors`, runs `hayami-tui sensor-helper run
  --allow <SID>` from a copy of hayami-tui in `%ProgramFiles%\hayami`
  (`hayami-sensors.exe`), as LocalSystem, started on demand.
- R2 It loads PawnIO's `AMDFamily17.bin` or `IntelMSR.bin` through sanshoku's
  `cpu` package, which calls only `ioctl_read_smn` on the thermal register or
  `ioctl_read_msr` on the two thermal MSRs.
- R3 The two modules are carried in hayami-tui (`internal/sensorhelper/modules`),
  pinned by SHA-256 to PawnIO.Modules 0.2.12, written beside the program on
  install, and compared byte for byte with the carried copy before each load.
- R4 It writes a hello and then a reading a second down a one-way pipe,
  `\\.\pipe\hayami-sensors`, whose access list gives SYSTEM and
  administrators full access and the installed account read; it reads
  nothing from the pipe. It loads the module when the first reader joins,
  unloads it when the last one leaves, and exits 15 seconds later.
- R5 Its errors (PawnIO not installed, a module refused or changed) go down
  the pipe as an error line, with what to do.

### The panel

- R6 Windows' processor chain is the sensor helper, then LibreHardwareMonitor.
  A helper that is not installed is passed over without a reason of its own,
  so LibreHardwareMonitor's account stands; one that is installed and cannot
  read gives its own reason.
- R7 The panel starts the service, opens the pipe anonymously, and refuses it
  unless the process serving it is the one the service manager started for
  `hayami-sensors`, as for the network helper.
- R8 The panel bounds what it reads: a protocol it does not know is refused,
  the helper's error loses its control characters, its label is cut short,
  and a reading outside -40 to 150 °C is not kept. A reading older than five
  seconds is no reading.
- R9 With neither source, the reason names the sensor helper and PawnIO, not
  LibreHardwareMonitor.

### Installing and removing

- R10 `hayami-tui sensor-helper install|remove|status` as for the network
  helper: install and remove elevate through Windows' prompt when not
  elevated.
- R11 An uninstall removes the sensor helper; an upgrade leaves it.
- R12 The network helper's service and pipe code is shared
  (`nethelper.Service`, `InstallService`, `ProbeService`, `RemoveService`,
  `RunService`, `Server`), not copied.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make check-no-binaries`; golangci-lint for
      windows shows nothing in the changed files.
- [ ] Tests: the chain asks the helper first and passes over one not
      installed (`TestWindowsAsksTheSensorHelperFirst`,
      `TestWindowsAsksLibreHardwareMonitorForTheProcessor`); the stream keeps
      the latest reading, refuses another version, passes an error and
      bounds what it reads (`internal/core/sensorhelper_test.go`); the
      modules are the release's (`TestTheModulesAreTheReleasesOwn`); the
      command says what is there and its run needs an account
      (`TestTheSensorHelpersCommand`); an uninstall removes it and an upgrade
      leaves it (`TestAnUninstallRemovesTheSensorHelper`).
- [ ] Live on the Windows test machine, LibreHardwareMonitor not running:
      `sensor-helper install` registers the service, the panel shows the
      processor's temperature, the service stops on its own after the panel
      closes, and `sensor-helper remove` removes the service, program and
      modules.
- [ ] The threat model's B1 and B7 sections, the architecture guide, the
      project guidelines' rule on the processor's temperature, and the
      README say what changed.

## Out of scope

- The installer's Sensors page, which still offers LibreHardwareMonitor. It
  moves to the sensor helper and PawnIO in a follow-up spec.
- Installing PawnIO's driver itself (`winget install namazso.PawnIO`).
- Removing the LibreHardwareMonitor reader.

## Risks & Assumptions

- **A LocalSystem service is a larger prize than Local Service.** What makes
  it acceptable is that it reads nothing from its pipe, its program and
  modules are in Program Files, and the panel checks who serves the pipe.
  See the threat model delta.
- **PawnIO is one maintainer's signed driver.** An adversarial review on
  2026-11-20 found no published vulnerability, no malware or blocklist
  report, an admin-only device, and module signatures checked before
  parsing. The newest signed driver (2.2.0) predates hardening fixes made in
  August 2026, and a signed module cannot be revoked; R3's byte comparison
  keeps the helper on the modules it was tested with.
- **Rollback:** revert. A desk with the helper installed keeps a stopped
  service; `sensor-helper remove` (from this version) or
  `sc.exe delete hayami-sensors` removes it, and LibreHardwareMonitor works
  as before.

## Alternatives Considered

- The network helper reading the temperature too, as the architecture guide
  said a second reading should: rejected, because it runs as Local Service
  and PawnIO needs SYSTEM or an administrator, so the network helper would
  have to become LocalSystem and its event-tracing code would run with those
  rights. Two services, each with the least it needs.
- Downloading the modules when installing, or at build time: rejected for
  carrying them, pinned by hash, so installing needs no network and the
  bytes that run are the ones reviewed.

## Threat model delta

- **B1 (privileged helpers).** A second helper, as LocalSystem. Defences:
  one-way pipe open to one account, remote clients refused, server process
  checked by the panel, program and modules in Program Files, modules
  compared with the carried copy before loading, only read functions called.
  `docs/threat-model.md` B1 updated.
- **B2 (installing).** The install copies the program and two modules to
  Program Files through the network helper's code (held-open copy, #244);
  the uninstaller removes the helper.
- **B7 (supply chain).** Two third-party signed binaries are committed,
  pinned by hash with their licence. `docs/threat-model.md` B7 updated.

## Verification

### Checks (2026-11-20)

`make test`, `make lint` and `make check-no-binaries` pass on Linux.
golangci-lint with `GOOS=windows` reports nothing in the changed files; the
one finding in `nethelper_squat_windows_test.go` was there before.

### Live run (2026-11-20, the Windows test machine, Ryzen 5 2600X)

PawnIO 2.2.0 installed, LibreHardwareMonitor not running. The installer was
built on the machine with `scripts\installer_windows.ps1 -Windowed` from
97bde2f and installed over 0.20.0 with `--cli --yes --launch=false`.

- `hayami-tui sensor-helper install`, from an elevated session: `installed:
  0.20.0-97bde2f-dev, stopped, until a panel reads the temperature`.
  `%ProgramFiles%\hayami` held `hayami-sensors.exe`, `AMDFamily17.bin`
  (10652 bytes) and `IntelMSR.bin` (5452 bytes) beside `hayami-network.exe`.
  The service was registered Manual and Stopped.
- `hayami-tui readings --sections cooler`: the processor probe read 59.75 °C,
  with LibreHardwareMonitor not running. The service was Running straight
  after, and Stopped 25 seconds later.
- The panel, started in the desk session: the service Running while it
  shows the cooler card.
- Pending: the maintainer's look at the panel on the desk, and
  `sensor-helper remove`.
