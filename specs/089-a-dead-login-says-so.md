# 089 — a dead login says so

**Issue**: #325

## Status: INCOMPLETE

## Context

On the Windows VM the usage section showed a reading 32 days old. The Claude
login had died: the access token expired on 2026-10-13, and the token endpoint
answered the refresh with a 400. Signing in again with Claude Code fixed it at
the next poll. Until then the panel gave no sign of the cause:

- `gather` (`internal/panel/usage.go`) reports a failed fetch only for an
  account that drew no windows. An account with figures from an older fetch
  drew them and dropped the error. The section was dimmed (spec 027's stale
  rule, #172), and its age, "read 32d ago", appeared only in the hover note.
  `doctor` said `usage ok`.
- `Refresh` (`internal/claude/refresh.go`) treats only a 401 or 403 from the
  token endpoint as `ErrNeedsLogin`. OAuth (RFC 6749 §5.2) answers a refresh
  token that is spent, expired or revoked with a 400 and `invalid_grant`.
- The five-hour window was drawn at its old 1 %, with a reset a month gone.
- The fetch is repeated at every gate, two minutes, with the same refused
  refresh token. After a month of that the endpoint answered 429 to the VM
  and to the desk behind the same address. A 429 from the token endpoint
  is not read for its Retry-After either.

Polls run every 30 s and a fetch at most every 2 min (`UsageInterval`,
`UsageTTL`). Between fetches `usage.Cached` serves the cache without an
error, so a reason built only from the fetch's own error would appear for one
poll in four and vanish for the other three.

## Requirements

### Saying why the figures are old

- R1 **The outcome of this process's last fetch for an account is kept** until
  a later fetch succeeds, or until the cache holds a reading obtained after it
  (another program fetched). The polls between fetches report it as the fetch
  did.
- R2 **A login the server refused is a line**: a Warn reason labelled with the
  account, "needs a login", Actionable, with the detail saying to sign in with
  Claude Code again. It is drawn whether or not the account still has figures.
  It reflows the panel once, when it appears, and once when the person signs
  in. The reflow rule forbids that for a transient value. A refused login is
  not transient: it holds until the person acts.
- R3 **Any other failed fetch that leaves figures standing is an aside**: a
  Warn reason, "not refreshed", with the error as its detail, for the hover
  note and `doctor`. A network that drops for a minute must not add a line
  (#172).
- R4 An account with no figures says why as now (`silence`). When the cause is
  a refused login, it says R2's line instead of "could not be read".

### Old windows

- R5 **A window whose reset passed more than `view.UsageStale` ago, in a
  reading obtained before that reset, is not drawn.** Its fraction describes
  a window that has since started over. The margin keeps a window in place
  across the minutes between its reset and the next fetch. An account left
  with no windows says why, by R4. Where nothing failed, the reason is "out
  of date" with the reading's age. The usage drawer's breakdown drops the
  same limits.

### The token endpoint

- R6 **A 400 whose error is `invalid_grant` is `ErrNeedsLogin`.** The error is
  read from the body as OAuth's string field or as an object's `type`. Any
  other 400 stays a plain error naming the status and, where the body gives
  one, the error code. The body is never echoed in full.
- R7 **A 429 is `Retryable` with its Retry-After**, capped as a usage 429's is
  (`MaxRetryAfter`). `fetchClaude` passes a retryable refresh's wait to the
  gate, as it does for the usage request.
- R8 **A refresh token the endpoint refused is not sent again by this
  process.** `Refresh` keeps the SHA-256 of each refused refresh token in
  memory. For a store still holding one, it returns `ErrNeedsLogin` without a
  request. A sign-in writes a new refresh token, which is asked about at the
  next open gate, within two minutes.

### Documentation

- R9 README: a changelog line under `### Unreleased`.
- R10 docs/threat-model.md B3: the defence R8 adds.

## Acceptance Criteria

Headless, each falsified:

- [ ] A store whose refresh the endpoint answers with 400 `invalid_grant`,
  over a cache holding figures, draws the figures and a "needs a login" Warn
  line (not an aside) for the account. Driven through the real `gather` with
  a test server for the token endpoint.
- [ ] That line stays across the polls between fetches, with the gate closed.
- [ ] The line goes once the cache holds a reading obtained after the failure,
  as when another program fetched it.
- [ ] A refused refresh token makes one request and none after it; a store
  with a new refresh token is asked again.
- [ ] A failed fetch other than a refused login, with figures standing, adds a
  Warn aside and no line.
- [ ] A 400 `invalid_grant` is `ErrNeedsLogin`, in both body shapes; another
  400 is not, and its error names the code without the body.
- [ ] A 429 from the token endpoint is `Retryable` with its Retry-After, and
  the gate is moved by it.
- [ ] A window whose reset passed more than five minutes ago in an older
  reading is not drawn, on the card or in the drawer; one that passed a
  minute ago is; an account left with none says "out of date", or "needs a
  login" when its login was refused.

On the machine, the installed `doctor` over the same files:

- [ ] `doctor` reports usage as partial, not ok, with a refused login, and
  prints the reason. The rule that a reason drawn on the card makes a section
  partial is `TestAMachineWithNoGPUIsOKAndSaysSo`'s.

Against the real token endpoint:

- [ ] A made-up refresh token is answered with a 400 that `Refresh` reads as
  `ErrNeedsLogin`.

On a real window (project rule: anything visual is tested on a real window):

- [ ] The panel run with a credential store whose refresh token is made up,
  over a cache with old figures: the "needs a login" line is drawn under the
  account's figures, and a five-hour window a month gone is not drawn.

## Risks & Assumptions

- Assumes the token endpoint answers a dead refresh token with
  `invalid_grant`, per RFC 6749. The real-endpoint criterion checks it with a
  made-up token. A token that was valid and then revoked may answer
  differently; if so, the panel says "not refreshed" with the code, as it
  does now, and backs off by the gate only.
- R8 is per process. A panel and a `doctor` run each send a refused token
  once. Writing the refusal into the shared cache would make it every
  program's decision; the cache's format is shared with the Python tools
  and is not changed.
- R5 hides a window that a reader may have expected to see dimmed. A month-old
  1 % is a wrong figure, not an old one.
- **Rollback**: revert the PR; no setting or file format changes.

## Threat model delta

B3 (credentials). The SHA-256 of a refused refresh token is held in memory
for the life of the process, never written or printed; the token itself is
not kept beyond the `Credentials` it was read into, as now. Fewer requests
carry a dead token. A hostile token endpoint already decides what a refresh
answers; it can now say "needs a login" with a 400 as it could with a 401,
and set a Retry-After capped at an hour, as the usage endpoint can. The
document's B3 defences gain R8.

## E2E Test Plan

1. Desk, against the real token endpoint: `Refresh` with a made-up refresh
   token returns `ErrNeedsLogin` (R6).
2. Desk, Plasma: the installed panel's settings copied, a throwaway home
   holding a credential store with a made-up, expired token and a cache with
   a month-old reading. The card shows the 7d figures, no 5h window, and
   "needs a login" under the account (R2, R5).
3. Windows VM: the same, by the same files (R2, R5).

### Prod visibility

Not applicable: a desktop program with no telemetry.
