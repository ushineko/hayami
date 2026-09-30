# 027 — a screenshot gallery

**Issue**: #97

## Status: COMPLETE

## Context

The README describes the panel in words and shows nothing. Five
photographs exist under `docs/img/`, one per spec that needed one, taken
by hand from a scratch harness. fynedesygn and angou photograph their
windows with a script that starts the program on the section it wants,
grabs the window by its class and geometry, and kills it, so a whole set
is one command and cannot quietly go stale. This spec brings that here:
the panel, the pane in each arrangement, and every page of the
preferences window, from the desk's live readings.

## Requirements

- R1 `tools/screenshot.sh`, ported from fynedesygn's (same author, MIT),
  taking `--out`, `--all`, and for one shot `--what panel|prefs:<page>`;
  it starts `./hayami` (with `--preferences` for a prefs page, and
  `--settings` pointing at a throwaway copy of the user's settings so the
  live arrangement and sections are shown but nothing is written back),
  finds the window it launched by comparing window ids before and after
  (not by class alone: the user's own panel is running), waits for two
  polls, captures the desktop with spectacle and crops to the window's
  geometry, and kills what it started. The pane shots use the existing
  `tools/shot-tui.sh` with an opaque terminal config.
- R2 The set, into `docs/img/`: `gallery-panel.png` (the panel, live);
  `gallery-pane-column.png` and `gallery-pane-row.png` (the pane at 100
  and 160 columns); one `gallery-prefs-<page>.png` per preferences page
  (sections, appearance, bandwidth, about, and any other the window has).
- R3 `make screenshots` runs the set. The Makefile line and the README
  Development block say it needs KDE/Wayland, kdotool, spectacle and
  alacritty.
- R4 README gains a "Screenshots" section near the top, one image per
  file with alt text that says what the picture shows, and the five
  spec photographs stay where their specs link them.
- R5 `readme_test.go`: every `docs/img/gallery-*.png` is displayed by the
  README, and every image the README displays exists. The spec
  photographs are exempt by prefix.
- R6 Changelog under `### Unreleased`.

## Acceptance Criteria

- [x] `make test` and `make lint` pass; the new README canary fails when
  an image is removed from the README (checked and reverted, noted).
- [x] `make screenshots` on this desk produces the whole set in one run,
  with the user's own panel left running, and the images are committed.
  Run 2026-09-30; the seven images are committed with the live readings.
  The user decided the public-repository rule was broader than its harm:
  it now forbids PII in a screenshot, not readings, and `.claude/CLAUDE.md`
  says so.
- [x] The README shows every gallery image with alt text.
- [x] README and changelog in the same commit; spec reconciled.

## Risks & Assumptions

- **Live values are the user's.** The user asked for current values; the
  usage card's figures are in the panel shot. No hostnames, addresses or
  paths appear in any window.
- **Rollback**: revert.

## Verification

`make screenshots`, 2026-09-30, on this desk (KDE Plasma on Wayland, two
outputs at scale 1.5) with the user's own panel running throughout (same
PID before and after; no `hayami-shot` terminal left behind):

```
tools/screenshot.sh --all
  gallery-panel.png  425x762  115K
docs/img/gallery-pane-column.png (1218, 546)
docs/img/gallery-pane-row.png (1938, 644)
  gallery-prefs-sections.png  1782x1194  175K
  gallery-prefs-window.png  1782x1194  226K
  gallery-prefs-appearance.png  1782x1194  339K
  gallery-prefs-about.png  1782x1194  247K
```

Each image was looked at against its alt text: the panel's four cards;
the pane's grid in two columns and its rows; the Sections, Window,
Appearance and About pages, each with its own navigation button lit.

The canary was broken twice and put back: with the Window page's line
removed from the README, `TestEveryGalleryImageIsInTheReadme` failed naming
`gallery-prefs-window.png`; with the panel's path misspelt,
`TestEveryImageTheReadmeShowsExists` failed naming the path. `make test`,
`make lint` (0 issues), `make build` and `shellcheck tools/*.sh` pass.

## Found while photographing

- **The preferences window carries the panel's class.** It is a second
  window of the same process, and on Wayland Fyne gives every window of an
  app the app's ID; `prefs.AppID` names the window's settings, not what a
  compositor sees. The harness tells the two apart by title.
- **The panel's translucency photographs what is behind it.** The first
  run cropped the desktop, as R1 says, and a line of a chat window showed
  through the Bandwidth card. The panel and the preferences window are
  grabbed alone (`spectacle -a`, after confirming focus), which keeps the
  panel's transparency as alpha and cannot show another window. The pane
  shots still crop the desktop, through an opaque terminal.
- **Two grid bugs in the pane**, fixed here because the gallery showed them:
  a painted cell was padded by its length including colour codes, so a dim
  title pulled the next column left by nine characters; and four sections
  in room for three columns filled two but measured them for three. Each
  has a test that failed before its fix.
- **About named the usual settings file** even under `--settings`, which
  would have put a home directory in the About shot. It names the file in
  use now.
- **There is no bandwidth page.** R2 lists "sections, appearance,
  bandwidth, about"; the interfaces are part of Sections, and the fourth
  page is Window. The harness reads the pages from `hayami --help`, so it
  photographs what the window has.
- **What the live shots show.** Device names (G502 X PLUS, Arctis Nova Pro
  Wireless), interface names (eno2, tailscale0, wlan0, tun0) and the usage
  figures, including the work account's spend in dollars, are in the
  images. The Risks section accepts the usage figures; the interface names
  are generic kernel names but the public-repository rule lists network
  interfaces, and that is the user's call before committing.
