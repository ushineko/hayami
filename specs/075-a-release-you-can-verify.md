# 075 — a release you can verify

**Issue**: #242

## Status: INCOMPLETE

## Context

Spec 073's boundary B7 is the supply chain: what goes into a release and
what proves a download is that release. People install hayami's releases
with administrator or root rights, and since spec 072 one of them installs
a service with kernel capabilities, so a tampered release is the worst case
the project has.

Measured on `main` at ac5caf3:

- `.github/workflows/build.yml` uses `actions/checkout@v5`,
  `actions/setup-go@v6`, `actions/upload-artifact@v4`,
  `actions/download-artifact@v4` and `softprops/action-gh-release@v2`, all
  pinned by tag. A tag can be moved to other code by whoever controls the
  Action's repository.
- The workflow declares `permissions` at its top and for one job; the rest
  inherit.
- A release carries SHA256SUMS, made by the same workflow. It shows a
  download is whole, not that the workflow built it from this repository.
- No secret scanner runs in CI.
- Since spec 072 a compiled BPF object is committed
  (`internal/netcount/netcount_bpfel.o`). A test ties it to its source's
  hash, which catches a source edited without rebuilding, not an object
  built from other source on purpose.

## Requirements

- R1 **Pinned Actions.** Every `uses:` in `.github/workflows` names a full
  commit SHA, with the version in a comment. A CI step fails the build on
  one that does not.
- R2 **Dependabot** keeps the Actions current (`.github/dependabot.yml`,
  ecosystem `github-actions`, weekly), and Go modules too (`gomod`, weekly,
  grouped so a week is one PR).
- R3 **Least privilege.** Each job declares the `permissions` it needs, the
  workflow's default is none, and only the release job may write contents
  (and `id-token` and `attestations` for R4).
- R4 **Provenance.** The release job attests every artifact it publishes
  (`actions/attest-build-provenance`), and the README's install section
  says how to check a download with `gh attestation verify <file> -R
  ushineko/hayami`. The installers say nothing new: a person verifies
  before running them.
- R5 **Secret scanning.** gitleaks runs over each PR's commits, with an
  allow-list for the documentation ranges and invented fixtures the
  public-repository rules require.
- R6 **Scorecard.** The OpenSSF Scorecard action runs on `main` weekly and
  publishes its result; the README links it.
- R7 **The BPF object.** A PR that changes `netcount_bpfel.o` says so; the
  PR template asks; a reviewer rebuilds it with `make bpf` and notes the
  difference. CI warns on a PR that changes the object without changing
  its source.
- R8 `docs/threat-model.md`'s B7 section records each defence and what is
  still open (reproducible builds are).

## Acceptance Criteria

- [x] Every Action is pinned by SHA, and the check fails a branch that pins
      one by tag.
- [ ] Dependabot is configured for Actions and Go modules, and has opened
      its first PR.
- [x] Every job declares `permissions`, the default is none, and only the
      release job writes.
- [ ] A pre-release tagged from the branch carries attestations that
      `gh attestation verify` accepts for each artifact, and the README
      says how.
- [x] gitleaks fails a branch with a planted fake key and passes `main`.
- [ ] The Scorecard action has run on `main` and the README links its
      result.
- [ ] The PR template asks about the BPF object, and CI warns on an object
      changed without its source.
- [x] B7's section of the threat model records the defences.

## Verification

- Pins: every `uses:` in both workflows names a 40-hex commit with its
  version in a comment. The `pinned` job's check, run locally, prints
  nothing on the branch and catches a planted `actions/checkout@v5`.
- Permissions: both workflows declare `permissions: {}`; every job
  declares its own. Contents are written only by `release`, which alone
  holds `id-token` and `attestations`; `scorecard` writes security events
  (and needs an OIDC token to publish). `secrets` reads pull requests,
  which gitleaks needs to list a PR's commits.
- E2E, PR #265 (a draft from a throwaway branch, closed and deleted): one
  `uses:` pinned by tag and a random, never-issued `ghp_` string in a Go
  file. `Actions are pinned by commit` failed naming
  `security.yml:26 actions/checkout@v5`; `No secrets in the commits`
  failed with `github-pat` in `tmpplant/plant.go`. On #264 itself, both
  pass.
- The PR template already asks about the BPF object (spec 072);
  the `bpf` job adds the warning.
- B7 in `docs/threat-model.md` lists the defences and what stays open.

## Threat model delta

B7: Actions move from tag pins to SHA pins, releases gain provenance, and
secrets and the BPF object gain checks. Nothing that runs on a desk
changes.

## Risks & Assumptions

- **A broken attestation step must not leave a release half-published.**
  The release job is tried on a pre-release tag first; attestation runs
  before the release is created.
- **Dependabot PRs are maintenance.** Grouped weekly to keep them to one or
  two.
- **gitleaks false positives** on fixtures (documentation MAC ranges,
  invented tokens) are allow-listed by path and pattern, not by turning
  rules off.
- **Provenance proves the workflow built the artifact**, not that the code
  is sound; it is one defence of B7, not the boundary.
- Rollback: CI configuration; each change is reverted on its own. A
  release already attested stays attested.

## E2E Test Plan

1. Push a branch with one Action pinned by tag and a planted fake key; CI
   fails on both. (AC 1, AC 5)
2. Tag a pre-release from the branch; download an artifact and run
   `gh attestation verify` on it. (AC 4)
3. Open a PR that changes only the BPF object; CI warns. (AC 7)

### Prod visibility

Not applicable: a desktop program with no production deployment.
