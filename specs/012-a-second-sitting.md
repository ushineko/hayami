# 012 — A second sitting

**Issue**: #34

## Context

Five things found by running the panel beside `peripheral-battery-monitor`,
the program it replaces. Four of them are one question asked of different
sections — what does this section actually *say* — and the fifth is the
preferences window wearing the panel's face.

They are one spec because they were one sitting and because three of the five
touch `internal/view`, where a section is described once for both shells. A
separate spec each would have three of them editing the same file for the same
reason.

The monitor is the behavioural reference until this program replaces it. Where
this spec says "the monitor does X", that is a claim about
`~/git/ag-scripts/peripheral-battery-monitor` and is checked against its source,
not remembered.

## Requirements

### 1. The peripherals section draws a cell per device

`view.Peripherals` produces rows. It should produce **cells**: a device's name,
its level, and what the battery is doing, as three parts a shell lays out
vertically rather than as a label and a value on one line.

The monitor's cell is the archetype — name centred, icon and percentage large
and together beneath it, state quiet beneath that — and the percentage is what
the eye lands on. The row form makes the percentage a small number at the right
margin with the same weight as the label, and says nothing about the state
unless the battery is charging.

This reverses a decision `view/peripherals.go` documents, so it replaces that
comment rather than sitting beside it. What was right in it stays: a row per
device that appears and disappears with the hardware, not two fixed slots
configured through a submenu.

The cell is a new shape in `internal/view`, drawn by both shells:

- The window lays cells out in a wrapping grid inside the card.
- The pane lays them out as the arrangement says, the way it already lays out
  rows: stacked in a narrow terminal, in columns in a wide one.

A device with several cells inside it (a pair of earbuds) keeps its existing
behaviour: one device is one cell and the separate batteries are the quiet line
under it.

### 2. The cooler plots the processor as well as the coolant

`view.Section.Trail` is one series. The section needs two, named, each with its
own scale and its own colour:

- **coolant**, raw, in the section's own verdict colour, and primary.
- **processor**, as a trailing mean over sixty seconds, in a muted blue, and
  secondary.

Raw processor temperature is unplottable at this size: it spikes to a hundred
degrees on any compile and swings about thirty-five where the coolant moves
under one. The monitor averages twelve samples at five seconds for this reason
and says so. The two traces are scaled separately for the same reason, and the
consequence is worth being explicit about — **heights are not comparable
between series**, and the real numbers are in the rows above.

The design system's sparkline already holds several series
(`glance.Sparkline.AddSeries`), so this is a change to the view's shape and to
what each shell hands it, not a library change.

### 3. The cooler keeps what it knew

Two changes, and the second is the one a reader notices:

- `cooler.Cooling` narrows liquidctl with `--match`, defaulting to `kraken` and
  overridable from the environment, as the monitor does. Opening every HID
  device on the bus to read one coolant temperature is what the monitor's own
  comment calls out: this machine has a documented history of hidraw contention.
- A poll that fails keeps the previous reading and marks the section `Gone`,
  rather than replacing it with an empty one. `Section.Gone` exists for this and
  the cooler does not use it. The rows are drawn dim with their last values, so
  a blip does not remove a row and change the height of the panel.

A reading that was never taken is still absent: `Gone` is for a source that
answered and has stopped, not for hardware that was never there.

### 4. The bar is the window that bites today

`view.nearest` picks the window furthest along, which puts a different window on
each account's bar: seven days on one, the monthly spend on another, the
Business limit on Codex. None of the three is the one that stops work this
afternoon.

The bar is the **shortest** window an account has — `5h` for Claude, the primary
window for Codex — and the others keep their figures in the stats row, where
they already are. Where an account has no short window the longest is still
better than nothing, so the rule is "the shortest there is" rather than "5h or
nothing".

The window's name stays beside the account on the meter, so which window the bar
is about is never inferred.

### 5. The panel's font stays in the panel

With `hayami.fontSize` at 20 against `appearance.textSize` at 12, the
preferences window's navigation and body both draw large; at 8 against 12 they
both draw small. `shell.Options.OwnAppearance` is set and does not hold.

The preferences window draws at its Appearance screen's size whatever the panel
is set to. If the cause is in the design system, the fix goes there and this
spec records it under Gaps found rather than working around it in
`internal/prefs`.

`appearance.scale` is out of scope: it goes through `FYNE_SCALE`, which is
process-wide, and no per-window mechanism can scope it. The Window screen says
so.

## Acceptance criteria

- [ ] `view.Peripherals` produces cells — name, level, state — and no rows.
- [ ] A device with no level draws its name and its state, and does not draw a
      percentage it does not have.
- [ ] A device with several batteries draws them as the quiet line under its
      cell, as it does today.
- [ ] Both shells draw the peripherals section as cells, and the parity test
      still passes.
- [ ] `view.Section` carries named series rather than one `Trail`, and the
      cooler fills two of them.
- [ ] The processor's series is a trailing mean over sixty seconds; a partial
      window is averaged as it stands, so the trace starts on the first sample.
- [ ] Each series is scaled to its own range in both shells.
- [ ] `cooler.Cooling` passes `--match`, and the value comes from the
      environment when it is set.
- [ ] A cooler poll that fails keeps the previous reading and sets
      `Section.Gone`; the rows draw dim with their last values.
- [ ] A cooler that never answered draws nothing, rather than an empty section.
- [ ] `view.Usage` puts the shortest window on the bar, for Claude and for
      Codex, and the other windows keep their figures in the stats row.
- [ ] An account with one window puts that window on the bar.
- [ ] The preferences window draws at `appearance.textSize` with
      `hayami.fontSize` set to something else, verified from a screenshot of the
      real window at two sizes.
- [ ] The panel still draws at `hayami.fontSize`.
- [ ] `go test ./...` passes; `cmd/hayami-tui` still builds with `CGO_ENABLED=0`.

## Risks & Assumptions

- **`Section.Trail` is a breaking change inside the program.** It is an internal
  package with two consumers, both in this repository, and both are changed in
  the same commit.
- **Reversing the peripherals decision.** The row form was argued for in a doc
  comment and is being replaced, not softened. The argument that was right — a
  row per device rather than two fixed slots — is kept; the rest goes, and the
  comment says what changed and why so the next reader does not reverse it back.
- **`--match kraken` is this machine's cooler.** A machine with another cooler
  needs another value, which is why it is read from the environment. A match
  that finds nothing is `ErrNoCooler`, which is already a section that draws
  what the kernel gave it.
- **The font isolation may be a library bug.** If it is, the fix lands in
  fynedesygn and this branch takes a version bump. That is a dependency on
  another repository and is the one part of this spec that may not close in one
  sitting.
- **Rollback**: revert the commit. Nothing here writes outside the program's own
  settings file, and no setting changes meaning.

## Status: INCOMPLETE
