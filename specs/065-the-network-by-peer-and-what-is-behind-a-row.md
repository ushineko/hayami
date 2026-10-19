# 065 — the network by peer, and what is behind a row

**Issue**: #210

## Status: INCOMPLETE

Partial, and accepted so far at the desk. The breakdown, the drill-downs,
the pause and the kill are built and used on Linux (Plasma 6, Wayland), and
the suite is green (18 packages). What is outstanding is tests of this
spec's own behaviour -- the existing tests were repaired, not extended --
and nothing run on Windows. The pattern has since been taken to the
cooler's lists (spec 066), which moved the shared parts out of here.

## Executive Summary

The bandwidth drawer listed processes and said what share of the interfaces'
traffic it accounted for. It could not say who a process was talking to,
which interface it used, or what the rest of the traffic was. This adds a
breakdown by interface, process and peer; drill-downs into the processes
behind a name, one process, and one peer; a pause; and a kill. Reviewers
should look first at `socketNet.Read`, which now groups by interface and
keeps the addresses it used to discard, and at `gui/drill.go`, which holds
everything the sections do not know about.

## Context

Spec 060 R11 gave the drawer the network by process, and said honestly that
it was partial: "TCP only", with the share of the interfaces' traffic the
list came to. Used at the desk, the question that follows a large number is
always the same one — *who is that?* — and the list could not answer it.

The traffic that prompted this was 5.45 GB through a local proxy to a phone
over Tailscale. The process list showed one row, `fpsd`, and the panel had
no way to show that the two ends of that flow were an Apple CDN and a
handset.

The kernel keeps the addresses in the same reply as the counters. The reader
read them for one thing — whether the connection was loopback — and threw
them away.

This needs a fynedesygn release with its spec 072 (a drawer that is worked
and not only read). Nothing here copies from the library.

## Requirements

### What is drawn

- R29 `view.DetailNameWidth` is 20, was 13. A peer is a hostname and a
  client's name, and at 13 the column read `162-254-195-...`, which is the
  whole value of the peer row gone. This needs the wider drawer that
  fynedesygn spec 072 R19 gives.

### What is read

- R1 `SocketCount` keeps `Local`, `Peer` and both ports. They are already in
  the reply the counters come in; they were parsed for the loopback test and
  discarded.
- R2 A socket is put to an interface by its local address
  (`core.localIfaces`). The kernel's socket ID carries an interface index
  only for a bound socket and most are not bound.
- R3 `NetProcess` gains `Iface` and `Peers`; a process talking over two
  interfaces is two entries, because the interface is what the breakdown
  groups by. `NetPeer` gains the sockets behind it.
- R4 A socket whose near end is a listening port is one this machine
  accepted, and its far end is a client of ours and not a destination we
  chose. The two read differently and the list says which (`Served`).
- R5 A reader built without an interface table counts every socket under no
  interface rather than refusing to count it.

### The breakdown

- R6 The bandwidth section offers a second detail, `peers`: one part per
  watched interface, each the interface's own rate, the processes that used
  it with their peers under them, and the traffic the list does not account
  for as its last line.
- R7 It is built of `Parts` and not `Lines`. The `Lines` path cuts to the
  drawer's height and never scrolls, and the cut is a prefix — so the
  remainder, which is last, would be the first row dropped, and a list whose
  purpose is to say it is incomplete would go quiet about exactly that.
- R8 Processes are summed by name within an interface (spec 060 R6): a
  browser is forty processes and a person asks about the browser.
- R9 A row moving nothing is not drawn. The unmeasured remainder and the
  interface total are always drawn, at nought if that is what they are.
- R10 A peer is named by its PTR record, asked for out of the way of the
  reading: a miss answers with the address and asks in the background, and
  the name arrives in a later reading. An address with no name is remembered
  as having none and asked about once.
- R11 A name of this network — the tailnet, the LAN — is cut to its first
  label; anything else keeps its last two. Repeating a suffix down a column
  says nothing.

### Walking into a row

