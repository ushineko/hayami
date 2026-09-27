# Spec 004: fetching the usage

**Issue**: [#8](https://github.com/ushineko/hayami/issues/8)

## Status: INCOMPLETE

## Executive Summary

(Populated before the PR opens.)

## Context

Spec 003 draws what the shared cache holds and never fills it, so a machine
that never ran the Python shows an empty usage section. This spec makes hayami
the process that goes upstream when the gate is open, and — because opening the
credential store is what makes it possible — also gives the section the plan
badge and the density the monitor has.

**This is the only code in the project that writes outside its own
directories.** An OAuth refresh puts a new token into Claude Code's own store.
That is correct — the token belongs there and every other reader expects to
find it — and it is also the one operation here that can break something a
person needs. It lands in its own commit, behind its own criteria.

### What the Python does, read out of it

| | |
|---|---|
| Default store | `~/.claude/.credentials.json`, called `max` on screen because `claude-max` reaches it |
| Profile stores | `~/.claude-credentials/<name>/.credentials.json`, the convention the wrapper scripts establish; root overridable for tests |
| Plan | `subscriptionType` in the credential file. The usage API does not report it, and a store whose token has expired should still be labelled |
| Usage | `GET https://api.anthropic.com/api/oauth/usage`, `Authorization: Bearer`, `anthropic-beta: oauth-2025-04-20` |
| Refresh | `POST https://console.anthropic.com/api/oauth/token`, `{grant_type, refresh_token, client_id}`; 401 and 403 are permanent |
| Retry-After | taken from the response header and handed to the cache's gate |
| Codex | `codex app-server --stdio`, JSON-RPC: `initialize`, `initialized`, then `account/rateLimits/read` with `excludeResetCreditDetails` |

Claude Code chooses its store with `CLAUDE_SECURESTORAGE_CONFIG_DIR`, which
relocates the credential file only. That variable is per-process, so a running
panel cannot ask the system which profiles exist and discovery keys on the
directory convention instead.

### Why the section changes shape here

Side by side with the monitor, the difference is density. The monitor gives an
account one line and one bar: the bar is the five-hour window, and the caption
carries the seven-day one as text. hayami spends a meter per window, so two
accounts and Codex become six meters and roughly twice the height, in a panel
260 px wide. The monitor is right, and the change belongs here because the
badge that completes the line needs the credential store open.

## Requirements

### Fetching

- R1 Credential stores are discovered by the directory convention: the default
  store and one per profile, with the profile root overridable.
- R2 A store yields an access token, a refresh token and a plan. A store that
  cannot be read is one account absent, not a failed section.
- R3 Claude usage is fetched with the access token, and a `Retry-After` is
  carried into the cache's gate.
- R4 An expired token is refreshed and **the new token is written back** to the
  store it came from, atomically, preserving every field this build does not
  know. A store that did not fully parse is never written.
- R5 A permanent failure — 401 or 403 — is not retried on the next poll. It
  pushes the gate like any other failure and says the account needs a login.
- R6 Codex usage is fetched through the app-server, and the absence of the
  `codex` executable is a section that is not drawn rather than an error.
- R7 No token is ever logged, printed, or included in `--readings`.

### Drawing

- R8 One meter per account: the bar is the window nearest its limit, and the
  caption carries the others.
- R9 A plan badge from `subscriptionType`, beside the account's name.
- R10 A window longer than a day shows the date it resets; a shorter one shows
  a countdown.

## Acceptance Criteria

- [ ] AC1 Stores are found under a root a test sets, including the default one, and named as the Python names them. (R1)
- [ ] AC2 A credential file missing or malformed leaves that account out and the others in. (R2)
- [ ] AC3 A fetch sends the bearer token and the beta header, against a test server. (R3)
- [ ] AC4 A 429 with `Retry-After` reaches the cache's gate. (R3)
- [ ] AC5 A refresh writes the new token back, and a field the test put in the file that this build does not know is still there afterwards. (R4)
- [ ] AC6 A refresh that fails leaves the store exactly as it was, byte for byte. (R4)
- [ ] AC7 A 401 is recorded as needing a login and does not fetch again before the gate. (R5)
- [ ] AC8 With no `codex` executable the section is absent, with no error shown. (R6)
- [ ] AC9 No test can find a token in anything the program writes or prints. (R7)
- [ ] AC10 An account with two windows draws one meter, its bar the nearer to its limit. (R8)
- [ ] AC11 The badge appears beside the name and comes from the file, not the API. (R9)
- [ ] AC12 A monthly window shows a date and a five-hour window shows a countdown. (R10)

## Risks & Assumptions

- **Writing a credential store is the risk in this project.** The write is
  atomic through a temporary file and a rename, the parse is whole-file, and a
  store that did not parse is never written — the file is Claude Code's, and
  the failure to avoid is leaving it with fewer fields than it had.
- **A token is not ours to move.** It is read, used and written back to the
  same file. It goes nowhere else: not to a log, not to the cache, not to
  `--readings`, which AC9 exists to prove rather than assert.
- **The endpoints belong to somebody else.** They are transcribed from a
  program that uses them today, and a change to them is a change this
  repository will find out about the way any client does.
- **The app-server is a subprocess with a protocol.** It is given a deadline
  and killed if it misses it, because a panel that hung on a JSON-RPC read
  would take its whole poll loop with it.
- Rollback: revert. Until this lands the section reads the cache the Python
  fills, which keeps working either way.

## Alternatives Considered

- Fetching without writing the refreshed token back. Rejected: every other
  reader expects the store to hold a usable token, and a hayami that refreshed
  privately would leave the widget refreshing again a minute later.
- Reading the plan from the usage API. It is not reported there, and a store
  whose token has expired should still be labelled.
- Keeping a meter per window. Rejected on the evidence of the two panels side
  by side; see Context.
