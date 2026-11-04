# 084 — the Fyne fork

**Issue**: #293

## Status: COMPLETE

## Executive Summary

The desktop panel builds against `github.com/ushineko/fyne`, tag
`v2.8.3-ushineko.4`, through a `replace`, with Fyne and fynedesygn at the
versions the sibling projects use. The tag adds a fix found here: the base's
glyph atlas blended text so that a translucent window lost alpha at the edges
of every glyph. Look first at `go.mod`, then the B7 "Accepted" paragraph in
`docs/threat-model.md`.

## Context

Three fixes not yet in a tagged Fyne release are on the fork: a resize that
makes the GL context current per event, which stalled a window drag on NVIDIA;
a font cache keyed by theme scope; and a theme cache that kept whatever an
override covered. nmsbonker builds against `.1`, and jira-viewer's work in
progress against `.3`.

The fork's base is Fyne `develop`, whose glyph atlas drew text with
`BlendFunc(srcAlpha, oneMinusSrcAlpha)`. Fyne 2.8.2 drew it with
`BlendFunc(one, oneMinusSrcAlpha)`. hayami's window is translucent, so it shows
this: text edges on a card of alpha 124 measured 109, and the desktop came
through them. `.4` premultiplies in the glyph shader and restores the 2.8.2
blend.

## Requirements

- R1 `fyne.io/fyne/v2` is replaced by the fork at `v2.8.3-ushineko.4`, required
  at the commit fynedesygn pins, and fynedesygn is 0.1.113.
- R2 `go.mod` says what the fork carries and when the `replace` goes.
- R3 The translucent panel's text does not lower the window's alpha below the
  card it is drawn on.
- R4 `cmd/hayami-tui` still builds with `CGO_ENABLED=0`.
- R5 `make vuln` still reports advisories filed under `fyne.io/fyne/v2`:
  govulncheck queries a replaced module by the fork's path, so it also scans a
  copy with the `replace` dropped.

## Acceptance Criteria

- [x] R1, R2: `go.mod`; `go mod verify` passes; `go version -m` on the built
      panel names the fork at `.4`.
- [x] R3: a picture of the real window: the lowest alpha in each text region is
      the card's 124, and the count of pixels below it matches 0.11.16's within
      a few hundred. (E2E 1: 16,000 against 15,980; 34,091 on `.3`.)
- [x] R4: built with `CGO_ENABLED=0`.
- [x] Tests, lint and `make vuln` pass.
- [x] R5: an invented advisory under `fyne.io/fyne/v2` fires on the second scan
      and not the first. (The refutation pass ran it.)
- [x] B7's adversarial review run; what survives filed, what is refuted under
      "Checked". (`make vuln`'s gap fixed here, R5; the rest accepted in B7.)

## Threat model delta

B7. The panel links code from a second repository, the fork, which has no
ruleset, and which Dependabot does not follow. It reaches the privileged paths,
as upstream Fyne did: `hayami-tui` links it through `fynedesygn/theme`. The
tag's contents are held by `go.sum` and the checksum database. What an attacker
gains is a place to put a new tag, which still needs a reviewed `go.mod` change
to reach a build. The document's B7 section, red-team log and "Checked" are updated in the same PR.

## Risks & Assumptions

- The base is Fyne `develop`, not a release, and it is a large change to the
  painter: text comes from a glyph atlas, with slightly different edges.
- The second vulnerability scan reads upstream's source at the base, so an
  advisory that only the fork's commits introduce or fix is not told apart.
- The drag and memory fixes are not measured in hayami; nmsbonker measured the
  drag, and the fork's commits describe the rest.
- Plain rectangles share the glyph batch. A translucent one now has the alpha
  it asked for, not that alpha squared; the panel's cards do not use the batch
  and measure the same.
- No automated test in the fork covers the blend, which needs a GL context; the
  picture of the window is the check.
- Rollback: revert the commit. The panel returns to Fyne 2.8.2.

## E2E Test Plan

1. Build `cmd/hayami` at this commit, run `tools/screenshot.sh --what panel`
   with it and with 0.11.16 and compare alpha by region. (R3)

### Prod visibility

Not applicable: a desktop program with no production deployment.