- R12 A drill-down is a detail whose ID carries the path to it:
  `peers/<iface>/<name>`, `peers/<iface>/<name>/<pid>`,
  `peers/<iface>/<name>/@<addr>`. The path is in the ID and nowhere else, so
  a drill is polled, closed and restored exactly as any detail is, and no
  shell needs a notion of where a person has navigated to.
- R13 `panel.Driller` says whether an ID is one of a section's drills. A
  drill is not in `Details()`: a list of every process a person could open
  would be a list of every process.
- R14 A detail line says what a click on it asks for (`view.DetailLine.Open`):
  a detail ID, or a named action. A line does one thing when it is clicked.
- R15 The name level lists the processes behind that name with their own
  rates, and the peers the name talked to.
- R16 The process level is that process: its rate now, its PID, parent,
  state, threads, sockets and children.
- R17 The peer level is the connections to that peer: how many there are,
  how many are moving, and a row per moving one.
- R18 The heading of a drill is the heading of the detail it was reached
  through. Without that the drawer is headed with the path itself, which is
  plumbing.
- R19 The way back is a button in the heading's left corner, present only
  when there is somewhere to go.

### Stopping

- R20 A button in the heading stops and starts the reading of the open
  detail. The cards behind the drawer are not touched: pause is a property
  of what is being read, which is what lets it serve every section.
- R21 The first two readings are taken whether or not the reading is
  stopped. Two and not one because a rate is a difference: opening a drill
  builds a reader with no history whose first answer is empty, and a page
  stopped on that reading would report a busy process as idle.
- R22 The pause is drawn as an engaged toggle while it is stopped.
- R23 Rows light under the pointer whether or not the reading is stopped,
  because they answer to a click either way.

### Ending a process

- R24 A kill is offered on the page about one process and nowhere else, and
  only while there is a process there to end. The section says whether it is
  available by carrying the line; the drawer decides how it is offered.
- R25 It is two presses: the button becomes a question and a cancel appears
  beside it. They share a pinned width with the button they replace.
- R26 A kill ends the process and everything it started: SIGTERM to the
  tree, children before parents, then SIGKILL after a grace to whatever is
  still there. Ending the parent alone leaves its children running,
  reparented to init and still holding their sockets, which to the person
  who pressed the button looks as though nothing happened.
- R30 Ending a process is platform code (`kill_unix.go`, `kill_windows.go`).
  Windows has no signals -- no TERM to ask with, no process group to ask
  across. There the ask is `WM_CLOSE` to the process's visible top-level
  windows, which is what closing them does and what `taskkill` without `/F`
  sends, and the force is `TerminateProcess`. A process with no window has
  nothing to ask through: it is neither sent to nor refused, and a tree in
  which nothing could be asked is forced at once rather than after a grace
  spent waiting for an answer that cannot come. The page names the two steps
  in the platform's words (`core.EndAsk`, `core.EndForce`): "TERM, then
  KILL" on Linux, "close, then end" on Windows.
- R31 The page about a process is read from the platform's own table
  (`core.ProcessInfo`, `ProcessChildren`, `ProcessTree`). On Windows that is
  the process table `NtQuerySystemInformation` gives, which the lists
  already read, and iphlpapi's owner tables for the sockets. Neither needs a
  handle to the process or an administrator, so there is no helper, unlike
  the network by process (spec 061). The state is the threads': running if
  any runs or is ready to, suspended if all are suspended, otherwise
  waiting. A parent is only a parent if it started before the child,
  because Windows never updates the ID it records and a later process may
  hold it. The sockets are TCP and UDP only: Windows keeps no table of a
  process's Unix sockets that this user can read.
- R32 The process a page is about is the last step of its ID
  (`panel.DrillPID`), whichever section the page is in. The kill once read
  it with the bandwidth section's own parser, which a cooler page's ID does
  not satisfy, so Kill on a cooler page ended nothing.
- R27 The outcome is said on the page with counts, because part of a tree
  can be refused: anything this user did not start answers `EPERM`, and
  `ESRCH` is success and not failure.
- R28 A process that has gone quiet is not a process that has gone. An idle
  entry is drawn as it was, at nought, and says it is idle; only a process
  that is no longer there is reported as gone.

## Acceptance Criteria

