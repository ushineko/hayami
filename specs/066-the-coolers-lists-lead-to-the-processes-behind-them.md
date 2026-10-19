# 066 — the cooler's lists lead to the processes behind them

**Issue**: #211

## Status: INCOMPLETE

Partial. Built and used at the Linux desk (Plasma 6, Wayland); the suite is
green. What is outstanding is tests of this spec's own behaviour, a run on
Windows, and the shared drill helpers' own tests.

## Executive Summary

Spec 065 gave the bandwidth breakdown a way into what is behind a row. The
cooler's three lists ask the same question and could not answer it: a list
is of names, and a name that is forty processes says nothing about which of
the forty. This takes that navigation across — one level shallower, since
these lists have no interface above a process and no peer beside it — and
moves what the two have in common into one place. Reviewers should look at
`internal/panel/drill.go`, which is the shared part, and at `Driller.DrillUp`,
which is the part that could not be shared.

## Context

Taking the pattern across was the point of building it on one section first.
Three things were worth sharing and one was not.

What is shared: the trail, the page about a single process, and the
arithmetic of a drill's ID. The process page especially — what a process is,
who started it, what it holds, and the offer to end it — is the same page
whichever list it was reached from, and a second copy of it would have been
a second place for it to drift.

What is not: how deep a path goes. The bandwidth breakdown has an interface
above the name and a peer beside the process; the cooler's lists have
neither. A shell that counted the parts of an ID would be a shell deciding
something the section knows, so the section says where its way out leads.

## Requirements

### Shared

- R1 `panel.Trail` is a drill's first line: the crumbs, headed by the list
  it was reached through. The way out is a button, not a row (spec 065 R19).
- R2 `panel.ProcessLines` is the page about one process: pid, parent, state,
  threads, sockets, children, and the kill. The same lines whichever list
  reached it.
- R3 A process that is no longer there carries no lines at all, and so
  offers no kill: `ProcessLines` returns nothing for it.
- R4 `panel.DrillRoot`, `DrillUnder` and `DrillOut` are the arithmetic of a
  drill's ID.
- R5 `view.DrillLine` draws a figure in the units of the list it was reached
  through, from the formatter that list already has (`processKinds`). A
  drill that formatted its own figures would be a second place for the units
  to be decided.
- R6 `panel.Driller` gains `DrillUp(id) string`: the section says where its
  way out leads. Nothing counts the parts of an ID it did not build.

### The cooler

- R7 Each of the three lists leads to the processes behind a name:
  `cpu/<name>`, `gpu/<name>`, `memory/<name>`, and a process under that.
  Two levels, not three.
- R8 Every row of a list carries what it leads to, except a heading and the
  row for what could not be attributed.
- R9 The name level is the processes of that name with their own figures,
  greatest first, and their total.
- R10 The process level is that process's figure now, then `ProcessLines`.
- R11 Each level reads the same source its list reads, and keeps the PID the
  list drops (spec 060 R6): a list is of names because a person asks about
  the browser, and a drill is the other question.
- R12 A name whose processes have all gone quiet says it is idle and stays
  where it is (spec 065 R28).
- R13 A drill keeps the switch of the list it was reached through, selected
  on that list, so a person inside one can see which of the three they are
  in and leave for another without coming back out first.
- R15 A drill is one part and not a list of lines (`panel.AsPage`): its trail
  becomes the part's heading and everything under it the part's lines. The
  lines path cuts to the drawer's height and the cut is a prefix, so a
  process page beside a short card kept "now" and "pid" and dropped parent,
  state, threads, sockets and children -- the answers the page was opened
  for -- and said nothing about dropping them. A page of parts is drawn
  whole and scrolls.
- R16 A drill's lines are read through `panel.DrillLines` wherever it keeps
  them, so moving them into a part does not take the kill button away.
- R17 Glyphs in a label are asked of the face (`fdtheme.FirstDrawable`,
  fynedesygn spec 073), each time the controls are drawn, because the font
  can change while the panel runs. A section's own text has no theme to ask
  and so uses ASCII: a layer that cannot ask the face does not guess.
- R14 Pause, hover, back and kill behave as spec 065 describes. Nothing in
  the shell is about bandwidth or about the cooler.

## Acceptance Criteria

- [x] `cooler/cpu/<name>` lists the PIDs behind a name with their shares.
- [x] `cooler/memory/<name>` lists them in bytes, `cooler/gpu/<name>` in
      per cent: the units follow the list.
