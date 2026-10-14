# 062 — the drawer opens from anywhere on a card

**Issue**: #197

## Status: INCOMPLETE

Built, tested and used at the Linux desk (Plasma 6, Wayland). The one thing
outstanding is the Windows real-window run: its tests are rewritten and
compile, and were not run, because this was built at a desk that is not a
Windows one. The issue stays open for that.

## Executive Summary

A card is one target: a click anywhere on it opens its drawer and a second
closes it. The cooler's three lists are a switch in the drawer, the usage
drawer shows every account and scrolls when it must, and the panel is
remembered where its cards are whatever the drawer is doing. Reviewers
should look first at `gui.moved` and `desktop.settler`, which replace a flag
that was wrong on the desk, and at the Windows tests, which were rewritten
and not run.

## Context

Spec 060 opened the drawer on what was clicked: a cooler row chose the
processor's list, the card's or the memory's, and a usage meter chose its
account. Used at the desk, that asks for aim on a panel that is glanced at.
A click on the cooler's heading means the processor and nothing says so, and
changing the list means going back to the card for another row.

The choice moves into the drawer, and a card becomes one target. The usage
card has no choice to make: its drawer shows every account.

On Plasma, closing the drawer also flashes the panel sideways. That fault is
the design system's (fynedesygn #206) and is fixed there; hayami carries a
workaround for an earlier form of it that this spec removes.

This needs a fynedesygn release with its #206 fix and its spec 069 (a switch
in a drawer, and a drawer that scrolls). Nothing here copies from the
library.

## Requirements

### The click

- R1 A primary click anywhere on a card whose section has details opens the
  drawer on that section. The row, meter, cell, heading or plot under the
  pointer makes no difference. A second click anywhere on the same card
  closes it; a click on another card with details changes what is shown and
  leaves it open (spec 060 R3, amended). A card without details does nothing,
  as now.
- R2 `view.Row.Opens` and `view.Meter.Opens` are removed: nothing reads them.
  `view.Section.Opens` stays and names the detail the section opens on.
- R3 In the row arrangement a click on any line of a section is a click on
  its card (spec 060 R20, amended).

### The cooler

- R4 The cooler's drawer has a switch under its heading with one choice for
  each list the source offers now (`Details()`): CPU, GPU, Mem, in that
  order, with the keys `c`, `g` and `m`. A list the machine does not have
  (no graphics card found) has no choice, and a machine with one list has no
  switch.
- R5 What a choice is called and its key are data: `view.DetailInfo` gains
  `Label` and `Key`. Neither shell decides them, and `hayami-tui detail`
  lists them.
- R6 Changing the choice stops the poll of the list that was shown and starts
  the other's (spec 060 R4). The heading changes with it; the window's
  rectangle does not. The filter is cleared, as it is when the drawer moves
  to another card.
- R7 The drawer opens on the choice last shown while the panel has been
  running, and on CPU the first time. When that list is no longer offered it
  opens on the first that is. Nothing is kept across a restart (spec 060
  R19).
- R8 The lists are as spec 060 left them: cut to the height, greatest first,
  filtered with `f`. They do not scroll.

### The usage

- R9 The usage section has one detail for the drawer, every account in the
  card's order: for each, its name as a heading, its limits as meters, then
  its facts and shares (spec 060 R13, R15), with a rule between one account
  and the next. It is `view.Detail.Parts`, a detail made of details; an
  account's own detail is what it was.
- R10 An account whose breakdown cannot be read keeps its heading and says
  why under it. The other accounts are unaffected.
- R11 When the whole breakdown is taller than the cards it scrolls inside the
  drawer, by the wheel and the keys the design system gives it. The window's
  height does not change and the breakdown that fits is drawn as now.
- R12 A reading that arrives while the breakdown is scrolled leaves it where
  it is.
- R13 `hayami-tui detail usage` prints every account. `hayami-tui detail
  "usage/<account>"` prints one, as before.

### The bandwidth

- R14 No change. Its list is by process over every interface and it has no
  choice to offer.

### The close

