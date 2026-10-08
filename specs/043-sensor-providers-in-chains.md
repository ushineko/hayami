# 043 — Sensor providers in chains, and one platform table

**Issue**: #137

## Status: COMPLETE

## Context

Phase 2 of the architecture review after spec 037 (`docs/architecture.md`,
rules 4 and 5). The cooler's sources were wired by hand, and what a platform
offered was spread across build-tagged files in two packages.

Inventory on `main` (6fdcea1) before this spec:

- **The processor's temperature** was one function per platform with its own
  branching and its own sentence: `core.HostCPUTemperature` in
  `cputemp_other.go:28` (hwmon.First over hwmon.CPU) and
  `cputemp_windows.go:18` (LibreHardwareMonitor, wrapping anything it did not
  account for), each beside a hand-written `CPUSensorDetail`
  (`cputemp_other.go:16`, `cputemp_windows.go:10`).
- **The graphics card** was an if-chain in `core.GraphicsReader.Read`
  (`processors.go:211`): native, hwmon, the busy file, nvidia-smi, each
  guarded by what the previous one left. `NewGraphicsReader`
  (`processors.go:203`) set `Root: hwmon.Root` and `Busy: BusyGlob` on every
  platform, so on Windows the reader looked under `/sys/class/hwmon` for a
  card every five seconds. The reason's detail was a sentence per platform
  (`graphics_other.go:20`, `graphics_windows.go:317`) that listed the routes
  by hand: change the chain and the sentence went on describing the old one.
- **`_other` assumed Linux.** `counters_other.go`, `graphics_other.go`,
  `host_other.go`, `cputemp_other.go` and `permission_other.go` were
  `//go:build !windows`: a third platform would have read `/proc/net/dev` and
  `/proc/stat`, looked under `/sys`, and told the user to install a udev rule.
- **`panel` kept platform knowledge.** `platform_other.go:14` and
  `platform_windows.go:13` (`bluetooth()`) decided whether the Bluetooth
  drivers were asked, and `cooler.go:89-92` and `devices.go:115` called the
  core platform functions directly.

Touch points to add a new sensor source, before:

| Source | Places |
|---|---|
| Processor temperature (another program on Windows, say) | the platform's `HostCPUTemperature` (its branching), its `CPUSensorDetail` sentence, and possibly `panel/cooler.go` — **3** |
| Graphics card (another vendor API) | a `GraphicsReader` field, a branch in `Read`, `NewGraphicsReader`, the platform's `GPUSensorDetail` sentence — **4** |

## Requirements

- R1 `core.Provider[T]` (a name a person reads, and `Read(ctx)` returning a
  value or an error — a `*core.Absence` where it can say why) and
  `core.Chain[T]` (providers in order, an optional `Merge` that fills what is
  missing and says when the reading is complete, and an `Outcome`: the value,
  whether any provider answered, every provider tried, the first account of
  absence and the first error). A nil `Merge` takes the first answer. A
  cancelled context stops the chain.
- R2 The processor's temperature is a chain: Linux, one provider per hwmon
  sensor in `hwmon.CPU`'s order; Windows, LibreHardwareMonitor. The graphics
  card is a chain built by `GraphicsReader.Chain` from the routes whose fields
  are set: Windows, D3DKMT and the GPU Engine counters then nvidia-smi;
  Linux, hwmon's GPU sensors, the AMD busy file, nvidia-smi. The PCI-ID name
  lookup is unchanged. The detail of a missing temperature is formatted from
  the chain's record of what it tried, never from a list kept elsewhere.
- R3 `core.Host`, declared by `core.NewHost` in `host_linux.go`,
  `host_windows.go` and `host_other.go`, carries: the processor chain and its
  account of absence, the graphics reader and its account, the load and name
  readers, the counters and Wi-Fi readers, the permission advice (spec 040's
  `PermissionAbsence`), and the Bluetooth drivers. The chains and accounts
  live in `host_tables.go`, with no build tag, so every platform's are tested
  on any system. Every `_other` file is honest absence (`!linux &&
  !windows`); Linux's former `_other` files are `_linux`.
- R4 `panel.Env.Host` carries the host (nil is `core.NewHost` over the
  settings, built once per `Sources`). The cooler, peripherals and bandwidth
  sources take their platform pieces from it. `panel` has no build tags and no
  `runtime.GOOS`, in code or tests; `platform_*.go` and the build-tagged
  `platform_*_test.go` and `sensors_*_test.go` are gone, their assertions
  moved to host-driven tests in `panel` and platform tests in `core`.
