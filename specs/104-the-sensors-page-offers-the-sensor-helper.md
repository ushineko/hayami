# 104 — the sensors page offers the sensor helper

**Issue**: #367

## Status: COMPLETE

## Executive Summary

The installer's Sensors page offers PawnIO, where it is missing, and the
sensor helper (spec 103), in place of LibreHardwareMonitor. It offers an
update where an older helper is installed, and nothing where this version's
is. `sensor-helper install --with-pawnio` does the same from a terminal.
winget runs as the person, never from the elevated step. Review
`installer/sensors.go` and `setUpSensors` in `installer/sensors_windows.go`
first.

## Context

Spec 103 added the sensor helper and kept the installer's Sensors page on
LibreHardwareMonitor, out of scope. A person installing hayami was still
offered a second elevated program with a web server for one reading.

## Requirements

- R1 The check reads, with no administrator: whether PawnIO's driver service
  is installed, what there is of the sensor helper, and whether
  LibreHardwareMonitor is running.
- R2 The page offers "Install PawnIO and the sensor helper" without PawnIO,
  "Install the sensor helper" with it, including on a desk that reads
  LibreHardwareMonitor, "Bring the sensor helper up to date" for another
  version's helper, and nothing for this version's.
- R3 Saying yes installs PawnIO with winget (`namazso.PawnIO`) where it is
  missing, as the person running the installer, then runs the installed
  hayami-tui's `sensor-helper install --allow <their SID>` as an
  administrator. winget is never run by an elevated process: it is found on
  the person's PATH, which they can write.
- R4 The steps and caution say what is agreed to: a kernel driver, a
  LocalSystem service, how many administrator prompts, and that uninstalling
  removes the helper and leaves PawnIO.
- R5 `hayami-tui sensor-helper install --with-pawnio` installs PawnIO the
  same way, before elevating.
- R6 A failed set-up is a warning; hayami is installed either way.
- R7 `install_windows.ps1 -WithSensors` is unchanged: it still sets up
  LibreHardwareMonitor.

## Acceptance Criteria

- [x] `make test`, `make lint`; `go vet -tags nogui` and golangci-lint for
      windows on the changed packages.
- [x] Tests: each state's words and offer
      (`TestTheCheckSaysWhatItFoundAndOffersOnlyWhatIsMissing`); the steps
      and caution (`TestTheStepsAndTheCautionSayWhatIsBeingAgreedTo`); the
      command line asks once and takes the answer, and asks nothing when
      there is nothing to offer; the hook installs only when asked and
      never fails the install; the window's check box, ticked by a flag, and
      no offer when ready (`installer/window_test.go`).
- [x] Live on the Windows test machine: the installer built from this branch,
      run on the command line with the page answered yes, updates
      0.20.0-97bde2f-dev's helper to this version's.
- [x] The README, the threat model and fynstall.yaml say what changed.

## Out of scope

- Moving `install_windows.ps1 -WithSensors` to the helper.
- Removing the LibreHardwareMonitor reader.

## Risks & Assumptions

- **winget with no version pinned**, as for LibreHardwareMonitor (#269):
  winget checks the installer against its manifest's hash. PawnIO 2.0.x was
  signed with a certificate shared with cheat software; winget's current
  version is 2.2.0.
- **Two prompts** where PawnIO is missing: one is PawnIO's own installer.
  Running winget elevated would save one and let the person's PATH choose
  what runs as an administrator.
- **Rollback:** revert. The page offers LibreHardwareMonitor again; an
  installed helper stays and is read.

## Threat model delta

- **B2 (installing).** The installer runs winget as the person, and the
  elevated step runs only the installed hayami-tui's `sensor-helper
  install`. `docs/threat-model.md`'s accepted winget paragraph names PawnIO.
- **B7.** PawnIO from winget, unpinned, as accepted for LibreHardwareMonitor.

## Verification

### Checks (2026-11-20)

`make test` and `make lint` pass on Linux; `GOOS=windows go vet -tags
nogui ./installer/` and golangci-lint for windows on `internal/sensorhelper`
and `internal/cli` report nothing.

### Live run (2026-11-20, the Windows test machine)

PawnIO installed; the helper from 0.20.0-97bde2f-dev installed. The
installer built on the machine from ab4478c, run as
`--cli --yes --sensors=yes --launch=false`, logged "Installing the sensor
helper, for the processor's temperature; the system asks for an
administrator", and `sensor-helper status` then read `installed:
0.20.0-ab4478c-dev`. The panel, started again on the desk, read the
temperature through it. PawnIO was already installed, so the winget step
did not run; the window's page was not looked at, as its layout is
unchanged and only its words are new.
