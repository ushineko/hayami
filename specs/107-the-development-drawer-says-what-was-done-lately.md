# 107 — the Development drawer says what was done lately

**Issue**: #374

## Status: COMPLETE

## Executive Summary

The Development drawer's empty space now says what was done lately: Recent,
the latest days across GitHub and GitLab, each day's repositories and
things, cut to the room so the drawer never scrolls; and Repositories, the
fortnight's three busiest with a bar each. Both come from the forges' last
reads. Part names and headings are bold and a day's lines are indented
(fynedesygn 0.1.125). `readings` and `doctor` leave the two parts out.
Review `internal/view/devrecent.go`, `fitPart` in `internal/gui/drawer.go`
and `fitPart` in `internal/view/drawertext.go` first.

## Context

The Development drawer (spec 102) is the cards' height. Under the usage
accounts and the GitHub and GitLab strips, the rest of it was empty. The
forges' reads already carry every item of the fortnight -- repository, kind,
number, title, what was done -- and the drawer showed them only a day at a
time, behind a click on a strip's cell.

Of five ideas offered on the desk (2026-11-21), the person chose to try the
two that need no new request: what was done lately, and where the
fortnight's work went. Tried in the real drawer, they asked for it to fill
the room and no more, for bold headers and indents, and, since the accounts
take most of the drawer on their desk, for three repositories, not five.

## Requirements

- R1 **Recent**: a part under the forges, the days with anything listed,
  newest first, across both forges: each day's heading with its count, its
  repositories one step in, the things worked on two steps in, as a day's
  page draws them. A forge's private count stays on its own strip.
- R2 **Repositories**: a part under Recent, the fortnight's three busiest
  repositories across the forges, a bar each against the busiest, with
  counts by kind. Owners are dropped (rule 9).
- R3 Neither part makes a request: both come from the forges' last reads,
  and neither is drawn before a forge has listed something.
- R4 Recent fills the room the drawer has left once everything else is
  drawn, in the window and in the terminal, and never makes the drawer
  scroll (`Detail.Fit`). A day's heading is not left at the bottom with
  nothing under it; with no room at all the part is not drawn.
- R5 In the overview every part's name is bold (`Detail.TitleBold`), and
  every heading line in a drawer is bold (`DetailLine.Heading`). Lines
  carry an indent (`DetailLine.Indent`): a day's page indents the same way.
  The terminal indents by two columns a step and has no bold.
- R6 fynedesygn 0.1.125: `Row.SetLabelBold`, `Row.SetIndent` (its spec 087),
  and `Drawer.Available` counting the heading's buttons, without which a
  fitted list beside the Fill button scrolled the drawer by ten pixels.

## Acceptance Criteria

- [x] `make test`, `make lint`.
- [x] Tests (`internal/view`): Recent is both forges, newest first, with its
      headings, indents and no private count or owner; Repositories is the
      busiest with a bar each; in a drawer of a given height Recent takes
      what is left, Repositories under it is still drawn, no heading is left
      alone, and with no room Recent goes; the overview's parts are bold.
- [x] Tests (`internal/panel`): the overview has Recent and Repositories
      under the forges once they list something.
- [x] Tests (`internal/cli`): `detail` prints Recent and `readings` leaves
      its repositories and titles out (`TestRecentIsInDetailAndNotInReadings`).
- [x] Falsified: the readings filter removed fails a test.
- [x] Real window: the drawer on the desk with the usage accounts shown,
      Recent cut to the room, the drawer not scrolling, the last bar whole;
      bold part names and day headings, indented repositories and things.
- [x] README changelog under `### Unreleased`.

## Threat model delta

B6 (what hayami shows): the drawer now shows repository names and titles of
pull requests and issues without a click, which a day's page already showed
behind one. Owners stay dropped (rule 9); nothing new is read or sent. The
overview is a listed detail, so `readings` and `doctor` would have printed
them, and their output is what gets pasted into a bug report: the parts are
marked `Personal` and left out there (`Detail.Listed`). The threat model's
B6 section notes it.

## Risks & Assumptions

- A Recent cut to the room shows less on a desk whose accounts take most of
  the drawer: two lines on the desk it was tried on. Hiding the accounts, or
  a taller panel, gives it more.
- Rollback is reverting the PR.

## E2E Test Plan

On the desk, from this branch's build, on a copy of the settings: open the
Development drawer with the accounts shown; Recent is cut to the room and the
drawer does not scroll (R4); the part names and the day are bold, a
repository and a thing are indented under it (R5); three bars (R2).

Done on the desk (Plasma 6.8 Wayland, scale 1.5), on a copy of the
settings, the drawer opened with a click: with the usage accounts shown, the
first build gave Recent no room (five bars), so Repositories went to three;
then Recent had room, but the last bar was clipped and the drawer scrolled by
about ten pixels -- fynedesygn's room under the heading left out its Fill
button (fixed there, 0.1.125). After the fix the drawer fits: Recent shows
the day in bold and the repository a step in under it, the three bars are
whole, and nothing scrolls.

### Prod visibility

Not applicable: a desktop program with no telemetry.
