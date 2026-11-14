# 089 — a dead login says so

**Issue**: #325

## Status: INCOMPLETE

## Executive Summary

When a Claude login is refused, the usage section now says "needs a login" on
the account's card, in every window and pane, instead of drawing month-old
figures with no reason. A refused refresh token is not sent again, a 429 from
the token endpoint is waited out, and a window that reset since the reading is
not drawn. A reviewer should look first at `gather` and `fetches` in
`internal/panel/usage.go`, then at `refusal` and `errorCode` in
`internal/claude/refresh.go`.

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
poll in four and vanish for the other three. Several viewers share one cache,
and only the one whose turn it is fetches.

## Requirements

### Saying why the figures are old

- R1 **A refused login is recorded in the shared cache**, as
  `login_refused_at`, so every viewer says it and not only the one that
  fetched. It is kept through a failure of another kind and dropped by a
  success. The Python tools read only the keys they know and write their
  entries whole: they ignore it, and a write of theirs drops it. **Any other
  failure is kept by the process that fetched** until a later fetch
  succeeds, or until the cache holds a reading obtained after it and not
  after the present.
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
  a code of OAuth's shape (`[a-z_]`, 64 at most), the code. The body is never
  echoed.
- R7 **A 429 is `Retryable` with its Retry-After**, capped as a usage 429's is
  (`MaxRetryAfter`). `fetchClaude` passes a retryable refresh's wait to the
  gate, as it does for the usage request.
- R8 **A refresh token the endpoint refused (401 or `invalid_grant`) is not
  sent again by this process.** `Refresh` keeps the SHA-256 of each refused
  refresh token in memory. For a store still holding one, it returns
  `ErrNeedsLogin` without a request. A sign-in writes a new refresh token,
  which is asked about at the next open gate, within two minutes. A 403 is
  not remembered: a proxy or a challenge page answers 403 too. A refusal
  re-reads the store, and when it holds another refresh token by then,
  Claude Code refreshed meanwhile: the refresh goes on with its tokens.

### Documentation

- R9 README: a changelog line under `### Unreleased`, and the usage paragraph
  of the user guide.
- R10 docs/threat-model.md B3: the defences R1 and R8 add, the review's
  record and what it checked.

## Acceptance Criteria

Headless, each falsified (19 mutations of the code, one per rule below, each
failed a test; `/tmp` script, not committed):

- [x] A store whose refresh the endpoint answers with 400 `invalid_grant`,
  over a cache holding figures, draws the figures and a "needs a login" Warn
  line (not an aside) for the account. Driven through the real `gather` with
  a test server for the token endpoint
  (`TestADeadLoginIsALineBesideTheFiguresItLeft`).
- [x] That line stays across the polls between fetches, with the gate closed;
  and a second viewer that never fetched says it too
  (`TestAViewerThatDidNotFetchSaysItToo`,
  `TestALoginRefusedIsRecordedUntilAFetchSucceeds`).
- [x] The line goes once the cache holds a reading obtained after the failure,
  as when another program fetched it (`TestTheLineGoesWhenANewerReadingArrives`);
  a reading dated in the future does not clear a failure
  (`TestAReadingFromTheFutureDoesNotClearTheFailure`).
- [x] A refused refresh token makes one request and none after it; a store
  with a new refresh token is asked again
  (`TestARefusedRefreshTokenIsNotSentAgain`). A 403 is asked again
  (`TestAForbiddenIsNotRemembered`); a refusal of a token spent meanwhile is
  not a login problem (`TestARefusalOfATokenSpentMeanwhileIsNotALoginProblem`).
- [x] A failed fetch other than a refused login, with figures standing, adds a
  Warn aside and no line, across the polls between fetches
  (`TestAnotherFailureBehindFiguresIsAnAside`).
- [x] A 400 `invalid_grant` is `ErrNeedsLogin`, in both body shapes; another
  400 is not, and its error names the code without the body; a code that is a
  sentence or carries control characters is left out
  (`TestAnInvalidGrantSaysTheAccountNeedsALogin`,
  `TestAnotherBadRequestNamesItsCodeOnly`, `TestACodeThatIsASentenceIsLeftOut`).
- [x] A 429 from the token endpoint is `Retryable` with its Retry-After, and
  the gate is moved by it
  (`TestATooManyRequestsFromTheTokenEndpointCarriesItsRetryAfter`,
  `TestATooManyRequestsFromTheTokenEndpointMovesTheGate`).
