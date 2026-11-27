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
stands alone. Both land in one PR: spec 115's review made the port serve the
WebSocket alone.

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
  - On the TCP port only, the challenge-response of spec 115 R8: `auth`,
    with the client's nonce and its proof over the service's challenge,
    within 5 s, or the connection is closed with 4401. The token is never
    sent, as a message or a header. (Changed with spec 115's review from
    "`auth` with the token".)
  - `{"type": "subscribe", "sections": [...], "details": ["key/id", ...]}`:
    replaces what the connection follows, within the bounds `/v1/events`
    keeps (`MaxStreamDetails`). A subscription naming a section or detail
    that is not there is refused whole, with an `error` message naming it
    and `known` as in the contract's errors, and the one before it stands.
    (Changed in implementation from "the rest holds": one rule is easier
    for a client to follow.)
- R5 Service to client:
  - On the TCP port only, `challenge` first, with the service's nonce.
  - `hello` once the client is known: `service`, `version`, `api`,
    `sections`, and on the TCP port the service's proof; null elsewhere.
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

Test strategy, approved with the spec: behaviour through a real service and a
real WebSocket client on loopback; each message checked against the
contract's schemas; no mocked transport.

- [ ] On the socket: hello first with a null proof; a subscription brings
      the latest reading at once; another replaces it without reconnecting
      and lets go of what it dropped; closing lets go of everything
      (`TestTheSocketsWebSocketFollowsSubscriptions`, R4–R6).
- [ ] A subscription naming a section that is not there is an `error` with
      `known`, and the subscription before it stands (R4, R5).
- [ ] On the TCP port: spec 115's challenge-response lets the token holder in
      and closes everyone else with 4401 (spec 115's tests, R4).
- [ ] Limits: WebSockets and streams share `MaxStreams`, and the next of
      either gets 503; an oversized, binary, unknown or over-specified message
      closes the connection with 1009 or 1003
      (`TestTheWebSocketKeepsItsLimits`, R2, R3, R7).
- [ ] `Origin` on the upgrade gets 403, on both listeners (R1).
- [ ] Contract: the messages the tests read validate against their
      components (R9).
- [ ] `api check` checks the WebSocket on the socket, and `api --tcp check`
      on the port, against the installed service on the desk, the Mac and the
      Windows VM; `api watch --ws` follows it (R10).
- [ ] Herdr: hayami-js follows readings over `ws://127.0.0.1:<port>/v1/ws`
      through the challenge-response, in herdr-mods' own spec.
- [ ] Falsified: with spec 115's (skipping either proof, the shared limits).
- [ ] Fuzz: `FuzzWsMessage`, on the socket and the port, 30 s clean.
- [ ] Adversarial review with spec 115's (B3, B4, B6, B7 for the library).
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
