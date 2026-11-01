<!-- Generated from the shared contributing policy. Edit the template and the project fragments, then re-render; do not hand-edit this file. -->

<!--
Read CONTRIBUTING.md before filling this in. The short version:
  - only maintainers merge
  - AI-written PRs are welcome, but you must be able to explain the change
  - tests are required, and so is an e2e reading from a real system
  - no Co-Authored-By trailers, no AI-attribution footers
-->

## What changed and why

<!-- One or two paragraphs. What does this do, and what problem does it solve? -->

## Related issue

<!-- refs #<number>, or "none". -->

## Tests

- [ ] Tests added or updated for the behaviour this changes
- [ ] `make test` passes
- [ ] `make lint` is clean

<details>
<summary>Test run output</summary>

```text
paste the real output here
```

</details>

## End-to-end reading from a real system

Required for any change to a data source (peripherals, bandwidth, cooler, usage) or to how a section renders. See CONTRIBUTING.md "Tests and e2e collateral".

- [ ] Reading taken on the hardware or data source the changed section reads
- [ ] Not applicable — this change does not touch a data source or how a section renders (explain below)
- [ ] Could not be taken (explain what was verified instead; a maintainer will decide)

**Environment**

| | |
| --- | --- |
| Device or data source the changed section reads | |
| OS / distribution and kernel | |
| Relevant software versions | |
| hayami version or commit | |

<details>
<summary>Observed output</summary>

```text
paste the real command and its real output here
```

</details>

## Threat model

See `docs/threat-model.md`. Tick one.

- [ ] No boundary touched
- [ ] Touches a boundary: which one (B1 helpers, B2 installing, B3 credentials, B4 parsing, B5 actions outside hayami, B6 what is shown or printed, B7 supply chain), what changed for the attacker, and the section of `docs/threat-model.md` updated in this PR:

<!-- A change to B1, B2, B3 or B7 gets the adversarial review described in docs/threat-model.md before it is merged. -->

- [ ] This PR changes `internal/netcount/netcount_bpfel.o` (a reviewer rebuilds it with `make bpf` and notes the difference)

## Checklist

- [ ] One logical change; no drive-by reformatting of unrelated code
- [ ] In scope for the project (see CONTRIBUTING.md "Scope")
- [ ] No secrets, credentials, or tokens in the code or in the pasted output
- [ ] No `Co-Authored-By` trailers and no AI-attribution footers in the commits
      or in this description
- [ ] I can explain what this change does and how it behaves at the edges
