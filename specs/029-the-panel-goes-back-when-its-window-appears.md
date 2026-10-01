# 029 — the panel goes back when its window appears

**Issue**: #106

## Context

The panel reopened centred on the left monitor instead of at its saved
position. Traced on the desk on 2026-09-30, with the settings file and the
window's geometry logged through a restart, two causes, either of which is
enough:

1. **The restore ran before there was a window.** `rememberPosition` slept a
   fixed 600 ms and then ran `kwin.PositionScript`, which moves a window that
   is on screen at that moment. A cold start put the window on screen 2.15 s
   after launch. The script found nothing, unloaded, and the panel stayed
   where the compositor placed it.
2. **A stale watch script blocked the new one.** The panel handled no
   signals, so a panel stopped with SIGTERM — by a service, a session, or a
   shell — never reached `Position.Stop` and left `hayami-geometry` loaded in
   KWin. KWin answers a second `loadScript` under a loaded name with id -1 and
   no error; the run of script -1 fails; `Watch` returns the error and
   `rememberPosition` returns before the restore. The stale script keeps
   reporting to the bus name the new panel owns, so the watch looks alive
   while nothing is ever restored. Both errors were discarded with `_ =`.

The watch half was sound throughout: a drag was saved, and the compositor's
initial placement was never written over the saved position.

## Requirements

- R1 The restore is `kwin.PlaceScript` (fynedesygn spec 049), loaded under
  its own name and left loaded: it places the window if one is on screen and
  otherwise the first of the class that appears, once. No delay in the
  program. `RestoreDelay` is gone.
- R2 `Position.load` unloads any script already under the name before
  loading. Unloading a name that is not loaded is answered `false` and
  nothing else.
- R3 `Position.Stop` unloads both scripts. `rememberPosition` returns the
  stop, and `Start` calls it after `ShowAndRun` returns, before the store is
  closed, so the process is still there to ask.
- R4 SIGINT and SIGTERM quit the window through the toolkit, so a signal is
  a close: scripts out, position written.
- R5 A failure to watch or to restore is printed to stderr with the
  program's prefix, as `main` prints its errors, rather than discarded.
- R6 `Restore` before `Watch` is refused with `ErrNoKWin`; `Stop` on a
  position that never watched is nothing.

## Acceptance Criteria

- [x] R6 (`TestRestoringBeforeWatchingIsRefused`,
      `TestStoppingAnUnwatchedPositionIsNothing`)
- [x] R1–R3 on the desk, over a deliberately stale script: the old build
      was killed with SIGTERM, `hayami-geometry` confirmed still loaded,
      the new build started; the window's first reported position, 1.4 s
      after launch, was the saved 3690,2052; both `hayami-geometry` and
      `hayami-place` loaded; nothing on stderr
- [x] R4 on the desk: SIGTERM to the new build; the process exited, both
      scripts unloaded, the bus name released
- [x] R5 by reading: both calls print; neither printed in the run above
- [x] `make test`, `make lint` pass

## Risks & Assumptions

- The compositor is not in the tests. The script text is asserted in
  fynedesygn and the behaviour is the desk trace above, as spec 037's was.
- A signal now quits the window rather than killing the process on the
  spot. `fyne.Do(a.Quit)` from the signal goroutine is the toolkit's own
  route; a second SIGTERM while it is quitting is delivered to the same
  channel and ignored, so a stuck quit still needs SIGKILL, as before.
- The preferences window carries the panel's class. `PlaceScript` places
  only the first window, so it is not moved; the watch still reports its
  moves as the panel's, which is a separate bug and is filed as its own
  issue.
- Rollback: revert the commit and the fynedesygn bump. No settings change.

## Status: COMPLETE
