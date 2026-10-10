# 056 — a slot per kind of device

**Issue**: #180

## Status: IN PROGRESS

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
  every device of a kind with no slot, go to the hover note.
- R6 Never wider. Both shells already lay cells out as many across as the
  width fits, with a minimum width of one cell; four cells in the panel's
  width are two lines of two.

## Acceptance Criteria

- [ ] View tests: four kinds give four slots in rank order; three kinds give
  four cells with a padding placeholder; one kind gives two; two devices of a
  kind put the live one in the slot and the other in the note; a fifth kind
  goes to the note; a quiet device keeps its kind's slot.
- [ ] The gamepad kind crosses from sanshoku to the view.
- [ ] On the real window on the Windows desk, with a mouse, keyboard,
  headphones and controller answering: the card's cells are two lines of two,
  read off a screenshot, and the window is no wider than with two cells.
- [ ] The terminal pane draws the same four cells.
- [ ] `go test ./...` passes; the window tests pass with
  `HAYAMI_WINDOW_TEST=1`.

## Risks & Assumptions

- The card grows the first time a third kind is heard on a machine, and every
  time one comes back after a week away. That is a reflow, and the glance
  rule allows it only for a section appearing or disappearing. It is accepted
  as the same order of event: rare, and caused by hardware the user just
  switched on.
- A device of a kind with no slot (a fifth kind) is only in the hover note,
  which the terminal does not draw.
- Rollback: revert; no settings or cache format changes.
