# 113 — a service other programs read

**Issue**: #396

## Status: INCOMPLETE

## Context

hayami's readings reach a person through two shells, the window and the
terminal pane, and through three commands that print and exit (`readings`,
`detail`, `doctor`). Each of them decides how a reading looks: the view cuts
a name to fit a drawer (`view.DetailNameWidth`), pads a value to a fixed
width, picks a colour. A program that wants the readings with its own look
has two choices today: run `hayami-tui --sections usage --arrangement row`
in a pane and accept hayami's drawing, or run `hayami-tui readings` on a
timer and pay a cold start, a first poll with no rates, and a three-second
settle for every detail.

The first consumer is herdr: herdr-app and herdr-core will run a JavaScript
client, `hayami-js` (not written yet, and not part of this repository), that
pulls the usage readings and draws them in herdr's own style. Others will
follow once the readings can be had without the drawing.

This spec adds a service that serves the readings, their details and drills
as JSON over a local socket, to programs of the same account, under a
published contract. It decouples function from presentation: the service
serves facts (whole names, numbers as numbers, times as times) and no
formatting. What the window and the terminal do with those facts is
unchanged.

## Decisions

Made by the operator on 2026-11-24.

- **Transport: HTTP/1.1 and JSON over a local socket.** A Unix socket on
  Linux and macOS, a named pipe on Windows, each open to one account. No TCP
  port, loopback included.
- **Updates: snapshots and a stream.** `GET` returns the latest reading; a
  Server-Sent Events stream sends each new one.
- **The installer installs it unless the person skips it.** Reinstalling or
  removing it afterwards is done by running the installer again.
- **Read-only.** Readings, details, drills and section metadata. No VPN
  control, no ending processes, no settings writes. Actions are B5 and get
  their own spec if they are ever wanted.
- **A Windows service, properly.** On Windows the service is registered with
  the Service Control Manager, not started from the Startup folder or a Run
  key. It must run as the person, because it reads their settings, their
  usage credentials and their cache: a per-user service (R6), which Windows
  instantiates for each signed-in account from one registered template.
- **macOS is in scope.** hayami runs on macOS for the usage readings, so
  the service runs there too. It is a launchd agent of the person's own
  (R7). Its install and removal go through the same installer page as on
  the other systems, which needs fynstall to gain a macOS target first
  (Gaps found).
- **Full values, regardless of the shells.** The service serves names whole
  and values unformatted. The shells' cutting and padding are presentation
  and stay in `view`; the service does not go through it.
- **A contract and a reference client.** The API is defined by a
  machine-readable contract, and a reference client in Go exercises it; the
  end-to-end tests are that client run against the installed service.

## Spike (before implementation)

Each answered on a real machine, and the findings written into this spec
before code beyond the spike is reviewed.

| Question | Why it matters |
|---|---|
| Does a service registered with `SERVICE_USER_OWN_PROCESS` (a per-user service template) through `golang.org/x/sys/windows/svc/mgr` get an instance per signed-in account on the Windows VM, running as that account, started at sign-in and stopped at sign-out? Does the instance see the account's profile (`%APPDATA%`, `%LOCALAPPDATA%`, `%USERPROFILE%\.claude`)? | R6 rests on it. Per-user services are documented by Microsoft mainly for its own services; a third-party template is expected to work and must be seen to |
| Can the instance be stopped and the template removed cleanly (instance `hayami-api_<LUID>` and template `hayami-api`), with no reboot, and can an upgrade replace the program and restart the instance? | R21–R23 |
| Does the instance's named pipe, created with an access list naming the account's SID, open from a program the person runs in their desktop session? | The transport on Windows |
| Does a systemd user socket unit (`ListenStream=%t/hayami/api.sock`, `SocketMode=0600`) start the service on first connect and let it exit when idle, on this desk? | R6 on Linux |
| Can a fynstall extension page take its default from what it found (installed: keep; not installed on an upgrade: leave off), or does fynstall need a change? | R21–R23. A fynstall change goes under "Gaps found" |
| On the Mac: can `hayami-tui` (built with `CGO_ENABLED=0`) take a socket launchd made from a `Sockets` entry (`launch_activate_socket`, in libSystem)? If not, the agent runs at sign-in and stays running, idle, as on Windows | R7's lifetime |
| On the Mac: does the agent, running in the `gui/<uid>` domain, read the login Keychain item the usage reader uses (spec 101, through `/usr/bin/security`) with no prompt beyond the one the panel already gets? | The usage section, which is why macOS is in scope |
| On the Mac: is the `$TMPDIR` the agent sees the same one a program the person starts sees (`os.tmpdir()` in Node, `os.TempDir()` in Go), and does the socket's path fit in macOS's 104-byte `sun_path` limit? | R3 on macOS |
| Which JSON Schema validator, if any, is added as a test dependency to check responses against the contract (R28)? | B7: a new dependency is a supply-chain decision, recorded here with its version and licence |

If a per-user service does not hold up, the spike stops and the operator
chooses again; this spec does not fall back to a Startup entry on its own.

## Spike findings

Run on 2026-11-24 with a throwaway program, built with `CGO_ENABLED=0` for
each platform. It serves `/v1/health` and a five-event SSE stream on a socket
it is given or makes, checks each peer's credentials, and exits when idle.

