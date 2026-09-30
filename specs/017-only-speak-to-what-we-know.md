# 017 — only speak to what we know

**Issue**: #62, #63

## Context

Spec 016 found devices by vendor and usage page and then spoke one command
family at whatever it found. Those are not the same scope, and on the machine
hayami is developed on they have already diverged: a SteelSeries Arctis Nova
Pro Wireless exposes usage page `0xFFC0`, so every peripherals poll has been
writing `0x92` and then `0xd2` to a headset that was never tested against.

```
hidraw14  usage pages: ['0xffc0', '0xff00']   SteelSeries Arctis Nova Pro Wireless
```

It does not answer — HeadsetControl drives that headset with a `0x06, XX`
family, nothing like `0x92` — so nothing has visibly broken. **That is not the
same as it being safe.** "Does not answer" is not "is a getter", and a command
sent with an empty payload is indistinguishable, for anything that takes one,
from "set this to zero". That assumption is what changed the Apex's lighting
during spec 016's investigation, and shipping it is worse than probing with it.

As this runs on more machines it will meet more devices, so what it detects and
what it polls have to be separately precise.

### The two vendors need different treatments

**Razer's commands are universal and its addressing is not.** OpenRazer drives
its whole range with one report struct, one XOR checksum and a single
`razer_chroma_misc_get_battery_level()`; class `0x07` command `0x80` is the
battery getter for every Razer device. What varies per model is the transaction
ID, and 016 hardcoded `0x1F`:

```
141  transaction_id.id = 0xFF     126  0x1F     114  0x3F     16  0x9F
```

Measured on the Mouse Dock Pro here, `0x1f`, `0x08` and `0x00` answered and
`0x3f` timed out — so a device tolerates some and not others, and a different
Razer device reads nothing unless `0x1F` is its one.

**SteelSeries' commands are not universal.** rivalcfg knows 33 devices; eleven
report a battery, in two incompatible families:

| command | reply | level | charging | devices |
|---|---|---|---|---|
| `0x92` | 2 bytes | `((v & 0x7f) - 1) * 5` | bit 7 | Aerox 3/5/9, Prime Wireless, and the Apex Pro TKL Wireless |
| `0xAA 0x01` | 3 bytes | `data[0]`, a plain percentage | `data[2]` | Rival 3 Wireless, Rival 3 Gen 2, Rival 650 |

Sending the first to a device that speaks the second is not a failed read; it
is an unknown write.

## Requirements

### Finding a node and speaking to it are separate decisions

Nodes are still found by vendor and usage page — the reason 016 gives stands,
and the Apex's product ID still moves between `0x1644` on 2.4 GHz and `0x1646`
on its cable. What changes is that finding a node no longer authorises talking
to it.

### A SteelSeries device is spoken to only if this build knows its protocol

A table keyed on product ID says which battery family a device speaks. A
product that is not in it is **not written to at all**, and the section says so
by name rather than staying silent — the panel already has the vocabulary for
that from spec 015.

Both of the Apex's product IDs are in the table, because both were measured.

The `0xAA` family is implemented from rivalcfg's device profiles and **has not
been verified against hardware**; the table is what keeps it away from anything
it was not written for, and the code says as much where the decode is.

### A Razer device is spoken to with getters, addressed by trying

The commands are OpenRazer's own getters and are safe to send to any Razer
device, so this does not need a product table to *poll*. The transaction ID
does need finding: an ordered list is tried until one answers, and which one
answered is remembered for that node so the cost falls on the first poll only.

A transaction ID that answers `timeout` or `busy` is not evidence either way —
those are what a sleeping or contended device says on the *right* ID — so a
list is only exhausted when every entry has been tried, and a device that
answered before keeps its ID rather than being rediscovered.

### A device's kind is known or it is not

`Kind` comes from a table of product IDs, not from a guess. A Razer Mouse Dock
relays a mouse and says `KindMouse`; anything else this build has not met is
`KindOther`, which is where spec 016's attempt to infer it from sibling
interfaces ended up after it read a keyboard as a mouse.

### A request carries an ID no other program here uses

