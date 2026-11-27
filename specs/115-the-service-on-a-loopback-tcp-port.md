# 115 — The service on a loopback TCP port

**Issue**: #417

## Status: INCOMPLETE

## Context

The service (spec 113) listens on a Unix socket (Linux, macOS) or a named
pipe (Windows) that only the account can open. Its main client outside this
repository, hayami-js in herdr-mods, runs in a Herdr mod, and a mod cannot
open either. That was tested in the app on 2026-11-27 (herdr-mods spec 010,
Transport note): `fetch` and `net.connect` reached a loopback TCP server with
no grant. Every form of the socket path failed in a host-name lookup or in
`fetch`'s URL builder. So hayami-js runs `curl` or `hayami-tui api get` once
a request, every 5 s for the bandwidth and cooler sections.

The service will also listen on a loopback TCP port. A loopback port is
reachable by every account on the machine and by every local process, where
the socket is reachable by one account. So a request on the port proves it
comes from the account's own program with a token that only the account can
read. The socket and the pipe stay as they are, for other programs.

Spec 116 (#418) adds a WebSocket endpoint on both listeners; this spec is the
port and its token.

## Decisions (operator, 2026-11-27)

- **On by default**, set up with the socket service, and removable on its
  own.
- **A token file** proves the account: a random token in a file only the
  account can read. The socket and the pipe keep their account check and
  take no token.
- **A fixed port, configurable per account.** The default is 47930 (not in
  `/etc/services`, not in use on the desk). systemd and launchd hold it and
  start the service on a connection, as they do the socket. A second account
  on one machine sets another port.

## Probe findings (the app, 2026-11-27)

| Question | Found |
|---|---|
| Can a mod reach loopback TCP? | Yes: `net.connect("127.0.0.1", port)` and `fetch("http://127.0.0.1:port/")`, with no grant |
| Can a mod reach the Unix socket? | No: every path form fails as a host-name lookup; `http+unix:`, `unix:` and `unix://` URLs fail in `fetch`'s URL builder |
| What does Herdr's `websocket` send? | `Host`, `Connection`, `Upgrade`, `Sec-WebSocket-Version`, `Sec-WebSocket-Key`, and headers the mod adds. No `Origin`, no `User-Agent` |
| Can a mod send `Authorization`? | Not on a WebSocket: refused as a credential header. `fetch` refuses only framing headers (`Host`, `Content-Length`) |

## Requirements

### The listener

- R1 `hayami-tui serve` also listens on `127.0.0.1:<port>`, never on any
  other address. Without a configured port it does not listen on TCP.
- R2 On Linux the port is a second systemd user socket,
  `hayami-api-tcp.socket` (`ListenStream=127.0.0.1:<port>`,
  `FileDescriptorName=tcp`), activating the same `hayami-api.service`. A
  port another program holds fails that unit alone, and the Unix socket
  keeps working. The service tells the descriptors apart by
  `LISTEN_FDNAMES`.
- R3 On macOS the agent's plist gains a second `Sockets` entry,
  `TCP` (`SockNodeName` 127.0.0.1, `SockServiceName` the port), and the
  service takes both from launchd.
- R4 On Windows each account's instance listens on its own port, from its
  own `api-tcp.json` (R6), once it starts. A port that is already taken is
  logged, and the pipe keeps working.
- R5 Both listeners serve the same routes, guard and limits (spec 113:
  GET only, no body, no `Origin`, `MaxConns` together, `MaxStreams`
  together).

### The token

- R6 `api-tcp.json` in the settings directory (`~/.config/hayami` on Linux,
  `~/Library/Application Support/hayami` on macOS, `%APPDATA%\hayami` on
  Windows) holds `{"address": "127.0.0.1:<port>", "token": "<token>"}`.
  It is readable only by the account: mode `0600` on Linux and macOS, and
  on Windows an ACL granting the account alone, set when the file is made.
- R7 The token is 32 bytes from the system's random source, base64url
  without padding. It is made when the port is set up and kept across
  upgrades. `hayami-tui api-service install --new-token` replaces it.
- R8 Every request on the port carries `Authorization: Bearer <token>`,
  except a WebSocket upgrade (spec 116 authenticates in the first message).
  The token is compared in constant time. A missing or wrong token gets
  401 with `WWW-Authenticate: Bearer` and the error code `unauthorized`,
  and the answer is the same whatever was wrong.
- R9 A request on the port whose `Host` is not `127.0.0.1:<port>` or
  `localhost:<port>` gets 403 before the token is looked at. This refuses
  DNS rebinding, in addition to the `Origin` rule.
- R10 The token is never logged, printed, put in an error, or written
  anywhere but `api-tcp.json`. `api status` names the file, not its content.
- R11 The service reads the file when it starts. A file that is missing,
  unreadable, readable by others, or malformed means no TCP listener, and
  says why in the log and in `api status`. The socket and the pipe serve
  regardless.

### Setting it up

- R12 The installer's Service page, on a first install, sets up the port
  with the socket service unless "Also on a local TCP port" is cleared.
  Its parameter follows `api`'s `auto` and `once` rule, so a `--yes`
  upgrade never brings back a port someone removed. Clearing it removes the
  unit or plist entry and `api-tcp.json`.
- R13 `hayami-tui api-service install --tcp-port <n>` sets or moves the
  port; `--tcp=false` removes it. On Linux it writes the socket unit, on
  macOS the plist entry, and on Windows `api-tcp.json` only. Uninstalling
  hayami removes all of them.

### The contract and the client

- R14 `api/openapi.yaml` gains the TCP server with a bearer security scheme
  that applies to it alone, and the `unauthorized` error code. The minor
  version goes up; nothing existing changes.
- R15 `hayami-tui api` takes `--tcp`, which reads `api-tcp.json` and talks
  to the port. `api status` reports the port: set up or not, listening or
  not, and why not. `api check --tcp` runs the whole check over the port.
- R16 `docs/api.md` describes the port, the token, the file and how to
  rotate it. The README's service section says the port exists and what it
  is for. `ARCHITECTURE.md` gains the port.

## Acceptance Criteria

Test strategy, for review before tests are written: behaviour through a real
service on loopback, as spec 113's harness does, not handler unit tests. A
mock appears only where the platform's activation cannot run in a test, and
in that case a real systemd or launchd run on the machine covers it.

- [ ] Listener: the harness serves the TCP listener from a real file; a
      request with the token gets 200, and one without it, with a wrong one,
      or with a token of another length gets 401 with the same body (R8).
- [ ] `Host` other than the two allowed gets 403 before the token is checked;
      a request with `Origin` gets 403 on the port as on the socket (R5, R9).
- [ ] The port binds `127.0.0.1` only: the listener's address is checked,
      and a connection to the machine's other addresses is refused (R1).
- [ ] The token file: made `0600` (Linux, macOS) or account-only (Windows,
      on the VM); a file readable by others, missing or malformed means no
      TCP listener, and the socket still serves (R6, R11).
- [ ] The token appears in no log line, error, `api status` output or
      `doctor` output across a run that fails authentication (R10); a test
      greps the captured output for it.
- [ ] Rotation: `--new-token` replaces it; the old token gets 401 (R7).
- [ ] Limits are shared: `MaxConns` and `MaxStreams` count both listeners
      (R5).
- [ ] Linux, real systemd: `hayami-api-tcp.socket` starts the service on a
      TCP connection; with the port held by another program the TCP unit
      fails and the socket still answers (R2).
- [ ] macOS, the Mac: launchd starts the agent on a TCP connection (R3).
- [ ] Windows, the VM: the instance listens on its account's port; a taken
      port is logged and the pipe answers (R4).
- [ ] Installer: a first install sets up the port; a `--yes` upgrade keeps
      it or leaves it removed; clearing the box removes the unit and the
      file (R12, R13).
- [ ] `api check --tcp` passes against the installed service on each system
      (R15).
- [ ] Falsified: dropping the token check, the `Host` check and the
      permission check on the file each fail a test.
- [ ] Contract test: the TCP server and `unauthorized` are in the contract,
      and the existing paths are unchanged (R14).
- [ ] Adversarial review (B3, and the new surface under B6), as the threat
      model asks; findings filed or listed under "Checked".
- [ ] `make test`, `make lint`, `make vuln`; changelog line.

## Threat model delta

- **B3 Credentials**: a new credential, the token, and the file it lives
  in. Kept by file permission alone. Anything running as the account can
  read it, as it can already read the account's Claude token.
- **B6 What hayami shows**: the readings, details and drills become
  reachable over loopback TCP by anyone holding the token. Without it, any
  local account or process, and any browser page, gets 401 or 403.
- **New surface**: a listening TCP port. Defences: loopback bind, `Host`
  allow-list, `Origin` refused, token, read-only routes, the existing
  limits. `docs/threat-model.md` gains these under B3 and B6.

## Risks & Assumptions

- **Assumes** only the account can read files in its settings directory
  with mode `0600`, and on Windows that an owner-only ACL holds. Root and
  administrators can read it, as they can anything.
- **Port collisions**: another program may already use 47930. The TCP unit
  fails on its own and `api status` says so; the person moves the port with
  `--tcp-port`.
- **Two accounts on one Windows machine** both default to 47930, and the
  second one's instance fails to bind until it sets another port.
- **hayami-js** changes in herdr-mods, in its own spec, after this one
  ships.
- **Rollback**: revert the PR; the installer's next run removes the unit
  and the file.

## E2E Test Plan

- The desk (Linux): install, `hayami-tui api status` shows the port
  listening; `curl -H "Authorization: Bearer $(jq -r .token …)"
  http://127.0.0.1:47930/v1/health` answers; without the header, 401.
  `api check --tcp` passes. (R2, R8, R15)
- The Mac and the Windows VM: the same with `hayami-tui api --tcp check`.
  (R3, R4)
- Herdr, after the herdr-mods change: the block reads over TCP with no
  `curl` process per read (seen in `ps` over a minute).

### Prod visibility

Not applicable: a desktop program with no telemetry.
