# 110 — a new tunnel scrolls rather than growing the card

**Issue**: #388

## Status: COMPLETE

## Executive Summary

The Network card's VPN run is held to one tunnel's three lines instead of
three tunnels' nine, so a second or third tunnel coming up scrolls inside
the run and the card keeps its height. Only the first tunnel coming up, or
the last going down, changes it. One constant, `view.VPNScrollLines`.

## Context

Spec 105 put the tunnels that are up in a scrolling run held to nine lines,
three tunnels. On the desk (2026-11-22) connecting a VPN that had no row
pushed the cards under the Network card down: the run grows with its rows up
to its cap, so every tunnel up to the third resized the card. Asked, the
person chose a run one tunnel high, the rest scrolled.

## Requirements

- R1 The run is one tunnel high: three lines (its state, its rates, its
  totals), in the window and the terminal.
- R2 With any tunnel up the card is one height; a tunnel coming up or going
  down while another is up changes nothing outside the run.
- R3 The first tunnel coming up and the last going down still show and hide
  the run (nothing to scroll is not drawn).

## Acceptance Criteria

- [x] `TestTheTunnelsAreARunOneTunnelHigh` (window): one tunnel up shows a
      run of three; two, three and five up keep the card at one tunnel's
      height; none up hides the run and gives the room back. Falsified with
      the constant at nine.
- [x] The run's workings (scroll, bar, keys) keep their tests at nine lines,
      set in the test, so they do not depend on the card's height.
- [x] README's VPN paragraph says one tunnel's height.
- [x] `make test`, `make lint`; changelog line.

## Risks & Assumptions

- With two tunnels up the second is a turn of the wheel away; the drawer's
  VPN view lists them all.
- Rollback: the constant back to nine.

## Threat model delta

No boundary touched: layout only.

## E2E Test Plan

The installed release with one VPN up, a second connected from the drawer:
the cards under Network do not move, and the wheel over the VPN lines shows
the second.

### Prod visibility

Not applicable: a desktop program.
