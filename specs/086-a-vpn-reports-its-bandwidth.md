# 086 — a VPN reports its bandwidth

**Issue**: #312

## Status: COMPLETE

## Executive Summary

A VPN that is up shows its rates and totals under its line on the Network card, in the
columns and bands of an interface, and joins the card's plot and `readings`. The VPN
drawer's latency plot now drifts between checks. A reviewer should look first at how a
tunnel is sampled in `BandwidthSection.record`, and at the amended glance rule: the card
now changes height with a tunnel, and a panel near the screen's bottom can run off it
(#313).

## Context

A chosen VPN's line on the Network card gives its state, latency and connected
time (spec 085). Its traffic is shown only if its interface is also chosen, as a
separate row under a different name. Spec 085 R8 chose that to avoid repeating
traffic; in use, the line that says a tunnel is up is where its traffic is wanted.

The VPN drawer's latency plot was left out of spec 064: it has no drift, so it
stands still and then steps when a check lands.

## Requirements

- R1 A VPN that is up and has an interface gets its rates and totals under its
  line, in the columns and colour bands of an interface's rates. The tunnel's
  line stays first, so the state is read first.
- R2 A VPN that is down, dialing, of unknown state or with no interface stays
  one line.
- R3 **The card changes height when a tunnel comes up or goes down.** The operator
  chose this over keeping blank lines for every down VPN. It amends spec 085 R6
  and the glance rule, which `.claude/CLAUDE.md` and `docs/architecture.md` state.
  A value arriving still moves nothing: an up tunnel's rate line is there, blank,
  before its first rate.
- R4 The card does not get wider for it: a rate line is the width of a VPN's line.
- R5 A tunnel's rates and totals are read from the same counters as an interface's.
  The totals are the kernel's for that interface, so they count from when the
  tunnel's interface was created.
- R6 A tunnel's rates join the card's plot, one colour per tunnel after the
  interfaces', on the shared scale. They are emptied when the tunnel goes down.
- R7 A card with only VPNs chosen reads their counters. A card with no VPN chosen
  is unchanged.
- R8 `readings` gives an up tunnel's rates beside it, and still no address.
- R9 The terminal's row arrangement names a tunnel's rate line with the tunnel, as
  a pane has no line above to lean on.
- R10 The drawer's plot drifts over the check interval, as a card's plot drifts
  over its poll interval. A failure trace that leaves the window is emptied, as a
  card's plot empties a trace, rather than kept.
- R11 The drawer plots the newest checks that ran, so a check that could not run
  does not shorten the window and break the drift.

## Acceptance Criteria

- [x] R1, R4: an up VPN's rate and totals lines are the width of its state line and
      of an interface's, with the figures in the same columns.
      `TestATunnelsRatesAreInAnInterfacesColumns`, falsified by widening the unit.
- [x] R1: a rate on a VPN's line takes the same band as on an interface's.
      `TestATunnelsRateTakesAnInterfacesBand`.
- [x] R2: down, dialing, unknown and no-interface VPNs are one row.
      `TestATunnelWithoutTrafficIsOneLine`.
- [x] R3: an up tunnel's rate line is there before its first rate, with blanks.
      `TestATunnelsFirstRateMovesNothing`, `TestAnUpTunnelHasItsTrafficOnTheCard`.
- [x] R5, R7: a tunnel's rates come from the counters, with no interface chosen, and
      a counter that went backwards gives no negative rate.
      `TestATunnelIsReadWithNoInterfaceChosen`, `TestACardWithOnlyVPNsReadsTheirCounters`,
      `TestATunnelsCountersAreTakenAsAnInterfacesAre`.
- [x] R6: trails follow the interfaces' series and empty when the tunnel goes down.
      `TestATunnelsTraceFollowsTheInterfaces`, `TestAnUpTunnelHasItsTrafficOnTheCard`.
- [x] R7: with no VPN chosen, the card is unchanged (`TestWithNoVPNChosenTheCardIsUnchanged`).
- [x] R8: `readings` carries an up tunnel's rates and no address.
      `TestReadingsCarryATunnelsRates`, and read on the desk from a real tunnel.
- [x] R9: the row arrangement labels the rate line with the tunnel's name.
      `TestAPaneNamesATunnelsRateLine`, and seen in `hayami-tui --once` on the desk.
- [x] R10: the drawer's plot is told the check interval; a trace that left is emptied.
      `TestTheDrawersPlotDriftsAndLosesATraceThatLeft`, falsified by removing `SetDrift`
      and, separately, the emptying.
- [x] R11: a window of checks with some that did not run still holds the newest
      points that did. `TestTheDetailPlotsTheNewestChecksThatRan`.
- [x] Integration: a real tunnel's rates and totals are read from the kernel's
      counters on a real window on the desk, with its figures lined up to the other
      rows, measured from the picture. On 2026-11-04, with one tunnel up: the down arrow
      at x=106, the down unit at 177 and the up unit at 312 on the tunnel's lines and on
      the interfaces'. The same measurement shows drift on a capture of the 0.12.0
      layout. A scripted run, since `tests/window` is Windows only and this is Linux
      only; it does not guard against a later regression.
- [x] The drawer's plot is seen moving between checks on a real window. Seen by the
      operator on the branch's build on the desk on 2026-11-04.
- [x] The card's height changes as R3 says on a real window, connecting a real VPN.
      On the desk on 2026-11-04, the operator connected and disconnected two VPNs on
      the branch's build: 649 px tall with three tunnels up, 587 with one, two lines
      each. The window's top moved 31 px down as it shrank, half the change, with no
      drag. A scripted reading of the window's geometry; it does not guard against a
      later regression.

## Risks & Assumptions

- **Rollback**: revert the commit. No settings or stored format changes.
- **Repeated traffic**: a tunnel whose interface is also chosen shows its rates
  twice. Accepted: the person chose both, and can untick the interface.
- **Reflow**: a bounce changes the card's height twice. The window already handles a
  row appearing, and a bounce is rare.
- **Off the screen**: the window has no bottom anchor. Placed near the bottom edge, it
  runs off the screen when a tunnel comes up, until it is dragged back, and the
  operator saw this on the desk. It shrank about its centre with no drag, and the new
  position was saved. The operator accepted this for now, and #313 tracks it. About 15
  px of room per tunnel that may come up avoids it. Not measured: a growth on its own.
- **Failure spikes**: the failure trace's height comes from the window's greatest
  latency, so it jumps rather than drifts when that greatest latency changes, and when
  a first failure appears.
- **Plot cost**: a drift redraws at 10 Hz for the whole check interval while the
  drawer is open. Spec 064 measured about 1.25 % of a core for that redraw rate.
- **Trail on reconnect**: the rates' trail is emptied when the tunnel goes down, so a
  reconnect starts it again. A bounce too quick for a poll to see keeps it, and a
  counter that went backwards gives a rate of nought for that sample.

## Threat model delta

No boundary touched. The counters are read as an interface's already are, and the
interface's name is already in the row's tip and in `readings`.

## E2E Test Plan

On the desk, with a real tunnel up:

1. Run a panel built from this branch. The tunnel's rates and totals sit under its
   line. Measure the columns from the picture (R1, R4, R5).
2. Open the VPN drawer and capture the plot repeatedly between checks: it changes
   between captures (R10).
3. Connect and disconnect a VPN, with the operator's agreement: the rate lines come
   and go, and the card's height follows (R2, R3).

### Prod visibility

Not applicable: a desktop program with no servers. A released build is checked on
the desk.

## Alternatives Considered

- Three lines for every VPN, blank while down: keeps the glance rule, but each down
  VPN costs two lines of dashes. The operator chose against it.
- Two lines with no totals: fixed height, but not what an interface shows.
- Totals and rates as detail text under the state line: a detail line has one colour,
  and a rate's band would be lost.