- R5 No change to what the panel reads or shows, except that the Linux
  account of a card with no temperature now names the busy file among the
  routes tried (it was asked and not named).
- R6 `docs/architecture.md` rules 4 and 5 describe this as in place, and its
  Status row says so. CLAUDE.md's nvidia-smi exception says where it sits.

## Acceptance Criteria

- [x] `core.Chain` tests: first answer stops the chain; nothing found tries
  every provider and keeps the first account; a merge fills what is missing
  and stops when complete; a cancelled context stops. Falsified: with the
  stop-when-complete rule removed, the merge test fails.
- [x] Each platform's chain order and wording is pinned on Windows by
  `host_tables_test.go`: Linux's processor sentence is the one spec 040
  pinned; Windows' card sentence is the one it was. Falsified: with the busy
  file and nvidia-smi swapped in `GraphicsReader.Chain`, the order test fails.
- [x] The cooler's reasons come from the host's chains: a fake host's
  processor providers and card route are named in the reasons. Falsified:
  with a hand-written GPU detail in the cooler, the test fails.
- [x] The host decides Bluetooth: a fake host with none asks none and draws no
  Bluetooth line; one with BlueZ asks it. A host's permission advice reaches
  a device that would not open.
- [x] `internal/panel/testdata/reasons.golden` is unchanged.
- [x] `hayami-tui doctor` and `readings`, built before and after and run back
  to back on this desk, are identical with digits masked.
- [x] `go test -tags migrated_fynedo ./...` on Windows: every package ok but
  `internal/usage`'s Python cross-check, which fails on `main` here too
  (`structlog` missing).
- [x] Window tests for the processors and LibreHardwareMonitor pass, after
  merging #139. They had failed on this desk, and on `main` too, because the
  panel opened under the resting pointer and a row's tip covered the card;
  #139 opens it in the corner farthest from the pointer.
- [x] `go vet` for linux and darwin (core, panel; cli, view and
  `cmd/hayami-tui` for linux) and windows; golangci-lint v2.12.2 0 issues on
  core, panel and cli for windows and linux; `hayami-tui` builds with
  `CGO_ENABLED=0` for windows and linux.

Touch points to add a new sensor source, after:

| Source | Places |
|---|---|
| Processor temperature | a provider, and a row in its platform's chain in `host_tables.go` — **2** |
| Graphics card | a provider (a `GraphicsReader` route), and its row in `GraphicsReader.Chain` or the platform's table — **2** |

The reason for a gap names the new source by itself.

## Risks & Assumptions

- **Linux is not run here.** Its chains are tested through `host_tables.go`
  on Windows (order, wording, an hwmon tree in a temp dir), `host_linux_test.go`
  runs in CI, and the code is vetted for linux. A Linux desk reading is the
  remaining check.
- **More hwmon reads on Linux.** The card's chain asks each hwmon GPU sensor
  until the reading is complete, so on an AMD card the later sensors are read
  after the first has given a temperature (the load is still missing then). A
  sysfs file read each, every five seconds.
- **The Linux card account gains a route.** "looked under /sys/class/hwmon and
  tried amdgpu/edge, amdgpu, nouveau, gpu_busy_percent, nvidia-smi": the busy
  file was asked before too, and now it is named.
- **`platform()`** in `panel` is a once-built host for the test seams
  (`newCooler`, `newPeripherals`); nothing reassigns it.
- **Rollback**: revert. No setting, cache or file format changes.

## Gaps found

- `internal/usage/path.go` chooses the cache directory by `runtime.GOOS`, and
  `internal/testenv/devices.go` the device scan. Neither is `panel`, `view` or
  a shell, so rule 5 holds; they could take their answer from the host later.
- `cli` imports `desktop`, which imports fynedesygn, so `cli` cannot be vetted
  for darwin with `CGO_ENABLED=0`; core and panel can.

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.26.0, MSYS2 UCRT64 gcc for the GUI,
LibreHardwareMonitor 0.9.6 running:

- `doctor` before and after, run back to back, digits masked: identical
  (`cooler partial Ryzen 5 2600X N % N °C, RTX 3060 Ti N % N °C`). `readings`
  likewise identical.
- Full suite: every package ok but `internal/usage` (above).
- Falsified three ways, each failing and passing again restored: the chain's
  stop-when-complete, the graphics chain's order, a hand-written GPU detail in
  the cooler.
