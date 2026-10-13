# 061 — a network helper

**Issue**: #194

## Status: COMPLETE

## Context

Spec 060 gave the bandwidth card a drawer that lists programs by network
rate. On Windows that list is read from the kernel's network provider through
event tracing, and Windows refused it to the panel: the drawer said "needs an
administrator" on every desk where the panel was not started elevated, which
is every desk.

Running the panel as an administrator is the wrong fix. The panel holds the
usage tokens, fetches over the network and draws a window with a toolkit; an
elevated process should do none of that. It also cannot start at login
without a prompt, and an elevated window takes no input from programs that
are not.

Spec 060 and the README said membership of Performance Log Users was the
other way in. It is not (see Measurements), so nothing a person can do to
their own account gives the list.

This spec adds a helper: the smallest program that may read the provider,
serving the panel only what the list needs.

## Requirements

### The helper

- R1 **A Windows service, `hayami-network`.** Its account is Local Service,
  the least of the three the provider's access list names. It is registered
  to start on demand and is never set to start with Windows.
- R2 **Its program is a copy.** `hayami-tui.exe` is copied to
  `%ProgramFiles%\hayami\hayami-network.exe` and the service runs the copy
  with `network-helper run --allow <SID>`. The install directory is the
  person's own and they can write it; a service whose program its user can
  replace is a way to become the service. The copy is under Program Files'
  own access list.
- R3 **It runs only while it is read.** A panel that opens the list starts
  the service. The trace starts when the first reader connects and stops when
  the last leaves. With no reader for fifteen seconds the service stops
  itself.
- R4 **One way, and to one account.** It serves a named pipe,
  `\\.\pipe\hayami-network`, opened for writing only: the helper reads
  nothing a panel sends. The pipe's access list names SYSTEM, Administrators,
  Local Service and the one account the helper was installed for. Readers on
  another machine are refused. The service's access list has one entry more
  than the usual: that account may start it and ask its state.
- R5 **What crosses the pipe** is lines of JSON: first who is speaking (the
  protocol's version, hayami's version, what the numbers cover), then about
  once a second the bytes each process ID received and sent since the line
  before. No names, no addresses, no ports. The panel finds names itself.
- R6 A reader that stops reading is dropped when its buffer fills and holds
  no other reader up. A trace that cannot start is said to the reader in the
  helper's words and the service ends.

### The panel

- R7 `core`'s network reader on Windows is a chain: the helper first, then
  this process's own trace. Only the helper not being installed sends a
  reading on. Any other answer from it, a reading or a failure, is the
  answer, so two programs never start a trace session of the same name.
- R8 The helper is looked for on every reading, so one installed while the
  panel runs is found without a restart.
- R9 Without a helper, and not elevated, the list is the typed absence
  `net-processes-denied`: "needs hayami's helper", with what to run. A helper
  installed for another account is the same absence, "helper is another's".
- R10 A helper that speaks another version of the protocol is refused with
  what to do, and none of its numbers are shown.

### Installing and removing

- R11 `hayami-tui network-helper` and `network-helper status` say whether the
  helper is installed, which version, and whether it runs. They change
  nothing and need no administrator.
- R12 `hayami-tui network-helper install [--allow SID]` copies the program,
  registers or updates the service and sets its access list. Run without
  elevation it asks Windows for an administrator and passes the caller's own
  SID, because consent may be given by another account. It is safe to run
  again.
- R13 `hayami-tui network-helper remove` stops the service, unregisters it
  and removes its program.
- R14 **The installer's Network page**, after Sensors: what the check found,
  and where there is something to offer one check box, clear, with what
  ticking it does and what a person agrees to. A helper of this version is
  not offered again; one of another version is offered as an update and said
  to go on working. On the command line it is the same text and one question;
  `--network=yes` answers it and `--yes` alone leaves the default, `no`.
- R15 After the install, when `network` is `yes`, the hook runs the installed
  `hayami-tui network-helper install` as an administrator. A refusal or a
  failure is a warning and never fails the install.
- R16 Before an uninstall the hook removes the helper, through the installed
  `hayami-tui` while it is still there. Not on an upgrade, whose first half
  is an uninstall. A removal that fails is a warning that says how to do it
  by hand.
- R17 `install_windows.ps1 -WithNetwork` runs the same command for a checkout
  install, and `-DryRun` says what it would do. `uninstall_windows.ps1`
  removes the helper before it removes the programs.

## Acceptance Criteria

- [x] On a desk with the helper installed, a panel that is not elevated lists
      programs by network rate, and the list says "TCP and UDP".
- [x] The service's account is Local Service and its start type is demand.
- [x] The service is stopped before the list is opened, running while it is
      open, and stopped again within thirty seconds of its closing, with no
      trace session left.
- [x] Without the helper the list says "needs hayami's helper".
- [x] A test reads a real pipe served by the server and finds this process's
      rate by name, with no service and no administrator.
- [x] A test shows nothing is traced with nobody reading, the trace is let go
      when the last reader leaves, and the server ends by itself.
- [x] A test shows the helper is asked before the process's own trace, and a
      helper's failure is not followed by a second session.