| Question | Finding | Consequence |
|---|---|---|
| systemd user socket unit (desk) | systemd made `%t/hayami-spike/api.sock` with mode `0600` in a `0700` directory. The first connection started the service on fd 3 (`LISTEN_FDS`), and the peer check (`SO_PEERCRED`) read the account's UID. SSE passed through. The service exited after its idle limit while the socket kept listening, and the next request started it again. `Restart=on-failure` did not restart it after the clean idle exit. **Stopping the socket unit left the socket file behind** | R5 holds. The unit sets `RemoveOnStop=yes`, so removal leaves no file (R25) |
| launchd socket hand-over without cgo (the Mac, macOS 27.2) | `launch_activate_socket` reached from Go with no cgo, through `//go:cgo_import_dynamic` and an assembly trampoline, as `golang.org/x/sys/unix` does for libSystem. launchd made the `SockPathName` socket at mode `0600` (`SockPathMode` 384) in a `0700` directory under the per-user temp directory. `LOCAL_PEERCRED` read the UID. SSE passed through. With `KeepAlive`/`SuccessfulExit` false, launchd started the agent at load, restarted it after `kill -9`, left it down after its idle exit (code 0), and started it again on the next connection. **`launchctl bootout` left the socket file behind** | R7 takes launchd's socket, and the agent exits when idle, as on Linux. Removal deletes the socket and its directory. `cmd/hayami-tui` stays `CGO_ENABLED=0` |
| Keychain from the agent (the Mac) | `/usr/bin/security find-generic-password -s "Claude Code-credentials"` from inside the agent returned the item with no prompt, as it does for the panel | The usage section reads from the service on macOS |
| `$TMPDIR` and `sun_path` (the Mac) | The agent's `TMPDIR` and `os.TempDir()` equal the shell's, `getconf DARWIN_USER_TEMP_DIR`. The socket's path was 70 bytes, under the 104 limit. launchd's `SockPathName` takes no variables, so the plist holds the absolute path, resolved when it is written | R3 holds. The installer resolves the directory with `confstr` when it writes the plist |
| Per-user service template (the Windows VM, build 26200) | `sc.exe create … type= userown` registered `hayami-api-spike` as `USER_OWN_PROCESS TEMPLATE` (type 0x50). **An ssh sign-in made no instance**: the template stayed stopped (exit 1077, never started), since sshd's key sign-in is not an interactive one. A console sign-in made `hayami-api-spike_<LUID>` (type 0xd0, `USER_OWN_PROCESS INSTANCE`), running as the account in session 1, with `%APPDATA%`, `%LOCALAPPDATA%`, the profile and `.claude` visible. Sign-out removed it; the next sign-in made a new one with a new LUID. **Registering the template while the account was signed in made no instance**, and starting the template fails (1058): the first instance comes at the next sign-in | R6 holds. Clients started from an ssh session find a service only while the account is also signed in at the console; `docs/api.md` says so. A first install takes effect at the next sign-in, and the installer page says so |
| Recovery actions (the Windows VM) | `sc failure` on an instance fails (87); on the template it succeeds. An instance made before the template had them did not restart after `taskkill /f` (exit 1067, stopped); the instance made at the next sign-in carried them and restarted under a new PID within seconds | The installer sets the recovery actions when it registers the template, before any instance exists. Changing them later takes effect at the next sign-in |
| The instance's pipe (the Windows VM) | `winio.ListenPipe` at `\\.\pipe\hayami-spike-<SID>` with `D:P(A;;GA;;;<SID>)`. A client in the desktop session (session 1, a `schtasks /it` task) and one over ssh (session 0) both read `/v1/health` and the SSE stream. A second server on the same name was refused ("Access to the path is denied") | The transport on Windows holds. R4's second-`serve` refusal is the pipe's |
| Stop, upgrade, removal (the Windows VM) | The running program could not be overwritten ("being used by another process"). `sc stop` on the instance, replacing the program, then `sc start` on the instance ran the new program. **`sc delete` on the template left the stopped instance registered**; `sc delete` on the instance removed it, with no reboot, and no `hayami-api-spike` key remained under `Services` | An upgrade stops each `hayami-api_*` instance, replaces the program and starts them again. Removal stops and deletes each instance, then deletes the template (R25) |
| fynstall page default | Read from fynstall 0.2.6 (`extension.Page`): under `--yes` a page is not run and each parameter takes its static default from `fynstall.yaml`, so a default of `yes` would reinstall a removed service on every `--yes` upgrade | No fynstall change. The `api` parameter is three-valued, with `once: true` and default `auto`: `auto` installs on a first install and leaves an upgrade as it found the service, and an explicit `yes` or `no` always wins. The page asks with what it found as the default. **Corrected in 3c:** the install half cannot tell an upgrade (see Gaps found), so `auto` reads a marker hayami writes in the install directory |
| JSON Schema validator | `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3: JSON Schema 2020-12, which OpenAPI 3.1's schemas are written in, Apache-2.0. The YAML is read with `go.yaml.in/yaml/v3`, already a dependency | Test-only. Recorded under B7 |
| Pipe listener | `github.com/Microsoft/go-winio` v0.6.4, `winio.ListenPipe` with an SDDL, chosen by the operator on 2026-11-24 over a listener written on `x/sys` | A shipped dependency on Windows. Recorded under B7 |

## Delivery

In phases, each its own PR; the criteria below are checked as the phase that
meets them lands.

| Phase | What | State |
|---|---|---|
| 1 | Spike findings (above) | Merged |
| 2 | R14: `panel.RawDetailer` on the bandwidth, cooler and usage sources. `ReadDetail` returns a `panel.RawDetail` (processes with their PIDs and whole names, the network reading with the peers' whole host names, a process's page without its command line, an account's breakdown, the overview, a forge's day, a change's page); `DrawDetail` builds the `view.Detail` from it and reads nothing; `PollDetail` is the one drawn from the other. `core.PeerNames` keeps whole names (`Full`) and shortens for a column (`Name`); a process's parent and children are names and PIDs. The shells are unchanged: the existing tests pass unmodified, and the bandwidth breakdown's pages, which had none, are pinned by tests that pass on the code before the change too. Found on the way: #399, a cooler row with a long name opens an idle page, fixed on its own | This PR |
| 3a | The contract (`api/openapi.yaml`, `internal/api/contract`), the server (`internal/api`: leases, one poller a source, snapshots, SSE with a latest-only queue, the limits, panic recovery), the Linux socket with its peer check and systemd's hand-over, and `hayami-tui serve`. Tests: every endpoint's golden response checked against the contract, each Go type held to its schema, the behaviour over a real socket, nothing secret over real sources, a fuzz target, and the load test. The adversarial review's five findings fixed and pinned; #401 filed | Merged (#402) |
| 3b | The reference client (`internal/api/client`: the address rule, strict decoding against the contract types, the stream, `Check`, `Load`) and `hayami-tui api address/status/get/watch/check/load`. The contract types carry `enum`, `format` and `nullable` tags, held to the schema by the parity test; `api/openapi.yaml` is validated as an OpenAPI 3.1 document against the OpenAPI Initiative's schema, vendored as a test fixture. The adversarial review's six findings fixed and pinned | Merged (#403) |
| 3c | The systemd user units (`internal/api/unit`, standard library only: written as the person, hayami's mark on each, `RemoveOnStop`, `try-restart` on an upgrade), the installer's Service page (`api`: `auto`, `once`; `auto` sets the service up the first time the installer decides, by a marker in the install directory, and never gives back one removed), `install.sh` and `uninstall.sh`, `hayami-tui api status` with the manager's state, `docs/api.md`, CLAUDE.md, and the desk's end-to-end run. The adversarial review's two findings fixed and pinned | This PR |
| 4 | Windows per-user service: the pipe address and listener, `serve` under the service manager, the client's pipe dial with its owner check, the template with its Program Files copy, the installer page on Windows | This PR |
| 5 | macOS agent and CI job | — |

## Requirements

### The process

- **R1 One subcommand.** `hayami-tui serve` runs the service in the
  foreground until stopped; it is what the systemd unit, the Windows
  service and the launchd agent run. Under the Service Control Manager it speaks the service
  protocol (`svc.Run`); started from a terminal it runs as a plain
  process. `--listen <path>` names the socket or pipe, for tests and for
  running by hand; empty is the default address (R3).
- **R2 A new shell, `internal/api`.** It sits beside `gui`, `tui` and `cli`
  in the layer table: it imports `panel`, `config` and the contract types
  (R26), and decides nothing about what a section reads. It does not import
  `view`'s formatters and serves nothing `view` has cut or padded. The
  listener's platform code (socket, pipe, peer check, service protocol)
  follows rule 5: `_linux`, `_windows`, `_darwin` and `_other` files.
- **R3 The address is one function.** `api.Address()` returns the default:
  `$XDG_RUNTIME_DIR/hayami/api.sock` on Linux; `\\.\pipe\hayami-api-<SID>`
  on Windows, the SID being the account's; `$TMPDIR/hayami/api.sock` on
  macOS, where `$TMPDIR` is the per-user directory under `/var/folders`
  (`confstr(_CS_DARWIN_USER_TEMP_DIR)` if the variable is unset). A path
  longer than the platform's `sun_path` (104 bytes on macOS, 108 on Linux)
  is refused with an error naming the limit, never cut short.
  `hayami-tui api address` prints it. The contract (R26) states the
  rule, and every client follows it.
- **R4 Only the account that runs it.** Linux and macOS: the directory is
  `0700` and the socket `0600`, and each connection's peer credentials
  (`SO_PEERCRED`, `LOCAL_PEERCRED`) are checked against the service's own
  user; a mismatch is closed without a response. Windows: the pipe's access
  list grants the account alone (`D:P(A;;GA;;;<SID>)`), and the pipe is
  created with `FILE_FLAG_FIRST_PIPE_INSTANCE`, so a pipe already at the
  name is a refusal to start rather than a second listener.
- **R5 Linux lifetime.** A systemd user socket unit and service,
  `hayami-api.socket` and `hayami-api.service`. systemd holds the socket,
  the first connection starts the service, and the service exits after five
  minutes with no request and no open stream.
- **R6 Windows lifetime.** A per-user service template, `hayami-api`, of type
  `SERVICE_USER_OWN_PROCESS`, start type automatic, whose program is a copy
  of `hayami-tui.exe` in Program Files, `hayami\hayami-api.exe` under the
  known folder (not `%ProgramFiles%`, which the environment sets), with the
  argument `serve`. Not the installed `hayami-tui.exe`: hayami installs under
  `%LOCALAPPDATA%`, which the person can write, and Windows makes an instance
  of the template for every account that signs in, so a program one account
  can write would run as another (decided with the operator, 2026-11-25). An
  instance for an account that has never run hayami (no settings directory)
  exits at once with success, which the recovery actions leave alone. Windows starts an
  instance as each signed-in account and stops it at sign-out. The instance
  restarts on failure (the service's recovery actions: restart after 5 s,
  twice, then stay stopped), which are set on the template when it is
  registered, since an instance takes them from the template when it is
  made and refuses them afterwards. Idle, it polls nothing (R9). Registering the
  template needs an administrator, once, through the installer's UAC
  prompt, as the network helper's install does; the instance has no right
  the account lacks. Windows makes instances only at an interactive
  sign-in, so a first install takes effect at the next sign-in and the
  installer page says so; an ssh sign-in makes none.
- **R7 macOS lifetime.** A launchd agent of the person's own,
  `io.ushineko.hayami.api`, at `~/Library/LaunchAgents/io.ushineko.hayami.api.plist`
  and loaded into `gui/<uid>` (`launchctl bootstrap`). Its program is the
  installed `hayami-tui` with the argument `serve`. It is restarted if it
  fails (`KeepAlive` with `SuccessfulExit` false, `ThrottleInterval` 5 s) and
  not restarted after a clean exit. If the spike finds launchd can hand over
  the socket, launchd holds the socket and the agent exits when idle, as on
  Linux (R5). Otherwise it starts at sign-in (`RunAtLoad`), makes its own
  socket, and stays running; idle, it polls nothing (R9). No administrator
  is asked for: the agent is the person's, in their home.
- **R8 The person's settings, followed.** The service reads the same
  settings file the panels do: chosen interfaces, VPNs, accounts and every
  `sectionSettings` value. A change to the file is picked up before the next
  poll, without a restart. The panels' section order and visibility do not
  limit what the service serves: a client asks for what it wants.

### Polling

- **R9 Nothing is polled that nobody reads.** A section is polled while a
  client is reading it: a request for it within the last three of its
  intervals (and never less than 30 s), or an open stream that names it. A
  detail is polled the same way, at its own `DetailInterval`, and
  `CloseDetail` is called when its lease lapses. A service with no client
  reads nothing.
- **R10 One source per section, shared.** Every client of a section is
  served from one `panel.Source`, polled at its own `Interval()` (or its
  `Next`, for a paced source such as the clock). Any number of clients cause
  one set of polls. The usage cache's freshness gate and lock are unchanged,
  so the service is one more reader of the shared cache and adds no upstream
  call the panels would not make.
- **R11 The first request waits for a reading.** A section asked for while
  nobody was reading it is polled at once, and the request waits for that
  poll up to the source's own timeout; then it is answered with what there
  is, and an `error` if the poll failed. A rate needs two readings, so the
  first answer has totals and no rates (`null`); the next poll has them.

### Full values

- **R12 Facts, not formatting.** Every value the service serves is the value
  the source holds, mapped to a contract type:
  - names whole: processes, devices, accounts, interfaces, VPNs,
    repositories, pull-request titles and descriptions. A process is still
    its name and nothing else (spec 060 R6): no command line, path or owner,
    in the API as in the shells. On Linux the name is `core.FullProcName`'s,
    which is longer than the kernel's 15 characters where the program is.
  - numbers as JSON numbers in base units: bytes, bytes per second, degrees
    Celsius, percent as 0–100, RPM, milliseconds of latency.
  - times as RFC 3339 in UTC; durations as seconds.
  - a status as a word (`good`, `warn`, `bad`, `info`, `dim`), the band's
    verdict on a value, which a client may use or ignore.
  - an absence as a typed reason: its `code` (`core.Absence`'s), the text a
    person reads, its detail, and whether a person can act on it.
- **R13 Text is cleaned, not cut.** A string from outside hayami (a device's
  name, a VPN's, a pull request's title) passes the printable filter the
  shells use (`internal/safetext`): control and bidirectional-override
  characters are removed. Nothing is truncated, padded or ellipsised. A
  string longer than 4 KiB is the exception and is cut, with `truncated:
  true` beside it, so a hostile device name cannot make a response
  unbounded.
- **R14 Raw details from the sources.** `panel.Detailer` gains a raw form
  of each detail: the plain values a source builds its `view.Detail` from
  (processes and their use, peers, an account's limits, a VPN's checks, a
  day's changes). One read serves both: the source reads once per detail
  poll, the shells build their `view.Detail` from the raw value, and the
  service maps the same raw value to the contract. No detail is read twice
  because two shells and the service want it. Drills (spec 065) are details
  whose ID is their path, as now, and their raw form names each child's
  drill ID, so a client can follow them.
- **R15 Section data through contract types.** `Source.Data()` is mapped
  into contract types per section in `internal/api`, not served as the
  source's Go type. A source refactor changes the mapping, not the API.

### Endpoints

All responses are `application/json; charset=utf-8`, except the stream.
Every path is under `/v1`. Only `GET` is served; any other method is 405.
Every shape below is defined in the contract (R26); the outlines here are
for review.

- **R16 `GET /v1`.** `{"service":"hayami","version":"<VERSION>","api":"1.0",
  "sections":["clock","bandwidth","usage","cooler","peripherals"]}`, the
  keys in `view.Sections()` order. `api` is the contract's version (R27).
- **R17 `GET /v1/sections`.** One entry per section from the registry:
  `key`, `title`, `default` (on by default), `shown` (on in the person's
  settings, for a client that wants to follow the panel), `interval_s`.
- **R18 `GET /v1/sections/{key}`.** One section's reading:

  ```json
  {
    "key": "usage",
    "polled_at": "2026-11-24T10:15:02Z",
    "interval_s": 60,
    "reading": { },
    "absences": [ {"code": "", "text": "", "detail": "", "actionable": false} ],
    "error": null,
    "details": [ {"id": "accounts", "title": "Every account"} ]
  }
  ```

  `reading` is the section's contract type (R15). Outline of each:
  - `clock`: `time`, `zone`.
  - `bandwidth`: per interface `name`, `rx_bps`, `tx_bps`, totals, a trail
    of recent rates; per chosen VPN `name`, `state`, `interface`,
    `connected_since`, `latency_ms`, its rates.
  - `usage`: per account `name`, `provider`, `plan`, and per window
    (`five_hour`, `seven_day`, model windows) `utilisation_pct`,
    `resets_at`, `stale`, `fetched_at`.
  - `cooler`: per probe `id`, `role`, `name`, temperatures, load, fan and
    pump speeds, as the probe has them.
  - `peripherals`: per device `name`, `kind`, `battery_pct`, `charging`,
    `last_seen`, `asleep`.

  An unknown key is 404 with the known keys in the body. An error names no
  path under the home (`safetext.WithoutHome`).

  As built (phase 3a; `api/openapi.yaml` is the contract): `reading` is an
  object with a field for every section, the section's own set and the
  others `null`, so a typed client reads `reading.usage` without a
  discriminator. R17's list is `{"sections": [...]}` and R19's map is
  `{"readings": {...}}`, each in an object so a field can be added beside
  it. The usage reading's accounts carry `plan_badge` (the card's letter);
  the plan's name is in the account's detail. A peripheral's `asleep` is
  not read by any source yet, and its `charge` is a word (`draining`,
  `charging`, `charged`).
- **R19 `GET /v1/readings?sections=a,b`.** A map of key to R18's object,
  for the sections named, or every section when the parameter is absent.
- **R20 `GET /v1/sections/{key}/details/{id...}`.** One detail's raw form
  (R14), with `polled_at`, `absences` and `children`: the drill IDs below
  it. `{id...}` is the detail's ID with each `/`-separated step
  percent-encoded, so a drill is its path: `details/cpu/firefox/4411`,
  `details/accounts/CC%20max`. Action links (`!kill`, `!pause`, the VPN
  actions) are not part of a raw detail and never appear. An ID the section
  does not offer, or a drill `panel.Offers` refuses, is 404 with the offered
  IDs.
- **R21 `GET /v1/events?sections=a,b&details=key/id,…`.** A Server-Sent
  Events stream. An initial event for each named section (`event: section`,
  `data:` R18's object) or detail (`event: detail`, `data:` R20's object)
  with the latest reading, then one per new poll. A comment line every 15 s
  keeps an idle stream from being taken for a dead one. A client
  disconnecting ends its lease. At most 16 streams are open at once; the
  seventeenth is 503.
- **R22 `GET /v1/health`.** `{"ok":true}` without polling anything.

### Hardening

- **R23 The server's limits.** `ReadHeaderTimeout` 5 s; a request with a body
  is 400; headers capped at 16 KiB; at most 64 open connections. No CORS
  headers are sent, and a request carrying an `Origin` header is 403:
  nothing in a browser reaches a Unix socket or pipe today, and a bridge
  that let one would have to be a decision, not an accident.
- **R24 Nothing secret leaves.** No endpoint carries a token, a refresh
  token, an API key, a credential path or a forge key. A test serves every
  section and every detail over fixtures holding known token strings and
  asserts none of them appears in any response.

### The installer

- **R25 A page, on by default.** The installer gains an API page beside the
  Network, Sensors and Keepalive pages, with a parameter `api` (`--api=yes`
  or `--api=no`) whose default on a first install is `yes`. The page says
  what the service is, that it serves the readings to programs of this
  account and no other, that it has no right the panel lacks, and on Windows
  that registering it asks for an administrator once.
  - **Running the installer again is how it changes.** When the service is
    installed the page offers to keep it (the default) or remove it. When it
    is not installed on an upgrade, the page offers it and defaults to
    leaving it off, so a person who removed it is not given it back by
    `--yes`. An upgrade restarts an installed service on the new program.
  - **Removal is complete.** Removing it, by the page or by uninstalling
    hayami, stops it and removes the socket and service units (Linux), the
    running instance and the template (Windows: each `hayami-api_*`
    instance is stopped and deleted before the template, because deleting
    the template leaves its instances registered), or the agent (macOS:
    `launchctl bootout`, then the plist). No socket file is left behind. A
    step that fails is a warning, as on the other pages: hayami works without
    it.
  - **The page on macOS** is the same page, in the macOS installer fynstall
    builds once it has the target (Gaps found).
  - **Where there is no service manager**, Linux without a systemd user
    manager: the page says the service is not set up on this system and that
    `hayami-tui serve` runs it by hand.
  - `hayami-tui api status` says whether it is installed, running, and at
    which address. Install and removal are the installer's; there is no
    `api install`.

### The contract

- **R26 An OpenAPI 3.1 document, `api/openapi.yaml`, is the contract.**
  Every path, parameter, status code, response schema, the SSE event names
  and their payload schemas, the error shape, and the address rule (R3) in
  its description. It is the source of truth: the Go contract types in
  `internal/api/contract` are written to it, and `hayami-js` generates or
  writes its types from it. `docs/api.md` is the prose: what each reading
  means, its units, when a field is `null`, and how to connect on each
  platform, with the reference client's use as the example.
- **R27 Versioning.** The contract carries `info.version` (`1.0` at first).
  Within `/v1` a field or endpoint may be added (minor version) and none
  removed, renamed, retyped or made required where it was optional; such a
  change is `/v2`. A field whose value is unknown is `null`, never omitted,
  so a client sees the shape it was promised.
- **R28 Conformance is tested.** Golden responses of every endpoint over
  fixture sources are checked against `api/openapi.yaml`'s schemas in a unit
  test, and the golden files pin the values. A change to a response that the
  schema does not allow fails; a change the schema allows but a golden file
  does not is reviewed as an addition. A test fails if a contract type in
  Go has a field the schema lacks, or the reverse.

### The reference client

- **R29 `internal/api/client`, in Go.** A typed client over the contract
  types: finds the address (R3), dials the socket or the pipe, calls every
  endpoint, reads the SSE stream into typed events, and reports a contract
  violation (a field missing, a type wrong, an unknown status) as an error
  rather than a zero value. It has no dependency on `panel`, `view` or
  `core`: only the contract types and the standard library, plus
  `golang.org/x/sys` for the pipe, so it shows what any client needs.
- **R30 `hayami-tui api` is the client's command line.** `api address`,
  `api status`, `api get <path>` (prints the response),
  `api watch --sections … --details …` (prints events), `api load` (R32), and
  `api check`: walks every endpoint, every section, every detail and every
  drill the service offers, to a depth of three, validates each response
  against the contract, and checks the full-value rules (R12, R13): no
  string in a response ends in padding or an ellipsis that is not in the
  source, numbers are numbers, times parse. It exits non-zero on the first
  violation and names it. This is what the end-to-end tests run.

### Many clients at once

- **R31 The server stays up and fair under concurrent clients.**
  - A request is answered from the section's latest snapshot and never waits
    on a poll, except the first request for an unread section (R11). Polls
    run on the service's own goroutines, one per watched source, guarded so a
    source is never polled twice at once.
  - A slow client cannot stall the others. Each stream has its own bounded
    queue that keeps only the latest event per section or detail. A stream
    whose queue stays full for 30 s is closed. A write to any connection has
    a deadline.
  - Over the limits (64 connections, 16 streams), the client gets an
    answer: 503 with a `Retry-After`. It is never left hanging. A connection
    that sends nothing is closed by `ReadHeaderTimeout`.
  - Clients leaving release everything: their leases lapse (R9), and the
    goroutine count, open handles and heap go back to their idle level.
  - A panic while serving one request is recovered, logged without a path
    under the home, and answered 500. The service keeps running.
- **R32 `hayami-tui api load`** is the reference client's load generator:
  `--clients N --streams M --duration D`. Each client loops over random
  `GET`s of sections, details and drills. Streams connect, read, and some
  disconnect and reconnect at random. It reports request counts, status
  codes, latency percentiles, events received per stream, and the errors
  seen. It runs at low priority by default (`nice` on Linux, below-normal
  priority on Windows) so it can run on a desk in use.

### Documentation

- **R33 Architecture guide.** `docs/architecture.md` gains `internal/api` in
  the layer table and the shape, rule 3 gains the raw detail form (R14),
  rule 9 gains the socket and the per-user service, and the Status table
  gains the service, in the same PR. CLAUDE.md's architecture rules gain the
  service's paragraph, beside the helpers'.

## Acceptance Criteria

### Spike

- [x] Spike findings are recorded in this spec: the per-user service on
      the Windows VM, the systemd socket unit on the desk, the launchd socket
      hand-over, Keychain access and `$TMPDIR` on the Mac, the fynstall default,
      the schema validator's choice. A fynstall change is under "Gaps found"
      with its own issue

### Contract and full values

- [x] `api/openapi.yaml` exists, validates as OpenAPI 3.1, and describes
      every endpoint, every section's reading, every detail's raw form, the
      SSE events and the errors
- [x] Golden responses for every endpoint over fixture sources validate
      against the contract (R28), and a Go contract type and its schema
      cannot drift apart without a test failing
- [x] A fixture process name of 40 characters, a device name of 40, an
      account name of 40 and a pull-request title of 200 come back whole
      from their section and detail endpoints; the same fixtures are still
      cut in the window's and the terminal's rendering (the shells
      unchanged)
- [x] A fixture name with control and bidirectional-override characters
      comes back with them removed and nothing else changed; a 10 KiB name
      is cut to 4 KiB with `truncated: true`
- [x] Rates, temperatures, percentages and times are JSON numbers and RFC
      3339 strings; an unknown value is `null`, not absent
- [ ] Each detail is read once per poll whether the window, the terminal and
      the service all show it (counting fake Detailer)

### Service behaviour

- [x] `hayami-tui serve --listen <tmp>` serves every endpoint R16–R22 over a
      real socket in `t.TempDir()`
- [x] A drill ID with `/` and a detail ID with a space both resolve through
      R20; an unoffered ID is 404 listing the offered ones; no action link
      appears in any response
- [x] Two clients of the same section cause one poll per interval
- [x] A section with no client for its lease stops being polled; a detail's
      lease lapsing calls `CloseDetail`
- [ ] The first request for an unread section waits for its first poll; the
      next poll's response carries rates
- [ ] A settings change (a chosen interface) shows in the next poll without
      a restart
- [x] The SSE stream sends an initial event, an event per new poll, a
      keepalive comment, and ends the lease on disconnect; the seventeenth
      stream is 503
- [x] Non-GET is 405, a body is 400, an `Origin` header is 403, an unknown
      section is 404
- [x] No response from any endpoint contains a fixture token, refresh token
      or forge key (R24)
- [x] A fuzz target covers the router and the ID decoding (B4)

### Concurrency and load

- [x] A load test in `internal/api`, run under `-race` over fixture sources
      for 60 s, passes. It runs 48 clients looping `GET`s over every section,
      detail and drill, and 16 SSE streams with random disconnects and
      reconnects. One stream never reads, and one connection sends half a
      request and stops. It asserts:
  - no response but the documented 503s past the limits, and no 500
  - p99 latency of a snapshot `GET` under 50 ms
  - one poll per source per interval, however many clients
  - every live stream still receiving events at the end
  - the stream that never reads is closed after 30 s, and the half request
    after `ReadHeaderTimeout`
  - after every client leaves: goroutines back to their idle count, and heap
    within 10% of its idle level once the leases lapse

  As built (`TestManyClientsAtOnce`, 2026-11-25: about 1.8 million requests
  a minute, snapshot p99 5.5 ms under `-race`): the streams are 14 and the
  one that never reads, so with it there is room for a reconnect under the
  limit of 16 (the limit itself is `TestOverTheStreamLimitIsA503`). The
  stream that never reads is closed by the write deadline once its socket's
  buffer is full, well inside 30 s. The idle level is taken after a warm-up
  from 64 goroutines, and the heap is allowed 10% or 1 MiB over it, whichever
  is more: a heap profile showed what remains to be the runtime's own
  threads, which it keeps once made. The p99 bound is 50 ms on eight
  processors or more and scales up on fewer, where the test's own clients
  and the race detector compete with the server for them (a two-processor
  CI runner measured 54 ms). Falsified with the stream queue's
  signal made blocking: the test fails, and the queue was put back.
- [x] The 65th connection and the 17th stream get 503 with `Retry-After`
      within a second, and are never left hanging
- [x] A source that panics in a poll, and a handler that panics, leave the
      service serving the other sections
- [x] The load test is falsified: a single unbuffered stream writer, or a
      poll taken under the request's lock, makes it fail. The fault is then
      put back
- [ ] `hayami-tui api load --clients 32 --streams 8 --duration 5m` against
      the installed service on the desk, on the Windows VM and on the Mac runs with
      no errors.
      The service's RSS is flat in the last three minutes, `api check`
      passes straight afterwards, and the panels keep updating throughout

  As built in 3c: passed on the desk against the installed service (E2E
  results below). The Windows VM and the Mac are 4 and 5.

### Platforms and installer

- [ ] Linux: the socket is `0600` in a `0700` directory; a connection from
      another UID is closed with no response
- [ ] Linux: the socket unit starts the service on connect and the service
      exits after the idle period (real user manager on the desk)
- [ ] Windows: the template is registered as `SERVICE_USER_OWN_PROCESS`; an
      instance runs as the signed-in account after sign-in and is gone after
      sign-out; the pipe's access list holds that account alone; a second
      `serve` against an existing pipe refuses to start (the Windows VM)
- [ ] macOS: the socket is `0600` in a `0700` directory under the per-user
      `$TMPDIR`; a connection from another UID is closed with no response
      (`LOCAL_PEERCRED`); the agent is loaded in `gui/<uid>`, restarts after
      a crash, and stays down after a clean stop; the usage section reads
      through the Keychain from the agent (the Mac)
- [ ] An address longer than `sun_path` is refused with an error naming the
      limit (unit test, Linux and macOS)
- [ ] CI runs the `internal/api` tests, the load test included, on Linux,
      Windows and macOS runners (a `macos-latest` job is added to
      `build.yml`)
- [ ] The installer page installs by default on a first install, keeps an
      installed service by default, offers removal, and does not reinstall a
      removed one under `--yes` on an upgrade (installer tests, plus real
      runs on the desk, on the Windows VM and on the Mac)
- [ ] An upgrade restarts the installed service on the new program (desk,
      the Windows VM and the Mac)
- [ ] Uninstalling hayami removes the units, the instance and template, or
      the agent, and the socket (desk, the Windows VM and the Mac)

  As built in 3c, Linux: the socket's mode and the other account's refusal,
  socket activation, the idle exit, the installer page's default, keep,
  removal and `--yes` behaviour, and the upgrade's restart all held on the
  desk (E2E results below). Uninstalling hayami was not run on the desk; its
  Linux half is held by the tests. Each criterion stays open for the
  Windows VM and the Mac.

### Reference client and documents

- [x] `internal/api/client` imports no `panel`, `view` or `core` (a test
      reads its imports)
- [ ] `hayami-tui api check` passes against the installed service on the
      desk, on the Windows VM and on the Mac, and fails, naming the violation, against a test server
      that serves a padded name, a string where a number belongs, and a
      missing field (falsified)

  As built in 3b: the test server's half holds (`TestCheckNamesWhatIsWrong`:
  a padded name, a name cut with an ellipsis, a string where a number
  belongs and a missing field each fail, named by path; the same stand-in
  unbroken passes). Against the real server over the fixtures it passes
  (`TestCheckPassesAgainstTheService`), and against `serve` on the desk over
  the real sources, run by hand on 2026-11-25, it passed: 45 details,
  every reading at once, the 404s and a stream, and again after a 20 s
  `api load` of 32 clients and 8 streams (about 300,000 requests, no
  errors, p99 2.9 ms). The installed service on the three machines is 3c,
  4 and 5.

  Two choices made in 3b. **Strict decoding, not the schema, in the
  client** (decided with the user): the client checks a response with the
  standard library against the contract types, every field present, none
  the contract lacks, null only for a pointer or a field tagged `nullable`,
  an `enum` tag's values, a `format:"date"` date, a `time.Time` as RFC
  3339. The parity test holds every tag to the schema, both ways, so the
  checker stays out of the binary (B7 unchanged) and `hayami-js` can do
  the same. **`api check` follows three drills of each detail by default**
  (`--breadth 3`, `0` for every one), to a depth of three (`cpu`,
  `cpu/firefox`, `cpu/firefox/4411`). A process list's drills are every
  process name on the machine, and the service reads at most 32 details of
  a section at once (R23), so every one is minutes of waiting for leases
  to lapse. Over that limit `check` waits as the 503 says and asks again,
  and a drill whose process ended between two reads is counted, not failed.
- [x] `docs/api.md`, `docs/architecture.md`, CLAUDE.md and
      `docs/threat-model.md` are updated in the PR
- [x] `cmd/hayami-tui` still builds with `CGO_ENABLED=0`
- [x] README changelog line under `### Unreleased`

