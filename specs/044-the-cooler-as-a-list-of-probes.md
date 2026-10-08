# 044 — The cooler as a list of probes

**Issue**: #138

## Status: COMPLETE

## Context

Phase 2 of the architecture review after spec 037 (`docs/architecture.md`,
rule 4), after spec 043 put the cooler's sources on chains. What the chains
read still reached the view as a fixed struct, and the window still built
every card with hidden spare rows.

Inventory on `main` (fa2261e) before this spec:

- **`view.CoolerReading`** (`view/cooler.go:22-75`) was a field per part: CPU,
  Coolant, PumpRPM, FanRPM, CPULoad, GPU, GPULoad, each with a `Has*` flag
  (eight flags), three trails, three names and `GPUStale`. `view.Cooler`
  (`view/cooler.go:82-131`) wrote one `if` per part, in a fixed order, and one
  block per trail.
- **The window** (`gui/gui.go`) built each card with `RowSlack = 4` hidden spare
  rows (`gui.go:143-160`, `:236-254`), pinned the label column by row
  position (`card.pinned []bool`, `gui.go:113`), and when a section had more
  rows than the spares, `rebuild` appended a row (`gui.go:562-587`), which the
  library put under the card's plot.
- **The readings cache** (`internal/readings`) stored `view.Section` itself with
  no version: any change of the section's shape was a silent change of the
  file's.

Touch points to add another part the cooler reads -- a second graphics card --
before:

| Place | What |
|---|---|
| `view/cooler.go` CoolerReading | GPU2, HasGPU2, GPU2Load, HasGPU2Load, GPU2Name, GPU2Trail, GPU2Stale |
| `view/cooler.go` Cooler, rows | another `if` and row |
| `view/cooler.go` Cooler, trails | another trail block, its series number |
| `panel/cooler.go` Poll | fill the new fields |
| `panel/cooler.go` record | the keep-when-missing rule for them |
| `gui/gui.go` RowSlack | check the four spares still cover the card |

Six places, one in the window.

## Requirements

- R1 `view.Opt[T]` (value and whether it is there), with `Some` and `Maybe`.
- R2 `view.Probe{ID, Role, Name, Load, Temp, RPM Opt, Trail, Stale}` and
  `view.CoolerReading{Probes}` replace the fixed fields. Roles are `cpu`,
  `gpu`, `coolant`, `fan`, `pump`.
- R3 `view.Cooler` draws whatever probes there are from a role table
  (`coolerRoles`): rows in the roles' order (processor, card, coolant, then
  one Speeds row for every speed, the fan before the pump), the coolant
  coloured by `CoolantBands` and the processors not, fixed widths unchanged; a
  second probe of a role is labelled "GPU 2"; traces banded first, then a
  series colour each in row order. A probe of an unknown role is not drawn.
- R4 `view.Row.ID`, generic: a cooler row carries its probe's ID, the Speeds
  row `speeds`.
- R5 The panel's cooler source keeps its per-part bookkeeping (`record`'s
  rules are about those parts) in an unexported `coolerState`, and hands the
  view and `readings` its probes. A source reporting more probes adds them
  there and changes nothing in the view.