- [x] `cooler/cpu/<name>/<pid>` is that process's share now, then its page.
- [x] The switch stays on a drill, selected on the list it came from.
- [x] Back, Pause and the hover work on the cooler's drills unchanged.
- [ ] A name with no live processes reads "idle" (built, not yet exercised).
- [ ] Tests: the drill levels, the shared helpers, and `DrillUp` per section.
- [x] A kill from a cooler drill ends the tree. It could not at first: the
      kill read the page's ID with the bandwidth section's parser and
      returned (spec 065 gap 10, fixed by R32). Pressed on a real window by
      `TestAKillFromTheCoolerEndsTheTree` (`tests/window`): CPU list, the
      process, Kill, Confirm; the process and its child are gone and the
      page says "end sent to 2". Passed on the Windows desk (Windows 11,
      2560x1440 at 100 %, Smart App Control off for the run); fails with the
      parser put back.

## Out of scope

- A peer level. The cooler's lists have no notion of one.
- Sorting or filtering within a drill. The list above has a filter; a drill
  is short enough to read.
- The peripherals and usage sections. Neither has a list of processes.

## Risks & Assumptions

- A drill's path is a string with `/` in it, and the cooler's steps are
  process names. A process named with a slash would parse as another level.
  Not seen; not guarded. The bandwidth breakdown has the same hole.
- `coolerPIDs` reads the whole process table for each drill reading, as the
  list above it does. It is the same read, not a second one, but it is not
  shared between the list and the drill either.
- GPU processes come from whichever provider the card's reading came from,
  and a drill asks again rather than keeping the list's answer. A card that
  changed provider between the two would answer differently.

## Gaps found

1. **A button was laid out at nought.** Found on the cooler and not on the
   bandwidth breakdown, though the fault was in neither: a button is built
   hidden and shown when its spec arrives, and the row holding it was
   measured in between. A container measures what is visible, so the row
   kept the width it had while its buttons were hidden, and they were
   there, correct and invisible. Fixed in fynedesygn (`headingChanged`),
   and it is why spec 072 is the library's and not this one's.
2. **The switch vanished on a drill.** `choices` returns nothing when the
   open detail is not among those offered, and a drill never is. The
   drawer now asks for the choices of the detail the drill was reached
   through. Without R13 a person drilling into a CPU row lost the way to
   the GPU and memory lists until they came back out.

3. **Two glyphs are missing on Windows.** Back's U+25C2 and the trail's
   U+25B8 draw as missing-glyph boxes in the face the panel gets on Windows
   (`�Back`, `CPU by process �window.test`), seen in the window test's
   pictures. Not fixed here: which glyph every platform's face has is a
   choice for the design system.
4. **A process page in a short panel shows two of its lines.** The drawer
   is the cards' height and its `Lines` are cut as a prefix (spec 065 R7),
   so beside the cooler alone the page is "now" and "pid"; the parent,
   state, threads, sockets and children are cut. Seen on Windows with one
   section; the Linux desk's panel is taller.

## Gaps closed

Gaps 3 and 4, both recorded from the Windows desk and both introduced by
spec 065 rather than by this one:

3. **The glyphs drew as boxes on Windows.** The cause was worse than the
   report: neither Fyne's bundled face nor Adwaita Sans contains U+25C2, so
   the Linux desk that "proved" it was substituting another font at draw
   time. A character checked by looking at one screen was never checked at
   all. Fixed by asking the face (R17, fynedesygn spec 073).
4. **A process page beside the cooler kept only "now" and "pid".** The
   lines path cuts to the drawer's height from the end. Fixed by making a
   drill one part (R15), with `TestADrillIsOnePartSoNothingIsCutFromIt`,
   which fails when the page is built the old way.

## Verification

Driven at the Linux desk, and read from the command line at each level:

- `cooler/cpu/claude` — six PIDs behind one name, `pid 25081 0.1 %`.
- `cooler/cpu/claude/25081` — parent `zsh (22671)`, 43 threads, 13 sockets,
  children `gopls`, `zsh`, `uv`.
- `cooler/memory/vivaldi-bin` — `all 3.2 GiB`, `pid 32353 499.5 MiB`.
- In the panel: Back, the CPU/GPU/Mem switch, the breadcrumb and Resume all
  drawn together on a cooler drill.

On Windows (spec 065 R31): `hayami-tui detail cooler/cpu/explorer/<pid>`
renders the page from the Windows process table.

Not verified: the idle path, macOS.

## E2E Test Plan

### At the Linux desk

1. Open the cooler, choose CPU, click a row with several processes behind
   it. Expect its PIDs with their shares. Covers R7–R9, R11.
2. Click a PID. Expect its page, with parent and children. Covers R10, R2.
3. Press Back twice. Expect the list, then the card. Covers R1, R6.
4. From a drill, choose Mem in the switch. Expect the memory list. R13.
5. Drill to a process and let it exit. Expect the page to say gone and to
   offer no kill. Covers R3, R12.

### Prod visibility

Not applicable: this is a desktop panel, not a service.
