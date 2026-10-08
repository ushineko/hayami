# 051 — The panel stands aside for a full-screen app

**Issue**: #167

## Status: IMPLEMENTED — one criterion is for the Linux desk

## Context

The panel is always on top. On Windows that keeps it over a borderless
full-screen window: a browser in full screen (F11, or a web player's
full-screen button) and a game in windowed or borderless full screen, which
are the cases that matter. Exclusive Direct3D full screen covers it anyway.
Asked for after the 0.9.x sit test: the panel should hide while a full-screen
app is in front and come back on returning to the desktop.

fynedesygn 0.1.89 (its spec 059, issue #178) does the watching:
`glance.Window.SetHideForFullscreen`. Full screen is the window in front
containing the whole of the panel's monitor, by its window rectangle or its
DWM frame; a maximised window, another monitor, this program's own windows,
the desktop and the taskbar do not count; two looks in a row at 500 ms decide
either way; the hide and the show are posted to the window's thread, and the
show never takes the focus. Measured on Terraria in windowed full screen: a
WS_POPUP window, not maximised, whose window, client and DWM rectangles are
all exactly the monitor's.

On KWin an active full-screen window is in a layer above keep-above windows,
so the window manager already does this.

## Requirements

- R1 A setting `hideForFullscreen`, a window setting beside the opacity. Nil
  means on, which is what a settings file written before it carries: a panel
  over a full-screen app is never wanted.
- R2 Preferences, Window: a checkbox, "Hide while a full-screen app is in
  front", where the panel has to do it itself (`desktop.HidesForFullscreen`,
  Windows). Saved through the store and applied live by `Panel.Apply`.
- R3 The panel applies it through fynedesygn's `SetHideForFullscreen`; the
  terminal panel is unaffected.
- R4 The window-test harness writes the setting explicitly off unless a test
  asks: a panel hidden because someone has a game in front could not be
  photographed.

## Acceptance Criteria

- [x] A settings file without the setting hides for a full-screen app;
  turned off, it stays off across a restart.
- [x] The checkbox is offered and ticked on a platform that needs it, not
  offered where the window manager does it, and unticking it is saved.
- [x] Real window (`HAYAMI_WINDOW_TEST=1`): a borderless popup exactly the
  panel's monitor, and one 8 px past every side, each hide the panel; closing
  the popup brings it back in the same place without the focus.
- [x] With the setting off, the same popup leaves the panel shown.
- [x] Falsified: without the `SetHideForFullscreen` call in `Apply` the panel
  does not hide.
- [x] The Explorer-launch test still passes.
- [ ] On the Linux desk: an active full-screen window covers the panel with
  nothing from this spec.
- [x] With the maintainer: Terraria in windowed full screen hides the
  installed panel, and leaving it brings the panel back. Also YouTube's
  full-screen button in the maintainer's browser.

## Risks & Assumptions

- **On by default** changes what an existing Windows install does: the panel
  now goes away while a full-screen app is in front. That is the request; the
  checkbox turns it off.
- **The window tests take the foreground for a few seconds** and skip when a
  full-screen window is already in front, so they never take over a game.
- **Rollback**: revert, or untick the checkbox.

## Gaps found

- Not checked against a real browser or game here: the maintainer was
  playing a game on the desk. The popup is the same shape; the check with
  Terraria is to be done with the maintainer after installing.

## Verification

2026-10-08, Windows 11 Pro 26200, a 3840x2160 monitor at 150 %:

- Window tests: exactly the monitor, hidden after 830 ms and back after
  980 ms; 8 px past every side, hidden after 1.02 s and back after 980 ms;
  back in place each time, not focused. Setting off: still shown after
  2.5 s.
- Falsified as above.

2026-10-08, the installed 0.9.4, started from its Start menu shortcut:

- A borderless window covering the panel's monitor (Terraria's shape), brought
  to the front: the panel hidden after 566 ms; closed, it was back after 1.0 s
  at the same place, (3462, 1448), and had not taken the focus.
- A full-screen Edge window in a throwaway profile: the panel hidden after
  1.9 s, Edge's start included; closed, back at the same place, not focused.
- The maintainer: Terraria in windowed full screen hides the panel and
  Alt-Tab brings it back; YouTube's full-screen button does the same.
