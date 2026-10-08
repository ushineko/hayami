# 037 — The Wi-Fi link in its row

**Issue**: #120

## Status: COMPLETE

## Context

A Wi-Fi interface in the bandwidth section was drawn as a wired one: rates and
totals. How strong the link is, which band it is on and what it negotiated are
as much a glance reading as the rates, and on a machine whose only network is
Wi-Fi they are the first thing to check when the rates look wrong.

Measured on Windows 11 Pro 26200, an Intel Wireless-AC 8265 on a 5 GHz access
point, from an ordinary desktop process (not elevated, not in System32):

- `WlanQueryInterface(wlan_intf_opcode_current_connection)` answered: signal
  quality 87 %, receive rate 780,000 kbit/s.
- `wlan_intf_opcode_rssi` answered -47 dBm and `wlan_intf_opcode_channel_number`
  channel 149.
- `wlan_intf_opcode_realtime_connection_quality` (Windows 11 24H2 and later)
  answered 296 bytes: the PHY type (VHT, Wi-Fi 5), the link quality, both
  rates, and per link the centre frequency (5745 MHz) and the RSSI.
- `netsh wlan show interfaces`, from System32, was refused: "Network shell
  commands need location permission to access WLAN information." The location
  consent store on this machine reads Deny at the top level and Allow for
  NonPackaged desktop apps. Since 24H2 Windows gates the connection's details
  on location consent, because the access point's address locates the
  machine; a desktop app is let through where the NonPackaged consent allows
  it.

The connection attributes also carry the profile name, the SSID and the BSSID.
None of them is wanted here, and all three identify a place.

