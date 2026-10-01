# 030 — the panel, not the preferences window

**Issue**: #107

## Context

The preferences window carries the panel's app ID, as every window of an app
does on Wayland, and the KWin scripts matched the app ID. So the watch
reported a drag of the preferences window and saved it as the panel's
position; the next start put the panel where the preferences window had
been. And spec 029's placement placed "the first window of the class", which
is the preferences window when the program is started onto a preferences
page: measured on the desk on 2026-09-30, `hayami --preferences=about` showed
"hayami preferences" 140 ms before "hayami".

fynedesygn 0.1.78 (spec 050) gives the scripts a `kwin.Target` of class and
caption. The panel's title is its own, set before the window is mapped.

## Requirements

- R1 `desktop.NewPosition(appID, title)` targets the window with the panel's
  title; `Start` passes `Options.Title`, which is the window's.
- R2 The watch reports only the panel; a drag of the preferences window is
  not saved.
- R3 The placement places the panel, whichever of its windows the compositor
  shows first.
- R4 fynedesygn is at v0.1.78.

## Acceptance Criteria

- [x] R1 (`internal/desktop/position.go`, `internal/gui/gui.go`; the
      package tests build the position with a title)
- [x] R2, R3 on the desk, on the real settings: started with
      `--preferences=about`, the preferences window appeared first at the
      compositor's 686,1442 and stayed there, the panel was at its saved
      3690,2052; a drag of the preferences window to 300,1300 left the
      settings at 3690,2052; a drag of the panel to 3600,2000 was saved and
      a drag back was saved; nothing on stderr; SIGTERM unloaded both
      scripts
- [x] R4 (`go.mod`)
- [x] `make test`, `make lint` pass

## Risks & Assumptions

- The caption match is exact against `Options.Title`, which `Start` also
  gives the window, so the two cannot drift apart without one line changing
  both. A title the user could change does not exist.
- The window rule (`desktop.Rule`) still matches the class and so still
  applies to the preferences window; that is unchanged and not this bug.
- Rollback: revert the commit and the bump. No settings change.

## Status: COMPLETE