- R6 The window matches a poll's rows to a card's by ID: set in place,
  inserted at its position (above the plot, fynedesygn v0.1.85's
  `Card.InsertRow`), moved if out of place, removed when gone; the panel is
  re-measured after. A row without an ID is given one: `label:` and its label,
  or `text:` and its text where it has no label (a reason's sentence), never
  its value; a repeat gets `#2`; a detail line is its row's ID and
  `/detail:N`. `RowSlack` and the positional `pinned` are gone; a pinned label
  is the row's own (`glance.NewRowWidth`), recorded by ID.
- R7 The readings cache gets `FormatVersion` (1) and a `{version, sections}`
  file; a file of another version, or of none, is a cold start.
- R8 No visible change on this desk; the text arrangements unchanged.

## Acceptance Criteria

- [x] `doctor` on this desk, before and after, digits masked: identical.
- [x] `--once --sections cooler` in stack, grid and row, before and after,
  digits masked: identical apart from a load that changed from one digit to
  two between the runs (same line lengths).
- [x] The window tests pass: processors, LibreHardwareMonitor, Wi-Fi, and
  peripherals (the Basilisk awake). The LibreHardwareMonitor picture looked at
  after: "Ryzen 5 2600X 20 % 51.5 °C" over "RTX 3060 Ti 1 % 43.0 °C", the
  coolant's reason, the plot under the rows -- as before.
- [x] A reading with two graphics cards draws two GPU rows, "GPU" and "GPU 2",
  sharing the columns, with the next series colour
  (`TestTwoGraphicsCardsAreTwoGPURows`).
- [x] Rows follow the roles' order whatever order the probes arrive in, and
  every row has an ID.
- [x] In the window (headless Fyne, `internal/gui/rows_test.go`): a card
  gaining a probe between polls inserts its row in place above the plot,
  without moving the rows above it; losing it removes the row and puts the
  rest back; a reason arriving later is above the plot; a value or a name
  changing moves no row.
- [x] Falsified: with `InsertRow` replaced by the old append, the arriving row
  and the reason are under the plot and both tests fail; with a gone row left
  in place, "the card's row is gone" fails; with the cache's version check
  removed, a later-version file loads and its test fails.
- [x] The readings cache writes its version and reads it back; a file from
  before versions and one from a later version are cold starts.

Touch points to add another part the cooler reads, after:

| Part | Places |
|---|---|
| Another probe of a known role (a second card) | 1: a probe from its source (in `panel/cooler.go`'s probes, or a provider's) |
| A new role (a motherboard sensor) | 2: the probe, and a row in `view.coolerRoles` |

No change in the window for either.

## The `readings` JSON

The cooler's `data` changes shape. Before:

```json
{"CPU": 49.4, "HasCPU": true, "Coolant": 0, "HasLiquid": false, "PumpRPM": 0,
 "HasPump": false, "FanRPM": 0, "HasFan": false, "CPULoad": 8.9,
 "HasCPULoad": true, "GPU": 43.4, "HasGPU": true, "GPULoad": 0,
 "HasGPULoad": true, "Trail": null, "CPUTrail": [49.4], "GPUTrail": [43.4],
 "CPUName": "AMD Ryzen 5 2600X Six-Core Processor",
 "GPUName": "NVIDIA GeForce RTX 3060 Ti", "GPUStale": false}
```

After:

```json
{"probes": [
  {"id": "cpu", "role": "cpu", "name": "AMD Ryzen 5 2600X Six-Core Processor",
   "load": {"v": 34.6, "ok": true}, "temp": {"v": 45, "ok": true},
   "rpm": {"v": 0, "ok": false}, "trail": [45]},
  {"id": "gpu", "role": "gpu", "name": "NVIDIA GeForce RTX 3060 Ti",
   "load": {"v": 1.5, "ok": true}, "temp": {"v": 43.4, "ok": true},
   "rpm": {"v": 0, "ok": false}, "trail": [43.4]}]}
```

Consumers: none in this repository or its scripts parse the cooler's data
(searched for the old field names and for `readings` in Go, PowerShell, shell,
Python and Markdown); the section's rows and reasons, which `doctor` reads, are
unchanged.

## Design notes

- **A probe has no bands of its own.** The issue sketched `Bands` on the probe;
  the verdict is a fact about the kind of reading -- a coolant at 56 is hot
  whoever read it -- so it lives with the role, in `coolerRoles`, and a source
  cannot colour a processor by accident.
- **The view draws a card with a load and no temperature.** Before, the GPU row
  needed a temperature; a processor-kind probe now draws on either value, as the
  CPU's did since spec 034. Nothing visible changes: the panel only emits a GPU
  probe when it has the card's temperature.
- **The panel keeps its bookkeeping by part.** `record`'s rules -- the coolant
  kept and the section Gone, the card kept and its row dim, a name kept once
  heard -- are about those parts; moving them onto probes generically would be
  a second refactor with no reading behind it. `coolerState.probes()` is the
  one place the parts become a list.
- **IDs for rows without one** are named by what does not change while the row
  is the same: a rate in the ID would make every poll a new row.

## Risks & Assumptions

- **The readings cache is discarded once** on the first start after upgrading:
  one blank first frame until the first poll, as spec 033 accepted for a move
  of the same file.
- **A label-only ID changes when the label does.** A row without a builder's ID
  whose label changes is removed and inserted, one structural change; the
  cooler's named rows have IDs, so a name arriving is not one.
- **The `readings` JSON** changes for the cooler; nothing here reads it.
- **Rollback**: revert. The fynedesygn requirement may stay at v0.1.85; nothing
  older than this spec uses the new card API.

## Gaps found

- `CellGrid` keeps its spare cells (`CellSlack`) for the peripherals; the
  library has no insert for cells. A later spec.
- The Wi-Fi `LinkReading` still carries `Has*` flags; moving it onto `Opt` is
  mechanical and left out to keep this change to the cooler (the guide's
  Status table lists it).
- Linux is not run on hardware: the change is the view and the window, both
  exercised here; CI runs the suite on Linux.

## Verification

2026-10-07, Windows 11, Go 1.26.0, fynedesygn v0.1.85, LibreHardwareMonitor
running:

- Full suite: every package ok but `internal/usage`'s Python cross-check, which
  fails on `main` on this desk too (its Python lacks `structlog`).
- Window tests (`HAYAMI_WINDOW_TEST=1`): processors, LibreHardwareMonitor,
  Wi-Fi and peripherals pass.
- `GOOS=linux` vet of view, panel, core, readings and `cmd/hayami-tui` clean;
  `hayami-tui` builds for Linux and Windows with `CGO_ENABLED=0`.
- golangci-lint v2.12.2: 0 issues on view, panel, readings, gui, cli and tests
  for Windows (cgo), and on view, panel, readings and core for Linux.
