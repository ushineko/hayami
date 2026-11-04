# 082 — a recorded plan outlives the record

**Issue**: #286

## Status: SUPERSEDED by spec 083 (#288)

Merged as #287 and shipped only in the 0.11.16-rc.2 pre-release. The memory
could only keep a name a record had shown, and none was showing, so the
desk check below was never run; spec 083 replaced the memory.

## Executive Summary

An Ultra account was called "CC max" again once another account used Claude
Code: the plan's name comes from Claude Code's account record, which
describes whichever account refreshed it last. The panel now remembers the
plan the record named for each credential store, in `claude-plans.json`
beside its other cache files, and forgets it when the store's own
subscription type or tier changes, or the store is removed. Look first at
`planOf` in `internal/panel/plans.go`.

## Context

#235 (#234, 0.11.9, without a spec) called the default store by the plan
`~/.claude.json` names ("Ultra"), believed only when the record's rate-limit
tier is the store's: an Ultra store's credential file still says
`subscriptionType: max`. There is one record for every profile; the wrappers
relocate only the credential file. Once an enterprise profile has used
Claude Code, the record carries that account's tier and no plan name, the
check fails, and the label falls back to "max". The check is right; the
plan was not kept anywhere.

The credential store names no account (its fields are the tokens, their
expiry, `subscriptionType`, `subscriptionTypeSource`, `rateLimitTier` and
`scopes`), so a remembered plan can only be keyed by the store.

## Requirements

- R1 When the record vouches for a store, the plan it names is remembered
  for that store, with the store's subscription type and tier at the time.
- R2 When it does not, a store is called by its remembered plan while its
  subscription type and tier are still the remembered ones.
- R3 A store whose type or tier differs from the remembered ones forgets
  the plan; the old values coming back do not restore it. The record
  vouching again does.
- R4 A store that is gone forgets its plan. A store that cannot be read
  this poll keeps it.
- R5 The memory is `claude-plans.json` in the panel's cache directory,
  written atomically and only when it changed. It holds no token. A
  missing, unreadable or malformed file is a memory of nothing.
- R6 The badge follows the same plan as the label, for every store; only
  the default store is renamed, as before, and not to a name a profile
  already has.

## Acceptance Criteria

- [x] R1, R2, R6: with the record naming Ultra the default store is
      "CC ultra" with U; with the record then another account's, it stays
      so, in the same panel and in a new one.
      (`TestARecordedPlanOutlivesTheRecord`.)
- [x] R3: the store's tier changing gives "CC max" with M; the old tier
      back stays "CC max"; the record vouching again gives "CC ultra".
      (Same test.)
- [x] R4: a profile store removed and made again with the same type and
      tier is badged M, not U. (`TestARemovedStoreForgetsItsPlan`.)
- [x] Each test fails with its part of `planOf` or `rememberPlans` broken:
      the memory never used, never forgotten, forgotten but kept on file,
      and never pruned.
- [x] #235's test passes unchanged. (`TestTheDefaultStoreIsCalledByItsRecordedPlan`.)
- [x] R6: a profile named `ultra` beside an Ultra default store keeps its
      caption, and the default store is "CC max", both badged U.
      (`TestAPlanNameAProfileHasIsNotTaken`; fails with the check removed.)
- [x] B3's adversarial review run; what survives filed, what is refuted under
      "Checked". (Two paths, both refuted; the caption collision held as a bug
      and is fixed here, R6.)
- [ ] On the desk, after the merge: the default store is "CC ultra" while the
      record describes the enterprise account. (E2E 1, 2.)

## Threat model delta

B3. The panel writes a new file derived from the credential stores: each
store's name, plan, subscription type and rate-limit tier, in its own cache
directory. The file is created `0600` (`os.CreateTemp`, then a rename); the
directory is made `0700` when it is absent, and an existing one keeps its
mode, which on the desk is `0755`. No token goes into it. What a writer of
that file can do is change a label and a badge: it chooses no token, no
store, and no cache file, which are still keyed by the store's name. B3's
defences are unchanged; the review is in the document's red-team log and
"Checked", updated in the same PR.

## Risks & Assumptions

- A different account logged into the same store with the same type and tier
  keeps the old plan's name until the record vouches again. A Max 20x login
  has an Ultra store's type and tier, so this is the case that cannot be told
  apart; the store holds nothing that could.
- The memory is read on every poll and on opening the drawer: a file of a
  few hundred bytes. It is written only when it changes.
- A store that drops out of the listing for one poll (its file missing when
  stat'ed) forgets its plan, and gets it back when the record next vouches.
- Rollback: revert the commit. A left-over `claude-plans.json` is read by
  nothing.

## E2E Test Plan

1. On the desk, with this build installed and the record describing the
   Ultra account (after using Claude Code on it): the panel draws "CC ultra"
   and `~/.cache/hayami/claude-plans.json` holds the plan, and no token.
   (R1, R5)
2. Use Claude Code on the enterprise profile, so the record is that
   account's: the panel still draws "CC ultra" with U, before and after a
   restart. (R2)

### Prod visibility

Not applicable: a desktop program with no production deployment.
