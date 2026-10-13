# 060 — a drawer of detail

**Issue**: #191

## Status: COMPLETE

Built and tested on Windows 11 and on Plasma 6.7.5, with every reader read
for real and every detail clicked on the real window on both desks. Four
faults found in fynedesygn on the way are fixed in its 0.1.98 to 0.1.100,
which `go.mod` names (Gaps found).

## Context

The panel says how much and not what is behind it. A rate jumps and the next
question is which program; a meter is nearly full and the question is what
the week went on. Both are answered today by opening something else.

The design system gains a drawer for this (fynedesygn #198, its spec 068): a
surface in the panel's own window, beside the cards and as tall as them,
opened by a click on a card or a row, with a filter typed from the keyboard
and the keyboard given back afterwards. This spec is hayami's half: what the
drawer shows, where that is read, and how it is asked for. It needs a
fynedesygn release that has spec 068; nothing here copies from the library.

hayami has none of the data today. `core` reads the processor's load, the
graphics card's and the interfaces' counters as totals; there is no process
reader, no memory reading, and the usage payload is decoded for three of its
keys. What each desk gives was measured on 2026-10-12 and is under
Verification.

## Requirements

### What is asked for and how

- R1 A detail belongs to a section and has an ID within it. A source that has
  details says so (`panel.Detailer`: the details it offers for its current
  reading, and a poll for one of them); a source without the interface has
  none and its card does nothing on a click.
- R2 What a detail says is data, as a section is: `view.Detail` has a title,
  a list of lines (a name, a fixed-width value, a status, an optional second
  value), a note, and reasons. Neither shell decides what it says.
- R3 A click on a card opens that section's detail; where a row has one of its
  own, a click on the row opens the row's (a new field, `view.Row.Opens`, names it by ID; `Row.Detail` is already the
  line of text under a reading).
  A click on the thing already shown closes the drawer; a click on another
  switches what is shown and leaves it open.
- R4 A detail is polled only while it is shown, and its poll stops when the
  drawer closes or the section is hidden. A closed drawer costs nothing. A
  list of processes is read every two seconds whichever section it belongs
  to (`panel.ProcessInterval`), and a usage breakdown at the usage section's
  interval. The draft said "at its section's interval", which for the cooler
  is five seconds: a list a person is watching for what changes reads as
  stopped at that pace.
- R5 The list is as long as the drawer's height fits, greatest first. Nothing
  scrolls. A line's value is formatted to a fixed width by the formatters in
  `view/format.go`, and a name is shortened to its column, so a new reading
  moves nothing.
- R6 A process is shown by its name and never by its command line, its path
  or its owner. Processes with one name are one line, summed.

### The lists

- R7 **Processor.** Processes by processor time since the last poll, as a
  percentage of the whole machine, on Linux from `/proc/<pid>/stat` and on
  Windows from `NtQuerySystemInformation(SystemProcessInformation)`, which
  needs no handle to the process.
- R8 **Memory, the reading.** The cooler gains a probe, `RoleMemory`: used and
  total, drawn as a row after the graphics card's. Linux: `MemTotal` less
  `MemAvailable` from `/proc/meminfo`. Windows: `GlobalMemoryStatusEx`. It is
  a provider in a chain in `core/host_tables.go` like the processor's
  temperature, and its absence is typed.
- R9 **Memory, the list.** Processes by resident memory: Linux `statm`,
  Windows the private working set from the same call as R7.
- R10 **Graphics.** Processes by use of the card the cooler row is for.
  Windows: the `GPU Engine` counters hayami already collects, which carry a
  process ID in each instance name that `ParseEngineInstance` discards today;
  the busiest engine per process, as the row's own figure is the busiest
  engine. Linux: DRM fdinfo (`drm-engine-*` in `/proc/<pid>/fdinfo`) for the
  drivers that publish it; for NVIDIA's proprietary driver `nvidia-smi pmon`,
  last in the chain and under the same two-second timeout as the row's own
  (architecture rule 9: still the one subprocess). A card whose own reading
  came from `nvidia-smi` is asked of `nvidia-smi pmon` alone: on a machine
  with a second card that publishes fdinfo, the kernel's answer would be a
  list of the wrong card (`core.Graphics.Via`).