## Threat model delta

| Boundary | What changes for an attacker | Section updated |
|---|---|---|
| B1 Privileged helpers | No new privileged component: the service runs as the person. But on Windows a service template is registered in the Service Control Manager, and its program path is a target: every account that signs in runs it. It is a copy in Program Files, which only an administrator writes (as the network helper's is, spec 061), never the installed `hayami-tui.exe` under `%LOCALAPPDATA%`, which one account could change to run code as the next to sign in. The copy is taken from a handle held shut against writes (#244), and an upgrade replaces it under the same UAC prompt, a `--yes` upgrade included (decided with the operator: one prompt, so the service is not left on an old program); refused, the old copy keeps serving and the installer says so. Checked by the adversarial review | B1 |
| B2 Installing | The installer, by default, writes a systemd user socket and service (Linux; no root), registers a per-user service template (Windows; one UAC prompt), or writes a launchd agent into `~/Library/LaunchAgents` (macOS; no administrator). A path in a unit gets the keepalive's quoting rules (spec 085's findings). The plist is written as XML by an encoder, never by joining strings, so a path cannot add a key. On macOS anything running as the person can rewrite their own agent, which is no more than it can already do to their shell profile | B2 |
| B3 Credentials | A long-running process holds the usage reader and its OAuth refresh, as the panel does. New: its responses leave the process, so a credential reaching a response would be served to every program of the account. R24's test is the defence | B3 |
| B4 What hayami parses | New input: HTTP requests on the socket or pipe, from the same account only (R4) but not trusted to be well formed. Request limits (R23) and a fuzz target on the router and the ID decoding. The reference client parses what answers on the socket: a response and a stream's event are read to 16 MiB at most and decoded strictly, and what it prints to a terminal is cleaned again (3b) | B4, B6 |
| B6 What hayami shows | The readings and every detail, the on-request ones included (a VPN's addresses, spec 085 R9; the Development drawer's activity, spec 107), are served whole to any program the account runs. `hayami-tui detail` gives the same account the same facts today, cut; now they are continuous, whole, and need no terminal. No other account reaches them (R4). Windows pipe-name squatting: another account creating the pipe first is defeated by the SID in the name, the access list and `FILE_FLAG_FIRST_PIPE_INSTANCE`; `docs/api.md` says how a client checks the server's process owner (`GetNamedPipeServerProcessId`), and the reference client does | B6 |
| B7 Supply chain | Possibly a JSON Schema validator as a test dependency (spike); nothing new in the shipped binaries beyond `golang.org/x/sys`, already present. A macOS release artefact is new: it carries the provenance the others do. Signing and notarising it is fynstall's open item ("signing the installer"), and until then Gatekeeper's quarantine applies to a downloaded installer | B7 |
| B5 Outside itself | Unchanged: nothing here acts | — |

