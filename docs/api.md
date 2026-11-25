# The service other programs read

`hayami-tui serve` gives the panel's readings, their details and drills, to
other programs of the same account as JSON over a local socket: a status bar,
a script, another panel. This is the prose beside the contract,
[`api/openapi.yaml`](../api/openapi.yaml), which is the source of truth: every
path, field and status is defined there, and a client in another language
generates or writes its types from it. Spec 113 is the design.

The examples are the reference client, `hayami-tui api`, which is also how the
service is checked: everything it does, any client can.

## Setting it up

- **Linux.** The installer's Service page sets it up on a first install
  unless the box is cleared (`--api=no` on the command line). It writes a
  systemd user socket and service, `hayami-api.socket` and
  `hayami-api.service`, under `~/.config/systemd/user`. systemd listens on
  the socket with every session; the first program to connect starts the
  service, and it stops itself after five minutes with nobody reading. An
  upgrade keeps it, restarted on the new program; running the installer again
  with the box cleared, or uninstalling hayami, removes it and its socket.
  `--yes` on an upgrade leaves it as it found it, so a service someone removed
  is not given back.
- **Without a systemd user manager** the page says so, and the service runs
  by hand: `hayami-tui serve`, in the foreground, until stopped.
- **Windows.** The same page, the same default. It registers a per-user
  service, `hayami-api`: a template from which Windows starts an instance as
  each account that signs in, `hayami-api_` and a number, and stops it at
  sign-out. Its program is a copy of `hayami-tui.exe` in Program Files,
  `hayami\hayami-api.exe`, not the install under `%LOCALAPPDATA%`: every
  account that signs in runs it, so it must be a file no one account can
  change. Registering it, and updating the copy on an upgrade, asks for an
  administrator (one UAC prompt, a `--yes` upgrade's included). It starts at
  the next sign-in; an ssh sign-in makes no instance, so a client started over
  ssh finds the service only while the account is also signed in at the
  console. An instance for an account that has never run hayami stops at once.
  Windows restarts one that fails, twice, five seconds apart. Removing it, by
  the page or by uninstalling, stops and deletes each instance, then the
  template, then the copy.
- **macOS** is a later phase of spec 113: a launchd agent.

```console
$ hayami-tui api status
address    /run/user/1000/hayami/api.sock
installed  yes, listening: the next program to connect starts it
hayami <version>, contract 1.0
  clock        every 1s  shown in the panel
  bandwidth    every 2s  shown in the panel
  …
```

`status` prints what the service manager says before it asks the service, since
asking starts one that is only listening.

## Connecting

HTTP/1.1 over a socket only the account running the service can open: the
directory is `0700`, the socket `0600`, and a connection from another account is
closed without an answer. There is no TCP port, so no browser page or other
machine reaches it.

| System | Address |
|---|---|
| Linux | the Unix socket `$XDG_RUNTIME_DIR/hayami/api.sock` |
| Windows | the named pipe `\\.\pipe\hayami-api-<SID>`, the account's SID |
| macOS (later) | the Unix socket `$TMPDIR/hayami/api.sock`, the per-user `$TMPDIR` |

A path longer than the system takes in a socket address (108 bytes on Linux,
104 on macOS) is an error, never cut short. `hayami-tui api address` prints the
address. The URL's host is ignored:

```console
$ curl --unix-socket "$XDG_RUNTIME_DIR/hayami/api.sock" http://hayami/v1/sections/cooler
```

### Who is at the address, on Windows

The pipe's access list grants the account and no one else, and the service
makes it as the pipe's first instance, so a second service, or a pipe someone
made first under the name, is a refusal to start. But a name can be taken
before the service starts, and a client cannot tell from the name alone. So a
client asks who serves the pipe before it sends a byte:
`GetNamedPipeServerProcessId` on the handle, then that process's token, whose
user must be the client's own. The reference client does
(`internal/api/client`, `Dial`), and opens the pipe anonymously
(`SECURITY_SQOS_PRESENT | SECURITY_ANONYMOUS`), so a server that is not the
service cannot act as the client's account with the connection. A pipe takes
no client from another machine.

Only `GET`. A request with a body is 400, one with an `Origin` header 403 (a
browser's), more than 64 connections or 16 streams at once 503 with
`Retry-After`.

## What there is

| Path | What |
|---|---|
| `/v1` | The service, its version, the contract's version, the section keys |
| `/v1/health` | `{"ok":true}`; reads nothing |
| `/v1/sections` | Each section: key, title, whether it is a default, whether the panel shows it, its interval |
| `/v1/sections/{key}` | One section's latest reading |
| `/v1/readings?sections=a,b` | Several sections at once, by key; all when the parameter is absent |
| `/v1/sections/{key}/details/{id}` | A detail or a drill |
| `/v1/events?sections=a,b&details=key/id` | A stream of readings, as Server-Sent Events |

```console
$ hayami-tui api get sections/cooler
$ hayami-tui api watch --sections usage,bandwidth --details cooler/cpu
$ hayami-tui api check
```

## The rules every response keeps

- **Facts, not formatting.** Names whole, as long as the source has them; the
  panel's cuts and padding are the panel's. Numbers are JSON numbers in base
  units. Times are RFC 3339 in UTC; durations are seconds.
- **Every field is always there.** A value that is not known is `null`, never
  left out. A list is never `null`, an empty one is `[]`, except a detail's
  `vpns`, which is `null` on a detail of another kind.
- **Strings are cleaned and bounded.** Control and bidirectional-override
  characters are removed, a change's description keeping its line breaks. A
  string longer than 4096 bytes is cut to it, on a character's boundary, and
  its response says `"truncated": true`.
- **A status is a verdict.** `status` fields (`good`, `warn`, `bad`, `info`,
  `dim`) are the panel's band for a value. A client may use them or decide for
  itself from the numbers.
- **The contract only grows within `/v1`.** A field or an endpoint may be
  added; none is removed, renamed, retyped or made nullable. A client ignores a
  field it does not know. `api` in `/v1` is the contract's version.
- **Nothing secret.** No token, refresh token or forge key is in any response.
- **No actions.** Ending a process, a VPN's controls: none is served.

## When it reads

A section is read only while a client reads it: a request within the last
three of its intervals (never less than 30 s), or a stream that names it. Any
number of clients share one reading, and a section nobody asks for costs
nothing. The first request for a section nobody was reading waits for its first
reading, up to 15 s; every other request is answered from the latest one at
once. A rate needs two readings, so it is `null` in the first. A detail is
read the same way, while it is asked for, and at most 32 of a section's at
once; past that the answer is 503 with `Retry-After`.

A stream sends each thing it names at once, then again with each new reading,
and a comment every 15 s so an idle stream is not taken for a dead one. A
stream keeps only the latest reading of each thing for a client that falls
behind, and one that leaves readings unread for 30 s is ended.

## The sections

Each section's reading is under its own key in `reading`; the others are
`null`. `polled_at` is `null` before the first reading has finished. What the
section could not read is in `absences`, each with a `code`, the missing
`label`, the `text` the panel shows, and whether a person can do something
about it (`actionable`). `error` is a reading that failed outright.

**clock.** `time` (of the reading), `zone` (the machine's), `utc_offset_s`.

**bandwidth.** `interfaces`: each with `name`; `present` (false for a chosen
interface the system does not have now); `rx_bps` and `tx_bps`, bytes a
second, `null` until there are two readings; the counters `rx_bytes` and
`tx_bytes`; `trail`, recent rates oldest first, one a reading; and `wireless`,
`null` on a wired interface: `connected`, `signal_pct`, `rssi_dbm`,
`frequency_mhz`, `channel`, `generation` and the link rates in Mbit/s.
`vpns`: each chosen VPN with its `state` (`up`, `down`, `connecting`,
`unknown`), `interface`, `connected_since`, `latency_ms`, `failing`, `stale`
and its rates.

**usage.** `accounts`: each with its `name`, the plan's `plan_badge`, and
`windows`: `name` (`5h`, `7d`, a model's), `utilisation_pct` 0 to 100,
`resets_at`, `span_s`, and where the provider gives them `used`, `limit`,
`severity` and `note`. `fetched_at` is when the usage was last read from the
provider. The service reads through the cache the panel shares, so the panel
and the service together read a provider once a freshness window, not once
each.

**cooler.** `probes`, one a part: `id`, `role` (`cpu`, `gpu`, `memory`, `io`,
`coolant`, `fan`, `pump`), `name`, and what the part has of `load_pct`,
`temp_c` (°C), `rpm`, `bytes` of `of_bytes`, `read_bps` and `write_bps`;
`trail`, its recent load; `stale` for one not read this time. `uptime_s`.

**peripherals.** `devices`: `name`, `kind` (`mouse`, `keyboard`, `headset`,
`gamepad`, `other`), `battery_pct`, or for a device without a fuel gauge
`band` and `segments`, `charge` (`draining`, `charging`, `charged`), `stale`
and `last_seen`.

## Details and drills

A section lists its details in `details`, each an `id` and a `title`: the
cooler's `cpu`, `gpu`, `memory` and `io` lists, the bandwidth section's
`processes` and `peers`, the usage section's accounts and its development
overview. A detail's `kind` names its body, the one field set of `processes`,
`process`, `network`, `vpns`, `account`, `overview`, `day` and `change`; the
others are `null`.

A detail's `children` are the IDs of the details below it, opened the same
way: a process list's are its process names, a name's are its processes by
PID, an overview's are its days and a day's its changes. A drill's ID is its
path, `cpu/firefox/4411`; in a URL each step is percent-encoded and the steps
joined by `/`:

```console
$ hayami-tui api get "sections/cooler/details/cpu/firefox/4411"
$ hayami-tui api get "sections/usage/details/CC%20max"
```

An ID a section does not offer is 404, with `known` listing those it does. A
process that ended between two readings is a 404 too, or a page with
`"gone": true`. A process is its PID, name, parent, state, threads, sockets and
children: never its command line, path or owner.

## Errors

Anything but a 200 is `{"error": {"code", "message", "known"}}`:

| Status | `code` | When |
|---|---|---|
| 400 | `bad_request` | A body, a stream naming nothing or too many details |
| 403 | `forbidden` | An `Origin` header |
| 404 | `not_found` | A section or detail that is not there; `known` lists what is |
| 405 | `method_not_allowed` | Not `GET` |
| 503 | `busy` | Over a limit; `Retry-After` says when to come back |
| 500 | `internal` | A reading that could not be written; the service goes on |

## Checking a client, or the service

`hayami-tui api check` walks the service: health, the index, every section, its
details and their drills to a depth of three (`--breadth` drills of each, 3 by
default, 0 for all), every reading at once, what is not there, and a stream. It
holds every response to the contract, every field present, none it lacks, null
only where it allows, enums and times, and to the rules above: no string padded
or ending in an ellipsis. It exits non-zero on the first violation and names
it, `$.reading.cooler.probes[0].temp_c: a string where a number belongs`.

`hayami-tui api load --clients 32 --streams 8 --duration 5m` puts many clients
and streams on the service at once, at low priority, and reports requests,
statuses, latency percentiles, events per stream and errors.

The reference client is `internal/api/client`: the standard library and the
contract types, no more, so it shows what a client in any language needs.
