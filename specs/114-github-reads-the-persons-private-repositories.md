# 114 — GitHub reads the person's private repositories

**Issue**: #411

## Status: COMPLETE

## Executive Summary

GitHub's contribution list leaves out every private repository, so the
GitHub reader also asks for the person's own private repositories and
counts their default branch's commits by the person on their authored day,
as GitHub counts public ones. Review `privateItems` and the history paging
at the end of `GitHubReader.Read` first.

## Context

On the desk (2026-11-25) the Development drawer's GitHub strip and the day
pages the heatmap opens listed nothing from two of the person's
repositories that had commits that fortnight. Both are private and owned by
the person.

Spec 109 assumed the person's own private repositories "are visible to gh's
`repo` scope and listed like any other". They are not: with gh's sign-in
(scopes `repo`, `read:org`, `gist`), `contributionsCollection` lists no
private repository in any of its lists (commits, pull requests, issues,
repositories created), and counts their work only in
`restrictedContributionsCount` (297 this fortnight), which also counts an
organisation's work that cannot be named. Spec 109 stopped asking for that
count, so the person's private work disappeared entirely.

The work is readable another way. `viewer.repositories(privacy: PRIVATE,
ownerAffiliations: [OWNER])` lists the person's own private repositories,
and each one's default branch history gives its commits. Compared on a
public repository, GitHub's contribution count for a day equals the default
branch's commits by the person on their authored day, merges included; this
spec counts private commits by the same rule.

## Requirements

- R1 The first request also asks for the person's own private repositories
  (owner only, no forks, the 50 pushed to most recently) with the default
  branch's commits in the fortnight, a hundred a page, and the viewer's
  login. An organisation's repositories are not asked for (spec 109's
  decision stands).
- R2 Each day's commits whose author is the person (the commit's author
  resolves to the viewer's account) become one commits item per repository
  and day, dated by the authored day. A commit by anyone else, or by an
  address no account claims, is not counted.
- R3 A private repository created in the fortnight is an item ("created").
  A repository the collection already lists is left to the collection, so
  nothing is counted twice if GitHub starts listing them.
- R4 A history with more than a page is read on after its cursor, sharing
  the reading's bound of 10 requests with the contribution lists.
- Out of scope: pull requests, reviews and issues in private repositories,
  which the collection also leaves out. They need a search query and the
  same organisation question; a separate issue if wanted.

## Acceptance Criteria

- [x] `TestGitHubListsThePersonsPrivateRepositories`: the query asks for
      private, owner-only repositories; a day's commits by the person and
      only theirs, in the window, on their authored day; a repository
      created; a repository the collection lists is not counted twice.
- [x] `TestGitHubReadsAPrivateHistoryToItsEnd`: a three-page history is read
      in three requests after its cursors; an endless one stops at 10.
- [x] Falsified: dropping the private items, the author filter, the
      deduplication or the paging each fail a test; removing the bound
      hangs the endless case.
- [x] `FuzzGitHubResponse` seeded with a private repository; 30 s clean.
- [x] Against the real GitHub, a probe printing totals by repository: the
      two missing repositories listed (56 and 124 commits, the second over
      two pages), with three other private repositories; the private sum
      (263) within the 297 restricted, the rest being the organisation's.
- [x] `make test`, `make lint`.
- [x] Changelog line under `### Unreleased`.

## Risks & Assumptions

- The first request grows: 50 repositories with up to 100 commits each,
  well under GraphQL's node limit; GitHub's point cost rises a little per
  reading.
- Author matching relies on GitHub resolving a commit's address to the
  account, as it does for contributions; a commit by an address the
  account does not claim is not counted here, and is not on GitHub's
  calendar either.
- A person with more than 50 private repositories pushed to in a fortnight
  would see only the 50 most recent.
- Rollback: revert the PR.

## Threat model delta

B4 (what hayami parses) and B5 (what it does outside itself): the GitHub
reading parses repository histories too, and its follow-up requests name a
repository from the answer, still bounded at 10 requests a reading. Both
sections of `docs/threat-model.md` updated.

## E2E Test Plan

The installed panel on the next release: the Development drawer's GitHub
strip, and a day page from the heatmap on 24 November listing the two
private repositories' commits.

### Prod visibility

Not applicable: a desktop program with no telemetry.
