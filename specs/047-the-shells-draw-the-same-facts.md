# 047 — The shells draw the same facts

**Issue**: #144

## Status: COMPLETE (the window's `row` waits on fynedesygn#170)

## Context

CLAUDE.md and `docs/architecture.md` rule 3: every section renders in all
three arrangements (stack, grid, row), in both shells, held by a parity test
with a documented allow-list. The architecture review after spec 037 found
neither held:

- `TestFeatureParity` (`tests/structure/parity_test.go`) built every source
  from `panel.Keys()` and checked the keys it got were `panel.Keys()`: true by
  construction. Its allow-list was empty and could never be needed. Nothing
  compared what the window drew with what the terminal drew.
- The window maps `row` to `stack` (`internal/gui/gui.go`, `arrangement`),
  while the preferences offer all three. Nothing caught it.

What `row` needs from the window: `glance.Arrangement` (fynedesygn v0.1.85)
has `Stack` and `Grid` and nothing else. A row arrangement is one full-width
line per reading with a stretching bar, no headings and no plot, laid out
across a panel's cards. CLAUDE.md: "A shape the library lacks goes into the
spec's 'Gaps found' for a library change, not into `internal/gui`."

## Requirements

- R1 A parity test that draws every registry section in the window
  (headless, Fyne's test driver, the real `gui.Panel`) and renders it in the
  terminal in stack, grid and row, from one fixture per section built by that
  section's own view builder, and checks every fact the section holds on
  both: its title, every line's label, value and detail lines, every cell's
  name, value and note, every meter's label, caption, reset and stats. A
  section in the registry without a fixture fails the test.
- R2 Differences go in an allow-list keyed by shell or arrangement and kind
  of fact, each with its reason. A second test fails on an entry that no
  longer excuses anything.
- R3 The window draws `row` or the gap is recorded: a fynedesygn issue, the
  allow-list or guide, and the preferences honest about it.
- R4 `TestFeatureParity` keeps what it checked that was not a tautology (each
  registry entry builds its source under its key, with the registry's title
  and icon) under a name that says so, and drops the rest.
- R5 `docs/architecture.md` and the README say what is true.

## Decision: the window and `row`

Option (b), the gap recorded. Drawing `row` in the window would mean a panel
layout of hayami's own, built from glance's card pieces: exactly what the
design-system rule forbids, and a second copy of a layout the library should
own. Filed as fynedesygn#170 (a panel arrangement for one line per reading).

The preferences keep offering `row`, marked "terminal only" (they already
were), rather than hiding it: the setting is one file both shells read, and
hiding the option in the window's preferences would take away the only
graphical way to set the terminal pane's arrangement. This differs from the
issue's suggestion to hide it, and is why.

`row` is not in the window's allow-list: the window draws the same *facts* in
stack and grid, and the arrangement is a layout the window does not have. The
guide's Status table carries it as "Waiting on fynedesygn#170".

## The allow-list

| Entry | Reason |
|---|---|
| `row: title` | The row arrangement draws no section headings. |
| `row: detail` | One line per reading; detail lines (an interface's totals, its Wi-Fi link) are the stacked shapes'. |
| `row: meter caption`, `row: meter reset`, `row: meter stats` | A usage meter in a row is the usage widget's line (spec 019): the same figures, spelled `10%` and `resets Oct 7` where the stacked shapes say `5h: 10 %` and `in 2h 0m`. |

The window has no entries. A cell's name or note the window elides to the
cell's width ("Basilisk Ultimat…") counts as drawn: that is the glance cell's
own rule for a fixed block, and the name's start is on the card. The test
accepts the elided form for cell names and notes only, from four letters up.

## Acceptance Criteria

- [x] `TestTheWindowAndTheTerminalDrawTheSameFacts` passes for bandwidth
  (a Wi-Fi row with parts and two detail lines, a wired row, a reason),
  cooler (CPU, GPU, coolant, pump and fan probes), peripherals (two cells, one
  name long enough to be elided) and usage (two accounts, three windows).
- [x] `TestEveryParityExceptionIsUsed` passes: every entry excuses a fact.
- [x] Falsified: with the window's detail lines dropped (`flatten`), the test
  fails on the bandwidth detail lines; with `row: detail` removed from the
  allow-list it fails on the row arrangement's missing detail lines; with the
  elision rule strict it fails on the elided cell name. Each restored.
- [x] fynedesygn#170 filed; the guide's rule 3 describes the parity test, its
  Status rows say "In place #144" for the test and "Waiting on fynedesygn#170"
  for the window's `row`; the README's arrangements section names the issue.
- [x] `tests/structure`: `TestEveryRegistryEntryBuildsItsSource` keeps the
  per-entry checks; the keys-equal-keys assertion and the empty allow-list
  are gone.

## Risks & Assumptions

- **Test only, plus comments and docs.** No program behaviour changes.
- **Containment, not layout.** The test judges what is written (spaces
  removed, since the shells pad and a value's parts can be separate text
  objects in the window), not where. Layout is the window tests' job
  (`tests/window`) and the arrangements' own tests'.
- **Width.** The terminal is rendered at 200 columns, so nothing is cut to
  fit; truncation at narrow widths is the arrangements' rule and tested there.
- **Rollback**: revert.

## Gaps found

- fynedesygn#170: a row arrangement for a glance panel.
- The window's grid and stack draw the same cards, so the test draws the
  window once; if an arrangement ever changes what a card holds, the test
  should draw each.

## Verification

2026-10-07, Windows 11, Go 1.26.0, MSYS2 UCRT64 gcc:

- `go test -tags migrated_fynedo ./...`: every package ok except
  `internal/usage`'s Python cross-check, which fails on `main` on this desk
  too (its Python lacks `structlog`).
- The parity tests pass; the three falsifications above failed as written
  and passed once restored.
- golangci-lint v2.12.2: 0 issues for `internal/gui` and `tests/structure`
  on Windows (cgo), and for the non-Fyne packages on Linux.
- Window tests (`HAYAMI_WINDOW_TEST=1`): processors, LibreHardwareMonitor and
  peripherals pass (the Basilisk was awake). The Wi-Fi test failed once in the
  full run and passed three times running after it; this spec changes no
  drawing, so it is the live rates under that test, noted for whoever next
  touches it.
