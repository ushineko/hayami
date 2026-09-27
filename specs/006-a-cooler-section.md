# Spec 006: a cooler section

**Issue**: [#15](https://github.com/ushineko/hayami/issues/15)

## Status: INCOMPLETE

## Executive Summary

(Populated before the PR opens.)

## Context

The third section, and the first whose data layer was settled by somebody else.

`hotaru` reads the processor's temperature from hwmon **by label** rather than
from a daemon over HTTP, and the comment in `internal/cooler/hwmon.go` says
what that bought: OpenLinkHub stopped being a dependency. hayami copies the
approach, not the code — the package is small and hotaru's is entangled with
the hardware it also writes to.

By label, never by index. `coretemp` was `hwmon10` when this was written and
will be something else after a reboot; a program that remembers the number
reports the wrong chip rather than failing, which is the worst way to be wrong.

The kernel does not have the coolant or the pump. This cooler is an NZXT Kraken
that `nzxt-kraken3` does not match, so no hwmon node exists for it, and
`liquidctl --json status` is the only source. It is the one subprocess this
section is allowed.

**The trap is that liquidctl reports every device it can see.** On the machine
this was written on that is a Kraken *and* a Corsair HX1000i power supply,
which reports a "VRM temperature" and a "Case temperature". A decoder matching
on temperature keys alone would put the power supply's numbers under a heading
that says coolant. The cooler is the device that reports a liquid temperature;
nothing else qualifies.

The thresholds are the hardware's and not round numbers. The monitor records
that on the cooler these came from, the pump-head over-temperature alarm
tripped at 57.1 °C and cleared near 50, so the coolant is green below 50, amber
to 55 and red above. The processor gets **no** colour: a high boost temperature
is normal, and a colour that is always on is not a signal.

A sparkline is the last piece. The window can use the design system's; the
terminal has none, and the archetype says a trend plot is the one thing a
reader takes from a panel they never touch.

## Requirements

- R1 `internal/cooler` reads a labelled temperature from hwmon, by chip and
  label, never by index. A sensor that is absent is absent, not an error.
- R2 The coolant, pump and fan come from `liquidctl --json status`, and the
  device chosen is the one reporting a liquid temperature.
- R3 `liquidctl` missing, failing or reporting no cooler is a section that
  draws what it has — the processor — rather than nothing at all. Neither
  source missing is a section that is not drawn.
- R4 `view.Sparkline` draws a series as one line of block runes, for the
  terminal. The window uses the design system's.
- R5 The coolant is coloured by the hardware's bands. The processor is not
  coloured at all.
- R6 The section keeps a series long enough to be worth plotting, and a
  restart starts it again rather than drawing a line through one point.

## Acceptance Criteria

- [ ] AC1 A temperature is read from a hwmon tree the test builds, by label, and the same tree renumbered gives the same answer. (R1)
- [ ] AC2 A `liquidctl` reply carrying both a cooler and a power supply yields the cooler's numbers. (R2)
- [ ] AC3 With no `liquidctl` the section still draws the processor; with neither source it is not drawn. (R3)
- [ ] AC4 A sparkline of a known series is a known string, and an empty series draws nothing. (R4)
- [ ] AC5 The coolant is green, amber and red at the hardware's bands; the processor is never coloured. (R5)
- [ ] AC6 The series holds its capacity and drops the oldest. (R6)

## Risks & Assumptions

- **One machine's hardware.** The bands come from one cooler's alarm, the
  device matching from one pair of devices, and the label from one chip. Each
  is recorded where it is used so the next person knows it is a measurement
  and not a convention.
- **A subprocess on a poll.** `liquidctl` is given a deadline and killed if it
  misses it, as the app-server is: a section that hung would take its poll
  loop with it.
- **A sparkline is not a reading.** It is drawn from what this process has
  seen since it started, so it says nothing for the first few polls and
  nothing at all after a restart. That is honest and worth saying in the
  panel's own terms rather than filling with a flat line.
- Rollback: revert. The section is drawn only when the settings name it.

## Alternatives Considered

- Importing `hotaru`'s cooler package. Rejected: it speaks HID to the device
  it also writes to, and hayami has no business opening that. The approach is
  what is copied, which is the hwmon-by-label rule and its reason.
- Asking hotaru's HTTP API. Rejected for the same reason hotaru stopped asking
  OpenLinkHub: a panel that needs a daemon running is a panel that is blank on
  a machine without one.
- OpenLinkHub, which the Python falls back to. Not implemented: everything it
  provided is in the kernel or in liquidctl, which is the finding hotaru's
  spec 012 already made.