B1, B2, B3 and B7 are touched, so the PR gets the document's adversarial
review: a reviewer given the code and the attackers (another local account;
a same-account program that should not get a credential; a browser page;
someone who can write where the service's program lives), and a second pass
that tries to refute each finding.

## Risks & Assumptions

- **Per-user services are the risk this spec carries.** They are a Windows
  10 1709+ mechanism, documented by Microsoft mostly for its own services.
  The spike tests it before anything else; if it fails, the operator decides
  again.
- **The macOS installer depends on fynstall.** fynstall has no macOS
  target today, and hayami's release builds no macOS binaries. Until both
  exist, the macOS installer criteria cannot be met. The rest of the macOS
  half (the agent's code, the socket, the peer check, the CI job, `serve` run
  by hand on the Mac) can land without them. Whether to ship Linux and
  Windows first and add the macOS page in a second PR is for the operator to
  decide once fynstall's spec has a size.
- **R14 touches every Detailer.** Each source that offers details gains a raw
  form and builds its `view.Detail` from it. This is the largest part of the
  work and changes code the shells depend on; the parity test and the
  existing detail tests must pass unchanged, which is how the refactor is
  shown to change no drawing.
- **Assumption: a source is safe to poll on one goroutine while another reads
  its data.** The shells already do so; the race detector runs over the
  service tests.
- **Default yes widens who has it,** and on Windows asks for an administrator
  during a first install that did not before. Stated on the page; `--api=no`
  skips both.
- **Rollback.** The service is its own process behind its own units or
  service: the installer page's removal takes it away without touching the
  panels. Reverting the PR removes the subcommand and the page; an installed
  unit or template then fails to start and is removed by the next installer
  run, or by hand (`systemctl --user disable --now hayami-api.socket`;
  `sc.exe delete hayami-api`; `launchctl bootout gui/$(id -u)/io.ushineko.hayami.api`
  and removing the plist).
- **Query budget.** None: no database. Upstream calls are bounded by the
  usage cache gate (R10).

## E2E Test Plan

The end-to-end tests are the reference client (`hayami-tui api check`, R30, and `api load`, R32)
run against the installed service, plus the lifecycle steps around it.

| Environment | Steps | Expected | AC |
|---|---|---|---|
| Desk (Linux) | Install with the fynstall CLI installer, defaults; `systemctl --user status hayami-api.socket` | Socket active, service not yet started | installer default, R5 |
| Desk | `hayami-tui api check` | Every endpoint, section, detail and drill to depth three conforms; exit 0 | contract, full values, client |
| Desk | `hayami-tui api get /v1/sections/usage/details/…` for an account, and the Development detail for a day with a long pull-request title | Name and title whole, compared with `gh pr view` for the same PR | full values |
| Desk | `hayami-tui api watch --sections usage,bandwidth` for two minutes | An initial event per section, then one per poll; keepalive comments | SSE |
| Desk | `hayami-tui api load --clients 32 --streams 8 --duration 5m`, with the panel running; sample the service's RSS every 10 s; `api check` afterwards | No errors but documented 503s; RSS flat after warm-up; panel keeps updating; check passes | concurrency |
| Desk | Wait past the idle period with no client | Service inactive, socket listening; the next `api get` starts it | R5 |
| Desk | `sudo -u nobody curl --unix-socket … http://hayami/v1/health` | Refused by mode or closed by the peer check | R4 |
| Desk | Re-run the installer and choose remove; then re-run with `--yes` | Units and socket gone; not reinstalled | installer |
| The Windows VM | Install with defaults (one UAC prompt); sign out and in; `hayami-tui api check` | Instance running as the account; check passes | R6, contract |
| The Windows VM | A process started from a program named with 40 characters, then `api get /v1/sections/cooler/details/cpu` | The name whole | full values |
| The Windows VM | The same `api load` run against the pipe, sampling the instance's working set | As on the desk | concurrency |
| The Windows VM | Second `hayami-tui serve` while the instance runs | Refuses to start, naming the existing pipe | R4 |
| The Windows VM | Sign out; check from an administrator's session that no instance runs | No instance | R6 |
| The Windows VM | Re-run removing it; then reinstall and uninstall hayami | Instance and template gone each time; no pipe left | installer |
| The Mac | Install with the macOS installer, defaults; `launchctl print gui/$(id -u)/io.ushineko.hayami.api` | Agent loaded, running `hayami-tui serve` | installer default, R7 |
| The Mac | `hayami-tui api check`, then `api get /v1/sections/usage` | Check passes; usage read from the Keychain, the same figures the panel shows | contract, usage on macOS |
| The Mac | The same `api load` run, sampling the agent's RSS | As on the desk | concurrency |
| The Mac | `kill -9` the agent; then `launchctl kill TERM` it | Restarted after the crash; stays down after the clean stop | R7 |
| The Mac | `sudo -u nobody` connection to the socket | Refused by mode or closed by the peer check | R4 |
| The Mac | Re-run removing it; then reinstall and uninstall hayami | Agent unloaded and plist gone each time; no socket left | installer |

### Results: the desk, 3c, 2026-11-25

Run with the installers `make installer` built from this branch, by the
fynstall CLI installer, user scope.

| Step | Result |
|---|---|
| Install, defaults | `hayami-api.socket` enabled and listening, the service started by the first connection; the marker `share/api-asked` written |
| `api check` | Passed: 73 requests, 5 sections, 46 details, 5 events |
| Upgrade (`--yes`) | The running service was stopped and the socket's next connection started the new program (`try-restart`, journal 11:30:01) |
| Idle | `idle, exiting after=5m0s` (journal 11:35:01); socket still listening; the next `api status` started it again |
| `sudo -u nobody curl --unix-socket …` | Refused, connect failed: the socket is `0600` in a `0700` directory under a `0700` runtime directory |
| `api watch --sections usage,bandwidth`, 2 min | One event for each section at once, then one per poll (64 bandwidth at 2 s, 5 usage at 30 s), a keepalive comment every 15 s |
| A change's page against `gh pr view 403` | Title whole and equal. The body's words are whole, but it is GitHub's `bodyText`, Markdown removed, which the contract does not say: #404 |
| `api load --clients 32 --streams 8 --duration 5m`, panel running, RSS every 10 s | 4,817,355 requests (200 and the 404s it asks for), 0 errors, p99 3.0 ms, max 495 ms. RSS 49–65 MB after warm-up, no trend over the last three minutes. `api check` straight afterwards passed (76 requests, 49 details) |
| Installer, `--api no` | "Removing the service other programs read": no unit files, no `sockets.target.wants` link, no socket; the marker kept |
| Then `--yes` upgrade, `api` left at `auto` | Not set up again |
| Then `--api yes` | Set up again; panel restarted |
| Uninstall of hayami | Not run on the desk (decided with the user): it would also remove the network helper, the VPN keepalive the work VPN depends on and the device rule. The uninstall path is held by the installer's tests (`dropAPI`) and `uninstall.sh`'s dry runs; the real uninstall runs on the Windows VM and the Mac in 4 and 5 |

### Results: the Windows VM, 4, 2026-11-25

Windows 11, build 26200. `hayami-tui.exe` cross-built from this branch; the
installer built on the VM from it (`scripts/installer_windows.ps1
-WithCliOnly`). The ssh session is elevated, so the installer's UAC prompt
is not shown there; on a desktop it is the one prompt the page names.

| Step | Result |
|---|---|
| `api-service install` | `hayami-api` registered: type `0x50` (`USER_OWN_PROCESS TEMPLATE`), automatic, `"C:\Program Files\hayami\hayami-api.exe" serve`, restart after 5 s twice, reset after a day. The copy in Program Files. `api status`: installed, not running for this account yet, starts at the next sign-in |
| Sign out and in at the console | `hayami-api_<n>` made: type `0xd0` (`USER_OWN_PROCESS INSTANCE`), running as the account in its console session, serving |
| `api check` over ssh (session 0) | Passed: 36 requests, 5 sections, 24 details, 5 events |
| A second `hayami-tui serve` | Refused, exit 1: "a service is already listening at `\\.\pipe\hayami-api-<SID>`, or someone else made the pipe first" |
| A pipe made first by SYSTEM, open to everyone, dialled by the client | Refused before writing, `ErrNotOurs` (`TestAPipeOfAnotherAccountIsRefused`). Falsified: with the owner check skipped the test fails |
| `api load --clients 32 --streams 8 --duration 5m`, working set every 10 s | 6,375,581 requests (200 and the 404s it asks for), 0 errors, p99 5.0 ms, max 76 ms. Working set 92–94 MB throughout. `api check` straight afterwards passed |
| `taskkill /f` on the instance | Restarted by Windows under a new PID within 9 s |
| `sc stop` on the instance | Stays stopped; `sc start` brings it back |
| Upgrade: `api-service install` from a build of another version, the instance running | The instance stopped, the copy replaced, the instance started again under a new PID on the new version |
| An administrator's instance (the account is in Administrators) | Runs with the limited token: not elevated, integrity Medium (`S-1-16-8192`), as the account's `explorer.exe` (the adversarial review's open question) |
| An update that fails after stopping the instances (`TestAFailedUpdateLeavesTheServiceRunning`) | The instances start again on the copy there. Falsified: without the restart the test fails |
| Installer `--yes`, `api` left at `auto`, the service installed on an older copy | "kept, on this version": the copy replaced, the instance started again under a new PID on the new version; the marker written |
| Installer `--yes --api=no` | "Removing the service other programs read": no instance, no template, no copy, no pipe; the marker kept |
| Then `--yes`, `api` at `auto` | Not set up again |
| Then `--yes --api=yes` | Registered: "set up, and starts at your next sign-in"; `api status` says so |
| Uninstall (`uninstall.exe --yes`) | The template, the copy and the marker gone |
| First install, `--yes`, `api` at `auto` | Set up by default, as on a first install anywhere |
| The template's registration, update and removal (`TestTheTemplateIsRegisteredAndRemoved`, elevated) | Registered with its type, start, program and recovery actions; registered again, updated in place; removed with its program. Falsified: with the first recovery action changed the test fails |

### Prod visibility

Not applicable: hayami is a desktop program with no telemetry. A person's
report is `hayami-tui api status`, `api check` and `doctor`.

## Alternatives Considered

- Loopback TCP with a token: rejected by the operator; the token is a
  credential to guard (B3), and any local account could try the port.
- JSON lines over the socket: rejected; every client writes its own framing.
- A Startup-folder entry or Run key on Windows: rejected by the operator in
  favour of a real service.
- A Windows service as LocalSystem or Local Service: rejected; it would read
  the person's credentials from another account, or not at all, and the
  stance says a component with more rights does one thing.
- Serving the shells' `view.Section` and `view.Detail`: rejected by the
  operator; they carry cut names and padded values, which is presentation.
- Serving from inside the running panel: rejected; the service would exist
  only while a window or pane is open.

## Gaps found

- **fynstall has no macOS target.** It needs one that builds a macOS
  installer from `fynstall.yaml`, runs hayami's extension pages there, and
  installs into the person's home. This is fynstall's own spec, issue and
  release, and hayami's release workflow gains macOS builds of both
  binaries (the windowed one with cgo, on a macOS runner). This blocks R25's
  macOS criteria only. Filed as ushineko/fynstall#75.
- The spike's question on an extension page's default added none, but
  for a reason the spike did not see (found in 3c on the desk): fynstall's
  install half cannot tell a first install from an upgrade.
  `Context.Upgrade` is set only in the old version's uninstaller, which is
  the old program, so `auto` first set the service up on every `--yes`
  upgrade. hayami keeps its own note instead, a marker in the install
  directory (decided with the user), and needs no fynstall change. A
  "this install replaces one" signal in fynstall's extension context
  would let a later version drop the marker; not filed.
