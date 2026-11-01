# 073 — a threat model

**Issue**: #240

## Status: INCOMPLETE

## Context

hayami began as a glance panel that read batteries and rates as the person
at the desk, and asked for nothing. It now ships:

- **Components with more than the user's rights.** A Windows service running
  as Local Service (spec 061), and a Linux systemd service with `CAP_BPF`,
  `CAP_NET_ADMIN` and `CAP_PERFMON` that loads eBPF programs into the kernel
  (spec 072). Both are installed by an elevated or sudo step, from a copy of
  `hayami-tui`.
- **Installers that run elevated** on Windows, and install a udev rule and
  LibreHardwareMonitor when asked (specs 036, 063).
- **Credentials.** OAuth tokens for Claude and Codex, refreshed by hayami
  and kept in a usage cache that other programs read and write
  (`internal/claude`, `internal/usage`).
- **Other accounts' information**: the network by process for every
  account, peer addresses and the names they resolve to, process names and
  trees, and a button that ends processes (specs 060, 065, 066, 072).
- **Release binaries** that people install with administrator or root
  rights, built by GitHub Actions from Go modules and, since spec 072, a
  committed compiled BPF object.

Security review today is Ralph's per-change pass: a dependency scan, an
OWASP Top 10 reading of the diff and a secrets scan. The OWASP list is
written for web applications. It does not ask who can become the helper,
what a planted file does to an installer, where a token can leak, what a
tampered release would do or whether a hostile `data.json` can take the
panel down. Those questions are answered today, where they are answered at
all, in the Risks section of whichever spec touched the code, and nothing
holds the answers together or checks that they stay true.

Measured on `main` at ac5caf3: no fuzz test exists; the workflow's Actions
are pinned by tag, not by commit; release artifacts carry SHA256SUMS and no
provenance; there is no `SECURITY.md` and no private way to report a
vulnerability; no secret scanner runs in CI.

This spec sets up an adversarial security model: the threat model as a
document held to the way `docs/architecture.md` is, a gate that makes a
change at a boundary say what it changed, and a review that has to show an
exploit rather than voice a worry, run once over the boundaries that hold
rights and credentials. Two specs follow from it: 074 fuzzes what hayami
parses (B4), and 075 hardens the supply chain and releases (B7).

## Requirements

### The model

- R1 **`docs/threat-model.md`**, one section per trust boundary. Each says:
  the assets; the attacker, named and with what they already have; what the
  code assumes; the defences in place, each with the file that makes it;
  the risks accepted and why; and what is not yet known. The boundaries:
  - **B1 Privileged helpers.** The Windows service and the Linux service:
    their programs, sockets and pipes, the protocol, the programs loaded
    into the kernel. Attacker: another local account, or a process running
    as the user.
  - **B2 Installing, upgrading and removing.** fynstall's installers,
    `install.sh`, `install_windows.ps1`, `network-helper install`, the copy
    into Program Files and `/usr/lib`, the udev rule, LibreHardwareMonitor's
    setup. Attacker: a local user or a planted file racing an elevated step.
  - **B3 Credentials.** The Claude and Codex tokens, their refresh, the
    shared usage cache and its lock. Attacker: malware running as the user;
    another program writing the cache; anyone who sees hayami's output.
  - **B4 What hayami parses.** The helper's protocol, LibreHardwareMonitor's
    `data.json` over loopback, HID reports and sysfs (through sanshoku),
    `/proc`, `sock_diag`, `nvidia-smi`'s output, the providers' API
    responses, settings files. Attacker: whoever controls the source.
  - **B5 What hayami does outside itself.** Ending processes, the KWin rule
    and script, autostart entries, the udev rule. Attacker: a process that
    can make a name, a PID or a path mean something else.
  - **B6 What hayami shows and prints.** The window, the pane, `doctor`,
    `readings` and `detail` as text and JSON, logs, screenshots. Peer host
    names, process names, account names, usage figures. Attacker: anyone who
    sees the screen, a shared terminal or a pasted bug report.
  - **B7 Supply chain and releases.** Go modules, the committed BPF object,
    GitHub Actions, the release workflow and its artifacts, the installer
    binaries. Attacker: a compromised dependency, Action, or maintainer
    account.
- R2 The document opens with the project's security stance in a few lines
  (what hayami will and will not do with rights it holds) and a table of
  boundaries with their status, as `docs/architecture.md` has for its
  mechanisms.
