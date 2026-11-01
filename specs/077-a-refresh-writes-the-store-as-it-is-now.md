# 077 — a refresh writes the store as it is now

**Issue**: #248

## Status: COMPLETE

## Executive Summary

hayami refreshed an expired Claude token and wrote the credential file back
from the copy it had read before the request, up to ten seconds earlier, so
anything Claude Code wrote meanwhile was lost: its own refreshed tokens, or
keys of its own. It now reads the file again just before writing: if the
refresh token there is not the one hayami used, Claude Code refreshed and
its tokens stand; otherwise hayami's tokens go into the file as it is now.
Look first at `writeBack` in `internal/claude/refresh.go`.

## Context

Found by spec 073's first assessment, boundary B3, and confirmed by its
refutation pass (#248, medium-low). The race is met in ordinary use: hayami
and Claude Code both refresh when the token expires. With rotating refresh
tokens, the loser of two refreshes from the same token can be left holding
a dead one.

Three ways to fix it were weighed (Alternatives Considered); the person who
owns the project chose to re-read and compare.

## Requirements

- R1 Before writing refreshed tokens, `Refresh` reads the store again.
- R2 If the refresh token in the store is not the one the refresh used,
  another program refreshed meanwhile: the store is not written, and the
  caller's credentials become the store's.
- R3 Otherwise the access token, refresh token and expiry are set into the
  store as it is now, keeping every key in it, including any written during
  the request.
- R4 A store that is gone when read again is not written, and the refresh
  is an error. A store that cannot be read or parsed is read again, up to
  five times twenty milliseconds apart (another program part-way through
  writing it); if it still cannot be, the refreshed tokens are written over
  the copy read before the request, as every earlier version did. They are
  the only usable ones: the refresh token just spent may not be accepted
  again.

## Acceptance Criteria

- [x] A store whose tokens another program changed during the request is
      left with that program's tokens, and the caller goes on with them.
      (`TestARefreshDefersToOneThatHappenedMeanwhile`.)
- [x] A key another program added during the request is kept, and the
      refreshed tokens are written. (`TestARefreshKeepsWhatWasWrittenMeanwhile`.)
- [x] A store removed during the request is not recreated.
      (`TestARefreshDoesNotRecreateARemovedStore`.)
- [x] A store emptied during the request and whole again thirty
      milliseconds later gets the refreshed tokens.
      (`TestARefreshWaitsOutAWriteInProgress`.)
- [x] A store that stays unparseable still gets the refreshed tokens, so the
      new refresh token is not lost.
      (`TestARefreshKeepsItsTokensWhenTheFileStaysUnreadable`.)
- [x] The tests fail with their part of the fix taken out: the first three
      with the write from the earlier copy put back, the fourth without the
      retry, the fifth with an unreadable store not written.
- [x] The existing refresh tests pass: the new token is written, every key
      is kept, a failed refresh leaves the file byte for byte.
- [x] `make test` and `make lint` pass.

## Threat model delta

B3: a refresh no longer reverts what Claude Code wrote to its credential
file. The window is now the time between a read and a rename, not a network
round trip. `docs/threat-model.md` moves #248 from open findings to defences
once it is on `main` (#252).

## Risks & Assumptions

- **A window remains.** Between the second read and the rename, a write by
  Claude Code can still be lost. It is milliseconds rather than seconds, and
  closing it would mean taking Claude Code's lock (below).
- **Two refreshes can still happen.** If both programs send the same refresh
  token, the server sees it twice; if it rotates refresh tokens and refuses
  the second use, hayami's refresh fails and the next poll reads the token
  Claude Code wrote. This change makes sure hayami never overwrites the
  winner, which is the part it controls.
- **The comparison is on the refresh token.** A program that changed the
  access token and kept the refresh token is not detected, and its access
  token is replaced with a newer one from the same refresh token. Nothing
  known writes the file that way.
- **Review of this change** (spec 073's procedure, one reviewer, findings
  checked against the code by the author). Fixed here: a second read that
  failed threw the refreshed tokens away, which with single-use refresh
  tokens logs the account out (R4 as it now stands). Accepted: the
  read-to-rename window (above); and that a different refresh token in the
  file is trusted, even if it is stale or another account's, since the file
  is the user's and Claude Code's to change.
- Rollback: revert the commit; the file format is unchanged.

## Alternatives Considered

- Take Claude Code's own lock: closes the window entirely, but depends on an
  undocumented scheme that could change under hayami. Rejected.
- Never write the file, leaving refresh to Claude Code: simplest, but an
  expired token would mean no usage until Claude Code next ran. Rejected.

## E2E Test Plan

Not applicable as a separate step: each criterion is a test against a real
file and a real HTTP server, with the file changed during the request the
way Claude Code changes it. After release, daily use on this desk is the
check: the account's reading continues across a token expiry, and Claude Code
stays logged in.

### Prod visibility

Not applicable: a desktop program with no production deployment.
