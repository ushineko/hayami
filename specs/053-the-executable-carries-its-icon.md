# 053 — The executable carries its icon

**Issue**: #163

## Status: COMPLETE

## Context

The Start menu entry the installer writes was blank, and so were the login
shortcut and the executable in Explorer: `hayami.exe` carried no icon resource
(spec 033 recorded the gap) and its shortcuts named no icon location. The
window's own icon is set at run time from `internal/gui/assets/hayami.svg`, so
only what Explorer draws from the file was missing. A file's Properties named
no version either.

clockwork-orange embeds its icon with `fyne package` in CI. hayami's installer
builds on the person's machine with `go build`, so the step has to work there
too, and the maintainer chose go-winres run with `go run` at a pinned version:
nothing installed globally, the same command in the installer and in CI.

## Requirements

- R1 `tools/appicon` draws `internal/gui/assets/hayami.svg` as a PNG with the
  rasteriser Fyne draws SVGs with (fyne-io/oksvg, srwiley/rasterx, already in
  the module graph; only moved from indirect to direct requirements). The PNG
  is drawn at build time, not committed: one drawing for the window and the
  file, and no image to drift from it.
- R2 `scripts/winres.ps1` runs it, then `go run
  github.com/tc-hib/go-winres@v0.3.3 simply` for `cmd/hayami` and
  `cmd/hayami-tui`, writing `rsrc_windows_amd64.syso` beside each: the icon,
  and a version resource (product and file version from VERSION, product name
  hayami, a description, the original filename; no company or copyright).
  `--manifest none`: the panel's DPI awareness is GLFW's to set at run time.
  The go-winres version is named in that script alone. `-DryRun` says what it
  would write. A failure stops the build.
- R3 `install_windows.ps1` runs it before `go build` and its dry run says so;
  each shortcut's `IconLocation` is the installed executable's icon.
- R4 CI's Windows job runs it before its build.
- R5 `.gitignore` keeps the `.syso` files out; they are build output.
- R6 README: the Windows section and Development; the changelog.

## Acceptance Criteria

- [x] A panel built after `winres.ps1` has an icon (`ExtractIconExW` counts
  more than none) and VERSION as its product version, "A glance panel" and
  "hayami" in its version strings
  (`TestThePanelsExecutableCarriesItsIconAndVersion`).
- [x] Falsified: with the panel left out of `winres.ps1` and its old `.syso`
  removed, that test fails on the icon and the version.
- [x] The installer's dry run names the resource it would write.
- [x] The real installer on this desk installs a panel with an icon, and the
  Start menu and login shortcuts point at it.
- [x] No `.syso` is tracked; `git status` does not list them.

## Risks & Assumptions

- **go-winres and its modules are fetched by `go run`** on the first build, as
  any module is; a machine that builds hayami already fetches from the module
  proxy.
- **The SVG's rounded corners are square in the PNG**: fyne-io/oksvg does not
  draw a rect's `rx`. Fyne draws the window's icon with the same rasteriser, so
  the two match; a rounder icon is a change to the SVG.
- **hayami-tui gets the same resources**: the same icon in Explorer and a
  version in its Properties. It costs nothing and keeps the two programs
  alike in the Start menu's search results.
- **Rollback**: revert; the programs go back to having no icon.

## Gaps found

- The installer test runs a dry run, which writes no shortcut, so a
  shortcut's `IconLocation` is checked by the live install below rather than
  a test. A real install into temporary folders would need built executables
  in the checkout's root.

## Verification

2026-10-08, Windows 11 Pro 26200, Go 1.26.0, gcc 16.2.0 (MSYS2 UCRT64):

- `scripts\winres.ps1` wrote `cmd/hayami/rsrc_windows_amd64.syso` (5696
  bytes) and `cmd/hayami-tui/rsrc_windows_amd64.syso` (5736 bytes).
- The test above passes in 25 s, the build included; falsified as above.
- The installer's tests pass, CRLF and the BOM included.
- The real `install_windows.ps1 -Autostart -WithSensors` from this branch:
  "icon and version resources" before the build; the installed `hayami.exe`
  holds 1 icon, ProductName hayami, ProductVersion 0.9.4, FileDescription "A
  glance panel"; the Start menu and login shortcuts' `IconLocation` is the
  installed `hayami.exe,0`. The running panel was moved aside, not restarted.
