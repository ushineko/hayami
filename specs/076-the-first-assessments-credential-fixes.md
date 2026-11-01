# 076 — the first assessment's credential fixes

**Issue**: #246, #247, #249, #250

## Status: COMPLETE

## Executive Summary

Four low-severity findings from spec 073's first assessment, fixed together
because none needed a design choice: the HTTP client follows no redirect, a
server's Retry-After is honoured for an hour at most, an error's text writes
the user's home as `~`, and everything the terminal pane, `doctor` and
`detail` write has its control characters taken out on the way. Look first at
`internal/safetext`, `view.Painter.paint` and `panel.reason`.

## Context

Spec 073's first assessment of boundary B3 (credentials) found five things
that survived its refutation pass (#246–#250). Four are here. The fifth,
#248, changes how hayami writes another program's credential file and is
spec 077.

- **#250.** The HTTP client (`internal/panel/http.go`) set no
  `CheckRedirect`. On a 307 Go resends a POST's body, which for the token
  refresh is the refresh token, to wherever the redirect points; on a
  redirect to the same host over plain http it keeps `Authorization`.
  Reproduced against a local server.
- **#247.** `retryAfter` (`internal/claude/fetch.go`) took any positive
  value. The wait is written into the usage cache every viewer shares and
  nothing clears it early, so a reply asking for years stopped usage for
  every program reading the cache.
- **#246.** A filesystem error names its file, and the file is under the
  user's home. `panel.reason` copied the error's text into a reason, which
  `doctor` prints and the readings JSON carries, contradicting `doctor`'s own
  promise and the repository's rule against home paths in what people paste.
- **#249.** Nothing filtered control characters from what hayami writes to a
  terminal. The assessment found it in the usage labels and Codex's errors;
  the fix covers every source, since a process can name itself with an escape
  sequence and `detail` lists every account's processes.

## Requirements

- R1 The HTTP client refuses every redirect: a redirect is the response, and
  the status code is the error, as for any other unexpected status.
- R2 A Retry-After is honoured for at most an hour, given as seconds or as a
  date.
- R3 A reason's detail, from an error or from an absence, writes the user's
  home directory as `~`. A home of `/` is left alone.
- R4 One function, `safetext.Printable`, takes control characters (C0, C1,
  DEL) and invalid UTF-8 out of a string. Every piece of a section is passed
  through it in `view.Painter.paint`, before a painter adds its own escape
  codes, whether there is a painter or not; `view.Detail.Text` passes every
  field through it; `doctor`'s `line` passes its three columns through it.
  The network helper's error text, cleaned on its own since 0.11.11, uses
  the same function.
- R5 JSON output is unchanged: `encoding/json` already escapes control
  characters.

## Acceptance Criteria

- [x] A POST whose server answers 307 comes back as the 307, and the
      redirect's target receives nothing.
      (`TestTheClientFollowsNoRedirect`.)
- [x] A Retry-After of `9000000000` seconds, or a date a century away, is
      an hour. (`TestARetryAfterIsHonouredForAnHourAtMost`.)
- [x] A credential store this account may not read, under a home of the
      test's, gives a reason whose detail names `~/.claude/.credentials.json`
      and not the home. The error is the filesystem's, from a real file of
      mode 0. (`TestAReasonDoesNotNameTheUsersHome`.)
- [x] A section with an OSC 52 sequence in its title, a row and a reason
      renders with no ESC and no BEL, with and without a painter, and a
      painter's own colour survives. (`TestASectionCarriesNoEscapeItWasGiven`.)
- [x] A detail and a `doctor` line likewise.
      (`TestADetailCarriesNoEscapeItWasGiven`, `TestADoctorLineCarriesNoEscape`.)
- [x] Each test above was seen to fail with its fix taken out.
- [x] `make test` and `make lint` pass; `go vet` passes for Windows on the
      changed packages.

## Threat model delta

B3 and B6. A redirect can no longer carry a token anywhere (B3); a server can
no longer stop the shared cache for longer than an hour (B3); a pasted report
no longer names the user's home (B6); a process, device, peer or provider can
no longer drive the terminal hayami writes to (B4, B6). `docs/threat-model.md`
moves #246, #247, #249 and #250 from open findings to defences once it is on
`main` (#252).

## Risks & Assumptions

- **Stripping is not escaping.** A label with a control character in it
  loses the character rather than showing it. A name with one in it is a
  name nobody chose for a person to read.
- **The pane's widths.** `truncate` measures before `paint` cleans, so a
  hostile name may be drawn a few columns short. Nothing ordinary has a
  control character in it.
- **`~` is the user's home only.** A path under another account's home is
  shown as it is; hayami reads no other account's files.
- Rollback: revert the commit; nothing is stored differently.

## E2E Test Plan

Not applicable as a separate step: each criterion is a test against a real
server, a real file or the real renderer. Checked by hand once: `hayami-tui
doctor` on this desk prints as before.

### Prod visibility

Not applicable: a desktop program with no production deployment.
