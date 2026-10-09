# 052 — A section moved in the preferences moves on the panel at once

**Issue**: #158

## Status: COMPLETE

## Context

Found in the 0.9.0 sit test on Windows, older than that: moving a section up
or down in Preferences → Sections changed the settings, and the window kept
the old order until it was restarted. `glance.Panel` could only add cards, so
the window's order was fixed when it was built; `Apply` said so in a comment
and nothing said so to the person moving the section.

fynedesygn 0.1.90 (its spec 060, issue #173) adds `Panel.SetOrder`: the cards
named first, the rest after in their own order, every card keeping its rows,
state and arrangement, one resize.

Two more things came out of looking. The window built a card and a source
only for the sections shown when it started, so a section ticked on later had
nothing to draw it until a restart; and it polled every source it held whether
the section was shown or not, against the rule that a hidden section costs
nothing. The terminal panel never looked at its settings again after starting,
so it did not follow a reorder either, live or otherwise.

## Requirements

- R1 Both shells hold a source for every section this build has, the shown
  ones first in the settings' order (`panel.Ordered`, `cli.Options.AllSources`).
- R2 A section the settings do not show is not polled, in either shell. One
  newly shown is polled at once, not at its next interval.
- R3 The window: `Panel.Apply` puts the cards in the settings' order with
  `glance.Panel.SetOrder`. Nothing is rebuilt; the window resizes once at most.
- R4 The terminal panel looks at its settings file every two seconds and, when
  its modification time has moved, takes the sections, their order and the
  arrangement it gives. A value given on the command line (`--sections`,
  `--arrangement`) stays as given. `--once` does not look.
- R5 Reading the settings does not write them.

## Acceptance criteria

- [x] AC1 A headless window takes the settings' order at once, and a row's
  handle is the same object after a reorder (`internal/gui/order_test.go`).
- [x] AC2 A hidden section is not polled; showing it polls it at once; `Apply`
  sets what is polled (`internal/gui/order_test.go`).
- [x] AC3 Moving a section up in the preferences saves the new order and tells
  the panel once (`internal/prefs/prefs_test.go`).
- [x] AC4 The terminal model draws the new sections, order and arrangement at
  its next look, polls a newly shown section at once, and polls nothing
  hidden (`internal/tui/watch_test.go`).
- [x] AC5 The settings watch sees a change once, leaves the file as it was,
  and keeps a pane's own `--sections` (`internal/cli/watch_test.go`).
- [x] AC6 On a real window: the cooler's up button, tapped in the preferences,
  puts the cooler above the bandwidth on the panel, and the window is the same
  size (`tests/window/reorder_windows_test.go`, from the pictures).

Each test was falsified: the poll gate removed, the wake removed, `Apply`
without the reorder, `Apply` without telling the pollers, the preferences'
swap removed, the terminal's watch not applied, its newly shown section not
polled, `Init` polling everything, and the pane's `--sections` not kept. Each
failed the test that covers it.

## Notes

- Under the Fyne test driver `fyne.Do` runs on the caller's goroutine, so two
  pollers drew at once in a test and crashed the text shaper. The window hands
  a poll's result over through `Panel.do`, which is `fyne.Do`; the test that
  runs pollers serialises it.
- The real-window test taps the preferences by posting the mouse's messages to
  the window rather than moving the pointer: GLFW takes the cursor's place from
  the move message.