- [x] The installer's page offers a clear check box when the helper is
      missing and nothing when it is current, in the window and on the
      command line.
- [x] An uninstall removes the helper; an upgrade leaves it.
- [x] README, `docs/architecture.md` rule 9 and its Status table say what the
      helper is and how it is installed.

## Out of scope

- **A helper on Linux.** The list there is TCP only because UDP has no
  per-socket counters without a privilege; a helper could give it one. That
  is a different mechanism (eBPF or a netlink capability) and its own spec.
- **More than one account on a computer.** The helper serves the account it
  was last installed for.
- **Other readings.** The processor's temperature stays LibreHardwareMonitor's
  (spec 036). The helper reads the network and nothing else.

## Risks & Assumptions

- **The provider's access list is Microsoft's and could change.** It names
  Local Service today. If an update removed that entry the trace would be
  refused and the list would say so in the helper's words (R6); the fix would
  be the service's account, in one constant.
- **A program that takes the pipe's name first** could feed a panel invented
  numbers. The helper refuses to start when the name is taken
  (`FILE_FLAG_FIRST_PIPE_INSTANCE`), so it cannot be shadowed while running,
  but a panel that finds a pipe does not check who serves it. What is at
  stake is a list of rates that is wrong; the panel parses numbers and
  nothing else from it. Accepted.
- **Process IDs and byte counts of every account's programs** reach the one
  account the helper serves. Task Manager shows an administrator the same;
  a person installing the helper is agreeing to it for their own account.
- **The copy goes stale on an upgrade.** It keeps working while the protocol
  is the same, and the page offers the update. A change of protocol is said
  by name (R10).
- **Two prompts.** Choosing both Sensors and Network asks for an
  administrator twice: the two steps are separate programs.
- **Uninstalling headless** (`--yes`) still shows Windows' prompt when the
  helper is installed. Refusing it leaves the helper and a warning.

## Measurements while scoping

Windows 11, build 26200.

- The provider, Microsoft-Windows-Kernel-Network
  (`7DD42A49-5329-4832-8DFD-43D979153A88`), has its own entry under
  `HKLM\SYSTEM\CurrentControlSet\Control\WMI\Security`:
  `D:(A;;0x120fff;;;SY)(A;;LO;;;LS)(A;;0x1207ff;;;BA)`. The default entry,
  which other providers fall back to, also names Performance Log Users.
- A process with a filtered, medium-integrity token, from a logon made after
  the account joined Performance Log Users: `StartTrace` succeeds and
  `EnableTraceEx2` returns access denied. Before joining, `StartTrace` itself
  is refused. The fresh logon's group list could not be printed (`whoami`
  fails in that kind of logon); the evidence that the group was in effect is
  that the refusal moved.
- A service running as Local Service starts the session, enables the provider
  and receives events. No change to any access list was needed.
- A process at medium integrity started the service and read its pipe with
  the access lists in R4.
- From the service being stopped to the pipe being open: under half a second.

## Gaps found

- `whoami /groups` and a console program started with `CREATE_NO_WINDOW` both
  fail in a batch logon from Task Scheduler with a filtered token
  (`0xC0000142`); the measurement used a detached process writing to a file.
  Nothing in hayami depends on it.
- fynstall has no way to say one elevated step serves two pages, so two
  offers are two prompts. Not raised with fynstall: two is tolerable.

## Verification

Windows 11, build 26200, on the desk this was written at.

- `go test ./...` passes, apart from `TestTheSlugAgreesWithThePythonItself`,
  which predates this and needs a Python module the desk lacks.
- Falsified by breaking what each holds: the chain's order
  (`TestTheHelperIsAskedBeforeThisProcessTraces`), the server ending when
  idle (`TestTheServerCountsOnlyWhileSomebodyReads`) and an upgrade leaving
  the helper (`TestAnUninstallRemovesTheHelperAndAnUpgradeLeavesIt`).
- The command-line installer with `--network=yes`, from an elevated shell:
  its log says "The network helper is installed (0.10.0)"; `sc qc` shows
  `DEMAND_START` and `NT AUTHORITY\LocalService`; the program is alone in
  `C:\Program Files\hayami`.
- A process with a filtered, medium-integrity token ran
  `hayami-tui detail bandwidth`: the service started and the list came back
  under "TCP and UDP". Twenty seconds later the service was stopped and no
  `hayami-network` trace session was left.
- `tests/window`'s `TestAClickOnTheBandwidthCardOpensItsDrawer` on the real
  window with the helper installed: the drawer shows five programs with
  rates and "TCP and UDP", and the service was running while it was open.
- `network-helper remove`: the service and `C:\Program Files\hayami` are
  gone, and the same medium-integrity read says "needs hayami's helper".

- The wizard, started without elevation and gone through by hand by the
  desk's user with the Network box ticked: reported working. Afterwards
  `sc qc` shows the service registered for that account, `DEMAND_START`
  under `NT AUTHORITY\LocalService`.

The Network page in the real wizard has a headless test of what it holds and
no test that reads its pixels; what it looks like was seen by a person and
not by a test.