- R11 **Bandwidth.** Processes by bytes per second, down and up. It ships
  partial, and says so:
  - Linux: `sock_diag` over netlink for TCP sockets (`tcp_info`'s
    `bytes_acked` and `bytes_received`), each matched to the process that
    holds it by the socket's inode in `/proc/<pid>/fd`. A socket whose process
    cannot be read is counted under "other". UDP has no counters and is not
    counted.
  - Windows: per-process network use exists only through ETW
    (`Microsoft-Windows-Kernel-Network`). Where the session cannot be started
    the detail is a typed absence that names what is needed. hayami does not
    ask for elevation and installs nothing.
  - The detail's note says what is not counted ("TCP only", and the share of
    the interfaces' total the list accounts for), so a list that is short of
    the row above it reads as partial and not as wrong.
- R12 The process readers are one reader in `core`, with `_linux`, `_windows`
  and `_other` files and a field in `core.Host`, read once per poll for
  whichever list is shown. `hayami-tui readings` prints it as JSON, as it
  prints every reader.

### The usage

- R13 A click on a provider's meter opens that provider's breakdown. Claude:
  the limits the payload lists (`limits`: session, the week, the week by
  scope, each with its percentage, severity and reset), the week by model
  (`seven_day_breakdown.rows`), and extra usage (`extra_usage`, `spend`).
  Codex: the plan, both windows, the individual limit with what remains, and
  credits where the payload has them.
- R14 The decoders stay tolerant: a key that is absent or null is a line left
  out, never an error, and the shared-cache test (the one that reads the real
  files and skips without them) covers the new keys. Nothing is added to what
  hayami writes to the cache, so the Python tools read it as before.
- R15 Quotas are meters in the drawer as they are on the card; the week by
  model is a list.

### The window

- R16 The drawer is fynedesygn's. The side is a setting, "where there is
  room", right or left, in the preferences' Window page; the default is where
  there is room.
- R17 The filter is fynedesygn's, on the lists (not on the usage breakdown).
  Its key is `f`.
- R18 The keyboard goes back when the drawer closes or the filter ends
  (fynedesygn spec 068 R20-R22). On Plasma that, the left side and the choice
  of side are KWin scripts, run by `desktop.Drawer` over a session bus
  connection of its own; hayami's rule does not set `NoFocus`, and must not
  while the filter exists. The window asks them through a seam (`gui.Desk`),
  and a panel built without one asks nothing: a test never reaches the desk.
- R19 The drawer's state is not kept across a restart: the panel opens closed.
- R20 In the row arrangement a click on a line opens its detail as in the
  others.

### The terminal

- R21 `hayami-tui detail <section>[/<id>]` prints a detail once, as text or
  with `--json`. The pane itself gains no detail view in this spec: it has no
  selection and no keys but quit, and what a detail looks like in a pane is
  its own decision (Out of scope). The parity test's allow-list names the
  difference.

## Acceptance Criteria

- [x] Each list, from fakes: order, the fixed widths at every magnitude, one
  line per name, and the count that fits a height.
- [x] The memory probe's row in all three arrangements in both shells, its
  absence, and a fixture in the parity test.
- [x] `ParseEngineInstance` keeps the process ID and the row's own figure is
  unchanged.
- [x] Bandwidth: the note says what is not counted; sockets with no readable
  process are "other"; on Windows without the session, the typed absence.
- [x] Usage: every key in R13 from an invented payload; each absent or null
  key leaves its line out; the real-cache test passes or skips.
- [x] A detail is not polled while the drawer is closed or its section is
  hidden.
- [x] No command line, path, user name or address reaches a `view.Detail`
  (a test over a process tree that holds all four beside each name).
- [x] On the real window on Windows: a click on the bandwidth card, on the
  processor's, the graphics card's and the memory's rows and on a meter each
  opens its detail at the cards' height; a second click closes it; the
  window's rectangle comes back; a value changing moves nothing.
