# 115 — The service on a loopback TCP port

**Issue**: #417

## Status: COMPLETE

## Executive Summary

The service also listens on `127.0.0.1:47930`, on by default, for clients
such as a Herdr mod that cannot open its socket. Because any account can
reach or take a loopback port, the port serves only the WebSocket, behind a
mutual challenge-response over a token in an account-only file; the token
never crosses a connection, and the reference client refuses an impostor.
Review `internal/api/ws.go` (`challenge`), `internal/api/tcp.go` and
`internal/api/tcpkey` first; the adversarial review and what it changed are
under "Review".

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

Spec 116 (#418) is the WebSocket; the review of this spec made the port serve
nothing else, so both land in one PR.

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
- R5 The port serves `/v1/ws` alone (spec 116's WebSocket), with the checks
  every request passes (GET, no body, no `Origin`); any other path is 404.
  It has its own limit, `MaxTCPConns` (16), apart from the socket's, and its
  WebSockets count against the 16 streams. A refusal closes its connection.
  (Changed by the review, F1 and F2, from "both listeners serve the same
  routes and share the limits".)

### The token

- R6 `api-tcp.json` in the settings directory (`~/.config/hayami` on Linux,
  `~/Library/Application Support/hayami` on macOS, `%APPDATA%\hayami` on
  Windows) holds `{"address": "127.0.0.1:<port>", "token": "<token>"}`.
  It is readable only by the account: mode `0600` on Linux and macOS, and
  on Windows an ACL granting the account alone, set when the file is made.
- R7 The token is 32 bytes from the system's random source, base64url
  without padding. It is made when the port is set up and kept across
  upgrades. `hayami-tui api-service install --new-token` replaces it.
- R8 The port's WebSocket begins with a mutual challenge-response, and the
  token never crosses the connection: the service sends a nonce; the client
  answers within 5 s with its own nonce and a proof, HMAC-SHA256 keyed with
  the token over its label, the service's nonce and its own; the service's
  hello carries its proof over the client's nonce, which the client checks
  before it trusts anything. Proofs are compared in constant time. A wrong,
  late or malformed answer closes the connection with 4401 and sends no
  reading. (Changed by the review, F1, from a bearer token in every
  request: a program of another account can take a fixed port first, and a
  bearer token is sent to whoever holds it. The operator chose this design,
  2026-11-27.)
- R9 A request on the port whose `Host` is not `127.0.0.1:<port>` or
  `localhost:<port>` gets 403 before anything else. This refuses
  DNS rebinding, in addition to the `Origin` rule.
- R10 The token is never logged, printed, put in an error, or written
  anywhere but `api-tcp.json`. `api status` names the file, not its content.
- R11 The service reads the file when it starts. A file that is missing,
  unreadable, readable by others, or malformed means no TCP listener, and
  says why in the log and in `api status`. The socket and the pipe serve
  regardless. A listener handed over on another address or port than the
  key's is closed (review F5). While it runs the service looks at the key
  every 2 s: a changed token ends the sessions the old one let in and lets
  in only the new one's holder; a key removed or refused lets nobody in
  (review F4). On Windows the port is bound with `SO_EXCLUSIVEADDRUSE`
  (review F6).

### Setting it up

- R12 The installer's Port page, after the Service page, on a first install
  sets up the port with the service unless "Also serve on a local TCP port"
  is cleared (`--apitcp`). Its parameter follows `api`'s `auto` and `once`
  rule, so a `--yes` upgrade never brings back a port someone removed.
  Clearing it removes the unit or plist entry and `api-tcp.json`. Without
  the service there is no port. (Changed in implementation from a second box
  on the Service page: the wizard's page holds one box, and a page of its
  own is the same page type, tested the same way.)
- R13 `hayami-tui api-service tcp` sets up the port (`--port <n>`, the one
  set up already or the default), moves it, replaces its token
  (`--new-token`) or removes it (`--remove`). It runs as the person and
  never elevates: on Linux and macOS it writes the installed service again,
  with that service's own program and the port's socket unit or plist entry;
  on Windows it writes `api-tcp.json` only. On macOS, where
  `api-service install` is the installer's route, a first install sets up
  the port too unless `--tcp=false`. Uninstalling hayami removes all of it.
  (Changed in implementation from `api-service install --tcp-port`: spec
  113 keeps `api-service install` the installer's step on Linux, so that a
  unit never names whatever program ran the command.)

### The contract and the client

- R14 `api/openapi.yaml` gains the TCP server, its challenge-response, the
  WebSocket and its messages (spec 116). The minor version goes up; nothing
  existing changes.
- R15 `hayami-tui api` takes `--tcp`, which reads `api-tcp.json`: `watch`
  and `check` run over the port's WebSocket through the challenge-response;
  `get` and `load`, which need the socket's routes, say so. `api status`
  reports the port: set up or not, listening or not, and why not.
- R16 `docs/api.md` describes the port, the token, the file, the
  challenge-response with an example a client in another language can
  follow, and how to rotate the token. The README's service section says the
  port exists and what it is for. `ARCHITECTURE.md` gains the port.

## Acceptance Criteria

Test strategy, approved with the spec: behaviour through a real service on
loopback, as spec 113's harness does, not handler unit tests; platform
activation on the real systemd, launchd and Windows.

- [x] Challenge-response: the token holder is let in, hello's proof is the
      token holder's, and the token is not in anything the client sends
      (`TestThePortsChallengeLetsTheTokenHolderIn`, R8).
- [x] A wrong proof, the token offered as the proof, the service's proof or
      nonce sent back, a short nonce, no answer, another message first and a
      binary answer are each closed with 4401 and sent no reading
      (`TestAClientThatCannotProveItIsClosed`, R8).
- [x] The reference client refuses a server that cannot prove it holds the
      token, having sent it nothing usable (`TestTheClientRefusesAnImpostor`,
      R8, review F1).
- [x] `Host` other than the two allowed and a browser's `Origin` get 403 on
      the port; `Origin` gets 403 on the socket's WebSocket too (R5, R9).
- [x] The port binds `127.0.0.1` only and serves nothing but `/v1/ws`, a
      bearer header included (`TestTheServiceListensOnItsKeysPortOnLoopbackAlone`,
      R1, R5).
- [x] The key file: made `0600` (Linux, macOS) or account-only (Windows, on
      the VM); a file readable by others, missing or malformed means no TCP
      listener, and the socket still serves (R6, R11).
- [x] The token appears in no log line across refused sessions, refused keys
      and a replaced token, nor in `api status` (R10).
- [x] Rotation: `--new-token` replaces it; the service ends the old sessions
      and refuses the old token without a restart (R7, R11, review F4).
- [x] The port's connections, more than the socket's whole limit, do not
      take the socket's places (`TestThePortsConnectionsDoNotTakeTheSockets`,
      R5, review F2).
- [x] Linux, real systemd: `hayami-api-tcp.socket` starts the service on a
      TCP connection; an install over a running service listens anew; with
      the port held by another program the TCP unit fails and the socket
      still answers (R2).
- [x] macOS, the Mac: launchd starts the agent on a TCP connection (R3).
- [x] Windows, the VM: the instance listens on its account's port; the key's
      access list is checked (R4, R6).
- [x] Installer: a first install sets up the port; a `--yes` upgrade keeps
      it or leaves it removed; clearing the box removes it (R12, R13).
- [x] `api --tcp check` passes against the installed service (R15).
- [x] Falsified: skipping the client's proof, skipping the service's proof
      in the client, serving every route on the port, one limit for both
      listeners, a new key that ends nothing, and listening on every address
      each fail a test.
- [x] Contract test: the TCP server, `/ws` and its messages are in the
      contract, and every message the tests read conforms (R14).
- [x] Adversarial review (B3, B6, and the new surface), with a refutation
      pass; its findings fixed or listed under "Checked".
- [x] `make test`, `make lint`, `make vuln`; changelog line.

## Review (2026-11-27)

One reviewer given the change and seven attackers, then a separate pass that
tried to refute each finding, with experiments on the desk. All are fixed in
this PR, and the threat model records them.

| Finding | Severity after refutation | Fix |
|---|---|---|
| F1: the client sent its bearer token to whatever held the port; another account can take a fixed port first (before sign-in, after the last sign-out) | Medium | The port serves the WebSocket alone, behind a mutual challenge-response; the token never crosses a connection (R8) |
| F2: connections to the port could take the socket's connection places | Low to medium | The port's own limit; a refusal closes its connection (R5) |
| F3: `docs/api.md` showed `curl` with the token on its command line | Low | Removed; the documents say never to put the token on a command line |
| F4: a replaced token kept working on Windows until the next sign-in | Low to medium | The service follows its key and ends the old sessions (R11) |
| F5: a port handed over that is not the key's was served | Low | Refused (R11) |
| F6: Windows' bind was not exclusive | Low (Windows already refuses another account) | `SO_EXCLUSIVEADDRUSE` (R11) |
| From the refutation: the Linux install restarted the port's socket while the service held it, which fails | Low to medium | The service is stopped first, and starts on the next connection |

## Threat model delta

- **B3 Credentials**: a new credential, the token, and the file it lives
  in, kept by file permission alone; it never crosses a connection.
- **B6 What hayami shows**: the readings, details and drills become
  reachable over loopback TCP by a client that proves it holds the token;
  anyone else, any browser page, and a program that took the port first get
  nothing.
- **B7**: a new dependency, `github.com/coder/websocket` (ISC).
- **New surface**: a listening TCP port. `docs/threat-model.md` gains these
  under B3 and B6, the review in its history, and what was checked.

## Risks & Assumptions

- **Assumes** only the account can read files in its settings directory
  with mode `0600`, and on Windows that an owner-only ACL holds. Root and
  administrators can read it, as they can anything.
- **Port collisions**: another program may already use 47930. The TCP unit
  fails on its own and `api status` says so; the person moves the port with
  `--tcp-port`.
- **The settings directory the service sees**: the key is found through
  `os.UserConfigDir`, so `XDG_CONFIG_HOME` set in a shell but not in the
  systemd user manager's environment would put the key where the service
  does not look. Seen with `systemd-socket-activate`, which passes no
  environment; the units rely on the default, `~/.config`.
- **Two accounts on one Windows machine** both default to 47930, and the
  second one's instance fails to bind until it sets another port.
- **hayami-js** changes in herdr-mods, in its own spec, after this one
  ships.
- **Rollback**: remove the port first (`hayami-tui api-service tcp
  --remove`, or the installer with `--apitcp=no`), then install the older
  version. An older installer does not know the port's unit and leaves it,
  and an older service handed the port's descriptor would serve it without
  any of this spec's checks (found reinstalling 0.23.2 on the desk after the
  E2E run). Reverting the PR alone does not remove a unit already written.

## E2E Test Plan

- The desk (Linux): the installed build; `hayami-tui api status` shows the
  port listening; `hayami-tui api --tcp check` passes; `--new-token` ends a
  `watch --tcp` session. (R2, R8, R11, R15)
- The Mac and the Windows VM: `hayami-tui api --tcp check`. (R3, R4)
- Herdr, after the herdr-mods change: the block reads over the WebSocket with
  no `curl` process per read (seen in `ps` over a minute).

### Results (2026-11-27)

- **The desk** (Linux, real systemd; this branch's build installed with the
  CLI installer, then 0.23.2 put back): the installer's Port page set the
  port up on the first decision; the first connection on the TCP port
  started the service, which took both descriptors and served both;
  `api --tcp check` passed (5 sections, 11 details); an install over the
  running service kept the port and listened anew; `--new-token` ended a
  running `watch --tcp` and the new token worked; moving the port onto one
  another program held failed `hayami-api-tcp.socket` alone, the socket
  answered and `api status` said "not listening", and moving it back
  recovered. Under `systemd-socket-activate`, a port handed over other than
  the key's was refused with the socket still serving.
- **The Mac** (launchd): `api-service tcp` wrote the plist's `TCP` entry and
  a `0600` key; with the agent stopped, `api --tcp check` made launchd start
  it from the TCP connection, and passed (5 sections, 4 details). The agent
  was put back on its own program, without the port. The package tests pass
  there.
- **The Windows VM**: the package tests pass, the key's access list checks
  among them; with this branch's copy registered, the account's instance
  read its key when it started and listened on 127.0.0.1:47930, and
  `api --tcp check` passed against it (5 sections, 7 details). The VM was put
  back on 0.23.1, without the port or the key.

### Prod visibility

Not applicable: a desktop program with no telemetry.
