# 093 — the drawers fit their lists

**Issue**: #335

## Status: COMPLETE

## Executive Summary

The CPU, GPU and Memory drawers open as wide as their lists need, room
allowing, and show their columns without filling; every other drawer is the
width it was. The width is worked out exactly from the row's parts and the
list's kind, never from a reading, and set through fynedesygn 082's
`SetWidthInEms`. On this desk three process drawers take 1379 of the screen
where three without columns took 1276. Two faults the trial found are fixed
with it: a window put back to the width of drawers of different widths was
taken for a drag, and sizes from 1000 to 1023.9 of a unit printed a digit
too wide. A reviewer should look first at `wantEms` and `sizeDrawer` in
`internal/gui/columns.go`, then `drawerShift` in `internal/gui/gui.go` and
`scale` in `internal/view/format.go`.

## Context

Spec 092 (#332) gives the CPU, GPU and Memory lists columns after each
program's figure: the processes behind a name, the other figure of the same
reading, the share of all memory. A drawer is `glance.DrawerWidthInEms`
(32) text sizes wide, which holds a name and a figure and leaves room for
the count alone, so spec 092 shows the columns only in a filled row
(R5, R7).

The columns are wanted in an ordinary drawer, the drawer as wide as its
list needs, worked out by hayami without a setting (#335). A drawer's width
may differ from another's and be chosen when it is drawn; nothing that
arrives afterwards may resize it (the glance rule).

fynedesygn spec 082 (ushineko/fynedesygn#244) lets a program set a drawer's
width (`Drawer.SetWidthInEms`), lays drawers of different widths side by
side, says how wide a drawer the room still takes (`Panel.Room`), and fills
a row of them.

## Requirements

- R1 **A list says how wide it wants its drawer**, from what it can show
  and never from a reading, worked out as a row is laid out: the drawer's
  margins, the pinned name column, the row's paddings, and the list's own
  figure with every column its kind can carry (spec 092 R2), gaps included,
  in the monospace face. `view` says the characters (`ColumnsWant`), the
  window the width. A detail with no columns wants the usual width.
  *Found in the trial*: the first build wanted the usual width and the
  columns. The usual 32 ems are sized for the widest figure any drawer
  shows, the bandwidth lists' two rates, and a process list's figure is a
  third of that: each drawer carried about twelve ems of nothing, and the
  desk's cap fell from three drawers to two. Worked out exactly, three
  drawers with every column took 1379 of the screen where three without
  took 1276.
- R2 **A drawer's want is the widest of what its switch can show** (CPU and
  Mem, and GPU where there is a card), so choosing another list on the
  switch does not resize it; a drill page in the drawer wants its list's
  width.
- R3 **A drawer opens at its want, or as wide as the room takes**, whichever
  is less; a list wanting more than the usual width is cut to the room but
  not below the usual width, and one wanting less is as wide as it wants.
  hayami asks the room (`Panel.RoomHere`, or the compositor on Plasma, as
  it asks `Fits` now) and sets the width before opening. Where the room is
  not known (spec 079 R8) it opens at the usual width.
- R4 **At the cap** (a drawer of the usual width does not fit), the next
  detail goes to the outermost drawer as now (spec 088 R9). The outermost
  takes the new detail's want if the room it stands in allows it (its own
  width and the room beyond), as wide as that room otherwise; its width is
  set in the same change of shape.
- R5 **Nothing but opening, the switch, a drill and the cap's replacement
  sets a width.** A reading, the filter, a page of a list, the drawer's
  rows changing: none does.
- R6 **The columns show whenever they fit**, filled or not: spec 092 R5's
  fit by `Drawer.Available()`, without its gate on `Panel.Filled()`. *Amends
  spec 092 R5 and R7.* A drawer whose list has no columns (every other
  section's) is the usual width and unchanged. **A drawer too short for a
  line of the list under the heading line shows no column**: the heading
  would take the list's last row. *Found in the tests*: a panel in the row
  arrangement is one card tall and its drawer fits two lines, which the
  switch and the heading took.
- R11 **No size or rate is wider than `NumberWidth`**: the formatter steps
  to the next unit when the number would be, as glance's does, as well as
  at 1024. *Found on the Windows VM*: from 1000 to 1023.9 of a unit the
  number was "1008.0", six characters; a process at 1008 KiB widened the
  Memory list's figure past its want, and the list dropped its last column
  at that reading. It reads "1.0 MiB" now, in the cards as in the drawers.
- R10 **A window the compositor puts back to a width the drawers had is
  known for the drawers'**, not taken for a drag (spec 079's place, kept
  where the cards are). Drawers of different widths are not a number of one
  drawer's reach, which is how it was known; the row's recent widths are
  remembered and matched. *Found in the tests*: two drawers on the left, a
  wide and a usual one, had their corner saved as the panel's place.
- R7 **Fill** (spec 090) widens drawers of different widths by the same
  pixels (fynedesygn spec 082 R8); Fill is offered when a drawer of the
  usual width no longer fits, as now.
- R8 A change of text size keeps each drawer's ems (fynedesygn 082 R4); the
  want is worked out again from the new face only when the drawer next opens
  or is given a new subject.
- R9 README changelog line under `### Unreleased`; the drawers paragraph of
  the user guide says the process lists open wide enough for their columns;
  spec 092's paragraph on Fill and columns is corrected.

## Acceptance Criteria

Headless. Six mutations each failed a test: the drawer never asking its
want (`TestAListOpensWideEnoughForItsColumns`), the outermost not taking the
room beyond it (`TestTheCapsReplacementTakesItsWant`), the place matched
without the row's widths (`TestThePlaceIsTheCardsWithTwoDrawersOnTheLeft`),
the heading kept over an empty list (`TestARowArrangementOpensDetailsToo`),
Memory's figure at a percent's width (`TestAListWantsItsWidestLine`), and
the columns still gated on filling (the first five GUI tests below). One
mutant is equivalent: the want taken from the shown list alone, not the
widest of its switch, changes nothing while all three lists want 34
characters; `TestTheSwitchDoesNotResizeTheDrawer` holds the rule for the day
one does not.

- [x] Each list's want is its figure and its kind's columns, a line's widest,
  the same for any reading, and a detail without columns wants none
  (`TestAListWantsItsWidestLine`).
- [x] The drawer's want is the widest of the switch's lists: opened on
  Memory it is the width it was on CPU, and choosing GPU, CPU and Memory
  changes neither it nor the window (`TestTheSwitchDoesNotResizeTheDrawer`).
- [x] With room for the want, the drawer opens wider than the usual width
  and shows every column unfilled under its heading
  (`TestAListOpensWideEnoughForItsColumns`); with less, as wide as the room
  and never under the usual width, the columns that fit, more room never
  fewer (`TestARoomShowsOnlyWhatFits`).
- [x] Room not known: the usual width (`TestNoRoomKnownIsTheUsualWidth`).
- [x] A reading that changes every value resizes nothing and moves no column
  (`TestAReadingMovesNoColumn`).
- [x] At the cap, a process list given to the outermost drawer, which had a
  list without columns, takes its want when the room beyond allows it
  (`TestTheCapsReplacementTakesItsWant`).
- [x] A drawer without columns is the usual width, no heading
  (`TestADrawerWithoutColumnsIsTheUsualWidth`).
- [x] A drawer too short for a line under the heading shows no column and
  its list's line (`TestARowArrangementOpensDetailsToo`); the heading's row
  comes out of the list's height (`TestTheListIsAsLongAsTheHeightFits`).
- [x] Every size and rate near every step below TiB is `NumberWidth`, and
  1008 KiB reads "1.0 MiB" (`TestASizeNearAStepIsNoWider`; falsified with
  the old step at 1024 alone).
- [x] Two drawers of different widths on the left: the window at their width
  and at the first's is known for the drawers' and the cards' place is
  kept; half a drawer more is a drag
  (`TestThePlaceIsTheCardsWithTwoDrawersOnTheLeft`,
  `TestThePanelIsRememberedWhereItsCardsAre`).

On a real window (Plasma, Wayland, at 1.5; the Windows VM at 150 %), the
installed panel, geometry recovered from grabs:

- [x] Plasma: the CPU drawer opens wider than a Network drawer beside it,
  unfilled, every column shown, values aligned; the cards did not move.
- [x] Plasma: open to the cap, Fill and Unfill: every drawer grew by the same
  pixels and went back to its own width.
- [x] Plasma: a minute of readings: the window's width does not change.
- [x] Windows VM: the same.

## Risks & Assumptions

- A wider drawer leaves room for fewer. On this desk three process drawers
  still fit at their exact width (1379 of 1440); a Network drawer beside two
  wide ones may bring the cap a drawer sooner elsewhere. Fill still offers
  the rest of the room.
- The name column stays `view.DetailNameWidth` (20) characters: on Linux a
  process's name is the kernel's, at most 15, so some of it is unused there;
  Windows names are longer. Narrowing it for Linux is not this spec.
- Needs a fynedesygn release carrying spec 082; built against its branch
  through a local `replace` until then.
- The monospace face's advance is the same for every digit and sign the
  columns use, so a width worked out from character counts holds for any
  reading. Spec 092's formatters already depend on this.
- **Rollback**: revert the PR; no setting or file format changes.

## Threat model delta

No boundary touched: layout only; the columns are spec 092's.

## Test strategy

- `view`: each kind's want, table-driven, and that no reading changes it.
- `gui`: headless, a compositor stub with given rooms; the width a drawer
  opens at, the columns shown unfilled, the cap's replacement, and that a
  reading, the filter and the switch leave the width alone.
- The real-window checks above for what is drawn.

## E2E Test Plan

On the desk, installed with fynstall (Plasma, Wayland, at 1.5):

1. Click the CPU row: the drawer opens wider than 32 ems with every column
   (R3, R6).
2. Shift-click a Network row: a usual-width drawer beside it (R6).
3. Switch CPU to Mem: no resize (R2, R5).
4. Shift-click to the cap, Fill, Unfill (R4, R7).
5. Watch a minute of readings: the window's width is constant (R5).

On the Windows VM, 150 %: steps 1–4.

Results, 2026-11-15. Grabs in the session's scratchpad; widths are device
pixels from the grabs, a surface being a run of columns not transparent.

Plasma 6.8 on Wayland at 1.5, the branch installed with fynstall (built as
0.16.0, fynedesygn through a local `replace`), clicked with ydotool, the
panel against the right edge of DP-2 at x 3711:

- A first trial wanted the usual width and the columns: three process
  drawers did not fit, the cap came at two (1271 wide), and each list stood
  a dozen ems from its names. Worked out exactly (R1), three fitted again.
- Step 1: the CPU drawer opened with every column, unfilled, the window 653
  wide where a usual drawer made it 618.
- Step 2: Bandwidth by process opened beside it at the usual width: the
  surfaces 481 and 533 pixels, one gap apart, the cards at x 3711.
- Step 4: a third drawer (GPU) at 534 pixels reached the cap, 1345 wide with
  95 of room left, and the outermost offered Fill. Filled: 1440 wide, the
  three grown by 47 pixels each (580, 530, 581), the cards at 3711 and 434
  wide. Unfilled: 1345 again.
- Step 5: twelve readings five seconds apart, filled and then unfilled: one
  geometry each time.
- Step 3 was made on the first trial build: CPU to GPU in the drawer, no
  resize. On this build the switch is the headless test's.

Windows 11 VM at 150 % (3840x2160), the branch cross-built with mingw and
run in place of the installed 0.15.0, input through QEMU's tablet:

- Steps 1 and 2: CPU added 557 pixels to the window, Bandwidth by process
  beside it 493, the usual drawer's.
- Step 4: with the panel at x 1100, CPU, Memory, Bandwidth by process and by
  peer reached the cap (2471 wide) and Fill was offered; filled, 2740 wide,
  the right edge on the screen's at 3840, the cards at x 1100; unfilled,
  2471 again.
- Found here (R11): the first build's Memory drawer showed procs and CPU but
  not "of mem", where the CPU drawer of the same width showed all three. A
  process at 1008 KiB had a figure of ten characters, one over the want.
  With the formatter fixed, Memory showed all three columns, Vivaldi at
  "1.0 GiB".
- Step 5: the CPU drawer open, twelve readings three seconds apart: one
  geometry (927 wide at x 2913) each time. The installed 0.15.0 was put
  back afterwards, at its place.

### Prod visibility

Not applicable: a desktop program with no telemetry.