- [x] A window whose reset passed more than five minutes ago in an older
  reading is not drawn, on the card or in the drawer; one that passed a
  minute ago is; an account left with none says "out of date", or "needs a
  login" when its login was refused (`TestAWindowThatResetAMomentAgoIsKept`,
  `TestTheDrawerDropsAWindowThatTurnedOver`,
  `TestAnAccountWithEveryWindowTurnedOverSaysWhy`,
  `TestACacheOnlyAccountTurnedOverIsOutOfDate`).

On the machine, the branch's `doctor` over the fixture below:

- [x] `doctor` reports usage as partial, not ok, with a refused login, and
  prints the reason. Desk and Windows VM, 2026-11-14 (E2E step 1).

Against the real token endpoint:

- [ ] A made-up refresh token is answered with a 400 that `Refresh` reads as
  `ErrNeedsLogin`. **Not run**: the endpoint has answered 429 to every
  request from this address since the VM's panel spent a month sending its
  dead token, made-up tokens included. The one refusal seen before the
  limit, on the VM, was a 400; its body was not read.

On a real window (project rule: anything visual is tested on a real window):

- [x] The panel run with a credential store whose token is made up, over a
  cache with old figures: the "needs a login" line is drawn on the account's
  card, and a five-hour window a month gone is not drawn. Desk and Windows
  VM, 2026-11-14 (E2E steps 2 and 3).

## Risks & Assumptions

- Assumes the token endpoint answers a dead refresh token with
  `invalid_grant`, per RFC 6749, in one of the two shapes read. The real
  endpoint's body was not seen (see the open criterion). If it says
  otherwise, a dead token whose access token has expired is reported as "not
  refreshed" with its status, as an aside, and is asked about at every gate:
  today's behaviour, no worse. An access token the usage endpoint refuses
  (401) is "needs a login" either way; that path was run on both machines.
- R8 is per process. A panel and a `doctor` run each send a refused token
  once.
- R1 adds an optional key to the shared cache file. The Python tools ignore
  it; a write of theirs after a failure drops it, and a hayami viewer that
  did not fetch then loses the line until hayami's next fetch.
- R5 hides a window that a reader may have expected to see dimmed. A month-old
  1 % is a wrong figure, not an old one.
- **Rollback**: revert the PR. An older build ignores `login_refused_at`, and
  its next write drops it.

## Threat model delta

B3 (credentials and the shared cache). The SHA-256 of a refused refresh
token is held in memory for the life of the process, never written or
printed. Fewer requests carry a dead token. A hostile token endpoint already
decides what a refresh answers; it can now say "needs a login" with a 400 as
it could with a 401, and set a Retry-After capped at an hour, as the usage
endpoint can. Its error code reaches the panel only as `[a-z_]`, 64 at most.
A writer of the shared cache can now set or clear `login_refused_at`, a time
and nothing else: it shows or hides a line telling the person to sign in,
beside figures that writer already chooses. The B3 section gains the
defences, and "Checked" what the adversarial review tried.

## E2E Test Plan

The fixture: a throwaway home holding a Claude store with a made-up access
token, and a cache holding a reading fetched 32 days ago, with a five-hour
window that reset then and a week that resets in three days. The access token
is made up and unexpired, so the real usage endpoint is asked and answers 401
(`ErrNeedsLogin` from `claude.Usage`). The expired-token variant, which asks
the token endpoint, waits on the rate limit above.

1. Desk and VM: the branch's `hayami-tui doctor` over the fixture (R1, R2,
   R5).
2. Desk, Plasma 6.8 on Wayland: the branch's panel started over the fixture
   after `doctor` had fetched, so it reads the refusal from the cache and
   asks nothing (R1, R2, R5).
3. Windows VM: the same, cross-built, in the signed-in session.

Results, 2026-11-14:

- Step 1: `usage partial CC max 7d: 31 %`, then `! CC max: needs a login`
  with its detail, and `Usage: stale, read 32d ago` as an aside, on both. The
  cache then held `login_refused_at` beside a gate two minutes out.
- Step 2: the AI Usage card shows "CC max · needs a login" in amber above the
  7d meter at 31 %, and no 5h meter. The panel grew by that line (427 to 446
  px). The first run, before R1 recorded the refusal in the cache, drew no
  line: `doctor` had fetched and the panel only read the cache. That run is
  why R1 is in the cache and not only in the process.
- Step 3: the same card on Windows 11 at 150 %.

### Prod visibility

Not applicable: a desktop program with no telemetry.
