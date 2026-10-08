# 042 — LibreHardwareMonitor set up the same way on every machine

**Issue**: #135

## Status: IMPLEMENTED — the real elevated step is for a person to run (it asks UAC)

## Context

Spec 036 reads the processor's temperature on Windows from
LibreHardwareMonitor's web server. Two things went wrong with it on the desk
it was written on.

1. **Closing its window quits it.** LibreHardwareMonitor is a desktop program
   that lives in the notification area, not a service, and its Minimize On
   Close option (`minCloseMenuItem`) is off by default. The window was closed
   by someone who did not know it was the thing serving the temperature; the
   process ended and the card lost its temperature. From its source
   (`LibreHardwareMonitor.Windows.Forms/UI/MainForm.cs`): `startMinMenuItem`
   (default off), `minTrayMenuItem` (on), `minCloseMenuItem` (off).
2. **The fix was done by hand**, and another machine would get none of it:
   - the three window options set in `LibreHardwareMonitor.config` while it
     was stopped (it rewrites the whole file on exit);
   - a scheduled task made the way its own Options → Run On Windows Startup
     makes one (`StartupManager.cs`): name `LibreHardwareMonitor` in the root
     folder, logon trigger, highest run level, interactive token, the
     executable with its folder as working directory, no time limit, start
     when available, not stopped on battery. Its menu shows the option as on
     when a task of that name runs that executable;
   - LibreHardwareMonitor started through the task, with one UAC prompt for
     the registration and the start.

   Afterwards: `data.json` answered, the process had no window
   (`MainWindowHandle` 0), the task ran at Highest, and `hayami-tui doctor`
   had the processor's temperature.

Also seen: on exit LibreHardwareMonitor wrote `listenerIp` back as `+`, every
interface, over the `127.0.0.1` spec 036's installer had written. The README
already says the setting is not honoured; the key is not one hayami needs.

## Requirements

- R1 `scripts/lhm_settings.ps1 -Path <file> [-Port N] [-DryRun]` makes the
  settings file say what hayami needs: `runWebServerMenuItem` true,
  `listenerPort` N (default 8085), `authenticationEnabled` false,
  `startMinMenuItem`, `minCloseMenuItem`, `minTrayMenuItem` true. Each key is
  added if missing and set if different; no other key and no other value is
  touched. Before the first change to an existing file it is copied to
  `<file>.bak-hayami`; an existing backup is never overwritten. With nothing
  to change it writes nothing. A missing file is made with the keys. It is a
  file of its own so that the elevated step and the tests run the same code.
- R2 `install_windows.ps1 -WithSensors`, each step only if not done already:
  - R2.1 winget install when LibreHardwareMonitor is not found (unchanged from
    spec 036, agreements accepted only under this switch).
  - R2.2 The port is hayami's `lhm` setting's, or 8085.
  - R2.3 Unelevated, it finds what is missing: the settings (R1's dry run),
    the task (registered for this executable at Highest), the process.
  - R2.4 One elevated step (`Start-Process powershell -Verb RunAs -Wait`,
    `-EncodedCommand`) does what is missing: stops a running
    LibreHardwareMonitor when its settings change (it is elevated, so stopping
    needs elevation; it is killed, since closing its window would now only
    hide it), runs R1, registers the task as above, and starts it through the
    task. Nothing missing, no elevation.
  - R2.5 Then it asks `data.json` for up to 30 s for the processor's
    temperature, by core's labels in core's order, and says which step is
    missing if none comes: no answer (not running, web server off, another
    port) or an answer without one (PawnIO).
  - R2.6 `-DryRun` says each step and changes nothing; with LibreHardwareMonitor
    running it also makes R2.5's read-only check.
- R3 `uninstall_windows.ps1` leaves LibreHardwareMonitor and its task alone
  and prints the commands that remove them; it runs neither.
- R4 `core`: `LHMHost.Task` is whether the startup task is registered, read
  from Task Scheduler's index in the registry
  (`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Schedule\TaskCache\Tree\LibreHardwareMonitor`),
  which any user may read; Task Scheduler's COM interface would need a COM
  runtime and the task's own file is not readable unelevated. No subprocess.
  When the server does not answer and LibreHardwareMonitor is not running:
  a registered task is `AbsenceLHMTaskStopped` ("though its startup task is
  registered: start it as administrator, or log off and on"); no task with
  PawnIO is `AbsenceLHMNotRunning`, now naming Run On Windows Startup and
  `-WithSensors`.
- R5 README: the manual steps match the script, including that closing the
  window quits it unless Minimize On Close is on, and the startup task;
  changelog under `### Unreleased`.

## Acceptance Criteria

- [x] R1 on a copy of a real-shaped settings file: six keys set (one
  corrected, five added, one already right left), three unrelated keys kept,
  the original kept as the backup; a second run says "unchanged" and leaves
  the file byte-for-byte; a later change keeps the first backup; a dry run
  writes nothing; a missing file is made, with no backup.
- [x] The installer's label list equals `core.LHMCPULabels` (a test reads the
  script).
- [x] A dry run with empty per-user directories (no LibreHardwareMonitor)
  says every step and writes nothing.
- [x] A dry run on this desk, set up by hand, says every step is done and its
  check reads the processor's temperature; it writes nothing.
- [x] `core`: a registered task and a missing one give different codes and
  sentences; the probe reads the task on this desk unelevated (`Task: true`).
- [x] The uninstaller names `Unregister-ScheduledTask -TaskName
  LibreHardwareMonitor` and runs no task removal.
- [x] Falsified: R1 always rewriting, a wrong value, the backup overwritten;
  the label list reordered; a dry-run step's wording; the task case removed
  from `core` — each fails its test, and passes restored.
- [ ] The real elevated step on a machine that needs it. Not run here: it asks
  UAC, and this desk was already set up by hand.

## Risks & Assumptions

- **Killing LibreHardwareMonitor** to edit its settings loses settings it
  changed since it last wrote the file (a column width, a hidden sensor). It
  is done only when a setting hayami needs is different, and the first
  backup keeps the file as it was before hayami touched it.
- **The task is registered for the current user's logon**, where
  LibreHardwareMonitor's own registers one for any logon. Its menu still shows
  the option as on (it matches the name and the executable).
- **A standard user elevating with another account's credentials** runs the
  elevated step as that account; the paths are passed absolute, so the
  settings and the task are the right ones, but the task's user is the one
  the script started as.
- **Rollback**: revert. A machine already set up keeps its settings and task;
  `Unregister-ScheduledTask` and the backup undo them.

## Gaps found

- LibreHardwareMonitor offers no service mode; a temperature before logon
  would need a wrapper around its library, which is not worth it for a panel
  read while someone is logged in.
- Its `listenerIp` setting is rewritten to `+` on exit (above); reported here,
  not upstream.

## Verification

2026-10-07, Windows 11 Pro 26200, Windows PowerShell 5.1, LibreHardwareMonitor
0.9.6 set up by hand as above:

- Dry run (`-DryRun -WithSensors`, throwaway destinations): "already
  installed"; "settings: web server on port 8085, no password, starts
  minimized, closing hides it"; "startup task 'LibreHardwareMonitor' at highest
  privileges"; "running"; "hayami can read it: Core (Tctl/Tdie) 44.9 °C, from
  http://127.0.0.1:8085/data.json"; nothing written.
- The host probe, unelevated: PawnIO true, running true, startup task true.
- Structure tests and core tests pass; the falsifications above each failed.
