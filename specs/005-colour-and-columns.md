# Spec 005: colour, and columns in a pane

**Issue**: [#10](https://github.com/ushineko/hayami/issues/10)

## Status: COMPLETE

## Executive Summary

`Render` gains a painter — a function from text and status to text — so the
terminal can colour without `internal/view` knowing what colour is. The row
arrangement is four columns: label, a bar that takes what is left, the figures
ranged left, and the reset hard right. A meter's label names the window its bar
is about, and a section's title is no longer repeated on every line of a pane.
Reviewers should start with `tools/shot-tui.sh`, which is why this spec found
the bug it did.

## Context

The row arrangement draws the right lines and the wrong pane. Beside the
program it replaces, four things are missing: colour, a label that names the
window its bar is about, two columns rather than one right-aligned blob, and
the absence of a section heading that a pane has no room for.

Colour is the one with a design decision in it. `internal/view` describes a
section and must stay free of any toolkit — the terminal binary builds with
cgo off and will keep doing so — so it cannot import lipgloss, and the window
does not want lipgloss anyway because Fyne colours its own widgets.

The answer is that `view` already says *what* a thing is. Every row and meter
carries a `Status`, and that is the whole of the claim: this reading is good,
this one deserves attention, this one is a failure. What a shell does about it
is the shell's. So `Render` takes a painter — a function from text and status
to text — and the terminal passes one backed by lipgloss while the tests pass
none and keep asserting plain strings.

That keeps the rule the two shells rest on: the view says what a section says,
and neither shell decides that for itself.

## Requirements

- R1 `Render` takes an optional painter. Nothing is painted without one, so
  every existing test and every pipe still sees plain text.
- R2 The terminal panel paints: a status colour for a value or a bar, a dim
  one for a label and a track, and nothing at all when the terminal says it
  cannot.
- R3 In `row`, a section's title is not repeated on every line. The label
  carries the section only where a row has no label of its own.
- R4 A meter's label names the window its bar is about.
- R5 In `row`, the figures and the reset are two columns: the figures ranged
  left from a fixed column, the reset hard against the right edge.
- R6 The window is unchanged. It already colours by status through the design
  system's own widgets, and this spec must not make it paint twice.

## Acceptance Criteria

- [x] AC1 `Render` with no painter returns exactly what it returns today; the existing tests are the assertion and none of them change. (R1)
- [x] AC2 A painter is called once per coloured piece, with the status the view gave it. (R1, R2)
- [x] AC3 `NO_COLOR` or a terminal that reports no colour yields a plain pane. (R2)
- [x] AC4 No line in `row` begins with the section's title where its rows are labelled. (R3)
- [x] AC5 A meter's label names its leading window. (R4)
- [x] AC6 In `row`, the reset ends at the right edge and the figures begin at the same column on every line. (R5)
- [x] AC7 The window's tests and the parity test pass unchanged. (R6)

## Gaps found

None. `view.Painter` is this repository's own and the design system needed no
change.

## What the photograph found

The first version gated colour on `lipgloss.ColorProfile() != 0`, which reads
as "does this terminal have colour". **Photographed in alacritty with
`TERM=alacritty`, that call answers `0` — the value meaning no colour at all —
on the same line that renders green.** Colour was therefore off everywhere,
and every test passed: under `go test` there is no terminal, so lipgloss
degrades every style to plain text and a good reading paints identically to a
bad one.

Two things follow, and both are in the code as comments. The gate now asks
only whether the person said `NO_COLOR`, because styles degrade on their own
where there is no colour. And `internal/tui`'s test deliberately does not
compare one verdict's colour with another's: a test that did would either fail
against correct code or pass against code that painted nothing, which is how
the bug got in. That claim belongs to the photograph.

`tools/shot-tui.sh` is the harness: it starts a real terminal of a known size,
runs the program in it, and grabs the window — the terminal's half of the rule
that anything visual is tested on a real window.

## Risks & Assumptions

- **A painter is a hook, and a hook is a way for a shell to start deciding.**
  It takes a status and returns text: it cannot see the reading, the section
  or the arrangement, so the most it can do is colour what the view already
  classified.
- **Colour is not information.** Every figure a colour emphasises is also in
  the text, because a pane is read over ssh, in a pipe, by somebody who cannot
  distinguish red from green, and into a file.
- **A headless test cannot see colour.** That is not a gap in the suite, it is
  the reason `tools/shot-tui.sh` exists, and pretending otherwise is what put
  this spec in the backlog.
- Rollback: revert. The row arrangement returns to the shape in the
  screenshot, which works.

## Alternatives Considered

- Returning styled segments from `Render` instead of strings. It is the more
  general answer and it changes every caller and every test to express
  something only one shell uses. The painter is smaller and reversible.
- Importing lipgloss into `internal/view`. Rejected: it would put a terminal
  library in the package the window also reads from, and the cgo-free build of
  the terminal binary is the one thing keeping that boundary honest.
