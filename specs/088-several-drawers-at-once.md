# 088 — several drawers at once

**Issue**: #320

## Status: INCOMPLETE

## Context

A click on a card opens its detail in the drawer, a click on another card
changes what the drawer shows, and a second click on the same card closes it
(spec 062). One detail is in view at a time. To compare the processor's
processes with the graphics card's, or the bandwidth by process with the
memory by process, the person switches back and forth and holds the other
in their head.

fynedesygn spec 079 gives a panel several drawers in a row beside the cards,
the click's Shift, and the room on the screen. This spec is what hayami does
with them.

`internal/gui/drawer.go` holds everything about the one drawer in one
`drawer` struct: what it shows, its poll, its pause and kill confirmation,
its drill, its filter, its switch, its VPN presses. Several drawers need
that state per drawer.

## Requirements

### Gestures

- R1 **A plain click is spec 062's** while one drawer is open or none: it
  opens the card's detail, changes what the drawer shows, or closes it.
- R2 **A plain click on a card with several drawers open closes them all.**
- R3 **A Shift-click on a card adds a drawer** for that card's next detail
  not already open, in the order its switch reads (the cooler: processor,
  graphics card, memory), starting with the one its card opens on (spec 062
  R7). With none open it opens the first drawer, as a plain click does.
- R4 **A Shift-click on a card whose details are all open closes that card's
  drawer opened last.** For a card with one detail this is a toggle.
- R5 **A Shift-click on a choice in a drawer's switch** opens that choice in
  a new drawer, or closes the drawer showing it when one does (that drawer
  itself included). A plain click on a choice changes what that drawer shows,
  as now.
- R6 A detail is open in at most one drawer. A plain click on a choice that
  another drawer shows changes this drawer to it and closes the other.
- R7 A click on a card with no detail does nothing, with Shift or without.

### The cap

- R8 **A drawer is added only where it fits on the screen** (fynedesygn 079
  `Panel.Fits`): on Windows the panel reads the room; on Plasma the room is
  asked of the compositor before each added drawer, off the UI thread as the
  first drawer's place is asked now (spec 060 R18); elsewhere the row is
  capped at `glance.DrawersWithoutRoom`.
- R9 **At the cap, the outermost drawer shows the new detail instead**: its
  poll stops, the new one starts, and nothing is resized.

### Each drawer

- R10 Each open drawer has its own detail, its own poll at that detail's
  interval, its own pause, kill confirmation and drill trail, its own filter
  and switch. A drawer's state never changes another's.
- R11 A detail is read only while a drawer shows it, as now: closing a
  drawer, closing all of them, or hiding a section stops its drawers' polls.
  Hiding a section closes each drawer showing one of its details and leaves
  the others open (spec 060 R4, per drawer).
- R12 What a card opens on next (spec 062 R7) is the detail its last plain
  or Shift opening showed.
- R13 The drawer side setting (spec 060 R16) applies to the row: every drawer
  is on that side.
- R14 **The panel's saved place is the cards'** with any number of drawers
  open on the left: the window's extra width is taken as the cards' offset
  when it is a whole number of drawer `Reach`es, as one is now (gui.go
  `drawerShift`).

### Documentation

- R15 README: the changelog line under `### Unreleased`; the drawer's
  paragraph in the user guide says Shift-click.
- R16 docs/architecture.md, if the drawer's place in it names one drawer.

## Acceptance Criteria

Headless, each falsified:

- [x] With one drawer open, a plain click on its card closes it and on
  another card changes it (existing tests pass unchanged;
  `TestAPlainClickWithSeveralOutClosesThemAll`).
- [x] A Shift-click on another card with the cooler's drawer open opens a
  second drawer on it; both details are polled
  (`TestAShiftClickOpensASecondDrawer`).
- [x] Shift-clicks on the cooler card open processor, then graphics card,
  then memory in three drawers; a fourth Shift-click closes the memory
  drawer (`TestThreeListsWhereThereIsRoom`). Where nothing reports the room
  the row stops at two and the outermost takes the next
  (`TestShiftClicksOnACardOpenItsDetailsInTurn`).
- [x] A Shift-click on a single-detail card whose drawer is open closes only
  that drawer; the other drawers' polls continue
  (`TestAShiftClickOnAnOpenCardClosesOnlyItsDrawer`).