- [ ] `tcpSockets` returns addresses and ports; loopback is still excluded.
- [ ] A process with sockets on two interfaces appears under both, split.
- [ ] A socket on a listening port is marked `Served`.
- [ ] The `peers` detail has one part per present interface with a rate.
- [ ] Each part's unmeasured row is the interface total less the attributed,
      never negative, and is drawn even at nought.
- [ ] Two processes of one name on one interface are one row.
- [ ] A peer with no PTR record keeps its address and is asked about once.
- [ ] A tailnet and a LAN name are cut to the first label; others keep two.
- [ ] `Drills` accepts the three drill forms and rejects anything else.
- [ ] Each drill level renders the content R15–R17 describe.
- [ ] A drill opened while paused renders — not blank, and not "idle" for a
      process that is moving.
- [ ] A drill's heading is "Bandwidth by peer", not its path.
- [ ] Back is absent at the top of the list and present on every drill.
- [ ] Pausing stops the reading of the detail and not the cards.
- [ ] The kill button is on the process page only, and not on a gone one.
- [ ] A kill ends a parent and its child; the page reports the count.
- [ ] A kill refused reports the refusal rather than claiming success.
- [x] `internal/core` tests pass with `NetProcess` carrying `Iface` and
      `Peers`: `byPID` drops the breakdown, so a test of the rate
      arithmetic stays about the rate arithmetic.
- [x] `internal/view` and `internal/cli` text expectations match
      `DetailNameWidth` 20.
- [x] `hayami-tui detail` polls the section twice, so a detail that is a
      share of a reading has something to be a share of.
- [ ] Tests of this spec's own behaviour: the breakdown's shape, the
      drill levels, idle versus gone, and the kill's tree.
- [ ] `hayami-tui detail bandwidth/peers/...` renders every level.

## Out of scope

- UDP. The kernel keeps no byte counters for a UDP socket and no privilege
  changes that; it is measured only by eBPF or by systemd's per-unit IP
  accounting, and both are their own spec. The list still says "TCP only".
- A kill on the name level, which is what would actually close a browser:
  its processes are usually siblings under a launcher and not descendants
  of the PID drilled into. Wanted, not yet asked for.
- SNI. A PTR record names infrastructure, not a service: the CDN this began
  with answers `uslax1-vip-fx-106.b.aaplimg.com`, never `updates.cdn-apple.com`.
- The cooler's lists, which spec 066 takes this to. The parts the two share
  moved into `panel/drill.go` and `view.DrillLine` when it did; what could
  not be shared is how deep a path goes, which is now `Driller.DrillUp`.

## Risks & Assumptions

- The remainder is honest about TCP but double-counts overlays: Tailscale's
  transport is UDP on the physical interface, so `tailscale0`'s traffic
  appears again inside `eno2`'s unmeasured. A reader comparing the two would
  count their own traffic twice. Not yet said anywhere in the panel.
- `ProcDescendants` reads the whole process table on every kill. That is one
  scan at the moment a person presses a button, not per reading.
- Killing by tree is a destructive act behind two presses and no undo. It is
  deliberately TERM-then-KILL rather than KILL, and deliberately counts
  rather than claims.
- The PTR resolver holds a cache that never expires. A long-running panel on
  a DHCP network will name a peer by whoever had the address first.
- `ProcSocketOwners` cannot read another user's fds, so root-owned sockets
  are counted under "other": 6 of 206 established sockets at one reading.
- The drill path is a string with `/` and `@` in it. A process named with
  either would parse wrongly. Not seen; not guarded.
- `DetailNameWidth` 13 -> 20 widens the name column of every detail in
  every section, not only this one. The cooler's and usage's lists gain
  room their names do not need; no list loses any.

## Gaps found

1. **Pause stopped the first read.** Drilling while paused never read the
   new page at all: the empty skeleton was drawn and the heading fell back
   to the raw path. Fixed by R21.
2. **One read is never enough.** Navigation builds a reader with no history,
   whose first answer is empty, so even after fixing the above a busy
   process read as idle. Fixed by R21 taking two.
3. **Idle and gone were the same thing.** `drillDetail` bailed whenever no
   process matched, so going quiet looked exactly like exiting. Fixed by R28.
