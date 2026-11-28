# 118 — A WebSocket client that sends nothing is closed

**Issue**: #424

## Status: COMPLETE

## Executive Summary

The service closes a WebSocket whose client has sent nothing for 90 s, so a
connection a Herdr reload left behind no longer stays open with its sections
polled. Clients resend their subscription every 30 s: `hayami-tui api watch`
and hayami-js. Contract 1.2. Review the idle check in `wsServe`.

## Context

Moving hayami-js to the WebSocket (herdr-mods spec 022) found that a Herdr
mod reload stops the old build's tasks but leaves its WebSocket open, and a
new build cannot reach the old build's objects. Herdr's runtime answers the
service's pings itself, so the service never takes the old client for
stalled. Each reload left a connection open, read by nobody, its sections
still leased and polled, until the app restarted or the token was replaced.

## Requirements

- R1 The service closes a WebSocket whose client has sent no message for 90 s
  (`ClientIdle`), with 1008. Any message counts, the challenge's answer
  included; resending the same subscription changes nothing and will do.
- R2 The contract says so under `/ws`, and goes to 1.2: a duty for clients is
  added. `docs/api.md` says it, with why.
- R3 `hayami-tui api watch` over the WebSocket resends its subscription every
  30 s (`client.ResubscribeEvery`).
- R4 hayami-js resends its subscription every 30 s (herdr-mods).

## Acceptance Criteria

- [x] `TestASilentClientIsClosed`: a client that subscribes and then sends
      nothing is closed with 1008; one that resubscribes stays open;
      falsified by not closing (R1).
- [x] The contract's version is 1.2 and `/ws` states the duty (R2).
- [x] `hayami-tui api watch` resends its subscription on a timer
      (`client.ResubscribeEvery`, 30 s) (R3).
- [x] hayami-js resends its subscription every 30 s (herdr-mods) (R4).
- [x] `make test`, `make lint`; changelog line.

## Threat model delta

B6, the service's availability: a client that holds a connection and sends
nothing no longer keeps a stream's place and its leases for ever; it is
closed within 90 s. `docs/threat-model.md` notes it under the TCP port.

## Risks & Assumptions

- A client of 1.1 that sends nothing after subscribing is closed after 90 s.
  The only clients are hayami's own and hayami-js, both changed here.
- **Rollback**: revert; connections stay as in 0.24.0.

## E2E Test Plan

After 0.24.1 is installed (the desk runs 0.24.0 until the release):

- The desk: with the block following this computer, save the hayami mod so
  Herdr reloads it; the old build's connection is gone within about 105 s,
  and one connection remains (R1, R4).

### Results (2026-11-27, 0.24.1 installed)

- The desk: a client opened through hayami-js's library (node's WebSocket,
  which answers pings itself and sends nothing, as Herdr does for a replaced
  build) subscribed once and fell silent; the installed 0.24.1 service closed
  it after 90 s with 1008, "the client sent nothing", having sent it 91
  readings. hayami-js resends its subscription every 30 s, so a live block is
  never closed for it.
- The desk, the Mac and the Windows VM run 0.24.1 (contract 1.2), each with
  `api --tcp check` passing.

### Prod visibility

Not applicable: a desktop program with no telemetry.
