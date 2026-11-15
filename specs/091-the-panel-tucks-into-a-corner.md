# 091 — the panel tucks into a corner ("minimize")

**Issue**: #328

## Status: INCOMPLETE

## Executive Summary

A button over the cards tucks the panel into a square with hayami's icon at
the cards' bottom corner on the screen-edge side; a click on the square
brings it back where it was, or where the square was moved to, drawers and
all. While tucked nothing is read and the square's place is not saved. On
Plasma the corner is held by fynedesygn's corner script, loaded in place of
the edge's. Tried on Plasma; the Windows check is still to run. A reviewer
should look first at `internal/gui/tuck.go`.

## Context

The panel stays on top of other windows. When it covers work for a while, the
only ways out of the way are to drag it somewhere and back or to quit it.
fynedesygn spec 081 (#241) lets a glance panel tuck into a small square with
the program's icon at its corner; this spec is hayami's use of it.

## Requirements

- R1 **A small glyph shows in the cards' top corner on the screen-edge side
  while the pointer is over the cards** (`Panel.SetTuckable`); a click on it
  tucks the panel. On Plasma the room is asked of the compositor first, off
  the UI thread, as for a drawer; on Windows the panel reads it.
- R2 **Tucked, the panel is a square with hayami's icon** at the cards'
  bottom corner on the screen-edge side (spec 081 R3). A click on it brings
  the panel back, the cards exactly where they were.
- R3 **While tucked, every poll stops**, the cards' and the drawers': a
  tucked panel costs what a hidden section costs. Untucked, every shown
  section and open drawer is read at once, then at its interval.
- R4 **Open drawers come back** with the panel, each showing what it showed,
  with its filter, pause and drill as they were (spec 081 R4).
- R4a **The square can be moved** as the panel is (Meta-drag on Plasma,
  press-drag on Windows), and the panel unfolds from where it was put,
  inside the work area (spec 081 R5a–R5c).
- R5 Tucking is not saved: a restart opens the panel, never the square. The
  saved place stays the cards'; a geometry report while tucked is not saved
  as the panel's place, and the cards' place after unfolding from a moved
  square is saved as any move is.
- R6 On Plasma, the corner is held by the design system's KWin script loaded
  through `internal/desktop` as the hold-edge scripts are; where no corner can
  be held (another compositor) the glyph is not shown.
- R7 The right-click menu is unchanged (Preferences, opacity, quit).
- R8 The terminal panel has nothing to tuck; nothing changes there.
- R9 README changelog line under `### Unreleased`; the user guide says how to
  tuck and bring the panel back.

## Acceptance Criteria

Headless, each falsified:

- [x] A click on the glyph tucks the panel into the corner away from the
  drawers; every section's and drawer's poll stops
  (`TestTuckingStopsEveryPoll`).
- [x] A click on the square untucks; every shown section is woken to read at
  once, and the drawers are back, the same details, read again
  (`TestUntuckingReadsAtOnceAndTheDrawersComeBack`).
- [x] A geometry report while tucked does not change the saved place
  (`TestATuckedPlaceIsNotSaved`).
- [x] Without a held corner the glyph is not offered
  (`TestNoGlyphWithoutAHeldCorner`).

Each was falsified with the mutations listed under spec 090 (sections still
shown while tucked, drawer polls not stopped or not restarted, no wake, the
square's place saved, the corner reversed, the glyph offered with nothing to
hold the corner).

On a real window, the installed panel, at the right edge with drawers on the
left and on the main screen with drawers on the right:

- [x] Plasma: tucked, the square's bottom and side edges are the cards' to
  the pixel; untucked, the cards' rectangle is the one before, and the
  drawers are back (results below).
- [x] Plasma: the square moved by Meta-drag unfolds the panel from its new
  place, and into the screen's top corner unfolds it inside the work area.
- [ ] Windows VM: the same, with a press-drag on the square.

## Risks & Assumptions

- Depends on a fynedesygn release carrying spec 081.
- **Rollback**: revert the PR; no setting or file format changes.

## Threat model delta

B5: one more KWin script, the design system's `HoldCornerScript`, loaded by
`internal/desktop` the way the hold-edge scripts are (a `0600` file, quoted
strings, only the panel's window). The threat model's B5 section names the
hold scripts; it gains the corner script's name in the same PR.

## E2E Test Plan

On the desk, installed with fynstall (Plasma, Wayland):

1. Hover the cards; click the glyph: the square at the cards' bottom corner
   on the screen-edge side (R1, R2).
2. Watch a reading interval: nothing is read (R3; `hayami` log at debug).
3. Click the square: the panel back where it was, readings current (R2, R3).
4. With two drawers open, repeat 1 and 3: the drawers come back (R4).
5. Tuck, quit, start: the panel opens untucked where the cards were (R5).
6. Tuck, Meta-drag the square to another place, click it: the panel unfolds
   there, inside the screen; quit and start: it opens there (R4a, R5).

On the Windows VM: steps 1, 3, 4.

Results, 2026-11-14, Plasma 6.8 on Wayland at 1.5, the branch build in place
of the installed panel, clicked with ydotool, places from KWin's geometry:

- At the right edge of DP-2 with no drawer (x 3711, y 1973, 289.33 x
  587.33): the glyph showed over the cards' top-right on hover; a click
  tucked the panel into a 30 x 30 square (45 pixels) bearing the icon at
  (3970.33, 2530.33), its corner the cards' (4000.33, 2560.33); a click on
  it unfolded the panel to (3711, 1973). Twice in a row, the same (step 1,
  3).
- With two drawers out on the left (x 3053, 947.33 wide): the square went
  to the cards' corner, away from the drawers; unfolded, the window, the
  drawers' widths and their details (GPU outer, CPU inner) were as before
  (step 4).
- Tucked, Meta-dragged to (3699.66, 1501.92), clicked: the panel unfolded
  at (3440.33, 944.58), its bottom-right corner the square's. Tucked there,
  Meta-dragged into the screen's top-right corner, clicked: the panel
  unfolded at (3710.67, 0), inside the work area, not 557 above it (step 6).
- On the main screen at x 300 with five drawers on the right, filled: the
  square went to the cards' bottom-left corner (300, 2087.33); unfolded, the
  filled row of five came back filled, at x 300.
- **Found on the desk and fixed**, in fynedesygn's corner script: (1) the
  panel unfolded from the corner KWin had put the square at, which a scale
  of 1.5 can move by a third of a unit; it now unfolds from the corner it
  shrank from unless moved by hand; (2) the pull back into the work area
  took in a panel that stood a third of a unit past the screen's edge, as a
  panel placed against the edge does at 1.5, so it tucked and unfolded a
  third of a unit off; it now acts only past half a unit.
- A square dropped under a window kept above (the terminal, by its own KWin
  rule) cannot be clicked until that window moves: the two are both above,
  and the one used last is on top. Not this spec's to change.
- Step 2 (nothing read while tucked) and step 5 (restart while tucked):
  covered by the headless tests; a restart during the run, with the panel
  tucked, opened it untucked at its saved place.
- Windows VM: not run.

### Prod visibility

Not applicable: a desktop program with no telemetry.
