# 108 — the clock is larger, and clear of the tuck button

**Issue**: #383

## Status: COMPLETE

## Executive Summary

The clock's heading is drawn at the theme's sub-heading size (18 against the
ordinary 14), and its date stops short of the tuck button instead of running
under it. Both come from fynedesygn spec 088 (`Card.SetTitleSize` and the
room a title end keeps); hayami asks for the size in one line where the
window builds the clock's card. Review the dependency bump and that line in
`internal/gui/gui.go` first.

## Context

On the desk (2026-11-22), the clock of spec 106 read small: it is drawn at
a card title's size, and it is the line read from furthest away. And with
the clock as the top card, the tuck button (fynedesygn spec 081) stands over
the end of its date: the button reaches 0.625 text sizes past where a card's
heading ends.

## Requirements

- R1 The clock's time, date and both icons are drawn at
  `theme.SizeNameSubHeadingText`, which follows the panel's text size. The
  other cards' titles, and the clock's margins, are unchanged.
- R2 The date stops a quarter text size short of the tuck button, wherever
  the clock is in the stack (fynedesygn spec 088 R3).
- R3 The terminal panel is unchanged: a terminal has one size of text.

## Acceptance Criteria

- [x] fynedesygn v0.1.126 (spec 088) required by go.mod; no `replace` for
      it.
- [x] `TestTheClockIsDrawnLarger`: the time and date at the sub-heading
      size, another card's title at the text size. Falsified: without the
      `SetTitleSize` call it fails.
- [x] The date's clearance of the tuck button is fynedesygn's
      `TestATitleEndStopsShortOfTheTuckGlyph`.
- [x] A capture of the real panel with the tuck button shown: the clock
      larger, the date clear of the button.
- [x] `make test`, `make lint`; `CGO_ENABLED=0` build of `hayami-tui`.
- [x] Changelog line under `### Unreleased`.

## Risks & Assumptions

- The clock's card is taller by the heading's growth (about 4 px at the
  default size), which moves the cards under it once, when the version is
  installed; nothing transient reflows.
- Rollback: revert the commit; the dependency bump is additive.

## Threat model delta

No boundary touched: layout only.

## E2E Test Plan

`tools/screenshot.sh --what panel` against the change build, with the
installed panel quit for the capture and restarted afterwards: the clock at
the larger size, the date clear of the tuck button. Done against a build
pointing at the fynedesygn branch with the button shown, and again against
v0.1.126, 2026-11-22: the clock larger, the date ending clear of where the
button stands.

### Prod visibility

Not applicable: a desktop program; the installed panel on the desk is where
it is seen.
