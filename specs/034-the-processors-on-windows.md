# 034 — the processors on Windows

**Issue**: #117

## Status: COMPLETE

## Context

On Windows the cooler card showed the graphics card and no processor
(spec 033 left it so):

1. **The processor's load is `/proc/stat`**, which Windows does not have.
2. **The processor's row needed a temperature.** `view.Cooler` drew it only
   when `HasCPU` was set, so a load alone would not have been shown either.
   Windows offers no processor temperature without a kernel driver: on the
   desk this was measured on (Ryzen 5 2600X, MSI B450) the ACPI thermal zone
   answers "not supported" and the thermal-zone counter has no instances.
3. **The processor's name is `/proc/cpuinfo`.**
4. **The card needed a subprocess.** With no hwmon and no busy file, every
   poll ran `nvidia-smi`, and a card of any other vendor had no reading.
5. **The reasons named Linux.** "looked under /sys/class/hwmon for
   coretemp/Package id 0, …" on a Windows machine.

What Windows offers without privilege, measured on that desk:

- `GetSystemTimes` (kernel32): idle, kernel and user time since boot. Kernel
  time includes idle. 18.7 % over one second against Task Manager's figure.
- `HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0\ProcessorNameString`:
  "AMD Ryzen 5 2600X Six-Core Processor".
- `D3DKMTQueryAdapterInfo(KMTQAITYPE_ADAPTERPERFDATA)` (gdi32): the card's
  temperature in tenths of a degree, 43.2 °C where `nvidia-smi` said 43, and
  its fan speed. Task Manager's source; any vendor's WDDM 2.4+ driver. Two
  adapters are listed: the RTX 3060 Ti and the Basic Render Driver, which is
  flagged `SoftwareDevice` and answers no performance data.
