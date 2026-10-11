# 058 — a device charging with no level keeps its cell

**Issue**: #184

## Status: COMPLETE

## Context

An AULA F75 on its 2.4 GHz receiver, plugged in to charge. The receiver
reports the keyboard as on its cable and sanshoku reads that as charging with
no level, because the level it gives there is not one (sanshoku spec 013).
hayami then forgets the level it had, rightly: a level from before the cable
is wrong, not stale. But a device with no level had no cell, so plugging the
keyboard in took it off the card, and with a slot per kind (spec 056) took
the keyboard's slot with it and changed the card's height.

Measured 2026-10-10 on Windows, through the receiver:

- Cable in: `13 4a 01 00 02 64 10 …`, level 100, state 0x10. `hayami-tui
  readings` listed no F75.
- Cable out, at once: `13 4a 01 00 02 64 01 …`, level 100, state 0x01.
- Some minutes later, on battery: 92 %. It was 88 % before it was plugged in.

So the state byte follows the cable (0x10 is "on its cable", not "full"), and
the 100 on the cable was not the battery's level: it is pinned, as the
published captures say, and lingers a moment after the cable comes out.

## Requirements

- R1 A device that says it is charging (or charged) and gives no level and no
  band is a cell: `PeripheralReading.NoLevel`. It is live, not stale; it keeps
  its kind, so its slot; and `Since` does not move, so charging is not taken
  for the device arriving.
- R2 The level from before the cable is still forgotten. The cell has no
  number: the dash a placeholder draws where the number goes, no unit, no bar
  and no verdict, with the state ("Charging", "Charged") on the quiet line.
- R3 A device first seen already charging with no level is a cell too.
- R4 A device with no level that is not charging is unchanged: no cell if it
  was never heard, and its last level, dim, if it was (the Arctis whose
  headset is switched off).
- R5 In the note for devices with no slot it has a dash for a level, and
  doctor's line says what it is doing: "F75 charging".

## Acceptance Criteria

- [x] Panel tests: a keyboard with a level, then charging with none, keeps one
  live cell with no level over two polls, with its kind and its `Since`; its
  level is drawn again when it gives one; first seen charging is a cell.
  Falsified: with the new case disabled, three tests fail.
- [x] View tests: the cell's dash, empty unit, state, no bar, not a
  placeholder; the note's dash.
- [x] Doctor's line.
- [x] On the real window, with the F75 charging on its cable through its
  receiver: the card holds a cell with its three lines of text and no bar
  (`TestADeviceChargingWithNoLevelKeepsItsCell`). Falsified: with the new
  case disabled the card is the mouse, the headphones, the controller and a
  placeholder, and the test fails.
- [x] `go test ./...` passes, except `internal/usage`'s cross-check against the
  Python, which needs `structlog` on this machine.

## Out of scope

- The keyboard switched to USB (258a:010c, another protocol). A battery query
  for it is described in other projects and unverified on an F75; sending it
  is a separate decision.
- Showing a real level while charging: the receiver does not give one.

## Risks & Assumptions

- A headset on a charging cradle that reports charging with no level becomes
  a cell with no number, where before it had no cell or its last level, dim
  and marked offline. That is the same case and the same answer.
- `NoLevel` is an additive, optional field of the remembered devices' file
  and of nothing else on disk.
- Rollback: revert.

## Verification

2026-10-10, Windows 11, the F75 on its cable and switched to 2.4 GHz, with a
Basilisk Ultimate, a QC35 and a DualSense answering. The window, one cell
across with only this card in it, drew "Basilisk Ultimat… 48 %", "qc35 80 %",
"F75 -- Charging" with no bar, and "DualSense Wir… 95 %": bands
`TTTTBTTTBTTTTTTB`. With the new case disabled: `TTTTBTTTBTTTBTT`, no F75.