- [x] The same on Plasma, with real clicks: the bandwidth card, the
  processor's, the card's and the memory's rows and three accounts' meters
  (Verification).
- [x] Each reader read for real once on each desk, with the reading under
  Verification; `hayami-tui readings` and `doctor` show them.
- [x] Each test falsified. `go test ./...`, the linter and `make vuln` pass;
  `cmd/hayami-tui` still builds with `CGO_ENABLED=0`.

## Out of scope

- A detail view in the terminal pane: a selection, keys and a layout for it.
  The parity test compares what a card says and has no entry for this: it is
  a difference in what each shell can be asked, stated in the test's comment
  and in `docs/architecture.md`.
- UDP and QUIC by process on Linux (eBPF is refused to unprivileged users on
  the Linux desk), and any helper that runs elevated on either system.
- Acting on a process (ending it, changing its priority).
- A history or a plot per process.
- Details for the peripherals section.

## Risks & Assumptions

- Depends on a fynedesygn release with spec 068. Until then nothing here can
  be built on the window.
- **Windows bandwidth needs an administrator**, measured: run at medium
  integrity, `StartTrace` is refused with access denied, and the drawer shows
  the typed absence. The installed panel runs as the person at the desk, so
  on most desks this list says "needs an administrator" and nothing else.
  Membership of Performance Log Users is what Windows documents as the other
  way in; it was not tried.
- The trace session is a kernel object that outlives a process that does not
  stop it. The panel stops it when the drawer closes, when it quits, and
  after twenty seconds with nobody reading; a session left by a panel that
  was killed is stopped by name by the next one to start. Two panels on one
  desk would take the session from each other.
- The Linux bandwidth list reads every TCP socket and walks `/proc/*/fd` each
  poll. On the Linux desk that is about 400 sockets and 1,450 processes, of
  which about 500 are readable. Its cost is to be profiled before it ships at
  the section's 2 s; the detail may poll slower than its section.
- A connection opened and closed between two polls is not counted, so a
  program making many short requests reads low.
- Per-process processor and memory on Linux for other users' processes
  (`stat` and `statm` are world-readable; `smaps_rollup` is not and is not
  used) was asked of the Linux desk and not reported back; to be confirmed.
- DRM fdinfo gives engine time, not a percentage; the list divides by the
  interval. A card whose driver publishes none has a typed absence.
- `nvidia-smi pmon` is a second use of the one permitted subprocess, only
  while the graphics list is open on an NVIDIA card with the proprietary
  driver.
- The usage payload's extra keys are the provider's and undocumented; they
  were read by name and type from one account's cache. Others may differ,
  which is what R14 is for.
- Process names are facts about a desk, like interface names (spec 027), and
  may appear in a screenshot; a name that identifies a person may not, and the
  gallery's shots are taken with a filter or a fixture where one would.
- Release safety: additive. A panel nobody clicks polls nothing new except the
  memory probe. Rollback is a revert; the memory row can be hidden by its
  section's settings if it has to be.
- This is several pull requests against one spec: the drawer wiring with the
  processor and memory lists and the memory row; the graphics list; the
  bandwidth list; the usage breakdown.

## Measurements while scoping

Taken on 2026-10-12, before anything was built.

Windows 11:

- `GPU Engine` counters: 351 instances, each named with a process ID; summed
  per process they rank the programs using the card.
- No performance counter for a process's network use (`Process` and
  `Process V2` carry I/O bytes of every kind, not the network's). The ETW
  provider `Microsoft-Windows-Kernel-Network` is registered.
- The usage cache's Claude payload holds `limits` (three: session, the week,
  the week by scope), `seven_day_breakdown.rows` (four: a key, a display name
  and a percentage each), `weekly_scoped_shares`, `extra_usage` and `spend`,
  beside the three keys hayami decodes.

Linux (CachyOS, kernel 6.18, an Intel and an NVIDIA card):

- DRM fdinfo: 17 processes publish `drm-engine-*` for i915; none for the
  NVIDIA card, where `nvidia-smi pmon -c 1` lists processes with their share.
- `sock_diag` (through `ss -tinp`): 416 TCP sockets, 413 with byte counters,
  400 with a readable process. No counters for UDP.
