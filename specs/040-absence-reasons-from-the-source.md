# 040 — Absence reasons from the source

**Issue**: #128

## Status: COMPLETE

## Context

The architecture review after spec 037 (phase 1) found `view.Reason` built in
about twenty places, in three styles:

1. **The panel writes the words.** Most sites: the panel knows the fact (an
   interface not listed, a vendor with nothing on the desk) and says it.
2. **Core returns a sentinel and the panel writes platform text.**
   `core.ErrWirelessDenied`, matched in `panel.go`, which then wrote the
   Windows location-consent sentence itself.
3. **Core supplies the words.** `core.SensorAbsence` from the
   LibreHardwareMonitor reader (spec 036), whose detail the panel copied.

Two things followed that do not scale with more devices:

- **A decision made on the words.** Whether a reason stays drawn on a full
  peripherals card was `present[i].Detail != permissionDetail`: change the
  advice's wording and the line silently went aside.
- **The panel retold core's story.** `cpuSensorDetail` and `gpuSensorDetail`
  in `panel/sensors_{other,windows}.go` listed the routes core's readers try
  ("asked D3DKMT and the GPU Engine counters, and tried nvidia-smi"); a change
  to the chain in core would leave the sentence stale.

### Inventory (before)

| Site | Reason | Style | After |
|---|---|---|---|
| `panel/cooler.go` Poll | CPU "no sensor", detail from `sensorDetail(err)` | 3 + panel fallback | `reason(base, err)`; detail `core.CPUSensorDetail()` or the source's `Absence` |
| `panel/cooler.go` Poll | "no GPU sensor", `gpuSensorDetail()` | 1 (retelling core) | literal, detail `core.GPUSensorDetail()` |
| `panel/cooler.go` liquid | "the cooler would not answer", `err.Error()` | 1 | `reason(base, err)` |
| `panel/cooler.go` liquid | Coolant "no cooler" | 1 | unchanged (panel's own fact) |
| `panel/devices.go` openFailure | "… is not permitted", `permissionDetail` | 1 + platform text | `core.PermissionAbsence` → `reason`; `Actionable` |
| `panel/devices.go` openFailure | "unsupported" | 1 | unchanged (sanshoku's sentinel, panel's words) |
| `panel/panel.go` Section | "not present" | 1 | unchanged |
| `panel/panel.go` Section | "no interfaces chosen" | 1 | unchanged |
| `panel/panel.go` Section | Wi-Fi "details withheld" | 2 | `ErrWirelessDenied` is an `Absence` carrying both; `reason(base, err)` |
| `panel/peripherals.go` Poll | "speaks HID++ 1.0" | 1 | unchanged |
| `panel/peripherals.go` Poll | full-card aside rule | **text comparison** | `!Actionable` |
| `panel/peripherals.go` pollVendor | "… device would not answer", `err.Error()` | 1 | `reason(base, err)` |
| `panel/peripherals.go` pollVendor | "no Bluetooth adapter", `err.Error()` | 1 | `reason(base, err)` |
| `panel/peripherals.go` pollVendor | vendor absent; "answered nothing" | 1 | unchanged |
| `panel/peripherals.go` receiverReason | two receiver lines | 1 | unchanged (Logitech-specific; review finding 3) |
| `panel/usage.go` Poll | "the usage cache could not be read" | 1 | `reason(base, err)` |
| `panel/usage.go` gather | "no Claude or Codex account" | 1 | unchanged |
| `panel/usage.go` gather | "unreadable reading" | 1 | `reason(base, err)` |
| `panel/usage.go` silence | "could not be read" | 1 | `reason(base, err)` |
| `panel/usage.go` silence | "waiting to retry", "nothing fetched yet" | 1 | unchanged |
| `core/lhm.go` CPUTemperature | six LibreHardwareMonitor absences | 3 | `Absence` with a code each |

"Unchanged" sites are reasons the panel composes from facts it alone holds
(the vendor's name, an interface the user named, a cache gate's time). No
error is mapped there, so there is nothing for the helper to do.

## Requirements

- R1 `core.Absence{Code, Text, Detail, Actionable, Err}` is an error: `Error()`
  is the detail, `Unwrap` the cause, and `Is` matches another absence with the
  same non-empty code. `core.AbsenceOf(err)` finds one in a chain.
  `core.SensorAbsence` remains as an alias for spec 036's callers.
- R2 Codes are typed constants, one per known cause: CPU sensor, the five
  LibreHardwareMonitor states and its unexpected answer, Wi-Fi withheld,
  permission.
- R3 `panel.reason(base view.Reason, err error) view.Reason` is the only
  mapping from an error to a reason: an `Absence` supplies its verdict, detail
  and `Actionable`; any other error gives its message as the detail unless the
  base has one.
- R4 `view.Reason.Actionable`, set only from an absence. The peripherals card
  keeps a reason drawn on a full card by that flag, not by its words.
- R5 The wording moves next to the code that knows the cause:
  `core.CPUSensorDetail`, `core.HostCPUTemperature` (formerly panel's
  `cpuPackage`/`cpuTemperature`), `core.GPUSensorDetail`,
  `core.PermissionDetail` and `core.PermissionAbsence` (formerly panel's
  `permissionDetail`/`permitted`), and `ErrWirelessDenied`'s verdict and
  detail. `panel/sensors_{other,windows}.go` are removed; panel's platform
  files keep only the Bluetooth vendor.
- R6 No reason changes its label, text, detail, status, aside or whether it
  is kept on a full card.

## Acceptance Criteria

- [x] A pin, `panel/testdata/reasons.golden`, written from the code before
  the change, covers every reachable reason: the cooler (no sensor, a load and
  no temperature, two LibreHardwareMonitor accounts, a cooler not permitted,
  unsupported, not answering, a failed scan), bandwidth (nothing chosen, a
  name not listed, Wi-Fi withheld), peripherals (an empty desk, a refused
  device alone and on a full card, unsupported, a vendor that would not list,
  a device that would not read, a dock answering nothing, both receiver
  lines, HID++ 1.0) and usage (a failed gather). Platform sentences are
  placeholders, so one file holds on both platforms.
- [x] The pin is unchanged after the refactor (commit 59281d7 is the pin on
  the old code; the refactor commit leaves the file untouched).
- [x] Falsified: with `reason` dropping `Actionable`, the pin and
  `TestADeviceThatMayNotBeOpenedSaysWhatToDo` fail; restored, they pass.
- [x] `hayami-tui doctor`, built before and after, prints identical output
  once numbers are masked.
- [x] core tests: an absence matches by code through wrapping and copies; no
  code matches nothing; `Error()` is the detail and the cause is reachable; a
  refused open is an actionable permission absence and a timeout is not;
  Wi-Fi withheld carries its verdict and the settings link.
- [x] Suite, Linux vet of the non-Fyne packages, both terminal builds and
  golangci-lint (Windows and Linux) pass.

## Risks & Assumptions

- **`ErrWirelessDenied.Error()` changes** from the WLAN sentence to the
  reason's detail. Nothing printed it; callers match it with `errors.Is`,
  which still holds (and now also for a copy with the same code).
- **The Bluetooth lines are not in the pin**: that vendor is asked off Windows
  only (spec 035) and the pin was written on Windows. `platform_other_test.go`
  holds them on Linux, and they pass through the same helper.
- **Rollback**: revert the refactor commit; the pin commit can stay.

## Gaps found

- Reasons the panel composes from its own facts are still literals. That is
  right for most; the Logitech receiver lines are vendor-specific and belong
  to the sanshoku `Presencer` work (review finding 3).
- `core.Host` (review finding 4) would gather `HostCPUTemperature`,
  `PermissionAbsence` and the detail functions into one platform table; this
  spec only moves them into core.

## Verification

2026-10-07, Windows 11, Go 1.26.0:

- Pin generated on 59281d7 (old code); unchanged on 143fde6.
- `go test -tags migrated_fynedo ./...`: every package ok except
  `internal/usage`'s Python cross-check, which fails on `main` on this machine
  (its Python lacks `structlog`).
- `GOOS=linux CGO_ENABLED=0 go vet` on core, panel, cli, view, usage and
  `cmd/hayami-tui`: clean. `CGO_ENABLED=0` builds of `hayami-tui` for windows
  and linux.
- golangci-lint v2.12.2 on core, panel, view: 0 issues for windows and linux.
- `doctor` before and after, numbers masked: identical.