4. **The pause did nothing at all.** The flag was a plain `bool` written on
   the UI thread and read from the poll goroutine through `p.do`, which
   queues rather than runs: the poll read it before it was ever set. It is
   `atomic.Bool`.
5. **Decorations compounded.** `renderDrawer` wrote the decorated detail
   back, so a drawer redrawn between readings grew a pause row each time.
   The reading is now kept as read and decorated on the way to being drawn.
6. **A kill killed one process.** Observed against a browser, reproduced
   against a throwaway: parent ended, child survived and was reparented.
   Fixed by R26.
7. **Two tests encoded the bugs.** `TestDetailPrintsTheSecondReadingAsText`
   asserted the section is polled exactly once, which is the thing that made
   the share unreachable; it now asserts two, with the reason written down.
   The `internal/core` rate tests compared whole `NetProcess` values, so
   every new field in the breakdown broke four tests that are not about the
   breakdown.
8. **The kill did not compile on Windows.** `syscall.Kill` is not there, and
   the code was written without a build tag because the desk it was written
   at is a Linux one. Caught by CI, not by review.
9. **`hayami-tui detail` could never show the share.** It polls the section
   once and the detail twice, so every interface reading has `HasRate` false
   and `netNote` returns the bare count. The share is unreachable from the
   command line by construction. Fixed by polling the section again after
   the settle.

10. **Kill on a cooler page did nothing.** The kill found its process with
    the bandwidth section's parser, and `cpu/firefox/4411` is not a
    bandwidth ID, so it returned before ending anything, on every platform.
    Found reading the code on the Windows desk. Fixed by R32.

## Verification

Driven at the Linux desk against live traffic, including a Steam download at
~60 MiB/s and ~180 concurrent connections generated on purpose to make the
list long enough to scroll.

- The breakdown, both ends of one flow visible: `fpsd` with
  `client: iphone182 ↑784 KiB/s` beside `uslax1-vip-fx-106.b.aaplimg.com
  ↓784 KiB/s`.
- Unmeasured remainder observed between 9 % and 51 % of an interface
  depending on what was running — the number that justifies or refuses the
  UDP work later.
- Drill levels entered and left; scroll position held across rebuilds.
- Kill: parent `1908194` ended, child `1908202` survived (before R26);
  parent `1922961` and its child both ended, page reported "gone (2 ended)"
  (after).

On the Windows desk (Windows 11, 100 % scale): the cooler's processor list
drilled to `explorer` showed its state, threads and eight children, an
empty parent (its starter had exited), and "kill: close, then end"
(`hayami-tui detail`). Tests in
`internal/core` and `internal/gui` drive the real process table and a real
kill: a child of the test is on its page with this test as its parent; a
windowless process is not asked and is ended by the force; a WinForms
window is asked and closes; System is refused and stays. The socket count,
the parent check and the window ask were each falsified by breaking the
code under them and watching the test fail.

Not verified: macOS, a second text size, and the Windows window itself.
`tests/window/kill_windows_test.go` drives Kill from a cooler page on the
real window and checks the tree is gone; when it was written the desk's
captures came back black for every window test, so it has not passed yet.

## E2E Test Plan

### At the Linux desk

1. Open the bandwidth card, choose "Peers". Expect one part per watched
   interface, each with a total, processes, peers and an unmeasured row.
   Covers R6–R9.
2. Start a large download. Expect its process to appear with its peers
   named, and the remainder to stay plausible. Covers R10, R11.
3. Press Pause. Expect figures to stop and the button to draw engaged, while
   the cards behind keep moving. Covers R20, R22.
4. Click a process row, then a PID, then Back twice. Expect each level to
   render, the heading to stay "Bandwidth by peer", and Back to be absent at
   the top. Covers R12–R19.
5. With a throwaway process that has a child and network traffic: drill to
   its PID, press Kill, then Confirm. Expect both processes gone and the
   page to report the count. Covers R24–R27.
6. Let a drilled process fall idle. Expect the page to stay, at nought,
   saying idle; kill it from a shell and expect it to say gone. Covers R28.

### Prod visibility

Not applicable: this is a desktop panel, not a service.
