# 090 — the drawers fill the room ("maximize")

**Issue**: #327

## Status: INCOMPLETE

## Executive Summary

(Filled in before the PR.)

## Context

Spec 088 opens drawers in a row until the screen has no room for another,
then puts the next detail in the outermost. The row stops short of the
screen's edge by up to a drawer. fynedesygn spec 080 (#240) gives a full row
a way to fill that strip, every drawer the same width; this spec is hayami's
use of it.

## Requirements

- R1 **The outermost drawer shows a Fill button when the row is at its cap**:
  when one more drawer would not fit (`Panel.Fits`/`FitsHere`, the room asked
  of the compositor on Plasma as spec 088 R8 does, off the UI thread), or the
  row is at `glance.DrawersWithoutRoom` where nothing reports the room. The
  cap is asked after each drawer opens, closes or changes. Where nothing
  reports the room the button is not shown: there is no room to fill to.
- R2 **A press fills the row** (`Panel.Fill`/`FillHere`); a second press
  unfills it. Each open drawer is the same width; the cards are unchanged.
- R3 Replacing the outermost drawer's detail at the cap (spec 088 R9) keeps
  the row filled. Closing a drawer, or a plain click closing all, ends it.
- R4 A filled drawer's content is laid out at its width: lists that truncate
  to the drawer's width use the wider width.
- R5 **The saved place is the cards'** with a filled row on the left:
  `drawerShift` takes the window's extra width as the cards' offset when it
  is `Panel.DrawersWidth()` to within two pixels, as well as a whole number
  of `Reach`es.
- R6 The terminal panel has no drawers; nothing changes there.
- R7 README changelog line under `### Unreleased`; the drawers paragraph in
  the user guide says how to fill.

## Acceptance Criteria

Headless, each falsified:

- [x] At the cap (room reported too small for another), the outermost
  drawer's Fill button is shown; below it, it is not
  (`TestTheFillButtonAtTheCap`). Where nothing reports the room it is never
  shown (`TestNoFillWithoutTheRoom`).
- [x] A press fills, a second unfills; the cards' size is unchanged
  (`TestFillAndUnfill`).
- [x] A Shift-click at the cap while filled replaces the outermost and the
  row stays filled; a plain click on its card ends it
  (`TestReplacingKeepsItFilledAndClosingEndsIt`).
- [x] A plain click closing several ends filling and Fill is no longer
  offered (`TestClosingSeveralEndsFilling`).
- [x] `drawerShift` returns the cards' offset for a filled row on the left
  (`TestThePlaceIsTheCardsWithAFilledRow`).

Thirteen mutations of the hayami side (Fill offered below the cap or without
the room, no offer after an opening or a close of all, a toggle that never
unfills, a filled place read by Reaches, and the tuck mutations of spec 091)
each failed a test.

On a real window (Plasma 6.8 Wayland at 1.5; Windows VM at 150 %), the
installed panel:

- [x] Plasma: drawers on the right and on the left, opened to the cap,
  filled: the window's outer edge is the work area's, the drawers' widths
  recovered from the grab are equal, the cards did not move. Unfilled: back
  to the drawers' own widths, the cards did not move (results below).
- [ ] Windows VM: the same.

## Risks & Assumptions

- Depends on a fynedesygn release carrying spec 080.
- One more room query after each change of the row on Plasma: a KWin script
  round trip of a millisecond or two, off the UI thread.
- **Rollback**: revert the PR; no setting or file format changes.

## Threat model delta

No boundary touched: the room is asked as spec 088 asks it, and no new
script is loaded by hayami.

## E2E Test Plan

On the desk, installed with fynstall (Plasma, Wayland):

1. Shift-click cards until the row is at its cap; the outermost shows Fill (R1).
2. Press Fill: the row reaches the screen's edge, drawers equal (R2).
3. Shift-click another card: the outermost changes, still filled (R3).
4. Plain click: all close (R3). Repeat 1–2 with the drawers on the left;
   quit filled, start again: the panel is where the cards were (R5).

On the Windows VM: steps 1–3.

Results, 2026-11-14, Plasma 6.8 on Wayland at 1.5, the branch build run in
place of the installed panel (fynedesygn from its branch through a local
`replace`), clicked with ydotool; widths are device pixels recovered from
spectacle grabs, a surface being a run of columns not transparent down the
grab:

- Left (the panel against the right edge of DP-2, x 3711): Shift-clicks on
  Hardware opened CPU, GPU and Memory; with 164 of room left, the outermost
  offered Fill (two drawers, with 493 left, did not). Filled: the window
  went from x 2724.33 and 1276 wide to x 2560.33 and 1440, the screen's
  width; the drawers 563, 563, 563 with 2 of padding at the far end; the
  cards 434 and against the right edge, as before. Unfilled: x 2724.33 and
  1276 again, drawers 481/482/482. Filled, a Shift-click on Network put
  Bandwidth in the outermost and the row stayed filled at the same widths.
  A plain click closed all: x 3711, 289.33 wide (steps 1-4).
- Right (the panel at x 300 on DP-3): five drawers to the cap, Fill offered
  on the fifth; filled, the window ran from 300 to 2560, the screen's edge,
  drawers 580 x 5, cards 435 and at x 300 throughout; unfilled, 1934 wide
  again.
- **Found on the desk and fixed**: the first build planned the cards at 435
  pixels and not 434 (289.333 in float32 times 1.5 is 434.00001, rounded
  up), and the cards' left edge moved a pixel when the row filled.
  fynedesygn `planFill` now forgives what float32 adds
  (`TestAWholePixelIsNotRoundedUp`), and the cards are stood on the planned
  pixels as the drawers are.
- Step 5 (quit filled, start again): not run; `TestThePlaceIsTheCardsWithAFilledRow`
  covers the place saved.
- Windows VM: not run.

### Prod visibility

Not applicable: a desktop program with no telemetry.
