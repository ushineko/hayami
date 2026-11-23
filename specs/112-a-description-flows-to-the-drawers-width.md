# 112 — a description flows to the drawer's width

**Issue**: #393

## Status: COMPLETE

## Executive Summary

A change's title, description and labels (spec 111) were wrapped by the view
at a fixed 36 characters, which left a third of the window's drawer empty and
ignored a drawer made wider. They are now prose lines (`DetailLine.Prose`), a
paragraph each, and each shell flows them to the width it has: the window
measures in the face it draws in against the drawer's room, the terminal
counts its columns (`view.Reflow`). Review `flowProse` in
`internal/gui/reflow.go` and the prose branch of `RenderDetail` first.

## Context

On the desk (2026-11-22), the first release of change pages showed a
description as a narrow column down the drawer's left: the view wrapped every
paragraph at `DevWideNameWidth`, a count of characters chosen for a line with
a value at its end, in a face that is not monospace. The person asked for the
description to be re-flowed to fit.

## Requirements

- R1 A change's title, each paragraph of its description, its labels and,
  while it is read, the day's line for it, are one prose line each. The view
  does not cut them (beyond 1,200 characters, spec 111 review F1).
- R2 The terminal flows a prose line to the drawer's columns, words whole
  where they fit, a word longer than a line broken; a flowed paragraph is
  one line for the selection however many rows it takes.
- R3 The window flows a prose line to the drawer's room, measured in the
  face and size its rows draw in, at the line's indent, and flows again when
  the drawer's width changes (Fill). The last result is kept per page and
  width, so a page drawn every poll is not measured every poll.
- R4 Every other line is as it was.

## Acceptance Criteria

- [x] `TestProseFlowsToTheWidth`: at 20, 36, 60 and 200 columns every row
      fits, no character is lost, a line that fits is one row, other lines
      unchanged.
- [x] `TestTheTerminalFlowsADescriptionToItsWidth`: a wider terminal drawer
      is fewer rows, each within the width. Falsified with a fixed 36.
- [x] `TestADescriptionFillsTheDrawersWidth` (window): every row fits the
      drawer's room, the words are all there in order, the longest row is
      over 80% of the room and longer than 36 characters. Falsified with no
      flowing and with a 36-character measure.
- [x] `TestAChangeReadIsItsWholePage`: the title and paragraphs are prose,
      the description cut at its paragraph limit.
- [x] Real window on the desk: a pull request's description fills the
      drawer and its title is one line; with Fill the description flows
      again to the wider drawer.
- [x] `make test`, `make lint`; changelog line.

## Risks & Assumptions

- The window measures with the panel's theme font; a face that draws wider
  than it measures would clip a row's last letters. The measure is the same
  one the drawer's columns already use (`monoWidth`).
- Rollback: revert the PR.

## Threat model delta

No boundary touched: layout of text already shown and cleaned.

## E2E Test Plan

The installed release: open a change with a long description, see it fill
the drawer, press Fill and see it flow again.

### Prod visibility

Not applicable: a desktop program.