- R15 The two delayed `Panel.Resize` calls after a close (120 ms and 500 ms)
  are removed: the library asks for its own width (fynedesygn #206). The
  real window on Plasma needed neither.

### Where the panel is remembered

Found while this was used at the desk: `settings.yaml` held the corner of the
window with its drawer out, and the panel came back a drawer's width to the
left (Gaps found).

- R16 The place saved is where the cards are, whatever the drawer is doing.
  The compositor reports the window's width with its corner; where the
  window is wider than the cards by the drawer's reach and the drawer is on
  the left, the cards begin that much further along. A window wider than
  its cards by anything else is one a person is dragging wider, and its
  corner is theirs.
- R17 The compositor's reports are taken in the order they were sent, by the
  serial number each bus message carries, compared within one sender's. A
  report older than one already taken is dropped.
- R18 A place is handed on once nothing newer has come for
  `desktop.SettleWait` (150 ms), so a corner the window passes through on its
  way is never written.
- R19 A panel dragged with its drawer out is remembered then, and not at the
  next close.
- R20 A place a pixel from the one held, worked out from a wider window, is
  the same place and is not written again: the corner and the width are
  whole pixels, each rounded.
- R21 On Windows the watcher reports no width, because a window's rectangle
  there is in physical pixels and the panel's widths are not. A move with
  the drawer out is not remembered until the drawer closes, as spec 060 left
  it.

## Acceptance Criteria

From fakes, each falsified:

- [x] `Details()` for the cooler carries a label and a key for each list, in
  the rows' order, and leaves out a list the reading has not
  (`TestTheCoolerOffersAListForEachRowThatHasOne`,
  `TestAProcessListSaysHowItIsChosen`).
- [x] The usage section's whole detail: every account in the card's order,
  each with its heading, meters and lines; an account that cannot be read
  has its reason under its heading and the others are whole
  (`TestUsageOffersABreakdownPerAccount`, `TestEveryAccountIsADetailOfParts`).
- [x] `Detail.Text()` of a detail with parts (the same test). `hayami-tui
  detail usage` was run against the desk's own cache and printed every
  account; `detail "usage/<account>"` is spec 060's and unchanged.
- [x] No command line, path, user name or address reaches a `view.Detail`:
  spec 060's test passes unchanged. A detail with parts is the usage's, which
  holds no process; the test was not extended, because there is nothing of a
  process in it to leak.

In the window, headless, each falsified:

- [x] A click on a cooler row, on its heading and on another row each open
  the same detail; a second click on any of them closes it
  (`TestAClickAnywhereOnACardOpensItsDetailAndASecondClosesIt`). The plot is
  not a separate target the test harness can name; a click on the card with
  no row under it stands for it.
- [x] A click on any usage meter opens the detail of every account, with one
  rule between two accounts (`TestAClickOnTheUsageCardOpensEveryAccount`).
- [x] The switch has the choices the source offers; choosing one stops the
  shown list's poll, starts the other's and clears the filter; a key typed
  into a filter does not (`TestTheSwitchChangesWhatIsShown`). The choice is
  made by its key, through the window's own key handler; the click on a
  choice is the library's and held there.
- [x] The drawer opens on the choice last shown, and on the first offered
  when that one has gone (`TestTheCardOpensOnTheListLastShown`).
- [x] A usage breakdown taller than the cards leaves the panel's height
  alone and scrolls; a reading arriving keeps the offset; a list after it is
  cut to the height again (`TestABreakdownTallerThanTheCardsScrolls`,
  `TestAListAfterABreakdownIsCutToTheHeight`).
- [x] A detail is not polled while the drawer is closed or its section is
  hidden (spec 060's tests pass).
- [x] The parity test passes with its allow-list unchanged.
- [x] The place saved is the cards': closed, open on the left, dragged with
  the drawer out, put back to the open width after a close, a pixel of
  rounding, a window wider by something that is not a drawer, a drawer on
  the right, and no width at all
  (`TestThePanelIsRememberedWhereItsCardsAre`,
  `TestADrawerOnTheRightLeavesThePlaceAlone`,
  `TestWithoutAWidthAMoveWithTheDrawerOutIsNotRemembered`).
- [x] Reports out of order, from a second sender, across a wrapped counter,
  a place passed through, and a watch stopped (`internal/desktop/settle_test.go`).

On the real window:

- [x] Plasma 6 on Wayland, with a real pointer and keyboard, by the person
  at the desk (Verification): the cooler's card, its choices, the usage card
  with its accounts and rules, the close without a flash, and the four
  position cases with the rectangle read from KWin each time.
- [ ] Plasma: twenty closes filmed at the display's rate. **Not done.** The
  close was judged by eye at the desk and reported gone; no frames were
  captured, and this criterion is left unchecked and not claimed.
- [ ] Windows 11 (`tests/window`, posted clicks): spec 060's tests rewritten
  for one target per card and the switch by its key. **Written, compiled
  with `GOOS=windows go vet`, not run.** A breakdown taller than the cards
  has no real-window test there; it is held headless.
- [x] `go test ./...`, `make lint`, `make vuln`; `cmd/hayami-tui` builds
  with `CGO_ENABLED=0`.

## Out of scope

- A detail view in the terminal pane (spec 060, still).
- A switch on the bandwidth card for a list per interface.
- Remembering the cooler's choice across a restart.
- Scrolling the process lists.

## Risks & Assumptions

- Depends on fynedesygn 0.1.101, which has #206 and spec 069, and which
  `go.mod` names.
- **The Windows tests cannot be run from the Linux desk.** They are
  rewritten here and run on the Windows desk before the spec is complete.
- A click on a card that used to open one account's breakdown now opens all
  of them. A person who wanted one account scrolls to it; with three
  accounts of three or four limits each the breakdown is expected to exceed
  the cards' height on a four-card panel, so scrolling will be the common
  case there and not the exception. To be measured on the desk's own panel.
- The cooler's keys are letters, and the filter's key is `f`. A choice whose
  key is the filter's would be a fault; none is.
- `Row.Opens` and `Meter.Opens` leave the JSON `hayami-tui readings` prints.
  Nothing known reads them.
- Release safety: additive apart from R2. Rollback is a revert.

## Gaps found

In hayami:

- **The panel was saved at its drawer's corner.** Spec 060 saved nothing
  while a flag said the drawer was out. The flag was cleared in the call
  that closed the drawer, and the window was still, or again, at its open
  width when the compositor's next report came. And the bus library runs
  each incoming call on a goroutine of its own, so reports a millisecond
  apart were applied in either order. Seen at the desk on 2026-10-13: the
  window 553 wide at x 3447, `x: 3447` in the file, the cards at 3696.
  fynedesygn's fix for the flash asks for the closed width at once, where
  hayami had waited 120 ms, which brings the wrong report and the right one
  closer together; whether that made it likelier was not measured. Fixed
  here (R16 to R20).
- On Windows a panel dragged with its drawer out is remembered at the next
  close and not before (R21). A rectangle in the panel's own units there
  would let it share the Linux rule.

In fynedesygn, for its own issues:

- `glance.Row` has no heading style: an account's name in the usage drawer
  is a row like the lines under it, and the rule is what sets accounts
  apart.
- `Panel.Size()` leaves out a width the user dragged, so it is not the size
  the window is when a panel is resizable. `Panel.CardsSize` was added
  beside it and does not.
- The drawer's scroll bar cannot be dragged: the catcher takes every press.

## Verification

Plasma 6 on Wayland, scale 1.5, grid arrangement, four cards, the drawer
choosing the left, 2026-10-13. The installed panel's autostart unit was
stopped and this branch's build run in its place as a transient unit; the
person at the desk used it with a real pointer and keyboard.

- The close: reported without the flash, by eye, with hayami's two delayed
  re-measures removed.
- The click anywhere, the cooler's switch and the usage drawer of every
  account: accepted, and the rule between accounts added on request and
  accepted.
- Where the panel is remembered, the rectangle read from KWin and the file
  read each time:
  - Dragged with the drawer closed: the window at 3696, 2037 and 304 wide,
    the file `x: 3696`; restarted, it came up at 3696, 2037.
  - Restarted with the drawer out: the window at 3447 and 553 wide, the file
    still `x: 3696` and not written since the drag; it came up closed at
    3696, 2037, and `hayami-hold-edge` was unloaded.
  - Dragged up the screen with the drawer out: the window at 3447, 1417 and
    553 wide, the file written at the drag with `x: 3696`; restarted, it came
    up closed at 3696, 1417.
- The running panel took a report sent to its bus object by hand, in the
  form the compositor sends, with its own place in it.
- `hayami-tui detail usage` against the desk's own cache printed each
  account under its name.

Headless on Linux: twenty-six breaks of one behaviour each, across the
view, the sources, the window and the position watcher, each failing a test
after three tests were tightened and one added (an unlabelled detail drawn
as a choice, a list left scrolling after a breakdown, a pixel of rounding
saved as a move, the cooler's lists out of the rows' order).

Not run: anything on Windows.

## Alternatives Considered

- Keep the row's click as a shortcut to its own list and add the switch as
  well: rejected, because two ways to choose leave the heading's click
  ambiguous again and a second click on another row would have to mean
  either "switch" or "close".
- A switch between accounts in the usage drawer: rejected in the issue; the
  drawer has the room, and scrolls when it does not.
- Asking the compositor once where the window is, after the reports have
  stopped, and not trusting the reports at all: rejected for the round trip
  and a second script, when the message's own serial number puts the
  reports in order.

## E2E Test Plan

- Environment: the installed panel on the Linux desk (Plasma 6, Wayland,
  grid arrangement) and on the Windows desk, after an install of the built
  release.
- Steps and what is expected: the two real-window blocks under Acceptance
  Criteria, run against the installed binary and recorded under
  Verification with the rectangles read.

### Prod visibility

Not applicable: a desktop program with no production service. What a person
sees at their desk is the observation, and `hayami-tui detail` and `doctor`
print the same data without a display.
