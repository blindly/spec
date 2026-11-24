---
covers: [main.go, commands.go, help.go, completion.go, spec_test.go, README.md]
created: 2026-11-23
id: FEAT-035
status: implemented
title: spec ps — the in-flight queue
type: feature
---

# spec ps — the in-flight queue

## Summary

`spec ps` prints only the work queue: an `In flight:` list naming the
specs that are not yet done — the spec equivalent of `ps` listing
running processes rather than system stats. `spec status` stays the
dashboard.

## Motivation

`spec status` reports counts by status ("accepted 2") but not which
specs those are; `spec ls` lists every spec with no status filter.
`ps` answers "what's in flight" — and printing only the queue keeps it
compact in busy repos where duplicating the whole status output would
grow without bound.

## Requirements

Addresses: REQ-011

## Design

- **`spec ps` is its own command**, not an alias: it prints the
  repository header then `In flight:` entries — every spec that is not
  archived, not an inbox idea, not in a `done_statuses` status, and
  not `implemented` — sorted by configured status order (unknown
  statuses last), then id, via `inFlightSpecs`. For non-feature
  (record) types — adr, imported — `accepted` is additionally settled:
  a proposed ADR awaits a decision, an accepted one is a settled
  record, not in-flight work; accepted features stay in flight until
  implemented. Empty queue prints
  `In flight: none`; otherwise the header carries the count. Columns:
  id, type, status, title.
- **`spec status` is the other half**: aggregate counts only, no
  queue — the two commands stay single-purpose. An earlier revision
  had `ps` run the status report plus the list; queue-only replaced it
  so `ps` output grows with the queue alone.
- Help and completion list `ps` as a real command (its own doc block,
  not an alias mapping).

## Acceptance Criteria

- [x] `spec ps` prints `In flight:` entries for each non-archived,
  non-idea spec whose status is neither done nor `implemented` —
  `accepted` counts as settled for record types (adr, imported), not
  for features — sorted by status order then id; nothing else but the
  repo header
- [x] `In flight: none` when the queue is empty
- [x] `spec status` is queue-free; `spec help ps` and completion treat
  `ps` as a first-class command
- [x] README documents `ps`; tests cover both commands
