# 067 — one reading is not a device

**Issue**: #218

## Status: COMPLETE

## Executive Summary

The Peripherals source keeps a device only once it has answered in two polls.
A device heard in one poll is drawn while it answers and dropped when it
stops, and it is never written to `peripherals.json`. Reviewers should look
first at `remembered.confirmed` in `internal/panel/peripherals.go`, and at the
five existing tests that now poll twice before a device goes quiet.

## Context

On a desk with a mouse and headphones the Peripherals card drew four cells:
the G502 X PLUS, the WH-1000XM6, "Logitech 0 %" (Offline) and a "no device"
placeholder. "Logitech" was one partial read of the G502 on its Lightspeed
receiver. The battery answered, the name and type did not, and sanshoku
reported it under a placeholder identity (ushineko/sanshoku#54, fixed by
sanshoku spec 020). It answered once and never again.

The panel keeps every device it has a level from as a quiet device for a week
(specs 022 and 032). So the one read became a remembered device. As kind
"other" it took a slot of its own (spec 056), three kinds pad to four cells,
and the card doubled in height for a week.

The driver fix stops this source of phantom devices. This spec stops the
panel from depending on every driver getting it right.

## Requirements

- R1 A device that answers is drawn from that poll, as before. A device
  appearing must still be prompt.
- R2 A device is **confirmed** once it has answered in two polls, whether or
  not they were consecutive. Only a confirmed device is kept as a quiet
  device after it stops answering (specs 022, 032), and only a confirmed
  device is written to `peripherals.json`.
- R3 An unconfirmed device that does not answer in a poll is dropped: not
  drawn dim, not written down. If it answers again it starts over as new.
- R4 A device loaded from `peripherals.json` is confirmed, since it was
  confirmed when it was written. The file format is unchanged.
- R5 A device present without a level whose last level is carried over (spec
  022's base-station case) has answered again, and is confirmed.

## Acceptance Criteria

- [x] A device heard in one poll beside a confirmed mouse is drawn in that
  poll and gone in the next; the card is back to two cells
  (`TestADeviceHeardInOnePollIsNotKept`).
- [x] A device heard in one poll, with a restart straight after, is not
  loaded (`TestADeviceHeardInOnePollIsNotWritten`).
- [x] Falsified: without the drop, the first test fails; without the write
  filter, the second fails.
- [x] The spec 022 and 032 contracts hold for a device that has answered
  twice: it goes dim at its last level when quiet, survives a restart, is
  forgotten after a week and is new again when it comes back. The five tests
  that had a device answer once now poll twice first.
- [x] `go test ./...` and `make lint` are clean.

## Risks & Assumptions

- A device seen for the first time that answers once and then sleeps leaves
  the card until it answers again, where it used to stay dim. A wireless mouse
  misses about one poll in fourteen (spec 022), so two answers come within
  seconds for anything that is really on the desk. A device already in
  `peripherals.json` is confirmed and unaffected.
- Rollback: revert the commit. The file is not changed in format; a file
  written by this version is readable by the last.

## Alternatives Considered

- Considered writing only confirmed devices to the file and leaving the
  running panel as it was; rejected because the phantom would still hold a
  slot until the next restart.
- Considered dropping devices named only by a vendor, or of kind "other"
  with no level; rejected because which names are placeholders is device
  knowledge, and that belongs in sanshoku.
