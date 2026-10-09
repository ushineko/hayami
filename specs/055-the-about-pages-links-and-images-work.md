# 055 — The About page's links and images work

**Issue**: #177

## Status: COMPLETE

## Context

The preferences window's About page shows the embedded README through
fynedesygn's `markdown.Pane`. Clicking a Contents entry did nothing, links to
other repository documents did nothing, and the screenshots showed only their
alt text.

- **Links.** Fyne hands a tapped link to the desktop's URL opener (rundll32
  on Windows, xdg-open on Linux) as written. `#platform-notes` and
  `docs/architecture.md` are relative strings with nothing to resolve
  against, so nothing useful opened.
- **Clicks.** Once anchors scrolled, a click on the middle of a Contents
  entry still did nothing. Fyne draws each link's box larger than its text,
  and in a list each box covered the lower half of the line above, so the
  click went to the next entry's box. Found by this spec's real-window test;
  fixed in fynedesygn (quirk 47 there).
- **Images.** hayami gave the pane no file system, so `docs/img/*.png`
  could not be read.

The owner decided that links to other documents are absolute URLs, not
resolved relative to a base: the README reads the same on GitHub and in the
window. #174 converted the README's links; this spec holds it to that.

## Requirements

- R1 hayami requires fynedesygn v0.1.93 (its spec 064, #187): a `#fragment`
  link scrolls the pane to its heading, an http(s) link opens in the browser,
  any other link is plain text, and a click anywhere over a link's text
  reaches it.
- R2 The README's gallery, `docs/img/gallery-*.png`, is embedded beside it
  (`hayami.Images()`) and given to the pane as `Options.FS`. The specs'
  photographs are not embedded: the README does not show them.
- R3 The pane's web links open through a package seam, `openURL`, nil in the
  program, so a test sees a tap arrive without a browser opening.
- R4 Every README link is a `#heading` that exists or an absolute http(s)
  address, and every image it shows is embedded. A test fails otherwise and
  names the full URL to write instead.
- R5 The About summary is platform-neutral: Linux and Windows, and the
  readings the README's introduction lists.

## Acceptance Criteria

- [x] Headless: tapping the Contents entries "Changelog", "Platform notes" and
  "Installing" scrolls the page so each heading's top is at the viewport's top
  (within half a point).
- [x] Headless: a web link's tap reaches the opener seam with its URL.
- [x] Headless: the seven gallery screenshots are drawn as images, none as
  alt text.
- [x] `readme_test.go`: no README link is relative; every anchor names a
  heading; every shown image is embedded.
- [x] Real window: Contents entries clicked by posted mouse messages bring
  their headings to the top, for "Platform notes", "Changelog" (the far end
  of the document) and "Installing". Falsified with the anchor handler made a
  no-op, and with fynedesygn's link unpadding switched off.

## Risks & Assumptions

- The embed adds 1.30 MB (seven PNGs, 78–347 kB) to the desktop panel's
  binary. The terminal panel does not import the root package and is
  unchanged. Accepted: About without its screenshots was the bug.
- The real-window test finds the Contents by the links' accent colour and
  identifies a heading by its height and by its width relative to its entry.
  A theme whose links are not blue-led would need the colour rule changed.

## Verification

- `go test ./...`: passes except `internal/usage`'s
  `TestTheSlugAgreesWithThePythonItself`, which fails on this machine on
  `main` too (the Python reference needs `structlog`).
- `HAYAMI_WINDOW_TEST=1 go test ./tests/window -run AContentsLink -v`:
  passes in 40 s. Pictures looked at: the Contents in view; after "Platform
  notes", the page starts at "Platform notes" with "Windows" below; after
  "Changelog", it starts at "Changelog" with "Unreleased" below; after
  "Installing", at "Installing". Heading width over entry width was 1.24,
  1.23 and 1.29.
- Falsified: with fynedesygn's anchor tap made a no-op, every click left the
  page where it was and the test failed on position, size and ratio. With its
  link unpadding off, the clicks on "Platform notes" and "Installing" did
  nothing and the one on "Changelog", the list's last entry, worked.
