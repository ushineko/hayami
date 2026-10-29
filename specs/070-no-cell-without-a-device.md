# 070 — no cell without a device

**Issue**: #231

## Status: COMPLETE

## Executive Summary

The peripherals card draws a cell per device and no "no device" placeholder
for an empty slot; an empty desk is still one dim "no mouse" cell. Devices of
no known kind still fill the cells a card of its kinds has room for
(`roomFor`), so a mouse and an unnamed Bluetooth device are still two cells.
Reviewers should look first at `Peripherals` and `fillWithOthers` in
`internal/view/peripherals.go`.

## Context

Spec 022 (#89) made the peripherals card always draw at least two cells and
spec 056 rounded an odd count up to an even one, so that two across the grid
stays rectangular. A slot with no device behind it is a dim placeholder,
"no device" (or "no mouse" when there is nothing at all). The point was that
the card keeps its shape when a device comes or goes.

On a desk with a mouse, headphones and a controller the card draws a fourth
cell, "no device". In a window three cells wide it sits alone on a second
line. The padding costs a line and says nothing a reader wants.

The operator decided (2026-10-29) that a slot with no device is not drawn.
The card resizes when a device connects or disconnects, and that is
accepted, reversing spec 022's reason for the padding. With no devices at
all the card keeps one dim "no mouse" cell, so it is not an empty card.

## Requirements

- R1 `view.Peripherals` emits one cell per device shown and no padding. With
  no device at all it emits a single placeholder, `Label` "no mouse",
  `Value` `NoQuantity()`, `Stale` and `Placeholder` true, as before.
- R2 The "no device" placeholder, `MinPeripheralSlots` and the even-count
  rounding (`cellCount`) go. `MaxPeripheralSlots` (four) is unchanged: a
  slot per kind, the rest in the overflow note (spec 056).
- R3 The window draws only the cells the section emits; the slack beyond
  them stays hidden (`gui.drawCells`, unchanged in behaviour). The pane
  draws the same cells.
- R4 `docs/architecture.md`'s glance rule names the exception: a
  peripheral connecting or going may resize the card.
- R5 README changelog under `### Unreleased`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make build` pass.
- [x] `view` tests: three devices are three cells with no placeholder
      (`TestThreeKindsAreThreeCells`); one device is one cell
      (`TestAMouseAloneIsOneCell`); no devices is one "no mouse" placeholder
      (`TestNoDevicesIsOnePlaceholder`); four kinds are four cells, the fifth
      device in the note (existing spec 056 test, unchanged).
- [x] `gui` test: the card shows one cell for a mouse alone and two once a
      headset answers, drawn from cells built at the start
      (`TestTheCardKeepsAQuietHeadsetAndDropsAnAbsentOne`).
- [x] Each new test fails with the padding put back (falsified: 21 `view`
      and 5 `gui` tests fail with it restored).
- [x] Doctor still reports no placeholder
      (`TestDoctorDoesNotReportThePlaceholders`).
- [x] A screenshot of the live panel with three devices shows three cells
      and no "no device" (`tools/screenshot.sh --what panel`, 2026-10-29: a
      mouse, headphones and a controller on one line; not committed, it is
      the desk's own).

## Risks & Assumptions

- The card changes height when a device crosses a row boundary at the
  window's width (three to four in a window three across). Accepted by the
  operator; the glance rule records it.
- A cached reading written by an older version may carry a "no device"
  cell. It is drawn once at startup until the first poll replaces it.
- Devices of no known kind keep the room the old padding gave them
  (`roomFor`): they fill out a line the kinds started, and the card is never
  larger than before.
- Rollback: revert the commit. View and GUI only; no settings or cache
  format change.

## E2E Test Plan

The operator runs the installed panel with a mouse, headphones and a
controller connected and checks that the card shows three cells and no
"no device" (R1, R3). Then disconnects the controller and checks that the
card shows two cells.
