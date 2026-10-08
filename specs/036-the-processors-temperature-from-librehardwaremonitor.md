# 036 — the processor's temperature from LibreHardwareMonitor

**Issue**: #119

## Status: COMPLETE

## Context

Spec 034 gave the processor its load and name on Windows. It has no
temperature there: Windows keeps one behind a kernel driver. On the desk this
was measured on (Ryzen 5 2600X, MSI B450), the ACPI thermal zone answers "not
supported" and the thermal-zone counter has no instances. hayami loads no
driver, and should not.

LibreHardwareMonitor does. Measured on that desk, version 0.9.6 installed by
winget (`LibreHardwareMonitor.LibreHardwareMonitor`, a portable zip whose hash
winget checks):

- **It needs administrator rights and PawnIO.** On its first start it offers
  to install PawnIO, the signed driver it reads the processor through; after
  that, a `PawnIO` service is installed and running.
- **WMI is gone.** The `root\LibreHardwareMonitor` namespace does not exist in
  0.9.6. The only live interface is its web server: opt-in under Options →
  Remote Web Server, port 8085 by default, serving `data.json`.
- **`data.json` is a tree.** Each node has `Text` and `Children`; hardware
  nodes have `HardwareId` (`/amdcpu/0`, `/gpu-nvidia/0`); sensor nodes have
  `SensorId` (`/amdcpu/0/temperature/2`), `Type` (`Temperature`, `Load`,
  `Fan`, …), `Value` and `RawValue`.
- **The values are text.** `Value` and `RawValue` are both `"51.5 °C"`,
  formatted by the server, presumably in its locale.
- **`SensorId` is not unique.** `/gpu-nvidia/0/load/3` appeared twice in one
  answer, as "GPU Memory" and "GPU Bus".
- **The processor's sensor** on that desk is `Core (Tctl/Tdie)`, 51.5 °C.
  LibreHardwareMonitor publishes separate Tctl and Tdie sensors only for parts
  with a known offset. Its offset table is k10temp's, and neither lists the
  2600X, so this is the figure Linux would show for the same chip.
- **The 127.0.0.1 setting is not honoured.** `HttpServer.cs` checks the
  configured address against `Dns.GetHostEntry(hostname)`, which never lists
  loopback, and falls back to `+`, every interface. With 127.0.0.1 configured,
  the server answered on the desk's LAN address too. The same server accepts
  `Sensor?action=Set`, which changes fan settings. On that desk only Windows
  Firewall keeps it off the network: the profile is Public, the default is
  block, and no rule allows 8085.

## Requirements

- R1 `core.LHM` reads the processor's temperature from LibreHardwareMonitor's
  `data.json`:
  - **Request:** one GET of `data.json` and nothing else, within one second
    (`LHMTimeout`), cancellable, the body capped at 8 MiB.
  - **Matching:** by hardware prefix (`/amdcpu/`, `/intelcpu/`), type
    `Temperature` and label, in hwmon's order: `Core (Tdie)`,
    `Core (Tctl/Tdie)`, `Core (Tctl)`, `CPU Package`. Never by index, and
    nothing keys on `SensorId`.
  - **Values:** read as text or number, with `.` or `,` as the decimal mark.
- R2 On Windows the cooler's processor temperature is that reader. Linux keeps
  hwmon. A missing temperature is a reason whose detail says which way it is
  missing; it is an aside when the row is drawn on its load, so it shows on
  hover and in `doctor`, not on the card:
  - (a) nothing answering, no PawnIO: Windows needs LibreHardwareMonitor and
    its PawnIO driver, see the README;
  - (b) nothing answering, LibreHardwareMonitor running: its web server is
    off, with the menu path to turn it on;
  - nothing answering, PawnIO installed but LibreHardwareMonitor not running:
    start it as administrator;
  - (c) an answer with no processor temperature: is PawnIO installed?
  - (d) 401: authentication is on, and hayami sends no password.

  PawnIO is found through the service manager (connect, query status) and
  LibreHardwareMonitor through the process list, both read-only, with no
  rights and no subprocess.
- R3 The address is the `lhm` setting in `settings.yaml`. Empty means
  `http://127.0.0.1:8085/data.json`. It has no preferences control: it is set
  once, to match a port changed in LibreHardwareMonitor.
- R4 `install_windows.ps1 -WithSensors`, opt-in, does three things and prints
  each:
  - installs LibreHardwareMonitor through winget, unless it is already there;
    passing the switch accepts winget's agreements for that package;
  - writes LibreHardwareMonitor's settings (web server on, port 8085, no
    authentication), only if it has none yet;
  - starts it elevated, so UAC and its own PawnIO prompt appear.

  It honours `-DryRun`, explains the firewall point, and asks for no rule.
  `uninstall_windows.ps1` leaves LibreHardwareMonitor alone and says so.