- [x] A plain click on any card with two drawers open closes both and stops
  both polls (`TestAPlainClickWithSeveralOutClosesThemAll`).
- [x] A Shift-click on the switch's "GPU" in the processor drawer opens the
  graphics card's drawer; a Shift-click on "GPU" again closes it
  (`TestAShiftClickOnAChoiceOpensItBeside`).
- [x] A plain click on a choice open in another drawer changes this drawer
  to it and closes the other (`TestAChoiceOpenElsewhereMovesHere`).
- [x] At the cap (room reported too small for one more), a Shift-click
  replaces the outermost drawer's detail; the window's width is unchanged;
  the replaced detail's poll stops (`TestNoRoomReplacesTheOutermost`).
- [x] Pausing one drawer leaves the other's poll running
  (`TestPausingOneDrawerLeavesTheOther`).
- [x] Hiding a section closes only its drawers
  (`TestHidingASectionClosesOnlyItsDrawers`).
- [x] `drawerShift` returns the cards' offset for one and for two drawers on
  the left, and nothing for a width a person dragged
  (`TestThePlaceIsTheCardsWithTwoDrawersOnTheLeft`).

Each was falsified: ten mutations (no close-all, a card's details not walked
in turn, the cap ignored, a choice open elsewhere left open, a Shift-click on
a choice never closing, hiding closing every drawer, no cap without a room,
`drawerShift` taking one drawer only, a pause pausing every drawer, a card
whose details are all out not closing one) each failed a test.

On a real window (project rule: anything visual is tested on a real window),
the installed panel on Plasma 6.8, Wayland, at a scale of 1.5, clicked with
ydotool and Shift held by key events:

- [x] Drawers on the left (the panel against the right edge of its screen):
  one, two and three drawers, then the cap (a Shift-click on another card
  replaced the outermost, the width unchanged), a plain click closing all
  three, one opened and closed: the width is the cards' and whole drawers at
  every step, the right edge is held, and the cards' screen position,
  recovered from the window's geometry and the cards' place in each grab,
  never moved (x 3711.0 throughout). **Shift reaches the panel on Wayland.**
- [ ] Drawers on the right: not verified. Moving the panel with kdotool to
  give it room on the right left Fyne's idea of the pointer stale and the
  clicks did not register (quirk 48), and each opening moved the window by
  half a drawer: Plasma 6.8 keeps a window's centre when it grows, and only
  a left drawer has a script holding an edge. The glance-monitor example
  built from fynedesygn `main` does the same with one drawer, so this is not
  this spec's; it is a gap of fynedesygn spec 071 on the right side.

Found on the real window and fixed in fynedesygn 0.1.116 (spec 079, "Found
after release"): closing several drawers one at a time left the window as
wide as they had made it, with the cards stretched across it; and KWin could
put the old x back after the close, moving the cards. hayami closes them with
`Panel.CloseDrawers`.

## Risks & Assumptions

- Depends on a fynedesygn release carrying spec 079. Until it is tagged,
  development runs with a local `replace` that is not committed.
- Shift on Wayland reaches the panel only with the keyboard focus; the
  installed KWin rule does not refuse focus (the filter needs it). Confirmed
  on the desk.
- More drawers is more polls; each is the detail's own interval and stops
  with its drawer. A process list is the costliest at `ProcessInterval`;
  three at once is three reads per interval.
- **Rollback**: revert the PR; no setting or file format changes.

## Threat model delta

No boundary touched: the details read are the ones already read, by the same
readers; only how many are shown at once changes.

## E2E Test Plan

On the desk, installed with fynstall (Plasma, Wayland):

1. Shift-click the cooler card three times: three drawers, processor,
   graphics card, memory, outward from the cards (R3).
2. Shift-click the bandwidth card: at the cap on a narrow room the outermost
   shows bandwidth; with room a fourth opens (R8, R9).
3. Plain click any card: all close (R2).
4. Drawer side Left: repeat 1; the cards do not move (R13, R14).
5. Quit with drawers open on the left, start again: the panel is where the
   cards were (R14).

On the Windows VM: steps 1–3.

### Prod visibility

Not applicable: a desktop program with no telemetry.
