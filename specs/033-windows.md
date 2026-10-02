# 033 — Windows

**Issue**: #115

## Status: INCOMPLETE

## Context

hayami was Linux-only by construction, and on a Windows 11 machine none of it
built. What stood in the way, in the order it was found:

1. **sanshoku did not compile off Linux**: three files call
   `golang.org/x/sys/unix` with no build constraint. Fixed there (sanshoku
   spec 011).
2. **The panel panicked before drawing**: fynedesygn's glance window set its
   transparent theme before `Show`, and on Windows a theme change asks every
   window for a native handle the unshown one does not have. Fixed there
   (fynedesygn spec 052, quirk 43).
3. **The panel could not be moved.** Frameless and always on top, it opened
   in the middle of the screen, and Windows has no Alt-drag. On Linux its place
   is KWin's to keep; here nothing kept it. Dragging is fynedesygn's (spec
   052); remembering the place is hayami's.
4. **Bandwidth read `/proc/net/dev`**, which Windows does not have.
5. **The tests were not sandboxed on Windows.** They took the home, cache and
   settings directories away by their Unix names; Go's `os` reads
   `USERPROFILE` and `APPDATA` there, and the usage cache prefers
   `LOCALAPPDATA`. The suite read the real usage cache (two failures that were
   the real cache's contents), and a test that took the credential store away
   with `HOME` alone would have found the real one.
6. **The preferences window offered a KWin rule**, and `hayami window`
   wrote `kwinrulesrc`, on a desktop with no KWin.
7. **A Windows checkout is CRLF by default**, which failed the byte-for-byte
   tests of the README and the desktop entries.
8. **No way to install** other than copying binaries by hand.

Reading peripherals and the cooler on Windows is not part of this: sanshoku
reads through Linux interfaces, and on Windows it now builds and finds
nothing. That is its own piece of work, device by device.

## Requirements

- R1 `core.ReadCounters` replaces `ReadNetDev`: `/proc/net/dev` everywhere
  but Windows; on Windows `GetIfTable2Ex`, keyed by interface alias, without
  the filter interfaces (WFP, QoS and the like) that repeat each adapter's
  bytes.
- R2 `ClassifyInterface` ignores case and knows Windows' own pseudo-interfaces
  as virtual (loopback, `Local Area Connection*`, 6to4, Teredo, IP-HTTPS,
  ISATAP, the kernel-debugger NIC); "Tailscale" is a tunnel.
- R3 On Windows `desktop.Position` finds the panel among the process's own
  windows by title, reports moves by polling its rectangle once a second, and
  restores a remembered place with `SetWindowPos` -- unless no monitor covers
  it.
- R4 `desktop.RulesApply` is true on Linux only. Where it is false the
  preferences window shows `desktop.NoRules` instead of the rule's checkbox,
  and every `hayami window` subcommand prints it and succeeds without writing
  anything.
- R5 `internal/testenv` sets every name a per-user directory is read by on
  either platform (`HOME`/`USERPROFILE`, `XDG_CACHE_HOME`/`LOCALAPPDATA`,
  `XDG_CONFIG_HOME`/`APPDATA`), and every test that sandboxed by environment
  uses it.
- R6 The last-readings cache prefers `LOCALAPPDATA`, as the usage cache does.
- R7 Tests of Linux-only things skip elsewhere with the reason (`install.sh`,
  the udev advice). The Python cross-check skips when no Python *runs*, not
  only when none is found: Windows' Store placeholders are found and exit
  9009.
- R8 `.gitattributes` pins LF, and CRLF for `*.ps1`.
- R9 `scripts/install_windows.ps1` builds both programs and installs them per
  user with a Start menu shortcut, `-Autostart` and `-DryRun`;
  `scripts/uninstall_windows.ps1` removes exactly that. Both run under
  Windows PowerShell 5.1 and carry CRLF and a UTF-8 BOM, which a test holds.
- R10 CI runs the suite and builds both programs on `windows-latest`.
- R11 `go.mod` requires the sanshoku and fynedesygn releases that carry their
  fixes.
- R12 README: an "On Windows" section and the changelog.

## Acceptance Criteria

- [x] `go test -tags migrated_fynedo ./...` passes on Windows 11, and no file
  under the real `%APPDATA%`, `%LOCALAPPDATA%` or `~/.cache` changes during
  the run (compared before and after).
- [ ] `make test` and `make lint` pass on Linux (CI).
- [x] On a real window on Windows 11: the panel starts, is translucent, is
  dragged by a real pointer by exactly the distance moved, saves the place,
  and reopens there. Screenshots looked at.
- [x] The bandwidth section reads live rates for two chosen interfaces on
  Windows, in the panel and in `hayami-tui`; the chooser lists three ordinary
  interfaces and hides the virtual ones.
- [x] The preferences window's Window section on Windows shows no KWin rule
  and says why. Screenshot looked at.
- [x] `install_windows.ps1 -Autostart` then `uninstall_windows.ps1`, under
  Windows PowerShell 5.1, installs and removes exactly two programs and two
  shortcuts.
- [x] Falsified: the filter test fails with the filter removed; the BOM test
  fails with the BOM removed.
- [ ] `go.mod` requires released sanshoku and fynedesygn versions, and the
  above still holds against them.

## Risks & Assumptions

- **Linux behaviour is unchanged** apart from `ClassifyInterface` ignoring
  case, which only regroups a capitalised name in the chooser.
- **`windows-latest` carries a mingw gcc on PATH.** If the image stops doing
  so, the CI job needs a setup step.
- **Position polling** is one `EnumWindows` and one `GetWindowRect` a second.
- **The readings cache moves** on Windows from `~/.cache/hayami` to
  `%LOCALAPPDATA%\hayami`; the old file is ignored, and costs one blank first
  frame. Nobody ran hayami on Windows before this.
- **Rollback**: revert; the libraries' fixes are independent of this.

## Gaps found

- The context menu does not always open on a right-click over a row that has
  a tip: two of three tries over the peripherals rows showed the tip and no
  menu, in a build with and without fynedesygn spec 052's change. Not investigated here.
- The Windows executable has no icon of its own in Explorer and the Start
  menu; the window's icon is set at runtime as on Linux.
- Peripherals and the cooler, as above.

## Verification

2026-09-30, Windows 11 Pro 26200, Go 1.26.0, gcc 16.1.0 (WinLibs UCRT),
against local branches of sanshoku (spec 011) and fynedesygn (spec 052):

- Full suite: 17 packages ok; the real per-user directories were listed
  before and after with modification times, and did not differ.
- Drag: pointer pressed at (1794, 821) and moved by (-600, -300); the window
  went from (1734, 781) to (1134, 481); `settings.yaml` gained `x: 1134`,
  `y: 481`; a restart opened the window at (1134, 481).
- Bandwidth: two interfaces chosen in the preferences, one of them a
  tunnel; the panel and `hayami-tui` showed rates and totals for both.
