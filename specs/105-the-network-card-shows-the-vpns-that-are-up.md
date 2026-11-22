# 105 — the Network card shows the VPNs that are up

**Issue**: #371

## Status: COMPLETE

## Executive Summary

The Network card shows only the VPNs that are not down, and holds them to
three tunnels' height (nine lines); past that the wheel scrolls them in the
window and the terminal, and the arrows scroll them in the terminal with the
card chosen. The view says what scrolls (`Row.Scrolls`,
`Section.ScrollLines`); the window puts those rows in fynedesygn's
`Scroller`, and the terminal cuts them and draws a bar. Review
`internal/view/scroll.go`, `Panel.rebuild` in `internal/gui/gui.go`, and the
mouse and key handling in `internal/tui/tui.go` first.

## Context

Spec 085 put every chosen VPN on the Network card, and spec 086 gave a tunnel
that is up its rates under its line. A VPN that is down is a line saying
"down": nothing to read, and the VPN drawer (`view.DetailVPN`, the switch's
"VPN") is where one is connected. With several VPNs chosen, the card is
mostly down lines, and each one pushes every card under it down.

The card is also unbounded: each VPN that comes up adds a line or two, so the
panel's height follows how many tunnels are up.

## Requirements

- R1 A VPN that is down has no row on the card. One that is connecting, up,
  or unknown keeps its row: connecting is what the person just asked for,
  and unknown is the system not answering, which is a fault worth seeing.
- R2 The drawer is unchanged: it lists every chosen VPN, down ones included,
  with their controls.
- R3 A VPN's colour on the plot is its place among the chosen VPNs, not
  among the ones shown, so a tunnel keeps its colour as others come and go.
  A down VPN has no trail on the plot.
- R4 The VPN rows sit in a scrolling run (fynedesygn spec 086, `Scroller`)
  that is never taller than three VPNs up take, each its state, its rates
  and its totals: nine lines (`view.VPNScrollLines`). Past that the wheel
  scrolls them a line a notch. The interfaces above and the plot below do
  not scroll, and the card keeps its height as VPNs come up past three.
  (Drafted as six, for two lines a tunnel; a tunnel that is up has a totals
  line too, and the person chose three tunnels in full over today's five
  lines.)
- R5 The view says what scrolls: `Row.Scrolls` marks the run's rows,
  `Section.ScrollLines` how many lines show, and `Section.ScrollFirst` is
  where a shell has scrolled it to, clamped by the view. Both shells read
  it; `view.RenderRegions` says where each section and its run were drawn,
  for a pointer.
- R6 With no interface chosen and no VPN shown, the card keeps one dimmed
  line, "no VPN up", so the card stays and a click on it still opens the
  VPN drawer. Where the VPN system is not answering, that is what the card
  says instead, not both.
- R7 The terminal panel scrolls the same run, nine lines high, two ways:
  - **Mouse**, where the terminal reports one (a terminal on the desktop):
    the wheel over the run scrolls it; over the rest of the pane it does
    nothing. The program turns on mouse reporting
    (`tea.WithMouseCellMotion`); with it on, selecting text takes Shift in
    most terminals, which the README says.
  - **Keys**, with the Network section chosen: `↓`/`j` and `↑`/`k` scroll
    the run first and move to the next or previous section only at its
    ends, as a nested list does. `tab` and `shift+tab` move straight away.
  The bar is a one-column `│`/`┃` at the right, drawn only while there is
  something to scroll; the section's other lines give up the same column,
  so its figures still end in one column. No parity allow-list entry: both
  shells show the same lines.
- R8 `row` and `grid` arrangements: each VPN still has its line; the cap is
  nine lines in the panel and in the terminal (in `row`, without the
  totals, two lines a tunnel).

## Acceptance Criteria

- [x] `make test` (`go test -race ./...`), `make lint` (0 issues);
      `CGO_ENABLED=0` build of `hayami-tui`; `go vet -tags nogui` for
      Windows on the changed packages.
- [x] Tests (`internal/view`, `internal/panel`): a down VPN has no row and
      no trail, and dialling, up and unknown have rows in the run
      (`TestOnlyAVPNThatIsNotDownHasARow`); a VPN's series is its place
      among the chosen ones (`TestATunnelsTraceFollowsTheInterfaces`); five
      tunnels draw nine of fifteen lines with a bar, every line the pane's
      width (`TestARunLongerThanItsLinesShowsItsLinesAndABar`), scrolled and
      clamped (`TestAScrolledRunStartsWhereItWasScrolledTo`), no bar when it
      fits, nine lines without totals in row, and the regions locate the
      run; no interfaces and no VPN up gives the "no VPN up" line opening
      the drawer (`TestAVPNAloneDrawsTheCard`); the drawer still lists a
      down VPN (`TestManyRoutesDoNotCrowdOutTheHealth`).
- [x] Window (`internal/gui`): the tunnels are in the card's run, the down
      one nowhere, nine lines shown, the card as tall with four or five
      tunnels up as with three, and the run hidden with none
      (`TestTheTunnelsAreARunHeldToNineLines`).
- [x] Terminal (`internal/tui`): the wheel over the run scrolls it and over
      the interface does not (`TestTheWheelOverTheRunScrollsIt`); with
      Network chosen, `↓` scrolls to the run's end and the next `↓` moves
      on, `k` scrolls back, `tab` moves at once
      (`TestTheArrowsScrollTheChosenRunBeforeMovingOn`); the parity test
      passes with no new allow-list entry.
- [x] Real terminal: `hayami-tui` in alacritty, scrolled with the wheel and
      with Tab and `k`, captured before and after (see E2E).
- [x] Real window: the card with the run scrolled by the wheel, its height
      and the plot's position the same before and after (see E2E).
- [x] Falsified: the down filter removed, the run not cut, the window's run
      rows put on the card, the arrows never scrolling, and the wheel taken
      anywhere in the card each fail a test.
- [x] README changelog under `### Unreleased`, and the VPN and terminal
      paragraphs; `docs/architecture.md` lists no view fields, so it is
      unchanged.

## Threat model delta

No boundary touched: what is read, from where, and what is shown are
unchanged; fewer rows are drawn.

## Risks & Assumptions

- Depends on fynedesygn spec 086's `Scroller`, released first; hayami adopts
  that release in this PR.
- Scrolling is by row, so a tunnel's rates can be at the top with its name
  scrolled off above. Accepted for now; scrolling by group is a later change
  if it reads badly.
- Rollback is reverting the PR; no setting changes shape.

## E2E Test Plan

The desk has three VPNs, aiq-corp up and two down, and three tunnels fit in
nine lines, so no real configuration there overflows the run. The scrolling
was checked on the desk's real readings with throwaway builds whose run is
two lines (`VPNScrollLines = 2`, never committed), on a copy of the
settings:

- Window, Plasma 6.8 Wayland at 1.5: the Network card shows eno2,
  tailscale0 and aiq-corp, and not pia-lv or aiq-labs, which are down (R1).
  The two-line run shows aiq-corp's state and rates with the bar; one wheel
  notch over it, sent with ydotool, shows its rates and totals, the thumb at
  the bottom of its track; the window stayed 851 px tall and the plot did
  not move (R4).
- Terminal, alacritty: the run's two lines with the bar beside them, the
  figures ending one column in from the edge on every line of the card; one
  wheel notch over them scrolls to the rates and totals; Tab marks
  "▸ Network" and `k` scrolls back to aiq-corp's line (R7).
- The release build with the nine-line run: the card as above, with no bar,
  since aiq-corp's three lines fit.

### Prod visibility

Not applicable: a desktop program with no telemetry.