On Linux the same facts come from nl80211, unprivileged: the interface's
frequency from `NL80211_CMD_GET_INTERFACE`, the station's signal and both
bitrates (with the modulation that names the generation) from
`NL80211_CMD_GET_STATION`. The user chose
[mdlayher/wifi](https://github.com/mdlayher/wifi) (MIT, pure Go) to ask, and
chose bars in the interface's row for the signal.

## Requirements

- R1 `core.ReadWireless(ctx)` describes every Wi-Fi interface, keyed by the
  name `ReadCounters` keys the same interface by: on Windows the alias (the
  WLAN interface's GUID through `ConvertInterfaceGuidToLuid` and
  `ConvertInterfaceLuidToAlias`), on Linux the netdev name. Each description
  (`core.Wireless`) carries Connected, signal percent, RSSI, frequency,
  channel, generation, and both link rates, each with its own flag. A machine
  without Wi-Fi, or with the WLAN service stopped, returns nothing and no
  error.
- R2 The SSID and BSSID are never read: on Windows the decoder reads only the
  state (offset 0), the PHY (568), the quality (576) and the rates (580, 584)
  of the connection attributes; on Linux the BSS is never asked for and the
  station's hardware address is not kept.
- R3 Windows: wlanapi and iphlpapi through `LazyProc`, no cgo. The realtime
  quality first (frequency, RSSI), then the connection, the channel and the
  RSSI opcodes for what it left out. `ERROR_ACCESS_DENIED` is reported as
  `core.ErrWirelessDenied` beside whatever was read.
- R4 Linux: mdlayher/wifi `Interfaces` and `StationInfo` for station-mode
  interfaces; an interface with no station is Connected false. Where nl80211
  does not answer, `/proc/net/wireless` gives the signal level.
- R5 The bandwidth section asks the WLAN on its first poll of a set of
  interfaces, and after that only while one of them is Wi-Fi; choosing
  interfaces again asks once more. A desk of wired interfaces pays one
  question. A test reading its own counters reads no Wi-Fi.
- R6 An interface once seen to be Wi-Fi stays a radio: losing its link blanks
  its bars and link line but keeps their place (glance rule).
- R7 View: a Wi-Fi row's value leads with `SignalBars`, the battery's four
  segments (spec 018), by RSSI (≥ -55 four, ≥ -67 three, ≥ -75 two, else one;
  Windows' percentage in quarters where there is no RSSI; none for no link).
  One bar is Warn, the rest Info. Under the totals a second detail line,
  `-47 dBm ·   5 GHz ch 149 ·  780 Mb/s`, every field at its widest width, and
  "not connected" right-aligned at the same width. The row's tip carries the
  generation, Windows' percentage and both rates. Wired rows are unchanged.
- R8 A row's `Detail` may hold several lines separated by `\n`
  (`Row.DetailLines`); both shells draw each as they drew the one.
- R9 `ErrWirelessDenied` becomes an aside reason, "Wi-Fi: details withheld",
  whose detail names the setting and `ms-settings:privacy-location`.
- R10 The readings JSON carries `wireless` on a Wi-Fi interface's entry.
- R11 README: the bandwidth row of the sections table, an "On Windows" note on
  location consent, credits for mdlayher/wifi, the changelog.

## Acceptance Criteria

- [x] On Windows, `hayami-tui doctor` and one frame show the Wi-Fi interface
  with four bars, its RSSI, band, channel and rate, and the wired interface
  without; `readings` carries the `wireless` object and no SSID.
- [x] A test holds the Wi-Fi interface's key against `ReadCounters`' names on
  the real tables.
- [x] Decoders tested from byte fixtures with invented values and the SSID
  region filled with a pattern; nl80211 through a fake client;
  `/proc/net/wireless` from text.
- [x] The poll asks once on a wired desk and every poll for a watched radio.
- [x] Neither the row nor the link line changes width across no link, a link
  with nothing said, and the widest values.
- [x] On a real window on Windows 11 the Wi-Fi row's four glyphs before ↓ are
  four bars of one width, and both rows' arrows are in one column. Screenshot
  looked at.
- [x] Falsified: the window test fails with the bars drawn after the rates and
  with no bars drawn.
- [x] `go test -tags migrated_fynedo ./...` passes on Windows but for the
  Python cross-check, which fails on `main` on this machine too.

## Risks & Assumptions

- **Linux is untested on hardware here.** The nl80211 path is tested through a
  fake client only; the first live read is a Linux desk's.
- **mdlayher/wifi is a new dependency**, with mdlayher/netlink, genetlink and
  socket behind it (all MIT). Pinned at v0.9.0.
- **6 GHz without a frequency.** Where only a channel is known (Windows before
  24H2), a channel above 14 is called 5 GHz.
- **The rate shown is the negotiated receive rate**, not throughput; the row's
  own rates are throughput.
- **Rollback**: revert. No setting, cache or file format changes; the JSON
  field is additive.

## Gaps found

- The bars sit close to a short label ("Wi-Fi 2▮▮▮▮"): the card's row puts its
  value as far right as it goes and the label as far left, with the library's
  gap between. Readable; a wider gap would be a fynedesygn change.
- A Wi-Fi adapter Windows does not list in the WLAN service (disabled, or with
  its service stopped) is drawn as a wired one.
- The window test identifies rows by position and by shape; it needs a
  connected Wi-Fi interface and another interface to line up with, and skips
  without them. Its capture helpers are spec 034's, copied unchanged so the
  two branches merge cleanly.

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.26.0, MSYS2 UCRT64 gcc for the window:

- `hayami-tui doctor` with a throwaway settings watching "Wi-Fi 2" and
  "Ethernet 2":

  ```
  bandwidth     ok      Wi-Fi 2 ▮▮▮▮ ↓ -- ↑ --, Ethernet 2 ↓ -- ↑ --
                        Wi-Fi 2: Wi-Fi 5, signal 91 %, link 867 down, 867 up Mb/s
  ```

- One frame:

  ```
  Wi-Fi 2                                        ▮▮▮▮ ↓    --        ↑    --
                                                        Σ ↓  30.8 GiB  ↑ 167.5 MiB
                                              -48 dBm ·   5 GHz ch 149 ·  867 Mb/s
  Ethernet 2                                          ↓    --        ↑    --
                                                        Σ ↓     0 B    ↑     0 B
  ```

- The live key test: `Wi-Fi 2: connected signal 91 % rssi -52 5 GHz ch 149
  "Wi-Fi 5" rx 866.7 tx 866.7`, keyed as `ReadCounters` keys it.
- Window: glyphs before ↓ at x 88–92, 99–103, 110–114, 120–124 (four bars of
  one width); the other row's are letters. Falsified both ways, as above.
- Full suite on Windows: every package ok except `internal/usage`'s Python
  cross-check (the Python found lacks `structlog`; fails on `main` too).
- `GOOS=linux go vet` on core, view, panel, cli, tui and `cmd/hayami-tui`;
  `CGO_ENABLED=0` builds of `hayami-tui` for linux and windows; golangci-lint
  v2.12.2: 0 issues for linux, and on windows only the `unsafe` notes (G103)
  the existing Windows syscall files carry.