- `KMTQAITYPE_ADAPTERREGISTRYINFO`: "NVIDIA GeForce RTX 3060 Ti".
- `\GPU Engine(*)\Utilization Percentage` (PDH): one instance per process per
  engine, named with the adapter's LUID. The busiest engine summed over
  processes read 12–18 %; LibreHardwareMonitor's "D3D 3D" for the same card
  read 16 %, and `nvidia-smi` 29 % (it measures the shader array's busy time,
  not an engine's).

## Requirements

- R1 `core.HostCPULoad` is the processor's load: `/proc/stat` off Windows,
  `GetSystemTimes` on Windows (busy = kernel + user − idle). `CPULoad` keeps
  its semantics (the first call takes two samples `CPUWarmup` apart; later
  calls difference against the previous) behind a sampler seam. No cgo.
- R2 `core.HostCPUName` is the processor's model: `/proc/cpuinfo` off
  Windows, the registry's `ProcessorNameString`, trimmed, on Windows.
- R3 The processor's row is drawn when there is a load or a temperature. With
  no temperature the temperature and its unit are spaces of their own widths,
  so the load stays in the card's load column and the row is as wide as the
  card's (glance rule). This holds on Linux too, for a machine with no
  processor sensor. With neither, there is no row.
- R4 The processor's "no sensor" reason is an aside when the row is drawn on
  its load, and on the card when there is no row. On Windows its detail says
  Windows offers no CPU temperature without a kernel driver; the card's
  reason names D3DKMT, the engine counters and `nvidia-smi`. Neither names a
  Linux path.
- R5 `GraphicsReader.Native` is the platform's own report of the card, asked
  first: on Windows the first hardware adapter (preferring one that reports a
  temperature) read through D3DKMT for temperature and name, and its load as
  the busiest engine of its LUID from the PDH query, held open across polls
  and taking two collections `CPUWarmup` apart on the first call. hwmon, the
  busy file and `nvidia-smi` fill only what it left out. Each seam is
  replaceable, so tests run no tool and touch no hardware.
- R6 `.claude/CLAUDE.md`'s `nvidia-smi` exception says that on Windows it is
  only the fallback behind D3DKMT.
- R7 README: the "On Windows" section, the cooler's sources, and a changelog
  entry under `### Unreleased`. `VERSION` unchanged.
- R8 A test drives the real panel on Windows and checks, from a picture of its
  window, that the processor's load lines up with the card's and nothing is
  drawn where its temperature would be.

## Acceptance Criteria

- [x] `go test -tags migrated_fynedo ./...` passes on Windows 11, but for the
  one pre-existing environmental failure noted under Gaps.
- [x] `GOOS=linux go vet` passes for every package that does not link Fyne,
  and `CGO_ENABLED=0 go build ./cmd/hayami-tui` succeeds for Windows and
  Linux.
- [x] golangci-lint v2.12.2 (the pinned config, Go 1.26.0) reports nothing in
  the changed packages for Linux, and nothing new for Windows beyond the
  `unsafe` audit notes every Windows syscall file already carries.
- [x] `hayami-tui doctor` on Windows shows the processor's load and name and
  the card's temperature, load and name.
- [x] A live test shows D3DKMT and PDH read the card with `nvidia-smi`
  replaced by a function that fails the test if called.
- [x] The window test passes on the desk, and the picture was looked at.
- [x] Falsified: the window test fails with the processor's row requiring a
  temperature again, and fails with the blank temperature left unpadded (the
  load drifts into the temperature's column). Both pictures looked at.

## Risks & Assumptions

- **Linux changes in one place**: a machine whose processor has a load and no
  sensor now has a row (the load, the temperature empty) where it had a
  reason on the card. Its reason becomes an aside, still in `doctor` and the
  hover note.
- **The load is Task Manager's "% Processor Time"**, not its "% Processor
  Utility"; on a boosting processor Task Manager can read higher.
- **The card's load is lower than `nvidia-smi`'s** for the same card: the
  busiest engine, not the shader array. It is Task Manager's figure, and the
  one a Windows user can check against.
- **The first poll is slower on Windows**: the first PDH collection over every
  process's engines, and two collections `CPUWarmup` apart. Measured at about
  0.45 s for the first read; later reads are one collection, and a read
  sooner than `CPUWarmup` after the last waits out the rest.
- **D3DKMT and PDH are called through `LazyProc`** with the structures laid
  out for 64-bit Windows, which is every Windows build the installer makes.
- **Rollback**: revert the commit. Nothing is stored or migrated.

## Gaps found

- `TestTheSlugAgreesWithThePythonItself` (`internal/usage`) fails on this
  desk on `main` as well: a Python runs and finds the `ag-scripts` checkout,
  whose `usage_cache.py` imports `structlog`, which that Python lacks. Spec
  033's R7 skips when no Python runs; it does not skip when one runs without
  the module. Not changed here.
- golangci-lint's `gosec` G103/G115 notes on `internal/desktop/position_windows.go`
  (spec 033) remain; CI lints on Linux, where the file is not built.
- The processor's temperature on Windows is spec 036 (#119), from
  LibreHardwareMonitor.

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.27.0 (lint under the pinned 1.26.0),
gcc 16.2.0 (MSYS2 UCRT64), Ryzen 5 2600X, RTX 3060 Ti:

- Full suite: every package ok but `internal/usage`, whose one failure is the
  pre-existing one above; `tests/window` skips without `HAYAMI_WINDOW_TEST`.
- Live: `TestOnWindowsD3DKMTReadsTheCardWithoutNvidiaSMI` logged
  "D3DKMT and PDH: 43.1 °C, 21 %, NVIDIA GeForce RTX 3060 Ti" (then 18 and
  17 % on two more runs), without running `nvidia-smi`. A debug read of the
  engine counters gave 12.5, 18.7 and 16.1 % over three seconds, all of it
  the 3D engine of one process.
- Found and fixed on the way: the first version of the test read the card
  twice within milliseconds and the second read said 0 % on a busy card. The
  engines' running time is counted coarsely, so the reader now never takes
  two collections closer than `CPUWarmup`.
- `hayami-tui doctor`:

  ```
  cooler        partial Ryzen 5 2600X 12 %, RTX 3060 Ti 24 % 43.0 °C
                        CPU: AMD Ryzen 5 2600X Six-Core Processor
                        GPU: NVIDIA GeForce RTX 3060 Ti
                        CPU: no sensor
                          Windows offers no CPU temperature without a kernel driver
  ```

- `hayami-tui --once --sections cooler`: the processor's "7 %" ends in the
  column the card's "15 %" does, and nothing follows it.
- Window test: the processor's line `[{25 143} {208 226} {240 249}]` and the
  card's `[{25 117} {209 226} {240 249} {273 312} {329 345}]` — both loads
  end at x=249, and the card's temperature follows. With the row requiring a
  temperature the card's line took the processor's place and ended at x=345,
  matching nothing below it; with the blank temperature unpadded the
  processor's load ended at x=357, in the temperature's column.
