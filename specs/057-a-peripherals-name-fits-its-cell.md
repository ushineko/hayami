# 057 — a peripheral's name fits its cell

**Issue**: #183

## Status: COMPLETE

## Context

With four cells on the peripherals card (spec 056), two across the panel, two
of the names on the Windows desk are cut with an ellipsis: "Basilisk
Ultimate…" (the device is "Basilisk Ultimate Dongle") and "DualSense Wirel…"
("DualSense Wireless Controller"). The words that are cut, and the ones that
push the name over the width, say nothing a reader needs: every device on the
card is wireless or it would have no battery, and a controller's cell does not
need to say it is a controller.

The cooler card already shortens its part names by a table of rules (spec
031, `view.ShortName`), each tested against a real name.

## Requirements

- R1 `view.PeripheralLabel`: a peripheral's name without the trailing words
  "Controller", "Wireless", "Dongle" and "Receiver", taken from the end one at
  a time. A name that would be left empty, or as one of those words alone, is
  kept whole.
- R2 A cell's `Label` is the short name. Its `Name` carries the full one where
  the two differ, and is empty where they do not.
- R3 The full name is said where there is room: the card's hover note has a
  line for each cell whose label was shortened, the note that lists the
  devices with no slot is unchanged, and doctor names a device in full.
- R4 Drawing only. A device is known across polls and restarts by its full
  name; nothing that identifies a device reads the label.

## Acceptance Criteria

- [x] A test per rule against a real name: "DualSense Wireless Controller" is
  "DualSense", "DualSense Edge Wireless Controller" is "DualSense Edge",
  "Arctis Nova Pro Wireless" is "Arctis Nova Pro", "Basilisk Ultimate Dongle"
  is "Basilisk Ultimate"; a word that is not at the end stays ("Wireless Mouse
  M185"); "Wireless Controller" alone is kept.
- [x] The cell carries the full name only where it was shortened, and the
  hover says it; doctor's line names the device in full.
- [x] On the real window, with the controller answering, its name is drawn
  whole: its ink is well short of its cell's width
  (`TestTheControllersNameIsDrawnWhole`). Falsified: with the rules removed
  the name runs to 88 px of the 126 px cell and the test fails.
- [x] `go test ./...` passes, except `internal/usage`'s cross-check against the
  Python, which needs `structlog` on this machine.

## Risks & Assumptions

- Two links to one mouse ("Basilisk Ultimate", "Basilisk Ultimate Dongle")
  read the same in a cell. Only one is ever in the mouse's slot (spec 050), and
  the other is named in full in the hover note.
- The readings cache stores cells; `Name` is an additive, optional field, so
  the cache's format version is unchanged.
- A name the rules do not shorten enough is still cut with an ellipsis, as
  before. A per-device alias was considered and left out: it is a setting for
  a problem four rules solve on the desks this runs on.
- Rollback: revert.

## Verification

2026-10-10, Windows 11, a Basilisk Ultimate through its dongle, a QC35 and a
DualSense answering. The window, one cell across with only this card in it,
drew "Basilisk Ultimate", "qc35" and "DualSense", each whole; the controller's
name was 57 px of a 124 px cell. With the rules removed it was "DualSense
Wir…", 88 px.
