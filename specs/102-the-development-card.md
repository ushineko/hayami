# Spec 102: the Development card

**Issue**: [#360](https://github.com/ushineko/hayami/issues/360)

## Status: INCOMPLETE

## Executive Summary

(Filled in before the PR.)

## Context

The usage card says how much AI a person has used. #360 asks for the
question next to it: what have I been doing in my repositories. The card is
renamed **Development**, and its drawer gains a GitHub part and a GitLab
part under the AI usage part: a fortnight as a strip of cells, a day each,
coloured against the busiest day; a click on a day opens what happened that
day, by repository.

A spike (standalone, against the live APIs and a patched fynedesygn, on the
desk's real window) settled the sources and the shape:

- **GitHub: one GraphQL query.** `viewer.contributionsCollection(from, to)`
  gives the contribution calendar (the profile heatmap's per-day counts), the
  commit contributions by repository by day, and the pull requests, reviews
  and issues opened with their titles: a fortnight in one call, cost 1 of
  5,000 an hour. The REST events list does not do: it stops at 300 events
  (five days for an active account), and its push and pull-request payloads
  no longer carry commit counts or titles. The query is sent through
  go-github's own client (`NewRequest`/`Do` on `graphql`), so there is no
  second GitHub library.
- **GitLab: the contribution events list.** `ListCurrentUserContributionEvents`
  (gitlab.com/gitlab-org/api/client-go) with `after`, paged, plus one project
  lookup per project for its path, cached. Each event counts once, as
  GitLab's own calendar counts: a push's commit count is the rebase's (one
  push read as 1,094 commits), not the work's.
- **The two count different things** (contributions, events), so each strip
  is coloured against its own busiest day.
- **The drawer is as tall as the cards**, never taller. With the AI usage
  part reduced to each account's meters (its lines move to the account's
  page), both strips fit with a sixth of the drawer to spare.
- **One row wider than the drawer cuts off everything in it.** Names and
  values are cut to fixed widths, as the drawers already do.
- **fynedesygn lacked clickable cells.** Its spec 085 (#252, PR #253) adds
  `HeatStrip.SetCellLabels`, `SetSelected`, `CellAt` and
  `Drawer.SetCellTapped`; this spec needs that release.

Decisions taken on #360 (2026-11-19): the git parts are **off by default**
and switched on in Preferences, since each is a new network destination;
#360 is the venue rule 9 asks a Discussion to be (Discussions are off on
this repository); a key entered in Preferences is kept in the settings file,
which is already written 0600.

## Requirements

### The card and its drawer

- **R1** The card's title is **Development**. Its key stays `usage`, so a
  settings file that names it is unchanged. The card itself is unchanged: a
  meter per account.
- **R2** The drawer's overview is up to three parts, in order: **AI usage**,
  **GitHub**, **GitLab**, each shown only when switched on (R12). Rules
  separate them.
- **R3** The AI usage part is each account's heading and meters. The
  account's other lines (plan, extra usage, spend, credits, "Week by use")
  are on the account's own page, which its heading opens; an account with no
  meters keeps its first line in the overview so it is not empty.
- **R4** A git part is a heading, a strip and a sum:
  - the heading names the provider ("GitHub", "GitLab"; never the host,
    rule 9 privacy) and says when it was read ("⟳ 3m ago", "⟳ reading…",
    "⟳ failed"); a click on it reads that part again (R9);
  - the strip is fourteen cells, oldest left, today right, each labelled
    with its day's initial; a day with nothing is an outlined empty cell, a
    day with something is coloured by its count against the fortnight's
    busiest (the least busy day with anything still visibly filled);
  - the sum says the fortnight's total and the busiest day's count
    ("586 contributions, most 146"; "138 events, most 30").
- **R5** A part that cannot be read says why, in a line under its heading,
  as a section's reasons do: no credential (and how to give one), the host
  refused the key, the network failed, rate-limited until a time. The last
  good reading stays, dimmed, under the reason.

### A day

- **R6** A click on a cell opens that day's page: a Back line, the part's
  heading and strip with the day marked, a rule, the date and its count,
  then the day by repository. A click on another cell moves to that day; a
  click on the marked cell goes back to the overview.
- **R7** A repository is a heading, its name without its owner (rule 9: an
  owner is a user name), with its counts in a fixed order and short
  (`24c 10PR 3i 2rev`, `11↑ 10MR +3`); under it one line per thing worked
  on: the reference and title, and the furthest it got that day ("merged",
  "open", "closed", "approved", "pushed ×4"). An MR opened, approved and
  merged in a day is one line, "merged"; GitLab pushes are one line per
  branch.
- **R8** GitHub's private contributions it will not itemise
  (`restrictedContributionsCount`) are counted in the strip and said on the
  day page as one line ("12 private contributions not listed").

### Reading

- **R9** A git part is read when the drawer shows it and its last reading is
  older than the interval (default 10 minutes, a preference: 5, 10, 15, 30,
  60), and when its heading is clicked. It is never read while the drawer is
  closed or the part is switched off: a closed drawer and a hidden part cost
  nothing.
- **R10** GitHub's credential is `gh auth token --hostname github.com`, run
  with a timeout, unless Preferences hold one. GitLab's is, in order: the
  key in Preferences; `GITLAB_TOKEN` or `GLAB_TOKEN` in `~/.secrets.env`;
  `glab config get token --host <host>`. The GitLab host is, in order:
  Preferences; `GITLAB_HOST` in `~/.secrets.env`; the first host in glab's
  config that glab has a token for. `~/.secrets.env` is read as `KEY=value`
  lines (optional `export`, optional quotes), never run by a shell, and only
  the three names above are kept.
- **R11** A token is held in memory for the reading and nowhere else: never
  logged, printed, put in an error, a reason or a cache. Requests go over
  HTTPS only, to `api.github.com` and to the GitLab host's `/api/v4` and
  nowhere else: no redirect is followed, a response is read through a size
  limit, and every request has a timeout.

### Preferences

- **R12** The Development section's preferences: AI usage, GitHub and GitLab
  switches (AI usage on, the git parts off by default); the refresh
  interval; the GitLab host; a GitHub token and a GitLab key, each a
  password field, empty meaning "from gh / ~/.secrets.env / glab". A change
  applies to the running panel without a restart.

### Both shells

- **R13** The terminal panel's drawer shows the same parts. A strip is one
  line of block glyphs (▁…█, `·` for an empty day) over a line of initials;
  ←/→ move the marked day when the strip's line is marked, Enter opens it,
  Esc goes back; a heading's line refreshes on Enter.
- **R14** `hayami-tui detail usage` prints the overview with the strips;
  `hayami-tui detail usage/github/2026-11-17` prints a day. A day page is
  `OnRequest`, so `readings` and `doctor`, which are pasted into bug reports,
  never carry repository names or titles.
- **R15** Titles, branch names and repository names are printed through
  `safetext.Printable`, as every other text from outside is.

## Acceptance Criteria

- [ ] AC1 The card and the drawer say "Development"; a settings file naming
  `usage` still shows it (R1). Unit.
- [ ] AC2 The overview's parts follow the switches, in order, and a switched
  off part is not read (R2, R9, R12). Unit, with a counting fake reader.
- [ ] AC3 The AI usage part carries meters only; the account page carries
  the lines it lost, including "Week by use" (R3). Unit on the view.
- [ ] AC4 GitHub: a recorded GraphQL response becomes fourteen days whose
  counts are the calendar's, items by day and repository, and the private
  count (R4, R8). Unit against a fixture of invented data.
- [ ] AC5 GitLab: recorded event pages become fourteen days counting each
  event once, pushes grouped by branch, MRs at their furthest state, project
  paths looked up once each (R4, R7). Unit against fixtures.
- [ ] AC6 A day page: repositories without owners, counts in the fixed
  order, one line per thing, cut to the drawer's widths (R6, R7). Unit.
- [ ] AC7 Credentials: the order in R10 for each provider, with
  `~/.secrets.env` parsed without a shell and only the named keys kept; no
  token in any error, reason or log line (R10, R11). Unit, with fake
  subprocesses and a temp HOME.
- [ ] AC8 HTTP: no redirect followed, a body over the limit refused, http://
  refused, a timeout honoured (R11). Unit against `httptest`.
- [ ] AC9 Refresh: a reading younger than the interval is reused; a click on
  the heading reads again; a closed drawer reads nothing (R9). Unit.
- [ ] AC10 Reasons: no credential, 401, network failure and a rate limit
  each give their reason, with the last reading kept (R5). Unit.
- [ ] AC11 The terminal drawer draws the strips and moves, opens and leaves
  a day with ←/→, Enter, Esc (R13). Unit on the model.
- [ ] AC12 `detail usage/github/<date>` prints a day; `readings` and
  `doctor` carry no day page (R14). Unit on the CLI.
- [ ] AC13 Integration: against the real GitHub API and the real GitLab
  host, with the desk's credentials, a reading returns fourteen days and the
  counts match the spike's (R4, R10). Skipped without
  `HAYAMI_DEV_E2E=1`.
- [ ] AC14 Real window: the drawer on Plasma at 1.5 shows the three parts
  inside the drawer's width, a real click on a cell opens the day and marks
  it, a click on Back and on a heading do what R6 and R9 say (R4, R6, R9).
  Screenshots, geometry read from them.
- [ ] AC15 Each test falsified by breaking what it covers.

## Threat model delta

Boundaries touched: B3 (credentials), B4 (what hayami parses), B5 (what it
does outside itself), B6 (what it shows), B7 (supply chain).
`docs/threat-model.md` is updated in the same PR, and because B3 and B7 are
touched the PR gets the document's adversarial review.

- **B3.** Three new credentials: gh's GitHub token, a GitLab key, and
  either typed into Preferences. New to the attacker: the settings file now
  may hold a key (0600, the same account's, as `~/.secrets.env` is); a
  reader of `~/.secrets.env` that must take only its keys and never echo a
  line; a configured GitLab host that receives the key. A same-account
  attacker who can rewrite the host could send the key elsewhere, but can
  already read `~/.secrets.env`; https only, no redirects, so a network
  attacker gets neither.
- **B4.** Two new response formats (GraphQL JSON, GitLab events JSON),
  read through a size limit, into typed structs; titles and branch names
  are attacker-chosen text (anyone can open an issue on a public repo) and
  reach the screen only through `safetext.Printable`.
- **B5.** Two new destinations (`api.github.com`, the GitLab host) and two
  new subprocesses (`gh`, `glab`), each run by absolute name from `PATH`
  with fixed arguments and a timeout, never with a shell. Only while the
  drawer shows a part the person switched on.
- **B6.** Repository names (without owners) and titles shown; day pages kept
  out of `readings` and `doctor`.
- **B7.** New modules: `github.com/google/go-github/v93`,
  `gitlab.com/gitlab-org/api/client-go`, and what they bring
  (`golang.org/x/oauth2`, `golang.org/x/time`,
  `github.com/hashicorp/go-retryablehttp` and others, listed in the PR).
  `make vuln` covers them.

`docs/architecture.md` rule 9 names the new destinations and subprocesses,
and says an issue serves where Discussions are off; its Status table gains
the row.

## Risks & Assumptions

- **GitHub's calendar is in the account's zone.** Commit contributions come
  as midnight in the account's zone, written in UTC (08:00Z for UTC-8); the
  date is taken as the day. On this desk the account's zone and the desk's
  match; a person whose do not would see a day shifted. Recorded, not fixed.
- **`contributionsCollection` lists at most 100 per kind**, and 100
  repositories; a fortnight that exceeds that loses detail on the day page
  but not counts in the strip, which come from the calendar.
- **glab's keyring.** `glab config get token` reads glab's config file; a
  token glab keeps in the keyring is not reached, and the reason says to put
  a key in `~/.secrets.env` or Preferences.
- **New dependencies** are larger than the code that uses them;
  client-go is the user's explicit ask (native bindings) and replaces
  hand-written REST.
- **Query budget**: GitHub 1 call per reading; GitLab 1 + pages + one per
  new project per reading. At the 10-minute default with the drawer open
  all day, about 6 calls an hour to each.
- **Rollback**: revert the PR. Settings gain keys an older build ignores;
  the fynedesygn bump is additive.

## Alternatives Considered

- GitHub's REST events list: rejected, 300-event cap and slimmed payloads
  (spike).
- Reading gh's keyring and glab's config directly: rejected, more code and
  breaks when either changes its store; the tools are asked instead.
- Counting GitLab commits from pushes: rejected, a rebase reads as
  thousands.

## E2E Test Plan

On this desk (Plasma 6 Wayland at 1.5), the installed panel replaced by this
build through fynstall, GitHub and GitLab switched on in Preferences:

1. Open the Development drawer: the three parts fit; both strips fill within
   a few seconds (AC14, R4).
2. Click a busy GitHub day: the day page opens with the cell marked; click
   another day, then the marked one: back to the overview (R6).
3. Click the GitLab heading: "reading…", then "now" (R9).
4. Switch GitHub off in Preferences: the part goes, and no request is made
   (R12, R9) — checked with `ss`/the log.
5. `hayami-tui` in a terminal: the same drawer, ←/→ and Enter on a strip
   (R13).
6. `hayami-tui detail usage/github/<a busy day>`; `hayami-tui readings`
   shows no repository name (R14).
7. Windows VM: the drawer opens and the GitHub part reads through gh if
   installed, else says how to sign in (R5, R10).

### Prod visibility

Not applicable: a desktop program; the desk is where it runs.
