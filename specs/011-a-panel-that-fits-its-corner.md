# Spec 011: a panel that fits its corner

**Issue**: [#28](https://github.com/ushineko/hayami/issues/28)

## Status: COMPLETE

## Executive Summary

The usage meters put every figure in one caption, and a caption sets the width
of the meter, the panel and the window. They are spread across the slots
fynedesygn gained in its spec 039 instead — the window the bar is about in the
caption, the others at the left of a row under the bar, the amounts at its
right, the reset in the header's trailing column. The panel measures **340 px
against 655 px**. Reviewers should start with `spread` in
`internal/view/usage.go`.

## Context

The panel was 655 px wide. Without the usage section it was 268 px — the
design system's floor plus padding — so one section was more than doubling the
width of the whole window, and every other card was padded out to match it.

The cause was a single caption:

```
5h: 0 %  7d: 0 % (6d left)  limit: 34 % 403.51 / 1200.00 (2d left) · in  4h 59m
```

`glance.Meter` drew its label and caption on one line, so that sentence was the
meter's minimum width. The window could not be made narrower by hand either —
`glance` fixes a window to its content, deliberately — and Fyne will not shrink
a window below its content's minimum in any case, so nothing but shorter
content would have helped.

The archetype does not lay it out that way. `peripheral-battery-monitor`'s
Codex section is three rows, each a widget, a stretch and a widget, and **the
stretch is the point**: a row laid out that way is as wide as its two ends
rather than as wide as everything in it laid end to end.

That shape was missing from the library, so it went there first — fynedesygn
spec 039 — rather than being hand-rolled here, which is what this repository's
rules require of a shape the design system lacks. This spec is the other half:
deciding which of hayami's figures go in which slot.

The division follows the archetype. The caption keeps the window the bar is
about, because the bar and its caption should agree. The other windows go to
the left of the row beneath, the lead window's own amounts to the right, and
the reset into the header's trailing column where it is the same distance from
the edge on every meter — which is what a column is for and what a sentence is
not.

Nothing is dropped. Spec 004's parenthetical stays: a window whose reset is not
the one in the countdown says how long it has left, because otherwise the
weekly window — the one worth planning around — would say nothing about when it
turns over.

A pane is unchanged. It has one line per meter, so `Meter.Line` folds the
figures back into the order they were always drawn in.

## Requirements

- R1 A usage meter's caption carries the window its bar is about, and nothing
  else.
- R2 The other windows' figures, and the lead window's amounts, are carried in
  the meter's stats slots.
- R3 The reset goes in the trailing slot, so it is in one column down the card.
- R4 No figure is lost, including the remaining-time parenthetical spec 004
  added.
- R5 A pane draws exactly what it drew before: one line per meter, every figure
  in reading order.
- R6 The panel is materially narrower, measured rather than asserted.

## Acceptance Criteria

- [x] AC1 The caption of a meter with several windows holds the lead window's figure and not the others'. (R1)
- [x] AC2 The other windows' figures are in the left stat and the lead's amounts in the right. (R2)
- [x] AC3 Every figure that was in the old caption is in `Line()`. (R4, R5)
- [x] AC4 A window whose reset is not the countdown's still says how long it has left. (R4)
- [x] AC5 A pane's rendering of a usage section is unchanged. (R5)
- [x] AC6 **On this machine**, the panel measures materially less than the 655 px it did, with the same four sections. Photographed. (R6)

## Gaps found

None remaining: the gap this work found was in the design system and was
filled there first (fynedesygn spec 039, #83). `Meter.SetTrailing` and
`Meter.SetStats` are that change, and `glance.Options.Resizable` came with it
from #84 — a fixed-size window greys out the compositor's own resize, so a
user who wanted a wider panel had no way to ask.

`Resizable` is deliberately **not** turned on here. The panel is 340 px now,
which is the size the archetype is built around; a window that can be dragged
to any size is a different decision from a window that is the wrong size, and
the second one was the bug.

## What the tests caught

**The remaining-time parenthetical.** The first version of `spread` built each
figure from the name and the percentage and dropped `(5d left)` — spec 004's
addition, and the reason a weekly window says anything at all about when it
turns over. `TestALongerWindowSaysHowLongItHasLeft` failed immediately. It was
written three specs ago against a behaviour that had no other guard.

**Two tests asserted the old contract and had to be rewritten rather than
deleted**, which is the distinction worth making: the claim they were making —
that nothing is lost — is still true, so they now assert it against `Line()`,
which is every figure in reading order. Deleting them would have dropped the
claim along with the implementation detail.

## Risks & Assumptions

- **340 px is this machine's figure**, with these accounts and these windows.
  An account with a longer name or a provider with more windows will be wider.
  The shape is what fixes it, not the number: the width is now the widest
  *pair* in a row rather than the sum of everything, so it grows far more
  slowly than it did.
- **The stats row is drawn only when there is something in it.** An account
  with one window has no other figures, so its meter is the header and the bar,
  exactly as before.
- Rollback: revert. The pane is unchanged either way, which AC5 asserts.
