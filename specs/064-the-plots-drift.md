# 064 — the plots drift between polls

**Issue**: #204

## Status: COMPLETE

## Context

The plots under the bandwidth and cooler cards stand still between polls and
then jump a step: about 4.5 px every 2 s for bandwidth and every 5 s for the
cooler, with the scale jumping when a peak arrives or leaves. While a trail is
filling, every poll also re-spaces every point.

Polling faster was considered and rejected. It costs reads, a byte counter
read faster than its driver updates it is noise, and the Kraken's status is
cached for 250 ms in sanshoku. The motion belongs in the plot, and fynedesygn
#211 (its spec 070) put it there: `Sparkline.SetDrift(every)` and
`Sparkline.SetSamples`, with points spaced by capacity. A prototype measured
the drift at about 1.25 % of one core at 10 redraws a second, against 0.42 %
for the plot as hayami fed it.

hayami fed the plot by clearing it and adding every sample back, on every
draw: sixty rebuilds of the plot for one new point, and a plot that could not
tell a new sample from a new history.

## Requirements

- R1 Each section's plot drifts over that section's own poll interval
  (`Source.Interval()`): 2 s for bandwidth, 5 s for the cooler.
- R2 A draw hands the plot the section's trails in one `SetSamples` call. A
  trace is declared once and recoloured in place (`SetColour`) on later
  draws, so its history and its drift survive a draw. A trace the section no
  longer has is emptied, which is what the clear did.
- R3 A draw whose trails have not changed (a settings change, a section
  shown again) does not restart or stop a drift.
- R4 The terminal panel is unchanged.
- R5 fynedesygn is v0.1.103, the release that carries spec 070.

## Acceptance Criteria

- [x] R1: a card's plot is told its source's interval, tested headless
      (`TestEachPlotDriftsOverItsSourcesInterval`; falsified by removing the
      `SetDrift` call).
- [x] R2: after two draws a trace holds the section's samples, keeps them
      across a draw that recolours it, and a trace that left the section is
      empty, tested headless (`TestADrawReplacesTheTrailsAndEmptiesOnesThatLeft`).
      This test passes against the old clear-and-add code too: both end in
      the same samples. What R2 adds, a trace declared once so its drift is
      not reset, is not visible headless, where the test driver runs a
      drift to its end at once; the real-window check is what shows it.
- [x] R3: covered by fynedesygn's `TestTheSameSamplesAgainDoNotRestartTheDrift`
      (an unchanged `SetSamples` is a no-op) together with R2's declare-once;
      not separately observable headless, for the reason under R2.
- [x] On the real window (Plasma 6, Wayland, spectacle): the `main` build,
      captured alone every ~0.5 s, changed in 2 of 9 consecutive captures,
      the two where a poll landed. This branch's build, installed over the
      desk's panel and captured full-screen every ~1.2 s, changed inside both
      plots in 7 of 7 pairs, while the bandwidth figures changed in 5 of 7
      (a 2 s poll in between) -- and the cooler plot polls every 5 s, so a
      stepping plot would have changed in about 2. A plot filling after the
      restart grew from the right edge.
- [x] `make test`, `make lint` and `make vuln` pass (vuln: nothing called;
      one advisory in a required module that hayami does not reach).
- [x] README changelog under `### Unreleased`.
- [x] R5: `go.mod` requires fynedesygn v0.1.103, with no `replace`.

## Risks & Assumptions

- The plot runs up to one interval behind the rows above it: the newest
  sample is fully in place only as the next is due. The rows show the value
  at once. Accepted when this was asked for.
- A poll that takes longer (the cooler's nvidia-smi fallback, up to 2 s)
  delays its draw, and the plot rests at its place until the sample arrives.
  It does not jump back.
- The panel redraws a drifting plot 10 times a second, about 0.8 percentage
  points of one core over the stepping plot on the desk it was measured on.
- Rollback: revert the commit and the fynedesygn bump.

## E2E Test Plan

- Environment: the Linux desk, Plasma 6 on Wayland, the panel built from
  this branch and installed over `~/.local/share/hayami/bin/hayami`.
- Steps: run the panel; capture its window with spectacle four times 300 ms
  apart; compare the captures inside the bandwidth and cooler plots. Watch
  both plots across several polls. (R1, R3)
- Expected: the plots move steadily left between polls and do not jump when
  a sample arrives; the rows above them update at once.

### Prod visibility

Not applicable: a desktop program with no service behind it. A release is
observed by running it.