- R3 **`SECURITY.md`**: how to report a vulnerability privately (GitHub's
  private vulnerability reporting, turned on for the repository), what is in
  scope (the boundaries of R1), and what to expect in reply.

### Keeping it true

- R4 **The gate.** A spec that touches a boundary of R1 carries a section
  "Threat model delta": which boundary, what changed for the attacker, and
  the document's section updated in the same PR. "No boundary touched" is
  an answer, and is given. The rule goes into `.claude/CLAUDE.md` and
  `CONTRIBUTING.md`, and the PR template asks it.
- R5 **Adversarial review**, written down in the document as a procedure,
  for a PR that touches B1, B2, B3 or B7:
  - The reviewer is given the changed code and the boundary's attacker, and
    not the author's reasoning or the spec's Risks section, so that the
    review is not anchored on the author's view.
  - The reviewer is asked for exploit paths: preconditions, steps, and
    what the attacker gains. A worry without a path is not a finding.
  - A second, separate pass tries to refute each path against the code. A
    path that survives is a finding; one refuted is recorded as checked.
  - Every finding becomes an issue labelled `security` (a private advisory
    while it is unfixed) and, once fixed, a regression test that fails when
    the fix is broken.
  This runs alongside Ralph's per-change security pass, which stays.
- R6 **A red-team exercise each release cycle that ships a change to B1,
  B2 or B7**, and at least once a quarter otherwise: one boundary, a fresh
  checkout, a virtual machine, and the goal of a working proof of concept.
  The document keeps a log of exercises: date, boundary, what was tried,
  what was found.

### The first assessment

- R7 The procedure of R5 is run once over B1, B2 and B3 as they stand,
  before this spec is complete: both helpers, every install path, and the
  token and cache code. What survives refutation is filed; what is refuted
  is recorded in the document as checked, so the next review does not
  repeat it.

## Acceptance Criteria

- [ ] `docs/threat-model.md` exists with the stance, the boundary table and
      a section for each of B1–B7 carrying every heading of R1, and every
      defence it names points at a file that exists.
- [ ] `SECURITY.md` exists, private vulnerability reporting is on, and a
      test report filed through it reaches the maintainer.
- [ ] `.claude/CLAUDE.md`, `CONTRIBUTING.md` and the PR template carry the
      gate of R4.
- [ ] The procedure of R5 is in the document, and the first assessment of
      R7 is run: its findings filed as issues, its refuted paths recorded
      as checked, each with the boundary and the code it was tried against.
- [ ] The red-team log of R6 exists, with the first assessment as its first
      entry.
- [ ] B4's and B7's sections name specs 074 and 075 as where their
      defences are being added.

## Threat model delta

This spec writes the model; every boundary is touched in that it is written
down. No code that runs on a desk changes, apart from fixes the first
assessment brings, each its own issue.

## Risks & Assumptions

- **A document that drifts is worse than none**: it says something is
  defended that no longer is. R4's gate and R5's procedure are what keep
  it true, and both depend on being followed. The defences in the document
  point at files, so a reader can check.
- **Writing the model down publishes it.** The repository is public, and
  the document tells an attacker where the edges are. That is accepted: the
  code is public already, and the document's value is to the maintainer and
  to reviewers. Findings not yet fixed are kept out of it and in private
  advisories until they are.
- **AI-assisted adversarial review finds plausible paths that are not
  real.** R5's refutation pass exists for that; a finding is what survives
  it. It also misses what a person would find, which is why R6 asks for a
  proof of concept on a machine.
- Rollback: documents only; reverted as one commit.

## Decisions

- **A `security` label**, beside `bug`, `enhancement`, `chore` and `docs`,
  for R5's findings, so they can be found again. Made on GitHub
  (2026-10-31); `.claude/CLAUDE.md`'s label list names it.
- **Split in three.** Fuzzing is spec 074 (#241) and the supply chain spec
  075 (#242); this spec keeps the model and the first assessment.

## E2E Test Plan

1. File a test report through private vulnerability reporting; it arrives
   and is visible only to the maintainer. (AC 2)
2. Open a PR touching `internal/nethelper`; the template asks the gate's
   question. (AC 3)

### Prod visibility

Not applicable: a desktop program with no production deployment.
