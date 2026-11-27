# 116 — Readings pushed over a WebSocket

**Issue**: #418

## Status: INCOMPLETE

## Context

`/v1/events` (spec 113) already pushes each reading as it is made, as
Server-Sent Events, so the service decides when new information goes out. A
client must keep an HTTP response open and parse the event-stream format, and
to follow something else it must open a new stream.

A WebSocket carries the same events as whole messages, and lets the client
change what it follows without reconnecting. Herdr mods have a `websocket`
module. A probe in the app (2026-11-27, spec 115's findings) connected to
`ws://127.0.0.1`, received messages pushed a second apart while the mod
redrew, and sent no `Origin`. Herdr refuses an `Authorization` header on the
handshake, so a client on the TCP port authenticates in its first message.

This spec depends on spec 115 for the TCP port; on the socket and the pipe it
stands alone.

## Decisions (operator, 2026-11-27)

- **On both listeners**: the socket and the pipe, for other programs, and
  the TCP port, for Herdr.
- **The events `/v1/events` sends**, `section` and `detail`, with the same
  payloads, plus messages to subscribe and unsubscribe.

## Requirements

### The endpoint

- R1 `GET /v1/ws` with a WebSocket upgrade, on every listener. The guard
  still applies (GET, no body, no `Origin`), and so do spec 115's `Host`
  rule and limits.
- R2 A WebSocket counts as a stream: `MaxStreams` covers streams and
  WebSockets together. Over the limit the upgrade gets 503 with
  `Retry-After`, as a stream does.
- R3 Messages are JSON text frames of at most 4 KiB from the client. A
  binary frame, a larger message, or one that is not a known message closes
  the connection with 1003 or 1009 and a reason.

### Messages

- R4 Client to service:
  - `{"type": "auth", "token": "<token>"}`: required first on the TCP port,
    within 5 s of the upgrade, or the connection is closed with 4401.
    A WebSocket upgrade there may carry `Authorization: Bearer` instead.
    On the socket and the pipe it is accepted and ignored.
  - `{"type": "subscribe", "sections": [...], "details": ["key/id", ...]}`:
    replaces what the connection follows, within the bounds `/v1/events`
    keeps (`MaxStreamDetails`). An unknown section or detail is an `error`
    message naming it (`known` as in the contract's errors), and the rest
    of the subscription holds.
- R5 Service to client:
  - `{"type": "hello", "version": "...", "contract": "..."}` once, after
    authentication.
  - `{"type": "section", "data": Section}` and
    `{"type": "detail", "data": Detail}`: the latest reading at once on
    subscribe, then one per new reading, the same values `/v1/events` sends.
  - `{"type": "error", "data": ErrorBody}` for a refused subscription.
- R6 Leases follow the subscription: what a connection follows is leased
  while it is open, as a stream's is, and is released when the connection
  closes or unsubscribes.
- R7 The service pings every `KeepAlive` (15 s). A client that leaves
  messages unread for `StallLimit` (30 s) is closed with 1008, as a stream
  is ended.

### The library, the contract and the client

- R8 One library, `github.com/coder/websocket` (ISC; v1.8.15 is the latest), pinned in `go.mod`,
  its licence added to `docs/credits.md`. It is a new dependency under B7.
- R9 `api/openapi.yaml` gains `/v1/ws` (101) and its messages as
  components (`WsAuth`, `WsSubscribe`, `WsHello`, `WsSection`,
  `WsDetail`, `WsError`), referencing the existing `Section`, `Detail` and
  `ErrorBody`. The minor version goes up.
- R10 `hayami-tui api watch --ws` follows readings over the WebSocket, on
  the socket or with `--tcp`. `api check` exercises `/v1/ws` and holds each
  message to the contract. `api load` can put WebSockets on the service.
- R11 `docs/api.md` gains a section on the WebSocket: when to use it rather
  than `/v1/events`, the messages, authentication on the port, and the
  close codes.

## Acceptance Criteria

Test strategy, for review before tests are written: behaviour through a real
service and a real WebSocket client on loopback (the harness, with the
library's client). Each message is checked against the contract's schemas,
as the golden tests check HTTP. No mocked transport.

- [ ] On the socket: subscribe to two sections and a detail; the latest
      readings arrive at once, then new ones as they are made, equal to what
      `/v1/events` sends for the same subscription (R4, R5).
- [ ] Re-subscribing changes what arrives without reconnecting; dropping a
      section releases its lease (`Hub.Active`), and closing releases all
      (R6).
- [ ] On the TCP port: no `auth` within 5 s closes with 4401; a wrong token
      closes with 4401 the same way; a right token gets `hello` (R4).
      `Authorization: Bearer` on the upgrade is accepted in place of `auth`.
- [ ] Limits: WebSockets and streams share `MaxStreams`, the next upgrade
      gets 503 (R2); a client that stops reading is closed after
      `StallLimit` (R7); an oversized, binary or unknown message closes with
      its code (R3).
- [ ] An unknown section in a subscription is an `error` message with
      `known`, and the rest of the subscription holds (R4, R5).
- [ ] `Origin` on the upgrade gets 403, on both listeners (R1).
- [ ] Contract: every message the service sends in the tests validates
      against its component (R9).
- [ ] `api check` and `api watch --ws` pass against the installed service on
      the desk, the Mac and the Windows VM; `--tcp` on each (R10).
- [ ] Herdr: a probe mod, or hayami-js once changed, follows readings over
      `ws://127.0.0.1:<port>/v1/ws` with `auth` in the first message.
- [ ] Falsified: skipping the `auth` deadline, the lease release and the
      shared limit each fail a test.
- [ ] Fuzz: inbound messages (`FuzzWsMessage`) never panic the service and
      never subscribe past the bounds; 30 s clean.
- [ ] Adversarial review with spec 115's (B3, B6, B7 for the library).
- [ ] `make test`, `make lint`, `make vuln`; changelog line.

## Threat model delta

- **B4 What hayami parses**: client messages, bounded at 4 KiB, JSON into
  typed structs, fuzzed.
- **B6**: the same readings as `/v1/events`, now also over a WebSocket;
  authentication on the port per spec 115, in the first message.
- **B7**: a new dependency, `coder/websocket`.

## Risks & Assumptions

- **Assumes** Herdr's `websocket` keeps sending no `Origin`. If a later Herdr
  sends one, its connections get 403, and allowing a Herdr origin would be a
  new decision, not a quiet change.
- A held WebSocket keeps the service from exiting idle while it is open, as a
  stream does; that is the point.
- **Rollback**: revert the PR; `/v1/events` is untouched.

## E2E Test Plan

- The desk: `hayami-tui api watch --ws --tcp --sections cooler` prints a
  reading every 2 s; `api check --tcp` passes. (R5, R10)
- The Mac and the Windows VM: the same. (R10)
- Herdr: the block follows readings over the WebSocket, with no process per
  read (seen in `ps` over a minute), after the herdr-mods change.

### Prod visibility

Not applicable: a desktop program with no telemetry.