- `kernel.unprivileged_bpf_disabled = 2`.

## Gaps found

In fynedesygn, for its own issues and specs:

- **A glance window grew a pixel wider each time its drawer opened and
  closed**, at a display scale that is not a whole number. `Panel.Resize`
  compared the width the window has with the width it asked for exactly, and
  kept the fraction of a pixel between them as a width somebody chose.
  Measured here at 150 %: 254 px, then 255, 256, 257. Found by this spec's
  real-window test.
- **The window stood too tall from the click until the next reading** on a
  panel with a peripherals card. The drawer's code laid the cards out once at
  the width left over in a window not yet widened, the card's four cells
  wrapped to four rows, and the window was sized to that: 656 px for 515 of
  cards, for up to two seconds. Found on the installed panel, which has that
  card; this spec's test panels did not, and the cooler now shares the
  real-window tests with a usage card but still not with cells.
- Both are fixed in fynedesygn 0.1.98 (its issue #201), which `go.mod` here
  names.
- **A tip took the next click.** hayami's panel has a theme of its own, which
  wraps its content in a theme override; the search for the tip layer did not
  look inside one, so a card's hover tip was a popup, and a popup is an
  overlay that swallows a click to dismiss itself. The click after the
  pointer had rested a second was lost: on Plasma one in eight, each time
  with an overlay on top when it arrived. In that state, on the panel at the
  screen's corner, the cards kept the open window's width after the drawer
  closed and reflowed into two columns. Fixed in fynedesygn 0.1.99 (its
  issue #203).
- **A window put back to its open width was taken as a width the user
  chose.** On Wayland the compositor answers a change of focus with the size
  it last knew. The drawer closes and hands the keyboard back in the same
  moment, the two crossed, and the window returned to its open width; the
  panel read that as chosen, gave it to the cards, and every later opening
  added a drawer's width. It was a race, seen on the usage card and not the
  others. Fixed in fynedesygn 0.1.100; hayami also measures the panel again
  shortly after a close, so the window is wide for a moment and not until
  the next reading.
- A drawer cannot have its filter taken away: `Drawer.SetFilter(nil)` leaves
  the canvas's key handlers calling a nil filter. The breakdown has no
  filter, so its key is set to one nobody can type while it shows.
- A click leaves the pointer on the row clicked, and the row's tip appears
  over the card and the edge of the drawer a moment later. A tip that waits
  for the pointer to move again after a click would not.
- `glance.Row` has no getter for its label, and a meter has no `SetShown`:
  the drawer's content is replaced when the number of meters changes.

In hayami:

- **The panel's place was saved with the drawer out.** A drawer on the left
  moves the window's corner by its own width, and the position watcher saved
  that corner. Fixed here: a move is not remembered while the drawer is out.
- The position watcher and the drawer's scripts find the panel by its app ID
  and title, so a second panel on the desk is found too: a scratch panel run
  for these checks was reported as the desk's own and its place saved. Older
  than this spec; two panels on one desk is not a supported arrangement.
- In the drawer a name of thirteen characters touches the arrow of a network
  line's value at the Linux desk's text size.
- `view.Size` and `view.Rate` are six characters wide for a value from 1000
  to 1023 of a unit above bytes ("1000.0"), one more than the column. It is
  older than this spec and shows in a memory list.

## Verification

Windows 11, 2026-10-12:

- The unelevated run: `hayami-tui detail bandwidth` at medium integrity
  prints "needs an administrator" and the sentence that says why; `doctor`
  lists it under the bandwidth section.
- Elevated, the trace counts this process's own request (the real-desk test,
  `HAYAMI_DESK_TEST=1`), three runs of three passing. The first events arrive about
  two seconds after the session starts and within a quarter of a second
  after that. No session was left behind (`logman query -ets`).
- `hayami-tui detail` printed each list: seventy-six programs by processor
  and by memory, with "18.2 GiB of 39.9 GiB in use"; twenty-three by the
  card, a game at the top; four by network; and the usage breakdown (three
  limits, extra usage, the spend and the week by use).
- On the real window, with the clicks posted to the panel's own window so
  the pointer on the desk is not moved (`tests/window`, four tests): each of
  the three cooler rows, a usage meter and the bandwidth card opens a drawer
  about 300 px wide at the cards' height with a list in it, the window is
  the same rectangle after another reading arrives, a click on another row
  changes the list and leaves the rectangle alone, and a second click brings
  the closed rectangle back to the pixel. The last held only with the
  fynedesygn fix above: without it the window was a pixel wider each time.
- The installed panel, after an unattended install of this build
  (`cli-installer -cli -yes`, 0.9.10 replaced) and started as the desk's
  own user: four cards, 252 by 515 px, 5 px from the right edge of the
  screen. Ten openings by posted clicks, of the bandwidth card and of the
  processor's row: the drawer opens on the left, where the room is, the
  window is 501 by 515 from the first 50 ms to the last, and it closes to
  the rectangle it had. The bandwidth drawer says "needs an administrator",
  as an unelevated panel's must.
- Forty-eight breaks of one behaviour each: every one fails a test, after
  four tests were tightened (a reused process ID, a DRM client two processes
  hold, a socket with no process, a click while the compositor is asked) and
  one added (a row that arrives after the card was built).
- `go test ./...` passes but for `TestTheSlugAgreesWithThePythonItself`,
  which fails without this change too: the Python it runs needs a module
  this desk does not have.

Linux (CachyOS, kernel 6.18, an Intel and an NVIDIA card), in a scratch
clone over SSH, with fynedesygn 0.1.97:

- `make lint`, 0 issues; `make test` with the race detector, every package
  passes; `make vuln` finds nothing this program calls; `cmd/hayami-tui`
  builds with `CGO_ENABLED=0`.
- `hayami-tui detail` printed each list: programs by processor and by
  memory, of every user; the card's list through `nvidia-smi pmon`, with a
  game at 79 % and the compositor at 11 %, which is the NVIDIA card the row
  is about and not the Intel one fdinfo would have listed; and the network
  by TCP socket, eight programs.
- **The first run of the window's unit tests there reached the desk.** They
  built panels whose drawer asked the real compositor, and one test's drawer
  chose the left and left `hayami-hold-edge` loaded in the session of the
  person using the desk. It was unloaded by hand. The window now asks the
  compositor through `gui.Desk`, a panel built without one asks nothing, and
  a test holds that; after the second run no script was loaded.

Plasma 6.7.5 on Wayland, scale 1.5, grid arrangement, with real clicks and
keys (ydotool) and the geometry read from KWin:

- The desk's own panel, four cards, 304 by 523 at the right edge of a
  screen, side by room: a click on the bandwidth card opens the drawer on
  the left, 3690 to 3441 and 304 to 553 wide, the right edge and the height
  where they were, with `hayami-hold-edge` loaded; another reading moves
  nothing; `f` and a letter typed with real keys narrow the list to two
  programs (photographed) and do not resize the window; Escape gives the
  keyboard back to the window it came from.
- With fynedesygn 0.1.98 the close failed there: the click after the pointer
  had rested was taken by a tip's popup, and the window was left 553 wide
  with its cards in two columns, growing by a drawer's width at each later
  click. Restarting the panel put it right.
- With the tip fix: the same panel in the same place opened to 553 and
  closed to 304 at x=3690 twice running, every click arriving, and the
  keyboard went back to the window in front each time. Scratch panels with
  the desk's settings did the same with the side named left, named right,
  and by room at the screen's edge; the scripts were unloaded when each
  panel stopped.
- With fynedesygn 0.1.99 the usage card still failed, on the second close:
  the log has the canvas at 304 after the close and at 553 at the next
  reading, taken as a chosen width. With 0.1.100's fix the same clicks
  opened and closed it four times running, 304 to 553 and back at x=3690,
  the window put back to 553 once and asked for 303 again.
- Then every detail, on the desk's own panel: the processor's row, the
  card's and the memory's, twice each; the bandwidth card; and the meters of
  three accounts. Each opened to 553 on the left and closed to 304 at
  x=3690, 523 high throughout, and the panel was still running after.

