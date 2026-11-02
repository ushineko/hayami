# 079 — a release is built apart from what publishes it

**Issue**: #268

## Status: INCOMPLETE

## Context

The adversarial review of #264 (`docs/threat-model.md`, B7) found that the
job that builds a release also ran other things, and that the job that signs
a release also built it. Provenance from spec 075 says which workflow signed
a file; it is worth what the signing job can trust.

## Requirements

- R1 `build-linux` and `build-windows` build everything a release publishes.
  They run no tests, no linter and no fuzzer.
- R2 Those jobs read no cache: `cache: false` on `setup-go`, and no other
  cache Action.
- R3 Those jobs hold `contents: read` only, and keep no token after checkout.
- R4 The `release` job holds the workflow's only write rights. It uses only
  `checkout`, `download-artifact` and `attest-build-provenance`, and its `run`
  steps run nothing from the repository or its modules. It keeps no token
  after checkout.
- R5 The release is created as a draft and published afterwards, with `gh`,
  so it keeps working when immutable releases are on. `softprops/action-gh-release`
  goes: one fewer third-party Action holding the write token.
- R6 A tag without a suffix must name a commit on `main`. A tag with a suffix
  is a pre-release and may be tried from a branch.
- R7 A structural test holds the workflow to R1 to R4, and is falsified
  against planted workflows.
- R8 B7 of the threat model records this and what stays open.

## Acceptance Criteria

- [x] R1 to R4: `TestTheReleaseIsBuiltApartFromWhatPublishesIt` passes on
      `build.yml`.
- [x] R7: `TestReleaseWorkflowFaultsAreFound` finds each planted fault.
- [x] R8: B7 records the separation, and that the attestation trusts the build
      jobs.
- [ ] R1 to R6 on GitHub: a pre-release tagged from this branch builds in the
      new jobs, publishes as a pre-release not marked latest, carries all six
      files, and each passes `gh attestation verify` with `--signer-workflow`
      and `--source-ref`.
- [x] R6: the check refuses a tag without a suffix on a commit not on `main`,
      run locally against the step's script.
- [ ] Immutable releases and a tag ruleset are on (repository settings,
      applied after this merges), and B7's "Not yet known" says so.

## Threat model delta

Boundary B7. Before: a module that runs in tests could reach the files a
release publishes, and code run during a build held the release's rights.
After: neither job does both. What is unchanged: a module linked into the
panel ships its code, and a maintainer account can edit the workflow in a
tag. `docs/threat-model.md` B7 is updated in this PR. The PR gets the
document's adversarial review.

## Risks & Assumptions

- A release takes longer: the build jobs have no cache, and run after the
  tests. Accepted; a release is not frequent.
- `upload-artifact` does not keep file modes. The release job does not run
  the files, and a downloaded release asset has no mode either.
- `gh` is on the runner image, as the image itself is trusted.
- `gh release create --notes-file --generate-notes` puts the notes above the
  generated ones. Checked by the pre-release.
- Rollback: revert the commit. The workflow is the only thing that changes
  until the repository settings are applied, and those can be turned off for
  later releases. A release published while immutable stays immutable.
- `windows` still builds and runs the installers on every PR, as a check. On a
  tag they are built twice; only `build-windows`'s are published.

## E2E Test Plan

Environment: GitHub Actions, a `v<next>-rc.N` tag pushed from this branch with
`VERSION` and a changelog heading to match, on a commit not intended for
`main`, as spec 075 did.

1. The run's jobs: `build-linux` and `build-windows` show no cache restored;
   `release` shows no Go. (R1 to R4)
2. `gh release view`: a pre-release, not latest, six files. (R5)
3. Each file with `gh attestation verify <file> -R ushineko/hayami
   --signer-workflow ushineko/hayami/.github/workflows/build.yml --source-ref
   refs/tags/<tag>`: exit 0. A file changed by a byte: exit 1. (R4)
4. An installer from the pre-release installs and runs on a desk.

### Prod visibility

The next tagged release's Actions run and `gh attestation verify` against its
files. There is no service to observe.