Spec #59 replaced a constant software ID with one taken from the process ID, so
two hayamis would stop accepting each other's replies. It picked freely from
1..15, and **solaar's own ID is 0x0B** — `SOLAAR_SOFTWARE_ID` in
logitech_receiver/base.py. One run in fifteen landed on it, which traded a
collision between two hayamis for one against the tool most likely to be on the
same receiver. The constant it replaced, 0x08, never collided with solaar.

The live comparison caught it while this spec was being written: hayami read
71 % in the same second solaar read 79 %. That is a reply belonging to somebody
else, not a battery moving.

Solaar's ID is excluded. Two hayamis can still collide, one chance in fourteen.

### Nothing is polled that was not asked for

A section the settings do not ask for builds no source, so a machine with the
peripherals section off writes nothing to any device. `doctor` is the exception
and says so: it probes every section by design, and that is the one place a
person is asking for exactly that.

## Executive Summary

Finding a device and being entitled to write to it are now separate decisions.
A SteelSeries product this build does not know is detected, named and left
alone; a Razer device is addressed by trying the transaction IDs OpenRazer uses
rather than the one that happened to work here; and a request no longer carries
solaar's software ID.

Reviewers should look at `steelseriesBattery` and `razerTransactions` first —
they are the two tables the rest follows from — and then at the split in
`panel/peripherals.go` between absences, which a working card suppresses, and a
device that is present and unreadable, which it does not.

## Acceptance Criteria

- [x] A SteelSeries product not in the table is never written to, and the
      section reports it by name
- [x] Both `0x1644` and `0x1646` resolve to the `0x92` family
- [x] The `0xAA` family decodes rivalcfg's shape: three bytes, `data[0]` as a
      percentage, `data[2]` as charging
- [x] A test proves an unknown SteelSeries product receives no write, by
      failing if the fake device is written to at all
- [x] The Razer reader tries transaction IDs in order and remembers the one
      that answered for that node
- [x] A Razer device answering `busy` or `timeout` does not cause the
      transaction ID to be relearnt on the next poll
- [x] `Kind` comes from a product table; an unknown product is `KindOther`
- [x] The Arctis Nova Pro Wireless on the development machine is detected,
      reported by name as unsupported, and written to zero times
- [x] The software ID never takes solaar's 0x0B, and never zero
- [x] Both devices on `cachyos` still read
- [x] The whole suite passes, and `go vet` and the linter are clean

## Risks & Assumptions

- **Rollback** is `git revert`. The change only narrows what is written to a
  device; a machine with the two known devices behaves as it did.
- **The `0xAA` decode is unverified.** It comes from rivalcfg and no hardware
  here speaks it. The product table is the control: it can only reach devices
  rivalcfg names.
- **Trying transaction IDs is a write per attempt**, to a Razer device, with a
  documented getter. That is the same class of traffic OpenRazer's own driver
  sends and is not the speculative kind this spec is about.
- **A product table goes stale.** A device released tomorrow is not in it and
  reads nothing, which is the failure this spec chooses: silence with a name
  attached, rather than an unknown write.

## Gaps found

**The `0xAA` family is named and not implemented.** Its framing differs from
the newer one in more than the command byte — rivalcfg reads the level out of
the reply's first byte, so there is no command echo to match an answer on,
which is the whole of how the newer family tells its reply from the other
traffic on that endpoint. Writing that from documentation alone would ship a
guess about framing to hardware nobody here has. A Rival 650 is therefore
detected, named and unread; the table is what tells it apart from a device this
build has never heard of.

**The product tables go stale by construction.** A device released tomorrow
reads nothing. That is the failure this spec chooses over an unknown write, and
the panel says which device it was.

## Alternatives Considered

Considered keeping the broad match and relying on "it did not answer" as proof
of safety; rejected — that is the reasoning that changed a keyboard's lighting,
and it is weaker in shipped code than it was in a probe.

Considered gating Razer on a product table too; rejected because its getters
are universal across OpenRazer's range, so the table would add staleness
without removing any unknown write.

## Status: COMPLETE