- R5 README "On Windows": the prerequisite steps, `-WithSensors`, the
  listener's every-interface behaviour, and the `lhm` setting. Changelog
  under `### Unreleased`. CLAUDE.md names LibreHardwareMonitor's HTTP as the
  Windows source: another program's output over loopback.

## Acceptance Criteria

- [x] With LibreHardwareMonitor running, `hayami-tui doctor` shows the
  processor's temperature on Windows.
- [x] Each absence state gives its own reason: (a), (b), (c), (d) and "not
  running" by unit test with fakes, and (b) live through `doctor` with the
  `lhm` setting pointed at a closed port.
- [x] Matching order, locale decimals, a duplicate `SensorId`, a numeric
  value, a missing sensor, 401, a slow server, an oversized body and
  cancellation are tested over an invented fixture.
- [x] On a real window the processor's temperature from LibreHardwareMonitor
  is in the card's temperature column. Falsified by pointing the panel's
  reader at a closed port. Screenshot looked at.
- [x] Spec 034's window test still holds, pointed at a closed port so a
  running LibreHardwareMonitor does not give the processor a temperature.
- [x] `-WithSensors -DryRun` under Windows PowerShell 5.1 names each step and
  writes nothing. The scripts keep CRLF and a BOM.
- [x] `go test -tags migrated_fynedo ./...` passes on Windows, apart from the
  Python cross-check that fails on `main` here too. golangci-lint finds 0
  issues on the changed packages, for Windows and for Linux.

## Risks & Assumptions

- **The format is not an API.** `data.json` is LibreHardwareMonitor's web
  page's data, not a documented interface; a release may change it. The
  reader takes text or numbers and either decimal mark. A tree it cannot read
  is a reason that quotes the error, not a crash.
- **The listener is LibreHardwareMonitor's to fix.** hayami cannot bind it to
  loopback. The README says so; the installer explains the firewall point and
  adds no rule.
- **A setting that points elsewhere is the user's.** The `lhm` address is not
  restricted to loopback, so a user can read another machine's
  LibreHardwareMonitor. Only a GET of the address is made.
- **Linux is unchanged.** hwmon is still its source. The cooler's sensor seam
  now takes a context, which Linux ignores.
- **Rollback:** revert. Nothing is migrated, and a settings file with `lhm`
  in it is read by an older build without complaint (unknown keys are
  ignored).

## Gaps found

- LibreHardwareMonitor's listener ignores 127.0.0.1 (Context). Worth
  reporting upstream; not done here.
- The coolant row reads "no cooler" on a desk that has never had one (noticed
  in spec 034's screenshots). Unchanged here.
- `internal/usage`'s Python cross-check fails on this machine's Python, as on
  `main` (spec 034).

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.26.0, MSYS2 UCRT64 gcc 16.2.0,
LibreHardwareMonitor 0.9.6 running elevated with its web server on 8085 and
PawnIO installed:

- **`doctor`**:
  `cooler partial Ryzen 5 2600X 26 % 53.3 °C, RTX 3060 Ti 14 % 43.9 °C`.
- **`doctor` with `lhm: http://127.0.0.1:9/data.json`**: `CPU: no sensor` /
  `LibreHardwareMonitor is running but its web server is off: Options →
  Remote Web Server → Run`, with the load still drawn.
- **Host probe, live**: PawnIO installed true, LibreHardwareMonitor running
  true, asked without administrator rights.
- **Window**: `processors-lhm.png` reads `Ryzen 5 2600X 25 % 57.4 °C` over
  `RTX 3060 Ti 16 % 43.6 °C`. Both lines end at x=345, and both loads end at
  x=249.
  - Falsified by aiming the panel's reader at port 9: the processor's line
    ended at x=249 against the card's 345, and the test failed.
  - Spec 034's test, pointed at port 9, passes with the processor's line
    ending at the card's load (x=249).
- **Unit tests falsified**: with `LHMCPULabels` reordered, the order test
  failed on "Tdie over Tctl" and "both names over Tctl".
- **Installer**: `-DryRun -WithSensors` under `powershell.exe` 5.1 printed
  "already installed", "its settings are left as they are", "would start it
  as administrator" and the firewall note. The temporary directory stayed
  empty.
- **Suite**: every package ok except `internal/usage` (above). The
  `GOOS=linux` vet of core, config, cli, view, panel and `cmd/hayami-tui` is
  clean. `CGO_ENABLED=0` builds of `hayami-tui` succeed for windows and linux.
  golangci-lint v2.12.2 reports 0 issues (Windows and `GOOS=linux`).
