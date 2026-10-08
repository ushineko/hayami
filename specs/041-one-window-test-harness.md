# 041 — One window-test harness

**Issue**: #129

## Status: COMPLETE

## Context

`tests/window` drives the real panel on Windows and reads what it drew from a
picture of the window (specs 034–037). Each spec that added a test added its
own way to run the panel: `panel` (034, also used by 036), `startPanel` and
`abs2` (037), `launch` (035), all building the binary, writing a throwaway
settings file and finding the process's window in the same way, around
capture and geometry helpers copied between branches. Each test also built
the panel again, a minute of a full run spent compiling the same program.

The processors tests found their rows by position -- "the heading, then the
processor, then the card" -- so a row added above them, or the order changed,
would read the wrong line and fail for a reason unrelated to what they check.

The first runs of the consolidated tests turned up a fault in all of them: the
trend plot's light-blue line is as bright as text where it is anti-aliased,
and when it slopes across fifteen rows it is read as a line of text. The Wi-Fi
test failed one run in three on an unmodified build because of it, and the
earlier falsifications of that test could have been failing on the plot rather
than on the change.

## Requirements

- R1 One harness file, `tests/window/harness_windows_test.go`: the panel built
  once per run in `TestMain`; one launcher, `start(t, panelSettings)`, over a
  throwaway home, cache, config and settings file, with the sections, chosen
  interfaces and LibreHardwareMonitor address as fields; `shoot` takes and
  saves the picture and finds its lines of text.
- R2 Rows are found by their label. `cardOf` polls the section's own source,
  in the test's process, the way the panel does, and flattens its lines as the
  window does (rows, the reasons that stay on the card, each row's detail
  lines under it). A row's line in the picture is the heading's plus its
  index in that card. A picture whose number of lines differs from the card
  fails, naming both.
- R3 Ink is bright **and** not one of the plot's blue-led colours, so the plot
  is never read as text.
- R4 Every window test uses the harness: processors (034), its
  LibreHardwareMonitor case (036), peripherals (035, which keeps its own cell
  check: its card is cells, not rows), Wi-Fi (037). The positional lookups are
  gone.
- R5 The gate stays `HAYAMI_WINDOW_TEST=1`.

## Acceptance Criteria

- [x] The processors, Wi-Fi and peripherals tests pass on a real window, and
  the pictures were looked at.
- [x] The Wi-Fi test passes five runs in a row on an unmodified build (it
  failed one in three before R3).
- [x] Each test falsified, then restored and passing:
  - processors: the empty temperature left unpadded fails ("ends at x=357,
    where no part of the card's line ends"); the row drawn only with a
    temperature fails ("the card has no row labelled …").
  - Wi-Fi: the bars after the rates fails on both arrow columns and on the
    bars; no bars fails on the bars.
  - peripherals: the Razer and AULA vendors given no drivers fails (seven
    lines, where a drawn device is at most three).
- [ ] The LibreHardwareMonitor case run: it skipped here, because
  LibreHardwareMonitor was not running during this work. It needs a run with
  LibreHardwareMonitor up.

## Risks & Assumptions

- **The card is computed in the test's process** a moment apart from the
  panel's own poll. A reason that appears in one and not the other makes the
  line counts differ and fails the test with both listings; on this desk it
  did not happen in a dozen runs.
- **`cardOf` calls `panel.Sources`.** The section registry (#126) may change
  that call; whichever merges second adjusts this one line.
- **The plot's colours** are measured (purple, light blue, teal). A status
  colour that is blue-led would be ignored as plot; none is.
- **Rollback**: revert. Test-only.

## Gaps found

- The peripherals card is cells, not rows, so it cannot be looked up by label;
  its check stays structural (a card of devices is at most three lines).
- A row added under the card's spare rows would land under the plot in the
  window (review finding on row IDs); the line-count check would catch it.

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.26.0, MSYS2 UCRT64 gcc:

- `HAYAMI_WINDOW_TEST=1 go test ./tests/window/ -v`: peripherals PASS
  (Basilisk Ultimate dongle answering), processors PASS, Wi-Fi PASS, the
  LibreHardwareMonitor case SKIP ("LibreHardwareMonitor is not running").
- Wi-Fi five runs in a row: PASS ×5.
- Falsifications as above, each restored, `git diff` clean afterwards.
- Full suite: every package ok except `internal/usage`'s Python cross-check,
  which fails on `main` on this machine too.
