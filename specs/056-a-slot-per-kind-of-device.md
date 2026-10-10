# 056 — a slot per kind of device

**Issue**: #180

## Status: COMPLETE

## Context

The Peripherals card has two slots (specs 022, 050): the mouse on the left,
and on the right whichever other device changed state last. The Windows desk
now reads four devices: a Basilisk Ultimate (mouse), an AULA F75 (keyboard), a
Bose QC35 (headphones, sanshoku 0.1.11) and a DualSense (controller, sanshoku
0.1.12). Two of them are drawn, and which two depends on what was switched on
last, so the headphones' level is not where it was at the previous glance.

The other desk (Linux) reads a mouse and headphones. Its keyboard is wired and
has no battery; its controller is an 8BitDo on a 2.4 GHz dongle that carries
no battery anyone has found (sanshoku spec 017, out of scope). A card with a
slot for every kind would carry two permanent "no keyboard" and "no
controller" cells there.

## Requirements

- R1 sanshoku v0.1.12, and `view.KindGamepad` mapped from
  `battery.KindGamepad`.
- R2 **A slot per kind, up to four.** The kinds are ranked mouse, headset,
  keyboard, gamepad, other. Each kind that has a device on the card's list
  gets one slot, in rank order, and the first four kinds take the four slots.
- R3 **A kind keeps its slot while a device of it is remembered.** The list
  already includes devices that have gone quiet, dim, for a week (spec 032),
  so a controller switched off keeps its slot and the card keeps its height.
  The card grows only when a kind turns up that has no remembered device, and
  shrinks when the last device of a kind has not been heard for a week. No new
  state is kept.
- R4 **Two cells or four.** Fewer than two kinds are padded to two, as today:
  "no mouse" when nothing is there, "no device" after the first slot. Three
  kinds are padded to four with "no device", so the grid stays rectangular two
  across.
- R5 **One device per slot.** Among the devices of one kind, the slot shows a
  live one before a quiet one, and among those the one whose state changed
  last (spec 050's rule for mice, applied to every kind). The others, and
  every device of a kind with no slot, go to the hover note. This changes
  spec 022, which let a device that went quiet recently hold the shared slot
  over a live one: with a slot for the headphones, the pair that is connected
  is the one worth showing.
- R5a **A device of no known kind** (`KindOther`: a Bluetooth device with no
  icon, a receiver that could not say) is not one device seen twice, as two
  mice are. Beyond the first, which has the "other" slot like any kind, such
  devices take the cells the card would otherwise pad with a placeholder, and
  only those, so they never make the card bigger.
- R6 Never wider. Both shells already lay cells out as many across as the
  width fits, with a minimum width of one cell; four cells in the panel's
  width are two lines of two.

## Acceptance Criteria

- [x] View tests: four kinds give four slots in rank order; three kinds give
  four cells with a padding placeholder; one kind gives two; two devices of a
  kind put the live one in the slot and the other in the note; a fifth kind
  goes to the note; a quiet device keeps its kind's slot; devices of no known
  kind fill spare cells only (falsified: without the fill, three tests fail).
- [x] The gamepad kind crosses from sanshoku to the view.
- [x] On the real window on the Windows desk, with a mouse, keyboard,
  headphones and controller answering: the card's cells are two lines of two,
  read off a screenshot by the bars under the levels, and the window is as
  wide as with the cooler card alone (`TestAKindOfDeviceIsACellTwoAcross`).
  Falsified: with the slots capped at two it fails, one line of two.
- [x] The terminal pane draws the same four cells.
- [x] `go test ./...` passes, except `internal/usage`'s cross-check against the
  Python, which needs `structlog` on this machine; the window tests pass with
  `HAYAMI_WINDOW_TEST=1`.

## Risks & Assumptions

- The card grows the first time a third kind is heard on a machine, and every
  time one comes back after a week away. That is a reflow, and the glance
  rule allows it only for a section appearing or disappearing. It is accepted
  as the same order of event: rare, and caused by hardware the user just
  switched on.
- A device of a kind with no slot (a fifth kind) is only in the hover note,
  which the terminal does not draw.
- Long names are cut at the cell's width ("Basilisk Ultimate…", "DualSense
  Wireles…"), as they were in two cells; nothing new.
- Rollback: revert; no settings or cache format changes.

## Verification

2026-10-09, Windows 11, the Basilisk Ultimate, AULA F75, Bose QC35 and a
DualSense answering. The window drew "Basilisk Ultimate… 93 %, qc35 80 %" on
the first line and "F75 88 %, DualSense Wirel… 75 %" on the second, under a
cooler card of the same width; `hayami-tui --once --sections peripherals` drew
the same four cells, four across at its width.
