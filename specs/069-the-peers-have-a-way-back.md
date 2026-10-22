# 069 — the peers have a way back

**Issue**: #226

## Status: COMPLETE

## Executive Summary

The Bandwidth drawer's "Bandwidth by peer" now draws the Procs / Peers switch
at its top, as "Bandwidth by process" does, so the process list can be reached
again by a click, and "Bandwidth by process" no longer draws a Back button
that led to the peers. Reviewers should look first at `renderParts` in
`internal/gui/drawer.go`, `Bandwidth.DrillUp` in
`internal/panel/bandwidthdrill.go`, and `holds` in
`internal/gui/export_test.go`, which is why the existing switch tests did not
catch the first.

## Context

The Bandwidth drawer offers two lists, by process and by peer, through a
switch in the drawer's heading. The peer list is a breakdown of parts, one per
interface, and the drawer draws a breakdown by another path than a list:
`renderParts`. That path built its content without the switch. Choosing Peers
therefore led to a page with no switch, and the only way back was the key `p`,
which nothing on screen named.

The process list had the opposite problem: a Back button. The drawer draws
Back when the section says a page's way out leads elsewhere, and
`Bandwidth.DrillUp` sent every page shallower than a name to the peers,
including the process list, which is not a drill. Back on the process list
led to the peers, which is likely why the missing switch went unnoticed.

The test helper `DrawerChoices` checked the switch's visible flag, which is
set whether or not any layout holds the switch, so the gap was invisible to
the tests.

## Requirements

- R1 A breakdown of parts is drawn with the drawer's switch first, as a list
  is. A breakdown with nothing to choose between (Usage by account) has a
  switch that draws nothing, as before.
- R2 From "Bandwidth by peer" the switch chooses "Bandwidth by process" by a
  click or by its key, and the process list is drawn with its switch again.
- R3 Back is drawn only on a drill-down: a top list ("Bandwidth by process",
  "Bandwidth by peer", the cooler's lists) has no way out.
- R4 The test helper reports a switch only when it is in the window's object
  tree.

## Acceptance Criteria

- [x] R1, R2: `TestTheSwitchHeadsTheBreakdownOfPeersAsWell` goes Procs →
      Peers → Procs and asserts the switch on each; it fails without the fix
      on "the peers have no switch".
- [x] R3: `TestOnlyADrillHasAWayOut` asserts each page's way out for both
      sections; it fails without the fix on "the way out of \"processes\"".
- [x] R4: `DrawerChoices` requires `holds(window content, switch)`; the
      existing switch tests pass through it.
- [x] The switch is visible and clickable on "Bandwidth by peer" on a real
      window (operator check, 2026-10-22).

## Risks & Assumptions

- The peer breakdown scrolls, and the switch is its first piece, so it
  scrolls with the rows rather than staying fixed in the heading.
- Rollback: revert the commit. GUI-only; no settings or cache change.

## E2E Test Plan

The operator opens the Bandwidth drawer on the live panel, chooses Peers,
and clicks Procs in the switch on the peer view (R2).
