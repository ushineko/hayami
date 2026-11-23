# 111 — a change on a day's page opens its details

**Issue**: #390

## Status: COMPLETE

## Executive Summary

A line on a Development day's page -- a pull request, merge request, issue,
push or repository -- opens a page of its own, a drill under the day. A pull
or merge request's page is its state, times, branches, size, checks,
reviews, labels, comments, description, commits and files; an issue's its
state, times, labels, comments and description; a push or a repository's
day its commits. The page is read once per click in the background and kept
for the refresh interval. Review `findLocked` in
`internal/panel/devchange.go`, `validRef` and the GitLab panic guard in
`internal/forge/change.go` first.

## Context

The Development drawer's day pages (spec 102) list what was done, one line a
thing, and Recent (spec 107) lists the latest days the same way. A line said
a reference, a title and how far it got, and nothing more. On the desk
(2026-11-22) the person asked for a click on a change to drill down again to
its details, with the whole drawer to say them in. Asked, they chose the
full page with its description, and for pushes and a repository's lines to
list their commits.

## Requirements

- R1 Every line of a day's page that names something with a page opens it:
  a pull request, a review (its pull request), an issue, a merge request, a
  comment (its merge request or issue), a push (its branch's pushes that
  day), a repository (its day's commits). The same lines in Recent open the
  same pages. A repository created opens nothing. The page's ID is the day's
  and a key, a hash: no repository or title is in an ID.
- R2 A pull or merge request's page: its reference and repository with its
  state as the heading; its title in full; opened, merged or closed; source
  and target branches; lines added and removed, coloured apart; files;
  commits; checks; reviews (approvals, changes requested, and the forge's
  verdict while it is open); labels; comments. Then its description, up to
  150 lines, its commits (short hash, title, lines) and its files (path kept
  at its end, status, lines). An issue's page is its heading, title, times,
  closing reason, labels, comments and description.
- R3 A push's page and a repository's day are the commits made that day: on
  GitHub the viewer's on the default branch, on GitLab on the branches
  pushed, each once, newest first, bounded by the day and not by a push's
  count (a rebase pushed counts every commit on the branch). Their totals
  leave merges out.
- R4 The page is drawn at once from what the day's line said, marked
  "reading…", and again when the reading lands; it is read once per click
  and kept for the refresh interval. It reads only what the forge's last
  reading listed: the key is looked up among that day's items. A page opened
  before the fortnight is read waits for it. Its way out is its day.
- R5 Every text from a forge reaches the page through `clean`; a
  description keeps its lines, each cleaned. GitLab's Markdown heading,
  code and bold marks are taken out, as GitHub's bodyText has them.
- R6 A repository path that is not one (segments of letters, digits, dots,
  dashes and underscores, none only dots) or a number that is not one is
  refused before any request carries the key. A panic in a client library's
  decoder, or anywhere in a forge's background reading, is that reading's
  error, not the end of the program.
- R7 The command line's `detail` prints a change's page, waiting for the
  fortnight and then the change.

## Acceptance Criteria

- [x] Forge: `TestGitHubReadsAPullRequest`, `TestGitHubReadsAnIssue`,
      `TestGitHubReadsADaysCommits` (a merge left out of the totals),
      `TestGitLabReadsAMergeRequest` (diffs counted over two pages, Markdown
      marks out), `TestGitLabReadsAnIssue`, `TestGitLabReadsADaysPushes`
      (each commit once, bounded by the day), `TestABadChangeIsNotRead` (no
      request made), `TestAnAnswerThatPanicsTheLibraryIsAnError`,
      `TestAChangeFailureIsTyped`.
- [x] View: `TestWhatALineOpens`, `TestAChangeIsADrillUnderItsDay`,
      `TestADaysLinesOpenTheirPages`, `TestAChangeBeingReadSaysWhatItsLineSaid`,
      `TestAChangeReadIsItsWholePage`.
- [x] Panel: `TestALineOpensItsChange` (no repository in the ID, way out its
      day, read once and again after the interval, text cleaned),
      `TestARepositorysLineOpensItsDay`,
      `TestAChangeNoLongerListedReadsNothing`.
- [x] Falsified: a line without its Open, `validRef` passing everything, the
      panic guard removed, and the cache read on every poll each fail a test.
- [x] Real forges from the command line: a merged pull request's page whole;
      a repository's day of four commits; a GitLab merge request's page with
      its 25 files; a GitLab branch's day of 14 commits, where the first
      try's push counts gave 100 commits and 400,000 lines (R3).
- [x] Real window on the desk (Plasma 6.8 Wayland, scale 1.5, a copy of the
      settings): a pull request opened from Recent, its heading, facts,
      description and, scrolled, its commits and files; a GitLab merge
      request opened from its day, lines added and removed coloured apart.
- [x] `make test`, `make lint`; changelog line; README and threat model.

## Risks & Assumptions

- A page is one request for GitHub's pull request or issue, two for a
  repository's day, and for GitLab up to six for a merge request (itself,
  approvals, commits, three pages of diffs) or one per push of the day. Only
  on a click, kept for the interval.
- A description can name people or hosts; it is the person's own work, shown
  only in the drawer and `detail`: a change's page is a drill, never listed,
  so `readings` and `doctor` never print one.
- GitLab gives no totals for a merge request; the lines are counted from its
  diffs, and a diff GitLab collapsed counts nothing.
- Rollback: revert the PR.

## Threat model delta

B4 (what hayami parses): new answers -- a pull request, an issue, a merge
request, its approvals, commits and diffs -- decoded into typed values,
bounded by `forge.MaxBody` and by the page's own limits, every text cleaned.
A repository path is checked before it goes into a request. client-go's
issue decoder was found to panic on an answer without an id; a forge's
background reading now turns a panic into its error. B5: requests on a
click, to the same two hosts. B6: a change's description, commit titles and
file paths are drawn, in a drill only. B3: the keys are found and held as a
fortnight's reading does and sent only to the forge they belong to. The
document's B4, B5 and B6 sections updated; the adversarial review recorded.

## E2E Test Plan

The installed release: open the Development drawer, a day, and a pull
request on it; the page shows the facts, description, commits and files,
and Back returns to the day.

### Prod visibility

Not applicable: a desktop program with no telemetry.
