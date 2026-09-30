# 018 — a battery in four bars

**Issue**: #67

## Context

hayami reads batteries through HID++ 2.0 features. A Logitech K800 — a keyboard
in daily use since about 2016 — does not have them. It answers a 2.0 root
request with `0x01`, *invalid sub-id*, which spec #66 now reads correctly and
reports as:

```
Logitech K800: speaks HID++ 1.0
```

Its battery is readable. The HID++ 1.0 register `0x07`, fetched with sub-id
`0x81`, answers on that keyboard:

```
register 0x07 (battery status): reply 05 00 00
```

`0x05` is one of four values the device can give, and register `0x0D` — the one
carrying an actual percentage — answers an error on it. **This generation has a
band, not a gauge.**

Spec 008 already decided what not to do about that:

> The percentage is used when there is one; the band is not turned into a
> number, because a device saying "good" does not mean 75 %.

That left the question of what a cell shows when there is no number, on a panel
whose cells are built around one. The answer chosen is the device's own: **four
segments, filled to the band**, which is how the keyboard's own indicator says
it.

## Requirements

### A band is a reading, and is not a percentage

`peripherals.Battery` gains a band alongside the level, and the two do not
substitute for one another. A device with a percentage keeps its number; a
device with a band has no `Level` at all rather than a plausible one.

The four bands are the protocol's: critical, low, good, full.

### The 1.0 battery is read where the 2.0 features are absent

A device that answered `errOldProtocol` is asked, on the index it answered on:

- register `0x0D` first, which carries a percentage on the devices that have it
- register `0x07` otherwise, which is the band

Only getters are sent — sub-id `0x81` is GET_REGISTER_REQ; `0x80` is the setter
and appears nowhere.

Register `0x07`'s reply is `band, charge, 0`. The band byte takes the values
`0x07`, `0x05`, `0x03` and `0x01`; the charge byte is zero when discharging.
That mapping is solaar's, checked against one keyboard. **Anything else is not
a reading**: an unknown band yields nothing rather than a guess.

### A band cell keeps the shape of a number cell

The segments sit where the percentage sits, so a row of cells still lines up
and the eye lands in the same place. The band's word goes on the quiet line,
where a percentage cell says "Discharging".

```
   G502 X PLUS        Logitech K800
      78 %               ▮▮▮▯
   Discharging            Good
```

A band device is obviously not a measured one without a caption saying so, and
nothing invents a figure the keyboard never gave.

The verdict follows the band, not a number: critical is Bad, low is Warn, and
the rest Good — the same thresholds a percentage cell uses, applied to the
information that exists.

### The charge state is still said

A band device that is charging says so, in the same place a percentage device
does. Where the band and the state would both be shown, the state wins the
quiet line: "Charging" is the more urgent fact and the segments already carry
the band.

## Executive Summary

A Logitech K800 from about 2016 now draws. It has no HID++ 2.0 features at all,
so its battery comes from a 1.0 register, and what that register gives is one
of four bands rather than a percentage. The cell draws four segments filled to
the band, which is how the keyboard's own indicator shows it, and invents no
number.

Reviewers should look at `internal/peripherals/hidpp10.go` first — it is the
whole of the new protocol, and it sends only getters — and then at `bandCell`
in `internal/view/peripherals.go`, which is the display decision.

## Acceptance Criteria

- [x] `peripherals.Battery` carries a band, and a band device has no `Level`
- [x] Register `0x07`'s four band values decode, and any other value yields no
      reading
- [x] Register `0x0D` is preferred where a device answers it
- [x] Only sub-id `0x81` is sent; nothing in the package sends `0x80`
- [x] A device that answered `errOldProtocol` is read rather than only named
- [x] A band cell draws four segments where a percentage cell draws its number,
      and the band's word where it draws its state
- [x] A charging band device says "Charging" rather than its band
- [x] The verdict follows the band: critical Bad, low Warn, otherwise Good
- [x] A band device still reports no percentage in `readings`
- [x] The K800 on `gamerson-cachyos` draws, photographed
- [x] The whole suite passes, and `go vet` and the linter are clean

## Risks & Assumptions

- **Rollback** is `git revert`. A machine with no 1.0 device is unchanged.
- **The band mapping is solaar's and one keyboard's.** A device whose bands
  differ reads as nothing rather than as the wrong band, because an unknown
  value is refused.
- **Only getters are sent.** Register reads on a device this build has just
  learned to talk to are the same class of traffic solaar sends continuously.
- **A quiet index is still never named** (#66). Reading a 1.0 battery must not
  become a route by which a leftover pairing slot acquires a cell: only an
  index that *answered* is read.

## Gaps found

**A 1.0 device wired in by its own cable is missed.** Such a device answers on
index 0xFF, and 0xFF is excluded because on a receiver it addresses the
receiver — which has no battery, and asking one for a register it does not have
made the whole section error. No wired 1.0 device is to hand to check against,
and a missed reading is the better of the two mistakes.

**The band-to-charge mapping rests on one keyboard.** `0x05` for good is
solaar's table and this K800's answer. The other three values are solaar's
alone.

## Alternatives Considered

Considered a number from the middle of each band, marked approximate; rejected
because it is a figure the device never gave and spec 008 refused it once
already.

Considered a shorter cell with no number slot; rejected because the row's
baseline breaks and a band device stops looking like its neighbours.

Considered segments for every device, filled proportionally; rejected because
it demotes a real measurement to look like a four-step guess.

Considered a different glyph pair after the first photograph made the empty
segment look like a sliver; rejected once measured. Both pairs advance
identically in the panel's own face, and what the eye had picked up was an
outline beside filled bars — which is the point of drawing an outline.

## Status: COMPLETE
