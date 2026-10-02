# 032 — a restart remembers the devices

**Issue**: #113

## Context

The peripherals source keeps what each device last said in memory
(`Peripherals.seen`, spec 022): a device that stops answering is drawn dim at
its last level, keeps its slot, and is listed as offline in the overflow
tooltip (0.8.4). A restart empties that memory. With the mouse asleep and the
headphones off, a restarted panel's first poll finds nothing and the card reads
"a Logitech receiver, with nothing paired to it", "no Bluetooth device with a
battery" — seen on njv-cachyos on 2026-10-01, where the panel before the
restart had been showing both devices dim.

The section cache (`~/.cache/hayami/sections.json`) does not cover this: it
holds the last drawn section and is used only until the first live poll
answers, and the first poll's answer, with the devices asleep, is "nothing".

## Requirements

- R1 The peripherals source saves its memory of devices to
  `~/.cache/hayami/peripherals.json` (the cache directory `readings` already
  uses) after a poll that changed it: each device's last reading (name, kind,
  level, charge, cells) with when it was first detected and last heard.
- R2 At start it loads that file, keeping every device heard within the last
  seven days, each marked as not answering. The first poll then treats them
  exactly as a running panel treats devices that went quiet: drawn dim at
  their last level, ranked by when they were last heard, live again the
  moment they answer.
- R3 **Seven days.** A device not heard for seven days is forgotten, at load
  and while the panel runs, so retired hardware drops out without anyone
  removing it. The constant is in `panel` with this reason.
- R4 The file is written atomically (temporary file and rename), only when its
  contents change, and a missing, unreadable or malformed file is a first
  run, not an error. Both shells build this source and share the file; the
  last writer wins, and both know the same devices.

## Acceptance Criteria

- [x] `go test ./...` and `make lint` pass.
- [x] A test: a device read, then the source rebuilt from the saved file with
      no device answering, gives that device dim at its last level, in the
      slot it had.
- [x] A test: a device last heard eight days ago is not loaded, and one six
      days ago is; a running source forgets a device after seven days unheard.
- [x] A test: a device saved as quiet that answers on the first poll is live,
      and its detection time is that poll.
- [x] A test: a malformed file loads as empty.
- [ ] **On the desk:** with the mouse asleep or the headphones off, a
      restarted panel on njv-cachyos shows them dim at their last level
      rather than "nothing paired".
- [x] README changelog under Unreleased.

## Risks & Assumptions

- **Names are the key**, as they are in memory today; two devices with one
  name are one entry, as now.
- **No device data leaves the machine**; the file sits beside the section
  cache in the user's cache directory.
- **Rollback**: revert; the file is ignored by an older build.

## Status: INCOMPLETE
