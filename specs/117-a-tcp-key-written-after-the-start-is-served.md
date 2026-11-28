# 117 — A TCP key written after the service starts is served

**Issue**: #423

## Status: COMPLETE

## Executive Summary

A service nothing handed its sockets to (the Windows instance, `serve` by
hand) watches for the TCP port's key and serves the port once one is written,
so the installer's and `api-service tcp`'s port works at once on Windows.
Review `tcpServer.waitForKey` in `internal/api/run.go`. Verified on the
Windows VM: the port answered four seconds after `api-service tcp`.

## Context

Installing 0.24.0 on the Windows VM, the installer said "The service also
listens on 127.0.0.1:47930", but nothing listened until the account's
instance was restarted. The installer updates the service's copy, which
restarts the instances, and only then writes the port's key; the instance
read its key once, at its start, and found none. `hayami-tui api-service tcp`
had the same gap, and told the person to sign out and in again.

On Linux and macOS the socket unit or the agent hands the port over and the
install restarts the service, so the gap is Windows', and `serve` run by
hand's.

## Requirements

- R1 A service that nothing handed its sockets to (the Windows instance,
  `serve` run by hand) and that starts without a key it may serve with looks
  at the key every 2 s, and serves the port as soon as there is one, with
  the same checks as at the start (spec 115 R1, R11).
- R2 A refusal while it waits (a key refused, a port held) is logged once for
  each reason, not at every look.
- R3 A service systemd or launchd started does not wait: its port is the
  socket unit's or the agent's.
- R4 `api-service tcp` on Windows no longer says to sign out and in.

## Acceptance Criteria

- [x] `TestAKeyWrittenAfterTheStartIsServed`: the service starts with no key,
      a key is written, and the port is served within seconds, through the
      challenge-response; falsified by not waiting (R1).
- [x] The Windows VM: with the instance running and no key, `hayami-tui
      api-service tcp` makes the port answer `api --tcp check` with no
      restart (R1, R4).
- [x] `make test`, `make lint`; changelog line.

## Threat model delta

No boundary touched: the port is served with the same key, checks and
limits as before, only sooner.

## Risks & Assumptions

- A key file checked every 2 s costs a `stat` and a read when it changes.
- **Rollback**: revert; the port waits for the next start, as in 0.24.0.

## E2E Test Plan

- The Windows VM: remove the key, restart the instance, run `hayami-tui
  api-service tcp`, then `hayami-tui api --tcp check` within seconds (R1).

### Prod visibility

Not applicable: a desktop program with no telemetry.
