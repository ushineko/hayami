# 050 — one mouse on the card

**Issue**: #162 (and #161)

## Status: COMPLETE

## Context

The Basilisk Ultimate is read through its dongle (1532:0088) and, charging on
its cable, as itself (1532:0086). On this desk the peripherals card showed it
twice: "Basilisk Ultimate Dongle 23 %, Offline", the mouse remembered from the
dongle (spec 032 keeps a quiet device for a week, dim), and "Basilisk
Ultimate 53 %, Charging", live on its cable. The window and the terminal
seemed to disagree about it (#162). They don't: both draw through
`view.SelectPeripherals`. The window test ran with a fresh cache and had no
remembered dongle, so it was a different input. Two things made the card
wrong:

1. sanshoku read 1532:0086 as `KindOther`, not a mouse, so the live mouse
   was a generic cell. Fixed in sanshoku v0.1.10 (its spec 015).
2. The mouse slot took the **first** mouse found, which could be the
   remembered one, and a second mouse was free to take the right slot as the
   device that changed last.

The peripherals window test (#161) decided a device was drawn by a level in a
battery colour. A charging device is drawn with a white level and a blue bar,
so with the mouse on its cable the test failed on a correct card.

Identifying the mouse across its two links (a serial behind an opaque ID) was
considered and left out: a display rule is enough for the card, and a desk
has one mouse.

## Requirements

- R1 sanshoku v0.1.10.
- R2 `SelectPeripherals`: the mouse slot shows a live mouse if there is one,
  else the one heard most recently; every other mouse goes to the overflow,
  named in the note, and never into the right slot. The right slot keeps its
  rule (the device whose state changed last) among the other devices.
- R3 The parity fixture (spec 047) holds one mouse remembered from its dongle
  and live on its cable: both shells draw the live one and the keyboard, and
  name the dongle in the note.
- R4 The peripherals window test recognises a drawn device by its bar, which
  every device cell has whatever its state.

## Acceptance Criteria

- [x] With the mouse on its cable and its dongle reading remembered,
  `hayami-tui --once --sections peripherals` draws one Basilisk, live:
  "Basilisk Ultimate 87 %, Charging", beside the F75. 0.9.1 drew the
  remembered dongle reading in the mouse slot.
- [x] The window draws the same: "Basilisk Ultimate 86 %, Charging" and the
  F75 (screenshot looked at).
- [x] The view tests (a live mouse takes the slot from a remembered one; the
  mouse heard last when none answers; two mice found at once show one) fail
  with the old first-mouse rule.
- [x] The parity test fails with the old rule ("the live mouse is not the one
  drawn").
- [x] The peripherals window test passes with the mouse charging (2 bars), and
  fails with the Razer and AULA vendors given no drivers (0 bars).

## Risks & Assumptions

- **A desk with two real mice** now shows one, and names the other in the note.
  The old AC15 test (two mice in name order) is replaced: a second mouse was
  more often one mouse seen twice than two mice.
- The reasons golden is unchanged.
- Rollback: revert; sanshoku v0.1.10 is additive.

## Verification

2026-10-08, Windows 11, the Basilisk Ultimate charging on its cable, its
dongle plugged in and its last dongle reading remembered:

- installed 0.9.1: "Basilisk Ultimate Dongle 23 % Offline" | "F75 100 %".
- this branch: "Basilisk Ultimate 87 % Charging" | "F75 100 %".
- window test: bars 2, pass; with no Razer or AULA driver: bars 0, fail.
