# 048 — The vendor list from sanshoku

**Issue**: #141

## Status: COMPLETE

## Context

docs/architecture.md rule 1 says device knowledge lives in sanshoku, and that
a device added there should need no hayami change beyond `go get`. The
peripherals source did not meet it. On `main` at 96f789b:

| # | Where | What it knew about sanshoku's drivers |
|---|---|---|
| 1 | `internal/panel/peripherals.go` `vendors()` | a table: each driver's display name, its absence line ("no Logitech receiver", "no Razer device", "no SteelSeries device", "no AULA receiver", "no Bluetooth device with a battery"), and a `quiet` flag (Razer, SteelSeries and AULA) |
| 2 | the same file's imports | `aula`, `logitech`, `razer`, `steelseries` for the table, `hidraw` and `logitech` for a receiver's presence |
| 3 | `pollVendor` | `dev.(logitech.Presencer)` and `hidraw.PairedChild(c.Phys)`, summed by its own `addPresence`, which dropped a paired child node's quiet count |
| 4 | `Poll` | `v.name == "Logitech"` before asking for a receiver's reason, and the text "speaks HID++ 1.0" |
| 5 | `core.Host.Bluetooth` (`internal/core/host.go`, filled in `host_linux.go`) | which drivers read Bluetooth batteries on which platform |
| 6 | `core.PermissionAbsence` on Windows and other systems | `fs.ErrPermission` asked beside `sanshoku.IsPermission`, which did not recognise Windows' `ERROR_ACCESS_DENIED` (spec 035) |

sanshoku v0.1.9 (its spec 014, #41) moves all of it into the drivers:
`sanshoku.Description` (`Name`, `Finds`, `Capabilities`, `Platforms`,
`Quiet`), through `sanshoku.Describe`; `sanshoku.Presence` and `Presencer` in
the root package, with a paired child node reporting no quiet slots of its own
and `OldProtocol` naming what a too-old device speaks; and `IsPermission`
recognising `ERROR_ACCESS_DENIED` (its #39).

## Requirements

- R1 `vendors(platform)` is built from `all.Drivers()`: the drivers that
  describe themselves, offer "battery" and read on the host's platform,
  grouped by `Name`, in the module's order. The absence line is
  `"no " + Name + " " + Finds`; `quiet` is the description's. A driver that
  does not describe itself is not asked (every driver in the module does, and
  a sanshoku test holds it).
- R2 `core.Host.Bluetooth` goes. `Host.Platform` ("linux", "windows",
  "other") is what the descriptions are asked about; the Bluetooth drivers
  are asked on Linux because they say they read there.
- R3 A receiver's presence is `sanshoku.Presencer`, summed with
  `Presence.Add`. `addPresence`, the `v.name == "Logitech"` branch and the
  `logitech`/`hidraw` imports go: any vendor whose devices report presence
  gets the receiver lines, worded from its name and what it finds, and the
  too-old line says `OldProtocol`.
- R4 `PermissionAbsence` asks `sanshoku.IsPermission` alone.
- R5 No change to what the panel, `doctor` or the reasons say, apart from the
  order noted under Risks.
- R6 docs/architecture.md: rule 1, the "Support for a device" row and the
  Status row say it is in place.

## Acceptance Criteria

- [x] `internal/panel/peripherals.go` imports no sanshoku driver package;
  no non-test file in hayami does except `internal/testenv` (`hidraw`, to
  empty the Linux device tree for tests).
- [x] The vendor lists are pinned per platform
  (`TestTheVendorListComesFromTheDrivers`): Windows is Logitech, Razer,
  SteelSeries, AULA; Linux adds Bluetooth (BlueZ and Apple's accessory
  protocol); a system no driver reads on asks none.
- [x] `internal/panel/testdata/reasons.golden` is unchanged
  (`TestEveryReasonIsPinned`).
- [x] `doctor`'s peripherals line before and after is identical with digits
  masked.
- [x] The window test for the peripherals passes with the Basilisk dongle and
  the F75 awake.
- [x] Falsified: ignoring the descriptions' `Quiet` fails four tests (the
  golden, the AULA and Razer "answered nothing" tests, the vendor pin);
  dropping the platform filter fails three (Bluetooth asked on Windows, the
  vendors named for an empty desk, the vendor pin). Restored, all pass.

## Touch points to add a device vendor

| | hayami | sanshoku |
|---|---|---|
| Before | 2: a `vendors()` row and its import; a third if the vendor had receivers (the name branch) or a platform restriction (`core.Host`) | the driver, its support entries |
| After | 0: `go get` the release | the driver, its support entries and its description |

## Risks & Assumptions

- **The vendors are in sanshoku's order.** `all.Drivers()` lists bluez and
  apple before aula, so on Linux the Bluetooth line now comes before AULA's
  when nothing is found, where hayami's table put AULA before it; within
  Bluetooth, BlueZ is asked before Apple's accessory protocol. Windows has no
  Bluetooth vendor and is unchanged. The lines say the same; only their order
  on an empty Linux card moves. Putting AULA before bluez in `all.Drivers()`
  would restore it, and is sanshoku's to do.
- **A Logitech child node's quiet count** is now 0 from sanshoku, where hayami
  used to discard it after reading the node's physical path. The sum is the
  same: `TestAChildNodesQuietIsNotCountedTwice` feeds what the driver reports.
- **Permission on Windows** rests on sanshoku v0.1.9; an older sanshoku would
  read a refused device as one that did not answer.
- **Rollback**: revert. sanshoku v0.1.9 keeps `logitech.Presence` and
  `Presencer` as aliases, so the old code builds against it.

## Gaps found

- The Linux order of the Bluetooth and AULA lines, above.
- `internal/testenv` still imports `hidraw` to take the Linux device tree
  away from tests; a scan that tests replace (spec 038's `Env.Scan`) covers
  Windows, and the Linux tree is the udev test's subject.

## Verification

2026-10-07, Windows 11, against sanshoku v0.1.9:

- `go test -tags migrated_fynedo ./...`: every package ok but
  `internal/usage`'s Python cross-check, which fails on `main` on this
  machine (its Python lacks `structlog`).
- `GOOS=linux CGO_ENABLED=0 go vet` on core, panel and cli; `GOOS=darwin` vet
  on core; `CGO_ENABLED=0 go build ./cmd/hayami-tui` for Windows and Linux;
  golangci-lint v2.12.2 for Windows and Linux: clean.
- `doctor`, before and after: `peripherals ok Basilisk Ultimate Dongle N%,
  FN N%` (digits masked), identical.
- Window test (`HAYAMI_WINDOW_TEST=1`): answering Basilisk Ultimate Dongle
  and F75; two lines in a battery colour; passed.
